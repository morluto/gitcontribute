package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const (
	facetPRCIReport              = "pr_ci_report"
	maxFeedbackItemsPerChannel   = 1000
	maxExactFeedbackPullRequests = 100
)

var (
	errFeedbackRepositoryNotStored  = errors.New("repository is not stored")
	errFeedbackPullRequestNotStored = errors.New("pull request is not stored")
)

type pullRequestWorkflowItem struct {
	Key          string                      `json:"key"`
	Status       mcpcontract.BatchItemStatus `json:"item_status"`
	HeadSHA      string                      `json:"head_sha,omitempty"`
	ResourceURI  string                      `json:"resource_uri,omitempty"`
	Code         string                      `json:"code,omitempty"`
	Message      string                      `json:"message,omitempty"`
	Recovery     *mcpcontract.RecoveryPlan   `json:"recovery,omitempty"`
	RetryAfterMS int                         `json:"retry_after_ms,omitempty"`
}

type pullRequestWorkflowResult struct {
	BatchStatus batchOperationStatus      `json:"batch_status"`
	Items       []pullRequestWorkflowItem `json:"items"`
	Requests    int                       `json:"requests"`
}

func (r *MCPReader) SyncPullRequestFeedback(ctx context.Context, in mcpcontract.SyncPullRequestFeedbackInput) (mcpcontract.JobReference, error) {
	refs, err := parsePullRequestRefs(in.PullRequests, "pull_requests")
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	in.PullRequests = refs
	if len(in.PullRequests) < 1 || len(in.PullRequests) > maxExactFeedbackPullRequests {
		return mcpcontract.JobReference{}, errors.New("pull_requests must contain 1 to 100 items")
	}
	if in.ThreadState == "" {
		in.ThreadState = "unresolved"
	}
	selection, err := corpus.ParseFeedbackSelection(in.Channels, in.ThreadState)
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	in.Channels, in.ThreadState = selection.Channels(), selection.ThreadState()
	if in.MaxItemsPerChannel == 0 {
		in.MaxItemsPerChannel = 300
	}
	if in.MaxItemsPerChannel < 1 || in.MaxItemsPerChannel > maxFeedbackItemsPerChannel {
		return mcpcontract.JobReference{}, errors.New("max_items_per_channel must be between 1 and 1000")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = 100
	}
	if in.MaxRequests < 1 || in.MaxRequests > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_requests must be between 1 and 1000")
	}
	id, err := r.submitJob(ctx, "sync_pull_request_feedback", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncPullRequestFeedback(ctx, in, selection, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_pull_request_feedback", "pull-request feedback synchronization job started"), nil
}

func (r *MCPReader) syncPullRequestFeedback(ctx context.Context, in mcpcontract.SyncPullRequestFeedbackInput, selection corpus.FeedbackSelection, report func(string, string) error) (pullRequestWorkflowResult, error) {
	in.Channels, in.ThreadState = selection.Channels(), selection.ThreadState()
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return pullRequestWorkflowResult{}, err
	}
	feedbackReader, ok := reader.(github.PullRequestFeedbackReader)
	if !ok {
		return pullRequestWorkflowResult{}, errors.New("GitHub reader does not support pull-request feedback")
	}
	release, err := r.acquireFeedbackWorkflow(ctx)
	if err != nil {
		return pullRequestWorkflowResult{}, err
	}
	defer release()
	budget := github.NewRequestBudget(in.MaxRequests)
	channels, providerChannels, threadState := selection.ChannelValues(), selection.Channels(), selection.ThreadState()
	out := pullRequestWorkflowResult{BatchStatus: batchOperationComplete, Items: make([]pullRequestWorkflowItem, len(in.PullRequests))}
	for index, ref := range in.PullRequests {
		if ref.Kind == "" {
			ref.Kind = string(domain.PullRequestKind)
		}
		item := pullRequestWorkflowItem{Key: pullRequestKey(ref), Status: mcpcontract.BatchItemComplete}
		snapshot, readErr := feedbackReader.GetPullRequestFeedback(ctx, ref.Owner, ref.Repo, ref.Number, github.PullRequestFeedbackOptions{
			Channels: providerChannels, ThreadState: threadState, MaxItemsPerChannel: in.MaxItemsPerChannel,
		}, budget)
		snapshot.ThreadState = threadState
		item.HeadSHA = snapshot.HeadSHA
		if snapshot.Header.Number > 0 {
			code, persistErr := r.persistPullRequestIdentity(ctx, ref, snapshot.Header, snapshot.SourceUpdatedAt)
			if persistErr != nil {
				failure := feedbackPersistenceFailure(ref, code, persistErr.Error())
				failure.HeadSHA = item.HeadSHA
				item = failure
				out.BatchStatus = batchOperationPartial
				out.Items[index] = item
				if err := report("pull_request_feedback", jobProgressCounts(index+1, len(in.PullRequests))); err != nil {
					return pullRequestWorkflowResult{}, err
				}
				continue
			}
		}
		if readErr != nil {
			var persistErr error
			if len(snapshot.Coverage) > 0 {
				persistErr = r.persistPullRequestFeedback(ctx, ref, snapshot, coveredFeedbackChannels(channels, snapshot.Coverage))
			}
			if persistErr != nil {
				item.Status, item.Code, item.Message = "failed", "persist_partial_feedback_failed", persistErr.Error()
			} else {
				item = workflowFailure(ref, readErr, mcpcontract.ToolSyncPullRequestFeedback)
			}
			out.BatchStatus = batchOperationPartial
		} else if err := r.persistPullRequestFeedback(ctx, ref, snapshot, channels); err != nil {
			item = feedbackPersistenceFailure(ref, feedbackPersistenceFailureCode(err), err.Error())
			out.BatchStatus = batchOperationPartial
		} else if !feedbackSnapshotComplete(snapshot, channels) {
			item.Status = "retryable"
			item.Code = "feedback_coverage_incomplete"
			item.Message = "one or more feedback channels reached max_items_per_channel"
			item.Recovery = feedbackCoverageRecovery(ref, in, item.Message)
			item.HeadSHA = snapshot.HeadSHA
			out.BatchStatus = batchOperationPartial
		} else {
			item.HeadSHA = snapshot.HeadSHA
			item.ResourceURI = fmt.Sprintf("gitcontribute://pull-request-feedback/%s/%s/%d", ref.Owner, ref.Repo, ref.Number)
		}
		out.Items[index] = item
		if err := report("pull_request_feedback", jobProgressCounts(index+1, len(in.PullRequests))); err != nil {
			return pullRequestWorkflowResult{}, err
		}
	}
	projectionCorpus, err := r.openCorpus(ctx)
	if err != nil {
		return pullRequestWorkflowResult{}, err
	}
	if _, err := projectionCorpus.RebuildPullRequestFeedbackProjection(ctx); err != nil {
		return pullRequestWorkflowResult{}, fmt.Errorf("rebuild pull-request feedback projection: %w", err)
	}
	out.Requests = budget.Completed()
	if out.BatchStatus == batchOperationPartial && allWorkflowItemsFailed(out.Items) {
		out.BatchStatus = batchOperationFailed
	}
	return out, nil
}

func feedbackCoverageRecovery(ref mcpcontract.ThreadRef, in mcpcontract.SyncPullRequestFeedbackInput, message string) *mcpcontract.RecoveryPlan {
	if in.MaxItemsPerChannel >= maxFeedbackItemsPerChannel {
		return nil
	}
	next := min(maxFeedbackItemsPerChannel, max(in.MaxItemsPerChannel*2, in.MaxItemsPerChannel+1))
	return recoveryPlan("facet_incomplete", message, mcpcontract.RecoveryAction(mcpcontract.SyncPullRequestFeedbackInput{
		PullRequests: []mcpcontract.ThreadRef{ref}, Channels: append([]string(nil), in.Channels...), ThreadState: in.ThreadState,
		MaxItemsPerChannel: next, MaxRequests: in.MaxRequests,
	}))
}

// persistPullRequestIdentity stores only the repository and exact PR identity
// needed by the feedback facets. It deliberately does not refresh repository
// metadata or unrelated thread headers.
func (r *MCPReader) persistPullRequestIdentity(ctx context.Context, ref mcpcontract.ThreadRef, header github.PullRequestDetails, sourceUpdatedAt time.Time) (string, error) {
	if header.Number != ref.Number || header.Number < 1 {
		return "pull_request_header_unavailable", fmt.Errorf("GitHub feedback did not return pull request %s", pullRequestKey(ref))
	}
	if header.UpdatedAt.IsZero() {
		header.UpdatedAt = sourceUpdatedAt
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return "persistence_retryable", err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return "persistence_retryable", fmt.Errorf("get repository identity: %w", err)
	}
	if repo == nil {
		payload, marshalErr := json.Marshal(map[string]string{
			"source": "pull_request_feedback_identity", "owner": ref.Owner, "repo": ref.Repo,
		})
		if marshalErr != nil {
			return "persistence_retryable", marshalErr
		}
		repo, err = corpus.RetryBusyValue(ctx, func(ctx context.Context) (*corpus.Repository, error) {
			return c.UpsertRepository(ctx, corpus.Repository{Owner: ref.Owner, Name: ref.Repo}, string(payload))
		})
		if err != nil {
			return "persistence_retryable", fmt.Errorf("upsert repository identity: %w", err)
		}
	}

	thread, err := threadFromPullRequestDetails(header, repo.ID)
	if err != nil {
		return "pull_request_header_unavailable", err
	}
	existing, err := c.GetThread(ctx, repo.ID, domain.PullRequestKind, ref.Number)
	if err != nil {
		return "persistence_retryable", fmt.Errorf("get pull request identity: %w", err)
	}
	if existing != nil {
		if thread.State == "" {
			thread.State = existing.State
		}
		// The feedback header does not carry GitHub's state reason. Keep that
		// richer observation instead of replacing it with an empty value.
		thread.StateReason = existing.StateReason
	}
	payload, err := json.Marshal(header)
	if err != nil {
		return "persistence_retryable", fmt.Errorf("marshal pull request identity: %w", err)
	}
	if _, err := corpus.RetryBusyValue(ctx, func(ctx context.Context) (*corpus.Thread, error) {
		return c.UpsertThread(ctx, thread, string(payload))
	}); err != nil {
		return "persistence_retryable", fmt.Errorf("upsert pull request identity: %w", err)
	}
	return "", nil
}

func threadFromPullRequestDetails(header github.PullRequestDetails, repositoryID int64) (corpus.Thread, error) {
	merge, err := parseGitHubMergeStatus(header)
	if err != nil {
		return corpus.Thread{}, fmt.Errorf("parse pull-request merge status: %w", err)
	}
	state, err := domain.ParseThreadState(header.State)
	if err != nil {
		return corpus.Thread{}, fmt.Errorf("parse pull-request state: %w", err)
	}
	thread := corpus.Thread{
		RepositoryID:      repositoryID,
		Kind:              domain.PullRequestKind,
		Number:            header.Number,
		State:             state,
		Title:             header.Title,
		Body:              header.Body,
		Author:            header.Author,
		AuthorAssociation: header.AuthorAssociation,
		Labels:            header.Labels,
		Assignees:         header.Assignees,
		Draft:             header.Draft,
		Locked:            header.Locked,
		Milestone:         header.Milestone,
		Merge:             merge,
		SourceCreatedAt:   header.CreatedAt,
		SourceUpdatedAt:   header.UpdatedAt,
	}
	if header.ClosedAt != nil {
		thread.ClosedAt = *header.ClosedAt
	}
	return thread, nil
}

func coveredFeedbackChannels(requested []corpus.FeedbackChannel, coverage map[string]github.FeedbackCoverage) []corpus.FeedbackChannel {
	channels := make([]corpus.FeedbackChannel, 0, len(requested))
	for _, channel := range requested {
		if _, ok := coverage[channel.String()]; ok {
			channels = append(channels, channel)
		}
	}
	return channels
}

func feedbackSnapshotComplete(snapshot github.PullRequestFeedback, channels []corpus.FeedbackChannel) bool {
	for _, channel := range channels {
		if !snapshot.Coverage[channel.String()].Complete {
			return false
		}
	}
	return true
}

func (r *MCPReader) persistPullRequestFeedback(ctx context.Context, ref mcpcontract.ThreadRef, snapshot github.PullRequestFeedback, channels []corpus.FeedbackChannel) error {
	for _, channel := range channels {
		facet, update, err := feedbackWorkflowFacet(snapshot, channel)
		if err != nil {
			return err
		}
		if err := r.persistPullRequestWorkflowFacet(ctx, ref, facet, snapshot.SourceUpdatedAt, update); err != nil {
			return err
		}
	}
	return nil
}

type workflowFacetUpdate struct {
	payload json.RawMessage
}

func incompleteWorkflowFacetUpdate() workflowFacetUpdate { return workflowFacetUpdate{} }

func completeWorkflowFacetUpdate[T any](value T) (workflowFacetUpdate, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return workflowFacetUpdate{}, err
	}
	return workflowFacetUpdate{payload: payload}, nil
}

func feedbackWorkflowFacetUpdate[T any](snapshot github.PullRequestFeedback, coverage github.FeedbackCoverage, selection string, items []T) (workflowFacetUpdate, error) {
	if !coverage.Complete {
		return incompleteWorkflowFacetUpdate(), nil
	}
	return completeWorkflowFacetUpdate(struct {
		HeadSHA   string                  `json:"head_sha"`
		Coverage  github.FeedbackCoverage `json:"coverage"`
		Selection string                  `json:"selection,omitempty"`
		Items     []T                     `json:"items"`
	}{HeadSHA: snapshot.HeadSHA, Coverage: coverage, Selection: selection, Items: items})
}

func feedbackWorkflowFacet(snapshot github.PullRequestFeedback, channel corpus.FeedbackChannel) (string, workflowFacetUpdate, error) {
	coverage := snapshot.Coverage[channel.String()]
	switch channel {
	case corpus.FeedbackIssueComments:
		update, err := feedbackWorkflowFacetUpdate(snapshot, coverage, "", snapshot.IssueComments)
		return channel.Facet(), update, err
	case corpus.FeedbackSubmittedReviews:
		update, err := feedbackWorkflowFacetUpdate(snapshot, coverage, "", snapshot.Reviews)
		return channel.Facet(), update, err
	case corpus.FeedbackInlineComments:
		update, err := feedbackWorkflowFacetUpdate(snapshot, coverage, "", snapshot.InlineComments)
		return channel.Facet(), update, err
	case corpus.FeedbackReviewThreads:
		update, err := feedbackWorkflowFacetUpdate(snapshot, coverage, snapshot.ThreadState, snapshot.ReviewThreads)
		return channel.Facet(), update, err
	default:
		return "", workflowFacetUpdate{}, fmt.Errorf("unsupported parsed feedback channel %d", channel)
	}
}

func (r *MCPReader) SyncCIFailures(ctx context.Context, in mcpcontract.SyncCIFailuresInput) (mcpcontract.JobReference, error) {
	refs, err := parsePullRequestRefs(in.PullRequests, "pull_requests")
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	in.PullRequests = refs
	if len(in.PullRequests) < 1 || len(in.PullRequests) > 20 {
		return mcpcontract.JobReference{}, errors.New("pull_requests must contain 1 to 20 items")
	}
	if in.Logs == "" {
		in.Logs = "none"
	}
	if in.Logs != "none" && in.Logs != "failures_only" {
		return mcpcontract.JobReference{}, errors.New("logs must be none or failures_only")
	}
	if in.MaxRunsPerPR == 0 {
		in.MaxRunsPerPR = 20
	}
	if in.MaxJobsPerRun == 0 {
		in.MaxJobsPerRun = 100
	}
	if in.MaxLogBytesPerJob == 0 {
		in.MaxLogBytesPerJob = 64 * 1024
	}
	if in.MaxRunsPerPR < 1 || in.MaxRunsPerPR > 100 || in.MaxJobsPerRun < 1 || in.MaxJobsPerRun > 100 {
		return mcpcontract.JobReference{}, errors.New("run and job bounds must be between 1 and 100")
	}
	if in.MaxLogBytesPerJob < 1024 || in.MaxLogBytesPerJob > 1024*1024 {
		return mcpcontract.JobReference{}, errors.New("max_log_bytes_per_job must be between 1024 and 1048576")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = 100
	}
	if in.MaxRequests < 1 || in.MaxRequests > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_requests must be between 1 and 1000")
	}
	id, err := r.submitJob(ctx, "sync_ci_failures", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncCIFailures(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_ci_failures", "CI failure synchronization job started"), nil
}

func (r *MCPReader) syncCIFailures(ctx context.Context, in mcpcontract.SyncCIFailuresInput, report func(string, string) error) (pullRequestWorkflowResult, error) {
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return pullRequestWorkflowResult{}, err
	}
	ciReader, ok := reader.(github.PullRequestCIReader)
	if !ok {
		return pullRequestWorkflowResult{}, errors.New("GitHub reader does not support CI diagnostics")
	}
	budget := github.NewRequestBudget(in.MaxRequests)
	out := pullRequestWorkflowResult{BatchStatus: batchOperationComplete, Items: make([]pullRequestWorkflowItem, len(in.PullRequests))}
	for index, ref := range in.PullRequests {
		if ref.Kind == "" {
			ref.Kind = string(domain.PullRequestKind)
		}
		item := pullRequestWorkflowItem{Key: pullRequestKey(ref), Status: mcpcontract.BatchItemComplete}
		snapshot, readErr := ciReader.GetPullRequestCI(ctx, ref.Owner, ref.Repo, ref.Number, github.CIFailureOptions{
			MaxRuns: in.MaxRunsPerPR, MaxJobsPerRun: in.MaxJobsPerRun, MaxLogBytes: in.MaxLogBytesPerJob, Logs: in.Logs,
		}, budget)
		if readErr != nil {
			var persistErr error
			// The PR lookup establishes the head even when the first child
			// collection consumes the remaining request budget. Advance the
			// facet in that case so an older complete report is not presented as
			// current after an unsuccessful refresh.
			if snapshot.HeadSHA != "" {
				persistErr = r.persistPullRequestWorkflowFacet(ctx, ref, facetPRCIReport, snapshot.SourceUpdatedAt, incompleteWorkflowFacetUpdate())
			}
			if persistErr != nil {
				item.Status, item.Code, item.Message = "failed", "persist_partial_ci_failed", persistErr.Error()
			} else {
				item = workflowFailure(ref, readErr, mcpcontract.ToolSyncCIFailures)
			}
			out.BatchStatus = batchOperationPartial
		} else {
			complete := ciSnapshotComplete(snapshot)
			update := incompleteWorkflowFacetUpdate()
			var persistErr error
			if complete {
				update, persistErr = completeWorkflowFacetUpdate(snapshot)
			}
			if persistErr == nil {
				persistErr = r.persistPullRequestWorkflowFacet(ctx, ref, facetPRCIReport, snapshot.SourceUpdatedAt, update)
			}
			if persistErr != nil {
				item.Status, item.Code, item.Message = "failed", "persist_ci_failed", persistErr.Error()
				out.BatchStatus = batchOperationPartial
			} else if !complete {
				item.Status = "retryable"
				item.Code = "ci_coverage_incomplete"
				item.Message = "one or more CI collections reached a configured item bound"
				item.Recovery = recoveryPlan("facet_incomplete", item.Message, mcpcontract.RecoveryAction(mcpcontract.SyncCIFailuresInput{PullRequests: []mcpcontract.ThreadRef{ref}, Logs: in.Logs, MaxRunsPerPR: in.MaxRunsPerPR, MaxJobsPerRun: in.MaxJobsPerRun, MaxLogBytesPerJob: in.MaxLogBytesPerJob, MaxRequests: in.MaxRequests}))
				item.HeadSHA = snapshot.HeadSHA
				out.BatchStatus = batchOperationPartial
			} else {
				item.HeadSHA = snapshot.HeadSHA
				item.ResourceURI = fmt.Sprintf("gitcontribute://ci-failure-report/%s/%s/%d", ref.Owner, ref.Repo, ref.Number)
			}
		}
		out.Items[index] = item
		if err := report("ci_failures", jobProgressCounts(index+1, len(in.PullRequests))); err != nil {
			return pullRequestWorkflowResult{}, err
		}
	}
	out.Requests = budget.Completed()
	if out.BatchStatus == batchOperationPartial && allWorkflowItemsFailed(out.Items) {
		out.BatchStatus = batchOperationFailed
	}
	return out, nil
}

func ciSnapshotComplete(snapshot github.PullRequestCI) bool {
	for _, coverage := range snapshot.Coverage {
		if !coverage.Complete {
			return false
		}
	}
	for _, run := range snapshot.Runs {
		if run.JobsTruncated {
			return false
		}
	}
	return true
}

func (r *MCPReader) persistPullRequestWorkflowFacet(ctx context.Context, ref mcpcontract.ThreadRef, facet string, sourceUpdatedAt time.Time, update workflowFacetUpdate) error {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil || repo == nil {
		if err == nil {
			err = errFeedbackRepositoryNotStored
		}
		return err
	}
	if ref.Kind == "" {
		ref.Kind = string(domain.PullRequestKind)
	}
	thread, err := c.GetThread(ctx, repo.ID, domain.PullRequestKind, ref.Number)
	if err != nil || thread == nil {
		if err == nil {
			err = errFeedbackPullRequestNotStored
		}
		return err
	}
	if sourceUpdatedAt.IsZero() {
		sourceUpdatedAt = thread.SourceUpdatedAt
	}
	if update.payload == nil {
		return c.AdvanceFacet(ctx, repo.ID, &thread.ID, facet, sourceUpdatedAt, false, 0)
	}
	return c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, facet, sourceUpdatedAt, []corpus.FacetObservationInput{{SourceUpdatedAt: sourceUpdatedAt, Payload: string(update.payload)}}, true, 0)
}

func feedbackPersistenceFailureCode(err error) string {
	switch {
	case errors.Is(err, errFeedbackRepositoryNotStored):
		return "repository_identity_unavailable"
	case errors.Is(err, errFeedbackPullRequestNotStored):
		return "pull_request_header_unavailable"
	default:
		return "persistence_retryable"
	}
}

func feedbackPersistenceFailure(ref mcpcontract.ThreadRef, code, message string) pullRequestWorkflowItem {
	item := pullRequestWorkflowItem{Key: pullRequestKey(ref), Status: "failed", Code: code, Message: message}
	if code == "persistence_retryable" {
		item.Status = "retryable"
		item.RetryAfterMS = 1000
		item.Recovery = recoveryPlan(code, message, workflowRetryCall(mcpcontract.ToolSyncPullRequestFeedback, ref))
	}
	return item
}

func workflowFailure(ref mcpcontract.ThreadRef, err error, tool string) pullRequestWorkflowItem {
	if errors.Is(err, github.ErrRequestBudgetExhausted) {
		return pullRequestWorkflowItem{
			Key: pullRequestKey(ref), Status: "retryable", Code: "request_budget_exhausted", Message: err.Error(),
			Recovery: recoveryPlan("request_budget_exhausted", err.Error(), workflowRetryCall(tool, ref)),
		}
	}
	status, code, message, retryAfterMS := githubBatchError(err)
	item := pullRequestWorkflowItem{
		Key: pullRequestKey(ref), Status: status, Code: code,
		Message: message, RetryAfterMS: retryAfterMS,
	}
	if status == mcpcontract.BatchItemRetryable {
		item.Recovery = recoveryPlan(code, message, workflowRetryCall(tool, ref))
	}
	return item
}

func workflowRetryCall(tool string, ref mcpcontract.ThreadRef) mcpcontract.ToolCall {
	if tool == mcpcontract.ToolSyncCIFailures {
		return mcpcontract.RecoveryAction(mcpcontract.SyncCIFailuresInput{PullRequests: []mcpcontract.ThreadRef{ref}, MaxRequests: 1000})
	}
	return mcpcontract.RecoveryAction(mcpcontract.SyncPullRequestFeedbackInput{PullRequests: []mcpcontract.ThreadRef{ref}, MaxRequests: 1000})
}

func pullRequestKey(ref mcpcontract.ThreadRef) string {
	return threadRefKey(ref)
}

func allWorkflowItemsFailed(items []pullRequestWorkflowItem) bool {
	for _, item := range items {
		if item.Status == mcpcontract.BatchItemComplete || item.Status == mcpcontract.BatchItemRetryable {
			return false
		}
	}
	return true
}

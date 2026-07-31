package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const (
	facetPRFeedbackIssueComments  = "pr_feedback_issue_comments"
	facetPRFeedbackReviews        = "pr_feedback_reviews"
	facetPRFeedbackInlineComments = "pr_feedback_inline_comments"
	facetPRFeedbackReviewThreads  = "pr_feedback_review_threads"
	facetPRCIReport               = "pr_ci_report"
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
	BatchStatus string                    `json:"batch_status"`
	Items       []pullRequestWorkflowItem `json:"items"`
	Requests    int                       `json:"requests"`
}

func (r *MCPReader) SyncPullRequestFeedback(ctx context.Context, in mcpcontract.SyncPullRequestFeedbackInput) (mcpcontract.JobReference, error) {
	if err := rejectDuplicateThreadRefs(in.PullRequests); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if err := validatePullRequestRefs(in.PullRequests, "pull_requests"); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if len(in.PullRequests) < 1 || len(in.PullRequests) > 50 {
		return mcpcontract.JobReference{}, errors.New("pull_requests must contain 1 to 50 items")
	}
	if in.ThreadState == "" {
		in.ThreadState = "unresolved"
	}
	if in.ThreadState != "unresolved" && in.ThreadState != "all" {
		return mcpcontract.JobReference{}, errors.New("thread_state must be unresolved or all")
	}
	if err := validateFeedbackChannels(in.Channels); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if in.MaxItemsPerChannel == 0 {
		in.MaxItemsPerChannel = 300
	}
	if in.MaxItemsPerChannel < 1 || in.MaxItemsPerChannel > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_items_per_channel must be between 1 and 1000")
	}
	if in.MaxRequests == 0 {
		in.MaxRequests = 100
	}
	if in.MaxRequests < 1 || in.MaxRequests > 1000 {
		return mcpcontract.JobReference{}, errors.New("max_requests must be between 1 and 1000")
	}
	id, err := r.submitJob(ctx, "sync_pull_request_feedback", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.syncPullRequestFeedback(ctx, in, report)
	})
	if err != nil {
		return mcpcontract.JobReference{}, err
	}
	return queuedJobReference(id, "sync_pull_request_feedback", "pull-request feedback synchronization job started"), nil
}

func validateFeedbackChannels(channels []string) error {
	if len(channels) < 1 || len(channels) > 4 {
		return errors.New("channels must contain 1 to 4 items")
	}
	seen := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		switch channel {
		case "issue_comments", "submitted_reviews", "inline_comments", "review_threads":
		default:
			return fmt.Errorf("unsupported feedback channel %q", channel)
		}
		if _, ok := seen[channel]; ok {
			return fmt.Errorf("duplicate feedback channel %q", channel)
		}
		seen[channel] = struct{}{}
	}
	return nil
}

func (r *MCPReader) syncPullRequestFeedback(ctx context.Context, in mcpcontract.SyncPullRequestFeedbackInput, report func(string, string) error) (pullRequestWorkflowResult, error) {
	reader, err := r.githubReader() //nolint:contextcheck // Client construction performs no request; operations below receive ctx.
	if err != nil {
		return pullRequestWorkflowResult{}, err
	}
	feedbackReader, ok := reader.(github.PullRequestFeedbackReader)
	if !ok {
		return pullRequestWorkflowResult{}, errors.New("GitHub reader does not support pull-request feedback")
	}
	budget := github.NewRequestBudget(in.MaxRequests)
	out := pullRequestWorkflowResult{BatchStatus: "complete", Items: make([]pullRequestWorkflowItem, len(in.PullRequests))}
	for index, ref := range in.PullRequests {
		if ref.Kind == "" {
			ref.Kind = corpus.ThreadKindPullRequest
		}
		item := pullRequestWorkflowItem{Key: pullRequestKey(ref), Status: "complete"}
		snapshot, readErr := feedbackReader.GetPullRequestFeedback(ctx, ref.Owner, ref.Repo, ref.Number, github.PullRequestFeedbackOptions{
			Channels: in.Channels, ThreadState: in.ThreadState, MaxItemsPerChannel: in.MaxItemsPerChannel,
		}, budget)
		snapshot.ThreadState = in.ThreadState
		if readErr != nil {
			var persistErr error
			if len(snapshot.Coverage) > 0 {
				persistErr = r.persistPullRequestFeedback(ctx, ref, snapshot, coveredFeedbackChannels(in.Channels, snapshot.Coverage))
			}
			if persistErr != nil {
				item.Status, item.Code, item.Message = "failed", "persist_partial_feedback_failed", persistErr.Error()
			} else {
				item = workflowFailure(ref, readErr, mcpcontract.ToolSyncPullRequestFeedback)
			}
			out.BatchStatus = "partial"
		} else if err := r.persistPullRequestFeedback(ctx, ref, snapshot, in.Channels); err != nil {
			item.Status, item.Code, item.Message = "failed", "persist_feedback_failed", err.Error()
			out.BatchStatus = "partial"
		} else if !feedbackSnapshotComplete(snapshot, in.Channels) {
			item.Status = "retryable"
			item.Code = "feedback_coverage_incomplete"
			item.Message = "one or more feedback channels reached max_items_per_channel"
			item.Recovery = recoveryPlan("facet_incomplete", item.Message, mcpcontract.ToolCall{Tool: mcpcontract.ToolSyncPullRequestFeedback, Arguments: &mcpcontract.ToolCallArguments{PullRequests: []mcpcontract.ThreadRef{ref}, Channels: append([]string(nil), in.Channels...), ThreadState: in.ThreadState, MaxItemsPerChannel: in.MaxItemsPerChannel * 2, MaxRequests: in.MaxRequests}})
			item.HeadSHA = snapshot.HeadSHA
			out.BatchStatus = "partial"
		} else {
			item.HeadSHA = snapshot.HeadSHA
			item.ResourceURI = fmt.Sprintf("gitcontribute://pull-request-feedback/%s/%s/%d", ref.Owner, ref.Repo, ref.Number)
		}
		out.Items[index] = item
		if err := report("pull_request_feedback", jobProgressCounts(index+1, len(in.PullRequests))); err != nil {
			return pullRequestWorkflowResult{}, err
		}
	}
	out.Requests = budget.Completed()
	if out.BatchStatus == "partial" && allWorkflowItemsFailed(out.Items) {
		out.BatchStatus = "failed"
	}
	return out, nil
}

func coveredFeedbackChannels(requested []string, coverage map[string]github.FeedbackCoverage) []string {
	channels := make([]string, 0, len(requested))
	for _, channel := range requested {
		if _, ok := coverage[channel]; ok {
			channels = append(channels, channel)
		}
	}
	return channels
}

func feedbackSnapshotComplete(snapshot github.PullRequestFeedback, channels []string) bool {
	for _, channel := range channels {
		if !snapshot.Coverage[channel].Complete {
			return false
		}
	}
	return true
}

func (r *MCPReader) persistPullRequestFeedback(ctx context.Context, ref mcpcontract.ThreadRef, snapshot github.PullRequestFeedback, channels []string) error {
	values := map[string]any{
		"issue_comments":    snapshot.IssueComments,
		"submitted_reviews": snapshot.Reviews,
		"inline_comments":   snapshot.InlineComments,
		"review_threads":    snapshot.ReviewThreads,
	}
	facets := map[string]string{
		"issue_comments": facetPRFeedbackIssueComments, "submitted_reviews": facetPRFeedbackReviews,
		"inline_comments": facetPRFeedbackInlineComments, "review_threads": facetPRFeedbackReviewThreads,
	}
	for _, channel := range channels {
		payload := struct {
			HeadSHA   string                  `json:"head_sha"`
			Coverage  github.FeedbackCoverage `json:"coverage"`
			Selection string                  `json:"selection,omitempty"`
			Items     any                     `json:"items"`
		}{HeadSHA: snapshot.HeadSHA, Coverage: snapshot.Coverage[channel], Items: values[channel]}
		if channel == "review_threads" {
			payload.Selection = snapshot.ThreadState
		}
		if err := r.persistPullRequestWorkflowFacet(ctx, ref, facets[channel], snapshot.SourceUpdatedAt, payload, snapshot.Coverage[channel].Complete); err != nil {
			return err
		}
	}
	return nil
}

func (r *MCPReader) SyncCIFailures(ctx context.Context, in mcpcontract.SyncCIFailuresInput) (mcpcontract.JobReference, error) {
	if err := rejectDuplicateThreadRefs(in.PullRequests); err != nil {
		return mcpcontract.JobReference{}, err
	}
	if err := validatePullRequestRefs(in.PullRequests, "pull_requests"); err != nil {
		return mcpcontract.JobReference{}, err
	}
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
	out := pullRequestWorkflowResult{BatchStatus: "complete", Items: make([]pullRequestWorkflowItem, len(in.PullRequests))}
	for index, ref := range in.PullRequests {
		if ref.Kind == "" {
			ref.Kind = corpus.ThreadKindPullRequest
		}
		item := pullRequestWorkflowItem{Key: pullRequestKey(ref), Status: "complete"}
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
				persistErr = r.persistPullRequestWorkflowFacet(ctx, ref, facetPRCIReport, snapshot.SourceUpdatedAt, snapshot, false)
			}
			if persistErr != nil {
				item.Status, item.Code, item.Message = "failed", "persist_partial_ci_failed", persistErr.Error()
			} else {
				item = workflowFailure(ref, readErr, mcpcontract.ToolSyncCIFailures)
			}
			out.BatchStatus = "partial"
		} else {
			complete := ciSnapshotComplete(snapshot)
			if err := r.persistPullRequestWorkflowFacet(ctx, ref, facetPRCIReport, snapshot.SourceUpdatedAt, snapshot, complete); err != nil {
				item.Status, item.Code, item.Message = "failed", "persist_ci_failed", err.Error()
				out.BatchStatus = "partial"
			} else if !complete {
				item.Status = "retryable"
				item.Code = "ci_coverage_incomplete"
				item.Message = "one or more CI collections reached a configured item bound"
				item.Recovery = recoveryPlan("facet_incomplete", item.Message, mcpcontract.ToolCall{Tool: mcpcontract.ToolSyncCIFailures, Arguments: &mcpcontract.ToolCallArguments{PullRequests: []mcpcontract.ThreadRef{ref}, Logs: in.Logs, MaxRunsPerPR: in.MaxRunsPerPR, MaxJobsPerRun: in.MaxJobsPerRun, MaxLogBytesPerJob: in.MaxLogBytesPerJob, MaxRequests: in.MaxRequests}})
				item.HeadSHA = snapshot.HeadSHA
				out.BatchStatus = "partial"
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
	if out.BatchStatus == "partial" && allWorkflowItemsFailed(out.Items) {
		out.BatchStatus = "failed"
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

func (r *MCPReader) persistPullRequestWorkflowFacet(ctx context.Context, ref mcpcontract.ThreadRef, facet string, sourceUpdatedAt time.Time, value any, complete bool) error {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil || repo == nil {
		if err == nil {
			err = errors.New("repository is not stored")
		}
		return err
	}
	if ref.Kind == "" {
		ref.Kind = corpus.ThreadKindPullRequest
	}
	thread, err := c.GetThread(ctx, repo.ID, ref.Kind, ref.Number)
	if err != nil || thread == nil {
		if err == nil {
			err = errors.New("pull request is not stored")
		}
		return err
	}
	if sourceUpdatedAt.IsZero() {
		sourceUpdatedAt = thread.SourceUpdatedAt
	}
	if !complete {
		return c.AdvanceFacet(ctx, repo.ID, &thread.ID, facet, sourceUpdatedAt, false, 0)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.ApplyFacetObservationSet(ctx, repo.ID, &thread.ID, facet, sourceUpdatedAt, []corpus.FacetObservationInput{{SourceUpdatedAt: sourceUpdatedAt, Payload: string(payload)}}, complete, 0)
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
		Key: pullRequestKey(ref), Status: mcpcontract.BatchItemStatus(status), Code: code,
		Message: message, RetryAfterMS: retryAfterMS,
	}
	if status == "retryable" {
		item.Recovery = recoveryPlan(code, message, workflowRetryCall(tool, ref))
	}
	return item
}

func workflowRetryCall(tool string, ref mcpcontract.ThreadRef) mcpcontract.ToolCall {
	args := &mcpcontract.ToolCallArguments{PullRequests: []mcpcontract.ThreadRef{ref}, MaxRequests: 1000}
	return mcpcontract.ToolCall{Tool: tool, Arguments: args}
}

func pullRequestKey(ref mcpcontract.ThreadRef) string {
	return threadRefKey(ref)
}

func allWorkflowItemsFailed(items []pullRequestWorkflowItem) bool {
	for _, item := range items {
		if item.Status == "complete" || item.Status == "retryable" {
			return false
		}
	}
	return true
}

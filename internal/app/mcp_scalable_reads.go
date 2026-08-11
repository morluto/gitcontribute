package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
	"github.com/morluto/gitcontribute/internal/radar"
)

// GetRepositories performs an offline, input-ordered corpus read and clears
// repository facts when metadata coverage has not been observed.
func (r *MCPReader) GetRepositories(ctx context.Context, in mcpcontract.GetRepositoriesInput) (mcpcontract.GetRepositoriesOutput, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 100 {
		return mcpcontract.GetRepositoriesOutput{}, errors.New("repositories must contain 1 to 100 items")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GetRepositoriesOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.GetRepositoriesOutput{}, err
	}
	out := mcpcontract.GetRepositoriesOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.RepositoryOutput], len(in.Repositories)), SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	repositoryKeys := make([]corpus.RepositoryKey, 0, len(in.Repositories))
	for _, input := range in.Repositories {
		if ref, err := domain.NewRepoRef(input.Owner, input.Repo); err == nil {
			repositoryKeys = append(repositoryKeys, corpus.RepositoryKey{Owner: ref.Owner(), Name: ref.Repo()})
		}
	}
	repositories, err := c.GetRepositoriesBatch(ctx, repositoryKeys)
	if err != nil {
		return mcpcontract.GetRepositoriesOutput{}, err
	}
	repositoryIDs := make([]int64, 0, len(repositories))
	for _, repo := range repositories {
		repositoryIDs = append(repositoryIDs, repo.ID)
	}
	coverageByRepository, err := c.ListRepositoryCoverageBatch(ctx, repositoryIDs, []string{"metadata"})
	if err != nil {
		return mcpcontract.GetRepositoriesOutput{}, err
	}
	dossiersByRepository, err := c.GetLatestDossierMetadataBatch(ctx, repositoryIDs)
	if err != nil {
		return mcpcontract.GetRepositoriesOutput{}, err
	}
	for i, input := range in.Repositories {
		key := input.Owner + "/" + input.Repo
		item := mcpcontract.BatchItem[mcpcontract.RepositoryOutput]{Key: key, Status: "complete"}
		ref, err := domain.NewRepoRef(input.Owner, input.Repo)
		if err != nil {
			item.Status, item.Reason, item.Message = "failed", "invalid_reference", err.Error()
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		repo := repositories[corpus.RepositoryKey{Owner: ref.Owner(), Name: ref.Repo()}]
		if repo == nil {
			item.Status, item.Reason, item.Message = "unavailable", "repository_not_indexed", "repository is not present in the local corpus"
			item.Recovery = recoveryPlan(item.Reason, item.Message, syncRepositoryContextCall(input.Owner, input.Repo))
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		value := repositoryOutput(repo)
		value.DossierStatus = "missing"
		if dossierMetadata, ok := dossiersByRepository[repo.ID]; ok {
			value.DossierStatus = "available"
			value.DossierAsOf = formatTime(dossierMetadata.AsOf)
		}
		coverage := coverageByRepository[corpus.RepositoryFacetKey{RepositoryID: repo.ID, Facet: "metadata"}]
		if coverage == nil {
			value.Metadata = mcpcontract.RepositoryMetadataOutput{Status: "missing", Recovery: recoveryPlan("facet_not_observed", "repository metadata is not observed", syncRepositoryContextCall(input.Owner, input.Repo))}
			clearRepositoryFacts(&value)
		} else {
			status := "complete"
			if !coverage.Complete {
				status = "partial"
				item.Status, item.Reason, item.Message = "partial", "facet_incomplete", "repository metadata coverage is incomplete"
				item.Recovery = recoveryPlan("facet_incomplete", "Repository metadata coverage is incomplete; refresh this exact repository before relying on its absence or facts.", syncRepositoryContextCall(input.Owner, input.Repo))
				out.Status = "partial"
			}
			metadata := mcpcontract.RepositoryMetadataOutput{Status: status, ObservedAt: formatTime(coverage.UpdatedAt), SourceUpdatedAt: formatTime(coverage.SourceUpdatedAt)}
			if !coverage.Complete {
				metadata.Recovery = recoveryPlan("facet_incomplete", "Repository metadata coverage is incomplete; refresh this exact repository before relying on its absence or facts.", syncRepositoryContextCall(input.Owner, input.Repo))
			}
			value.Metadata = metadata
		}
		item.Value = &value
		out.Items[i] = item
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.GetRepositoriesOutput{}, err
	}
	return out, nil
}

func repositoryOutput(repo *corpus.Repository) mcpcontract.RepositoryOutput {
	return mcpcontract.RepositoryOutput{Ref: "repository:" + repo.Owner + "/" + repo.Name, Owner: repo.Owner, Repo: repo.Name, UpdatedAt: formatTime(repo.SourceUpdatedAt), Description: ptr(repo.Description), DefaultBranch: ptr(repo.DefaultBranch), Language: ptr(repo.Language), License: ptr(repo.License), Topics: append([]string(nil), repo.Topics...), Stars: ptr(repo.Stars), Watchers: ptr(repo.Watchers), Forks: ptr(repo.Forks), OpenIssues: ptr(repo.OpenIssues), Archived: ptr(repo.Archived), Fork: ptr(repo.Fork)}
}

func clearRepositoryFacts(v *mcpcontract.RepositoryOutput) {
	v.Description = nil
	v.DefaultBranch = nil
	v.Language = nil
	v.License = nil
	v.Topics = nil
	v.Stars = nil
	v.Watchers = nil
	v.Forks = nil
	v.OpenIssues = nil
	v.Archived = nil
	v.Fork = nil
}
func ptr[T any](v T) *T { return &v }

// GetThreads performs an offline, input-ordered exact-thread read. Compact mode
// omits bodies to keep broad triage responses bounded.
func (r *MCPReader) GetThreads(ctx context.Context, in mcpcontract.GetThreadsInput) (mcpcontract.GetThreadsOutput, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 100 {
		return mcpcontract.GetThreadsOutput{}, errors.New("threads must contain 1 to 100 items")
	}
	if in.View == "" {
		in.View = "compact"
	}
	if in.View != "compact" && in.View != "full" {
		return mcpcontract.GetThreadsOutput{}, errors.New("view must be compact or full")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GetThreadsOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.GetThreadsOutput{}, err
	}
	out := mcpcontract.GetThreadsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.ThreadOutput], len(in.Threads)), SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	parsed := make([]*parsedThreadReference, len(in.Threads))
	repositoryKeys := make([]corpus.RepositoryKey, 0, len(in.Threads))
	for i, input := range in.Threads {
		ref, parseErr := parseThreadReference(input)
		if parseErr != nil {
			continue
		}
		parsed[i] = &ref
		repositoryKeys = append(repositoryKeys, ref.repositoryKey())
	}
	repositories, err := c.GetRepositoriesBatch(ctx, repositoryKeys)
	if err != nil {
		return mcpcontract.GetThreadsOutput{}, err
	}
	threadKeys := make([]corpus.ThreadKey, 0, len(in.Threads))
	for _, ref := range parsed {
		if ref == nil {
			continue
		}
		repo := repositories[ref.repositoryKey()]
		if repo != nil {
			threadKeys = append(threadKeys, ref.threadKey(repo.ID))
		}
	}
	threads, err := c.GetThreadsBatch(ctx, threadKeys)
	if err != nil {
		return mcpcontract.GetThreadsOutput{}, err
	}
	for i, input := range in.Threads {
		key := threadRefKey(input)
		item := mcpcontract.BatchItem[mcpcontract.ThreadOutput]{Key: key, Status: "complete"}
		if parsed[i] == nil {
			item.Status, item.Reason, item.Message = "failed", "invalid_reference", "invalid thread reference"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		ref := *parsed[i]
		wire := ref.wire()
		item.Key = threadRefKey(wire)
		repo := repositories[ref.repositoryKey()]
		if repo == nil {
			item.Status, item.Reason, item.Message = "unavailable", "repository_not_indexed", "repository is not present in the local corpus"
			item.Recovery = recoveryPlan(item.Reason, item.Message, syncRepositoryContextCall(ref.repository.Owner(), ref.repository.Repo()))
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		thread := threads[ref.threadKey(repo.ID)]
		if thread == nil {
			item.Status, item.Reason, item.Message = "unavailable", "thread_not_indexed", "thread is not present in the local corpus"
			item.Recovery = recoveryPlan(item.Reason, item.Message, syncThreadCall(wire))
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		value := corpusThreadToMCPOutput(thread)
		value.Owner, value.Repo = ref.repository.Owner(), ref.repository.Repo()
		value.SnapshotToken = snapshotIdentity(in.SnapshotToken, revision)
		if in.View == "compact" {
			value.Body = ""
		}
		item.Value = &value
		out.Items[i] = item
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.GetThreadsOutput{}, err
	}
	return out, nil
}

// GetJobs reads up to 100 durable job records without waiting for completion.
func (r *MCPReader) GetJobs(ctx context.Context, in mcpcontract.GetJobsInput) (mcpcontract.GetJobsOutput, error) {
	ids := append([]string(nil), in.IDs...)
	if len(ids) < 1 || len(ids) > 100 {
		return mcpcontract.GetJobsOutput{}, errors.New("ids must contain 1 to 100 items")
	}
	format, err := parseResponseFormat(in.ResponseFormat)
	if err != nil {
		return mcpcontract.GetJobsOutput{}, err
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GetJobsOutput{}, err
	}
	var storedJobs map[string]*corpus.Job
	if format.includesDetails() {
		storedJobs, err = c.GetJobsBatch(ctx, ids)
	} else {
		storedJobs, err = c.GetJobSummariesBatch(ctx, ids)
	}
	if err != nil {
		return mcpcontract.GetJobsOutput{}, err
	}
	out := mcpcontract.GetJobsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.GetJobOutput], len(ids))}
	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		item := mcpcontract.BatchItem[mcpcontract.GetJobOutput]{Key: id, Status: "complete"}
		stored := storedJobs[id]
		if stored == nil {
			item.Status, item.Reason, item.Message = "unavailable", "not_found", "job is not present in the local corpus"
			out.Status = "partial"
		} else {
			job := jobResultToMCP(ptr(jobResult(stored)), format)
			if !format.includesDetails() && (job.Status == "succeeded" || job.Status == "failed" || job.Status == "cancelled") {
				item.Recovery = recoveryPlan("blocked", "Read the detailed typed artifact and follow-up references.", mcpcontract.RecoveryAction(mcpcontract.GetJobsInput{IDs: []string{id}, ResponseFormat: detailedResponse.String()}))
			}
			item.Value = &job
		}
		out.Items[i] = item
	}
	return out, nil
}

type portfolioReadSet struct {
	coverage     map[corpus.ThreadFacetKey]*corpus.Coverage
	observations map[corpus.ThreadFacetKey]corpus.FacetObservationBatch
}

func loadPortfolioReadSet(ctx context.Context, c *corpus.Corpus, pullRequests []corpus.PortfolioPullRequest, format responseFormat) (portfolioReadSet, error) {
	threadIDs := make([]int64, 0, len(pullRequests))
	for _, stored := range pullRequests {
		threadIDs = append(threadIDs, stored.Thread.ID)
	}
	facets := portfolioFacets()
	coverage, err := c.ListThreadCoverageBatch(ctx, threadIDs, facets)
	if err != nil {
		return portfolioReadSet{}, err
	}
	singletonFacets := make([]string, 0, len(facets)-1)
	for _, facet := range facets {
		if facet != FacetPRReviews && (format.includesDetails() || (facet != FacetPRClosingIssues && facet != FacetPRFiles)) {
			singletonFacets = append(singletonFacets, facet)
		}
	}
	observations, err := c.ListThreadFacetObservationsBatch(ctx, threadIDs, singletonFacets, 1)
	if err != nil {
		return portfolioReadSet{}, err
	}
	reviews, err := c.ListThreadFacetObservationsBatch(ctx, threadIDs, []string{FacetPRReviews}, 100)
	if err != nil {
		return portfolioReadSet{}, err
	}
	for key, batch := range reviews {
		observations[key] = batch
	}
	return portfolioReadSet{coverage: coverage, observations: observations}, nil
}

func portfolioItem(stored corpus.PortfolioPullRequest, now time.Time, readSet portfolioReadSet, format responseFormat) (mcpcontract.PullRequestPortfolioItem, error) {
	t := stored.Thread
	out := mcpcontract.PullRequestPortfolioItem{Ref: fmt.Sprintf("%s/%s#%d", stored.Owner, stored.Repo, t.Number), Owner: stored.Owner, Repo: stored.Repo, Number: t.Number, Title: t.Title, State: string(t.State), Author: t.Author, Draft: t.Draft, SourceUpdatedAt: formatTime(t.SourceUpdatedAt), StatusCoverage: "missing"}
	coverage := portfolioCoverage(&out, t.ID, readSet.coverage, format)
	details, err := applyPortfolioDetails(&out, t.ID, coverage[FacetPRDetails], readSet.observations, format)
	if err != nil {
		return out, fmt.Errorf("decode pull-request details for %s: %w", out.Ref, err)
	}
	if err := applyPortfolioReviews(&out, t.ID, coverage[FacetPRReviews], readSet.observations); err != nil {
		return out, fmt.Errorf("decode pull-request reviews for %s: %w", out.Ref, err)
	}
	mergeabilityKnown, err := applyPortfolioHealth(&out, t.ID, coverage, readSet.observations)
	if err != nil {
		return out, err
	}
	if err := applyPortfolioSupplementalDetails(&out, t.ID, coverage, readSet.observations, format); err != nil {
		return out, err
	}
	addPortfolioCoverageReasons(&out, coverage, mergeabilityKnown)
	setPortfolioAttention(&out, t, details, coverage, mergeabilityKnown, now)
	return out, nil
}

func portfolioCoverage(out *mcpcontract.PullRequestPortfolioItem, threadID int64, all map[corpus.ThreadFacetKey]*corpus.Coverage, format responseFormat) map[string]*corpus.Coverage {
	facets := portfolioFacets()
	coverage := make(map[string]*corpus.Coverage, len(facets))
	complete, observed := true, 0
	for _, facet := range facets {
		cov := all[corpus.ThreadFacetKey{ThreadID: threadID, Facet: facet}]
		coverage[facet] = cov
		status := "missing"
		if cov != nil {
			observed++
			status = "incomplete"
			if cov.Complete {
				status = "complete"
			}
		}
		if cov == nil || !cov.Complete {
			complete = false
		}
		if format.includesDetails() {
			entry := mcpcontract.FacetCoverageOutput{Facet: facet, Status: status}
			if cov != nil {
				entry.Complete, entry.UpdatedAt = cov.Complete, formatTime(cov.UpdatedAt)
			}
			out.Facets = append(out.Facets, entry)
		}
	}
	if observed > 0 {
		out.StatusCoverage = "partial"
	}
	if complete {
		out.StatusCoverage = "complete"
	}
	return coverage
}

func applyPortfolioDetails(out *mcpcontract.PullRequestPortfolioItem, threadID int64, coverage *corpus.Coverage, observations map[corpus.ThreadFacetKey]corpus.FacetObservationBatch, format responseFormat) (github.PullRequestDetails, error) {
	var details github.PullRequestDetails
	if coverage == nil || !coverage.Complete {
		return details, nil
	}
	observedAt, err := decodeLatestFacet(observations, threadID, FacetPRDetails, &details)
	if err != nil {
		return details, err
	}
	out.Mergeable, out.StatusObservedAt = details.Mergeable, observedAt
	if format.includesDetails() {
		out.HeadRef, out.HeadSHA, out.BaseRef, out.BaseSHA = details.HeadRef, details.HeadSHA, details.BaseRef, details.BaseSHA
	}
	return details, nil
}

func applyPortfolioReviews(out *mcpcontract.PullRequestPortfolioItem, threadID int64, coverage *corpus.Coverage, observations map[corpus.ThreadFacetKey]corpus.FacetObservationBatch) error {
	if coverage == nil || !coverage.Complete {
		return nil
	}
	latest := make(map[string]github.Review)
	for _, observation := range observations[corpus.ThreadFacetKey{ThreadID: threadID, Facet: FacetPRReviews}].Observations {
		var reviews []github.Review
		if err := json.Unmarshal([]byte(observation.Payload), &reviews); err != nil {
			return err
		}
		for _, review := range reviews {
			key := strings.ToLower(review.Author)
			previous, ok := latest[key]
			if !ok || review.SubmittedAt.After(previous.SubmittedAt) {
				latest[key] = review
			}
		}
	}
	changes, approved := false, false
	for _, review := range latest {
		switch strings.ToUpper(review.State) {
		case "CHANGES_REQUESTED":
			changes = true
		case "APPROVED":
			approved = true
		}
	}
	if changes {
		out.ReviewDecision = "changes_requested"
	} else if approved {
		out.ReviewDecision = "approved"
	}
	return nil
}

func applyPortfolioHealth(out *mcpcontract.PullRequestPortfolioItem, threadID int64, coverage map[string]*corpus.Coverage, observations map[corpus.ThreadFacetKey]corpus.FacetObservationBatch) (bool, error) {
	mergeabilityKnown := false
	if cov := coverage[FacetPRMergeState]; cov != nil && cov.Complete {
		var value github.PullRequestMergeState
		if _, err := decodeLatestFacet(observations, threadID, FacetPRMergeState, &value); err != nil {
			return false, err
		}
		out.MergeStateStatus = strings.ToLower(value.MergeStateStatus)
		if mergeability, known := value.Mergeability(); known {
			mergeabilityKnown = true
			mergeable := strings.EqualFold(mergeability, "MERGEABLE")
			out.Mergeable = &mergeable
		}
	}
	if cov := coverage[FacetPRChecks]; cov != nil && cov.Complete {
		var checks []github.PullRequestCheck
		if _, err := decodeLatestFacet(observations, threadID, FacetPRChecks, &checks); err != nil {
			return false, err
		}
		out.ChecksTotal, out.ChecksStatus = len(checks), classifyChecks(checks)
	}
	if cov := coverage[FacetPRReviewThreads]; cov != nil && cov.Complete {
		var threads []github.PullRequestReviewThread
		if _, err := decodeLatestFacet(observations, threadID, FacetPRReviewThreads, &threads); err != nil {
			return false, err
		}
		unresolved := 0
		for _, thread := range threads {
			if !thread.IsResolved && !thread.IsOutdated {
				unresolved++
			}
		}
		out.UnresolvedReviewThreads = &unresolved
	}
	if cov := coverage[FacetPRMergeQueue]; cov != nil && cov.Complete {
		var queue *github.PullRequestMergeQueueEntry
		if _, err := decodeLatestFacet(observations, threadID, FacetPRMergeQueue, &queue); err != nil {
			return false, err
		}
		if queue != nil {
			out.MergeQueueState, out.MergeQueuePosition = strings.ToLower(queue.State), queue.Position
		}
	}
	return mergeabilityKnown, nil
}

func applyPortfolioSupplementalDetails(out *mcpcontract.PullRequestPortfolioItem, threadID int64, coverage map[string]*corpus.Coverage, observations map[corpus.ThreadFacetKey]corpus.FacetObservationBatch, format responseFormat) error {
	if !format.includesDetails() {
		return nil
	}
	if cov := coverage[FacetPRClosingIssues]; cov != nil && cov.Complete {
		var issues []github.PullRequestClosingIssue
		if _, err := decodeLatestFacet(observations, threadID, FacetPRClosingIssues, &issues); err != nil {
			return err
		}
		for _, issue := range issues {
			out.ClosingIssues = append(out.ClosingIssues, fmt.Sprintf("%s#%d", issue.RepositoryFullName, issue.Number))
		}
	}
	if cov := coverage[FacetPRFiles]; cov != nil && cov.Complete {
		var files []github.PullRequestFile
		if _, err := decodeLatestFacet(observations, threadID, FacetPRFiles, &files); err != nil {
			return err
		}
		for _, file := range files {
			out.ChangedFiles = append(out.ChangedFiles, file.Path)
		}
	}
	return nil
}

func addPortfolioCoverageReasons(out *mcpcontract.PullRequestPortfolioItem, coverage map[string]*corpus.Coverage, mergeabilityKnown bool) {
	for _, facet := range []string{FacetPRChecks, FacetPRReviewThreads, FacetPRMergeState, FacetPRMergeQueue} {
		if coverage[facet] == nil || !coverage[facet].Complete {
			out.Reasons = append(out.Reasons, facet+" coverage is incomplete")
		}
	}
	if coverage[FacetPRMergeState] != nil && coverage[FacetPRMergeState].Complete && !mergeabilityKnown {
		out.Reasons = append(out.Reasons, "GitHub mergeability is still computing")
	}
}

func setPortfolioAttention(out *mcpcontract.PullRequestPortfolioItem, thread corpus.Thread, details github.PullRequestDetails, coverage map[string]*corpus.Coverage, mergeabilityKnown bool, now time.Time) {
	detailCoverage := coverage[FacetPRDetails]
	healthComplete := completePortfolioHealthCoverage(coverage, mergeabilityKnown)
	switch {
	case thread.Merge.IsMerged():
		out.Attention = "merged"
		out.Reasons = append([]string{"pull request is merged"}, out.Reasons...)
	case thread.State == "closed" && thread.Merge.Known():
		out.Attention = "closed_unmerged"
		out.Reasons = append([]string{"pull request is closed and GitHub reports it was not merged"}, out.Reasons...)
	case thread.State == "closed":
		out.Attention = "unknown"
		out.Reasons = append([]string{"pull request is closed but merge state has not been observed"}, out.Reasons...)
	case detailCoverage == nil:
		out.Attention = "unknown"
		out.Reasons = append([]string{"pull-request status has not been synchronized"}, out.Reasons...)
	case details.Mergeable != nil && !*details.Mergeable:
		out.Attention = "conflicted"
		out.Reasons = append([]string{"GitHub reports the pull request is not mergeable"}, out.Reasons...)
	case out.ReviewDecision == "changes_requested":
		out.Attention = "changes_requested"
		out.Reasons = append([]string{"latest reviewer decisions request changes"}, out.Reasons...)
	case out.ChecksStatus == "failing":
		out.Attention = "checks_failing"
		out.Reasons = append([]string{"one or more observed checks are failing"}, out.Reasons...)
	case out.ChecksStatus == "pending":
		out.Attention = "checks_pending"
		out.Reasons = append([]string{"one or more observed checks are pending"}, out.Reasons...)
	case strings.EqualFold(out.MergeStateStatus, "behind"):
		out.Attention = "behind_base"
		out.Reasons = append([]string{"GitHub reports the head is behind the base branch"}, out.Reasons...)
	case out.UnresolvedReviewThreads != nil && *out.UnresolvedReviewThreads > 0:
		out.Attention = "review_threads_unresolved"
		out.Reasons = append([]string{"review conversations remain unresolved"}, out.Reasons...)
	case out.MergeQueueState != "":
		out.Attention = "merge_queue"
		out.Reasons = append([]string{"pull request is in the merge queue"}, out.Reasons...)
	case !healthComplete:
		out.Attention = "unknown"
		out.Reasons = append([]string{"required pull-request health coverage is incomplete"}, out.Reasons...)
	case now.Sub(thread.SourceUpdatedAt) > 14*24*time.Hour:
		out.Attention = "stale"
		out.Reasons = append([]string{"pull request has not been updated for more than 14 days"}, out.Reasons...)
	case out.ReviewDecision == "approved":
		out.Attention = "approved"
		out.Reasons = append([]string{"latest stored reviewer decisions include approval"}, out.Reasons...)
	default:
		out.Attention = "awaiting_review"
		out.Reasons = append([]string{"no approval or change request is stored"}, out.Reasons...)
	}
}

func completePortfolioHealthCoverage(coverage map[string]*corpus.Coverage, mergeabilityKnown bool) bool {
	for _, facet := range []string{FacetPRChecks, FacetPRReviewThreads, FacetPRMergeState, FacetPRMergeQueue} {
		if coverage[facet] == nil || !coverage[facet].Complete {
			return false
		}
	}
	return mergeabilityKnown
}

func portfolioFacets() []string {
	return []string{FacetPRDetails, FacetPRReviews, FacetPRChecks, FacetPRReviewThreads, FacetPRMergeState, FacetPRMergeQueue, FacetPRClosingIssues, FacetPRFiles}
}

func decodeLatestFacet[T any](observations map[corpus.ThreadFacetKey]corpus.FacetObservationBatch, threadID int64, facet string, target *T) (string, error) {
	batch := observations[corpus.ThreadFacetKey{ThreadID: threadID, Facet: facet}]
	if len(batch.Observations) == 0 {
		return "", fmt.Errorf("complete %s coverage has no observation", facet)
	}
	if err := json.Unmarshal([]byte(batch.Observations[0].Payload), target); err != nil {
		return "", err
	}
	return formatTime(batch.Observations[0].ObservedAt), nil
}

func classifyChecks(checks []github.PullRequestCheck) string {
	status := "passing"
	for _, check := range checks {
		value := strings.ToUpper(check.Status)
		conclusion := strings.ToUpper(check.Conclusion)
		if value != "" && value != "COMPLETED" && value != "SUCCESS" {
			status = "pending"
		}
		switch conclusion {
		case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
			return "failing"
		}
		if check.Kind == "StatusContext" {
			switch value {
			case "ERROR", "FAILURE":
				return "failing"
			case "EXPECTED", "PENDING":
				status = "pending"
			}
		}
	}
	return status
}

// RankOpportunities performs deterministic offline Radar ranking across stored repositories.
func (r *MCPReader) RankOpportunities(ctx context.Context, in mcpcontract.RankOpportunitiesInput) (mcpcontract.RankOpportunitiesOutput, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 50 {
		return mcpcontract.RankOpportunitiesOutput{}, errors.New("repositories must contain 1 to 50 items")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.MaxResultsPerRepository == 0 {
		in.MaxResultsPerRepository = 10
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.RankOpportunitiesOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.MaxResultsPerRepository < 1 || in.MaxResultsPerRepository > 100 {
		return mcpcontract.RankOpportunitiesOutput{}, errors.New("max_results_per_repository must be between 1 and 100")
	}
	evaluationTime := r.now().UTC()
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.RankOpportunitiesOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.RankOpportunitiesOutput{}, err
	}
	out := mcpcontract.RankOpportunitiesOutput{
		Status: "complete", GeneratedAt: formatTime(evaluationTime),
		Candidates:    make([]mcpcontract.OpportunityCandidateOutput, 0, in.Limit),
		Repositories:  make([]mcpcontract.BatchItem[mcpcontract.RepositoryOpportunitySummaryOutput], len(in.Repositories)),
		SnapshotToken: snapshotIdentity(in.SnapshotToken, revision),
	}
	var candidates []radar.Candidate
	for i, input := range in.Repositories {
		key := input.Owner + "/" + input.Repo
		item := mcpcontract.BatchItem[mcpcontract.RepositoryOpportunitySummaryOutput]{Key: key, Status: "complete"}
		report, err := r.contributionRadarAt(ctx, contracts.RadarOptions{Repo: contracts.RepoRef{Owner: input.Owner, Repo: input.Repo}, Limit: in.MaxResultsPerRepository}, evaluationTime)
		if err != nil {
			item.Status, item.Reason, item.Message = "unavailable", "repository_not_indexed", err.Error()
			item.Recovery = recoveryPlan(item.Reason, item.Message, syncRepositoryContextCall(input.Owner, input.Repo), mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: []mcpcontract.RepositoryRef{{Owner: input.Owner, Repo: input.Repo}}, Kind: "issue", State: "open"}))
			out.Repositories[i] = item
			out.Status = "partial"
			continue
		}
		summary := mcpcontract.RepositoryOpportunitySummaryOutput{
			Repo: report.Repo, TotalOpenIssues: report.TotalOpenIssues, Considered: report.CandidatePopulation,
			Returned: len(report.Candidates), Truncated: len(report.Candidates) < report.CandidatePopulation,
			PopulationCapped: report.PopulationCapped,
		}
		if summary.Truncated || summary.PopulationCapped {
			item.Status, item.Reason, item.Message = "partial", "ranking_population_truncated", "the repository ranking population exceeded the requested bound"
			out.Status = "partial"
			nextLimit := min(100, max(in.MaxResultsPerRepository*2, in.MaxResultsPerRepository+1))
			summary.Recovery = recoveryPlan("ranking_population_truncated", "The repository ranking population was bounded. Refresh issue headers, then rerun with a larger per-repository result bound before treating the ranking as exhaustive.", mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: []mcpcontract.RepositoryRef{{Owner: input.Owner, Repo: input.Repo}}, Kind: "issue", State: "open"}), mcpcontract.RecoveryAction(mcpcontract.RankOpportunitiesInput{Repositories: []mcpcontract.RepositoryRef{{Owner: input.Owner, Repo: input.Repo}}, Limit: in.Limit, MaxResultsPerRepository: nextLimit}))
		}
		out.Total += report.CandidatePopulation
		out.Truncated = out.Truncated || summary.Truncated || summary.PopulationCapped
		item.Value = &summary
		out.Repositories[i] = item
		candidates = append(candidates, report.Candidates...)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Eligibility != candidates[j].Eligibility {
			return eligibilityRank(candidates[i].Eligibility) < eligibilityRank(candidates[j].Eligibility)
		}
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Ref < candidates[j].Ref
	})
	out.Truncated = out.Truncated || len(candidates) > in.Limit
	end := min(in.Limit, len(candidates))
	for i, candidate := range candidates[:end] {
		mapped := radarCandidateToMCP(candidate)
		mapped.Rank = i + 1
		out.Candidates = append(out.Candidates, mapped)
	}
	if out.Truncated {
		nextLimit := min(100, max(in.Limit*2, in.Limit+1))
		out.Recovery = recoveryPlan("ranking_truncated", "The cross-repository ranking is bounded. Refresh issue headers, then rerun with a larger result limit before treating the returned candidates as exhaustive.", mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{Selection: "repositories", Repositories: append([]mcpcontract.RepositoryRef(nil), in.Repositories...), Kind: "issue", State: "open"}), mcpcontract.RecoveryAction(mcpcontract.RankOpportunitiesInput{Repositories: append([]mcpcontract.RepositoryRef(nil), in.Repositories...), Limit: nextLimit, MaxResultsPerRepository: in.MaxResultsPerRepository}))
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.RankOpportunitiesOutput{}, err
	}
	return out, nil
}

func radarCandidateToMCP(c radar.Candidate) mcpcontract.OpportunityCandidateOutput {
	out := mcpcontract.OpportunityCandidateOutput{Ref: c.Ref, Repo: c.Repo, Number: c.Number, Title: c.Title, URL: c.URL, Score: mcpcontract.RadarScore(c.Score), Eligibility: string(c.Eligibility), Confidence: c.Confidence, SourceUpdatedAt: formatTime(c.SourceUpdatedAt)}
	for _, signal := range c.PositiveSignals {
		out.PositiveSignals = append(out.PositiveSignals, signal.Summary)
	}
	for _, signal := range c.Risks {
		out.Risks = append(out.Risks, signal.Summary)
	}
	for _, signal := range c.Blockers {
		out.Blockers = append(out.Blockers, signal.Summary)
	}
	for _, unknown := range c.Unknowns {
		out.Unknowns = append(out.Unknowns, unknown.Summary)
	}
	for _, linked := range c.LinkedPullRequests {
		out.LinkedPullRequests = append(out.LinkedPullRequests, linked.Number)
	}
	for _, work := range c.RelatedWork {
		out.RelatedWork = append(out.RelatedWork, mcpcontract.OpportunityRelatedWorkOutput{
			Ref: work.Ref, Relation: string(work.Relation), Direction: string(work.Direction), State: work.State,
		})
	}
	return out
}
func eligibilityRank(v radar.Eligibility) int {
	switch v {
	case radar.EligibilityReadyToCode:
		return 0
	case radar.EligibilityNeedsDiagnosis:
		return 1
	case radar.EligibilityNeedsCoordination:
		return 2
	default:
		return 3
	}
}

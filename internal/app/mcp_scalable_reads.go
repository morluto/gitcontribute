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
	"github.com/morluto/gitcontribute/internal/precedent"
	"github.com/morluto/gitcontribute/internal/radar"
	"github.com/morluto/gitcontribute/internal/ranking"
	"github.com/morluto/gitcontribute/internal/similarity"
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
	out := mcpcontract.GetRepositoriesOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.TypedRepositoryOutput], len(in.Repositories))}
	repositoryKeys := make([]corpus.RepositoryKey, 0, len(in.Repositories))
	for _, input := range in.Repositories {
		ref := domain.RepoRef{Owner: input.Owner, Repo: input.Repo}
		if ref.Validate() == nil {
			repositoryKeys = append(repositoryKeys, corpus.RepositoryKey{Owner: ref.Owner, Name: ref.Repo})
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
		item := mcpcontract.BatchItem[mcpcontract.TypedRepositoryOutput]{Key: key, Status: "complete"}
		ref := domain.RepoRef{Owner: input.Owner, Repo: input.Repo}
		if err := ref.Validate(); err != nil {
			item.Status, item.Reason, item.Message = "failed", "invalid_reference", err.Error()
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		repo := repositories[corpus.RepositoryKey{Owner: ref.Owner, Name: ref.Repo}]
		if repo == nil {
			item.Status, item.Reason, item.Message = "unavailable", "not_indexed", "repository is not present in the local corpus"
			item.NextAction = "Call github.sync_repository_context for this repository."
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		value := typedRepository(repo)
		value.DossierStatus = "missing"
		if dossierMetadata, ok := dossiersByRepository[repo.ID]; ok {
			value.DossierStatus = "available"
			value.DossierAsOf = formatTime(dossierMetadata.AsOf)
		}
		coverage := coverageByRepository[corpus.RepositoryFacetKey{RepositoryID: repo.ID, Facet: "metadata"}]
		if coverage == nil {
			value.Metadata = mcpcontract.RepositoryMetadataOutput{Status: "missing", NextAction: "Call github.sync_repository_context for this repository."}
			clearRepositoryFacts(&value)
		} else {
			status := "complete"
			if !coverage.Complete {
				status = "partial"
			}
			value.Metadata = mcpcontract.RepositoryMetadataOutput{Status: status, ObservedAt: formatTime(coverage.UpdatedAt), SourceUpdatedAt: formatTime(coverage.SourceUpdatedAt)}
		}
		item.Value = &value
		out.Items[i] = item
	}
	return out, nil
}

func typedRepository(repo *corpus.Repository) mcpcontract.TypedRepositoryOutput {
	return mcpcontract.TypedRepositoryOutput{Ref: "repository:" + repo.Owner + "/" + repo.Name, Owner: repo.Owner, Repo: repo.Name, UpdatedAt: formatTime(repo.SourceUpdatedAt), Description: ptr(repo.Description), DefaultBranch: ptr(repo.DefaultBranch), Language: ptr(repo.Language), License: ptr(repo.License), Topics: append([]string(nil), repo.Topics...), Stars: ptr(repo.Stars), Watchers: ptr(repo.Watchers), Forks: ptr(repo.Forks), OpenIssues: ptr(repo.OpenIssues), Archived: ptr(repo.Archived), Fork: ptr(repo.Fork)}
}

func clearRepositoryFacts(v *mcpcontract.TypedRepositoryOutput) {
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
	out := mcpcontract.GetThreadsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.ThreadOutput], len(in.Threads))}
	repositoryKeys := make([]corpus.RepositoryKey, 0, len(in.Threads))
	for _, input := range in.Threads {
		ref := domain.RepoRef{Owner: input.Owner, Repo: input.Repo}
		if ref.Validate() == nil && input.Number > 0 {
			repositoryKeys = append(repositoryKeys, corpus.RepositoryKey{Owner: ref.Owner, Name: ref.Repo})
		}
	}
	repositories, err := c.GetRepositoriesBatch(ctx, repositoryKeys)
	if err != nil {
		return mcpcontract.GetThreadsOutput{}, err
	}
	threadKeys := make([]corpus.ThreadKey, 0, len(in.Threads))
	for _, input := range in.Threads {
		repo := repositories[corpus.RepositoryKey{Owner: input.Owner, Name: input.Repo}]
		if repo != nil && input.Number > 0 {
			threadKeys = append(threadKeys, corpus.ThreadKey{RepositoryID: repo.ID, Kind: input.Kind, Number: input.Number})
		}
	}
	threads, err := c.GetThreadsBatch(ctx, threadKeys)
	if err != nil {
		return mcpcontract.GetThreadsOutput{}, err
	}
	for i, input := range in.Threads {
		key := fmt.Sprintf("%s/%s#%d", input.Owner, input.Repo, input.Number)
		item := mcpcontract.BatchItem[mcpcontract.ThreadOutput]{Key: key, Status: "complete"}
		ref := domain.RepoRef{Owner: input.Owner, Repo: input.Repo}
		if err := ref.Validate(); err != nil || input.Number < 1 {
			item.Status, item.Reason, item.Message = "failed", "invalid_reference", "invalid thread reference"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		repo := repositories[corpus.RepositoryKey{Owner: ref.Owner, Name: ref.Repo}]
		if repo == nil {
			item.Status, item.Reason, item.Message = "unavailable", "repository_not_indexed", "repository is not present in the local corpus"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		thread := threads[corpus.ThreadKey{RepositoryID: repo.ID, Kind: input.Kind, Number: input.Number}]
		if thread == nil {
			item.Status, item.Reason, item.Message = "unavailable", "not_indexed", "thread is not present in the local corpus"
			item.NextAction = "Call github.sync_threads in thread selection mode with this exact reference."
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		value := corpusThreadToMCPOutput(thread)
		value.Owner, value.Repo = ref.Owner, ref.Repo
		if in.View == "compact" {
			value.Body = ""
		}
		item.Value = &value
		out.Items[i] = item
	}
	return out, nil
}

// GetJobs reads up to 100 durable job records without waiting for completion.
func (r *MCPReader) GetJobs(ctx context.Context, in mcpcontract.GetJobsInput) (mcpcontract.GetJobsOutput, error) {
	ids := append([]string(nil), in.IDs...)
	if len(ids) < 1 || len(ids) > 100 {
		return mcpcontract.GetJobsOutput{}, errors.New("ids must contain 1 to 100 items")
	}
	if in.ResponseFormat == "" {
		in.ResponseFormat = "concise"
	}
	if in.ResponseFormat != "concise" && in.ResponseFormat != "detailed" {
		return mcpcontract.GetJobsOutput{}, errors.New("response_format must be concise or detailed")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GetJobsOutput{}, err
	}
	storedJobs, err := c.GetJobsBatch(ctx, ids, in.ResponseFormat == "detailed")
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
			job := jobResultToMCP(ptr(jobResult(stored)), in.ResponseFormat == "detailed")
			if in.ResponseFormat == "concise" && (job.Status == "succeeded" || job.Status == "failed" || job.Status == "cancelled") {
				item.NextAction = "Call jobs.get with response_format=detailed to read typed artifact and follow-up references."
			}
			item.Value = &job
		}
		out.Items[i] = item
	}
	return out, nil
}

// ListPullRequestPortfolio performs an offline projection over stored authored
// PRs and status facets; unsupported health facets remain explicitly unknown.
func (r *MCPReader) ListPullRequestPortfolio(ctx context.Context, in mcpcontract.ListPullRequestPortfolioInput) (mcpcontract.ListPullRequestPortfolioOutput, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("state must be open, closed, or all")
	}
	if in.ResponseFormat == "" {
		in.ResponseFormat = "concise"
	}
	if in.ResponseFormat != "concise" && in.ResponseFormat != "detailed" {
		return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("response_format must be concise or detailed")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ListPullRequestPortfolioOutput{}, err
	}
	page, err := c.ListPullRequestPortfolioPage(ctx, strings.TrimSpace(in.Author), in.State, in.Limit)
	if err != nil {
		return mcpcontract.ListPullRequestPortfolioOutput{}, err
	}
	format := portfolioResponseFormat(in.ResponseFormat)
	readSet, err := loadPortfolioReadSet(ctx, c, page.PullRequests, format)
	if err != nil {
		return mcpcontract.ListPullRequestPortfolioOutput{}, err
	}
	out := mcpcontract.ListPullRequestPortfolioOutput{Status: "complete", ResponseFormat: in.ResponseFormat, RuleVersion: "portfolio.v2", GeneratedAt: formatTime(r.now()), PullRequests: make([]mcpcontract.PullRequestPortfolioItem, 0, len(page.PullRequests)), Total: page.Total, Truncated: page.Truncated}
	for _, storedPR := range page.PullRequests {
		item, err := portfolioItem(storedPR, r.now(), readSet, format)
		if err != nil {
			return mcpcontract.ListPullRequestPortfolioOutput{}, err
		}
		if item.StatusCoverage != "complete" {
			out.Status = "partial"
		}
		out.PullRequests = append(out.PullRequests, item)
	}
	return out, nil
}

type portfolioReadSet struct {
	coverage     map[corpus.ThreadFacetKey]*corpus.Coverage
	observations map[corpus.ThreadFacetKey]corpus.FacetObservationBatch
}

type portfolioResponseFormat string

const (
	portfolioConcise  portfolioResponseFormat = "concise"
	portfolioDetailed portfolioResponseFormat = "detailed"
)

func (f portfolioResponseFormat) includesDetails() bool {
	return f == portfolioDetailed
}

func loadPortfolioReadSet(ctx context.Context, c *corpus.Corpus, pullRequests []corpus.PortfolioPullRequest, format portfolioResponseFormat) (portfolioReadSet, error) {
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

// The projection deliberately keeps coverage, observation decoding, and the
// portfolio.v2 classification together so unknown facets cannot become facts.
//
//nolint:gocognit,cyclop
func portfolioItem(stored corpus.PortfolioPullRequest, now time.Time, readSet portfolioReadSet, format portfolioResponseFormat) (mcpcontract.PullRequestPortfolioItem, error) {
	t := stored.Thread
	out := mcpcontract.PullRequestPortfolioItem{Ref: fmt.Sprintf("%s/%s#%d", stored.Owner, stored.Repo, t.Number), Owner: stored.Owner, Repo: stored.Repo, Number: t.Number, Title: t.Title, State: t.State, Author: t.Author, Draft: t.Draft, SourceUpdatedAt: formatTime(t.SourceUpdatedAt), StatusCoverage: "missing"}
	facets := portfolioFacets()
	coverage := make(map[string]*corpus.Coverage, len(facets))
	complete, observed := true, 0
	for _, facet := range facets {
		cov := readSet.coverage[corpus.ThreadFacetKey{ThreadID: t.ID, Facet: facet}]
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
		entry := mcpcontract.FacetCoverageOutput{Facet: facet, Status: status}
		if cov != nil {
			entry.Complete, entry.UpdatedAt = cov.Complete, formatTime(cov.UpdatedAt)
		}
		if format.includesDetails() {
			out.Facets = append(out.Facets, entry)
		}
	}
	if observed > 0 {
		out.StatusCoverage = "partial"
	}
	if complete {
		out.StatusCoverage = "complete"
	}
	detailCoverage, reviewCoverage := coverage[FacetPRDetails], coverage[FacetPRReviews]
	var details github.PullRequestDetails
	if detailCoverage != nil && detailCoverage.Complete {
		observedAt, err := decodeLatestFacet(readSet.observations, t.ID, FacetPRDetails, &details)
		if err != nil {
			return out, fmt.Errorf("decode pull-request details for %s: %w", out.Ref, err)
		}
		out.Mergeable = details.Mergeable
		if format.includesDetails() {
			out.HeadRef, out.HeadSHA, out.BaseRef, out.BaseSHA = details.HeadRef, details.HeadSHA, details.BaseRef, details.BaseSHA
		}
		out.StatusObservedAt = observedAt
	}
	if reviewCoverage != nil && reviewCoverage.Complete {
		reviewObservations := readSet.observations[corpus.ThreadFacetKey{ThreadID: t.ID, Facet: FacetPRReviews}].Observations
		latest := make(map[string]github.Review)
		for _, observation := range reviewObservations {
			var reviews []github.Review
			if err := json.Unmarshal([]byte(observation.Payload), &reviews); err != nil {
				return out, fmt.Errorf("decode pull-request reviews for %s: %w", out.Ref, err)
			}
			for _, review := range reviews {
				previous, ok := latest[strings.ToLower(review.Author)]
				if !ok || review.SubmittedAt.After(previous.SubmittedAt) {
					latest[strings.ToLower(review.Author)] = review
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
	}
	mergeabilityKnown := false
	if cov := coverage[FacetPRMergeState]; cov != nil && cov.Complete {
		var value github.PullRequestMergeState
		if _, err := decodeLatestFacet(readSet.observations, t.ID, FacetPRMergeState, &value); err != nil {
			return out, err
		}
		out.MergeStateStatus = strings.ToLower(value.MergeStateStatus)
		if value.MergeableKnown {
			mergeabilityKnown = true
			mergeable := strings.EqualFold(value.Mergeable, "MERGEABLE")
			out.Mergeable = &mergeable
		}
	}
	if cov := coverage[FacetPRChecks]; cov != nil && cov.Complete {
		var checks []github.PullRequestCheck
		if _, err := decodeLatestFacet(readSet.observations, t.ID, FacetPRChecks, &checks); err != nil {
			return out, err
		}
		out.ChecksTotal = len(checks)
		out.ChecksStatus = classifyChecks(checks)
	}
	if cov := coverage[FacetPRReviewThreads]; cov != nil && cov.Complete {
		var threads []github.PullRequestReviewThread
		if _, err := decodeLatestFacet(readSet.observations, t.ID, FacetPRReviewThreads, &threads); err != nil {
			return out, err
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
		if _, err := decodeLatestFacet(readSet.observations, t.ID, FacetPRMergeQueue, &queue); err != nil {
			return out, err
		}
		if queue != nil {
			out.MergeQueueState, out.MergeQueuePosition = strings.ToLower(queue.State), queue.Position
		}
	}
	if cov := coverage[FacetPRClosingIssues]; format.includesDetails() && cov != nil && cov.Complete {
		var issues []github.PullRequestClosingIssue
		if _, err := decodeLatestFacet(readSet.observations, t.ID, FacetPRClosingIssues, &issues); err != nil {
			return out, err
		}
		for _, issue := range issues {
			out.ClosingIssues = append(out.ClosingIssues, fmt.Sprintf("%s#%d", issue.RepositoryFullName, issue.Number))
		}
	}
	if cov := coverage[FacetPRFiles]; format.includesDetails() && cov != nil && cov.Complete {
		var files []github.PullRequestFile
		if _, err := decodeLatestFacet(readSet.observations, t.ID, FacetPRFiles, &files); err != nil {
			return out, err
		}
		for _, file := range files {
			out.ChangedFiles = append(out.ChangedFiles, file.Path)
		}
	}
	for _, facet := range []string{FacetPRChecks, FacetPRReviewThreads, FacetPRMergeState, FacetPRMergeQueue} {
		if coverage[facet] == nil || !coverage[facet].Complete {
			out.Reasons = append(out.Reasons, facet+" coverage is incomplete")
		}
	}
	if coverage[FacetPRMergeState] != nil && coverage[FacetPRMergeState].Complete && !mergeabilityKnown {
		out.Reasons = append(out.Reasons, "GitHub mergeability is still computing")
	}
	healthComplete := coverage[FacetPRChecks] != nil && coverage[FacetPRChecks].Complete && coverage[FacetPRReviewThreads] != nil && coverage[FacetPRReviewThreads].Complete && coverage[FacetPRMergeState] != nil && coverage[FacetPRMergeState].Complete && mergeabilityKnown && coverage[FacetPRMergeQueue] != nil && coverage[FacetPRMergeQueue].Complete
	switch {
	case t.Merged:
		out.Attention = "merged"
		out.Reasons = append([]string{"pull request is merged"}, out.Reasons...)
	case t.State == "closed" && t.MergedKnown:
		out.Attention = "closed_unmerged"
		out.Reasons = append([]string{"pull request is closed and GitHub reports it was not merged"}, out.Reasons...)
	case t.State == "closed":
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
	case now.Sub(t.SourceUpdatedAt) > 14*24*time.Hour:
		out.Attention = "stale"
		out.Reasons = append([]string{"pull request has not been updated for more than 14 days"}, out.Reasons...)
	case out.ReviewDecision == "approved":
		out.Attention = "approved"
		out.Reasons = append([]string{"latest stored reviewer decisions include approval"}, out.Reasons...)
	default:
		out.Attention = "awaiting_review"
		out.Reasons = append([]string{"no approval or change request is stored"}, out.Reasons...)
	}
	return out, nil
}

func portfolioFacets() []string {
	return []string{FacetPRDetails, FacetPRReviews, FacetPRChecks, FacetPRReviewThreads, FacetPRMergeState, FacetPRMergeQueue, FacetPRClosingIssues, FacetPRFiles}
}

func decodeLatestFacet(observations map[corpus.ThreadFacetKey]corpus.FacetObservationBatch, threadID int64, facet string, target any) (string, error) {
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
	out := mcpcontract.RankOpportunitiesOutput{
		Status: "complete", GeneratedAt: formatTime(evaluationTime),
		Candidates:   make([]mcpcontract.OpportunityCandidateOutput, 0, in.Limit),
		Repositories: make([]mcpcontract.BatchItem[mcpcontract.RepositoryOpportunitySummaryOutput], len(in.Repositories)),
	}
	var candidates []mcpcontract.OpportunityCandidateOutput
	for i, input := range in.Repositories {
		key := input.Owner + "/" + input.Repo
		item := mcpcontract.BatchItem[mcpcontract.RepositoryOpportunitySummaryOutput]{Key: key, Status: "complete"}
		report, err := r.contributionRadarAt(ctx, contracts.RadarOptions{Repo: contracts.RepoRef{Owner: input.Owner, Repo: input.Repo}, Limit: in.MaxResultsPerRepository}, evaluationTime)
		if err != nil {
			item.Status, item.Reason, item.Message = "unavailable", "not_indexed", err.Error()
			item.NextAction = "Sync repository metadata and open issue headers before ranking."
			out.Repositories[i] = item
			out.Status = "partial"
			continue
		}
		summary := mcpcontract.RepositoryOpportunitySummaryOutput{
			Repo: report.Repo, TotalOpenIssues: report.TotalOpenIssues, Considered: report.CandidatePopulation,
			Returned: len(report.Candidates), Truncated: len(report.Candidates) < report.CandidatePopulation,
			PopulationCapped: report.PopulationCapped,
		}
		out.Total += report.CandidatePopulation
		out.Truncated = out.Truncated || summary.Truncated || summary.PopulationCapped
		item.Value = &summary
		out.Repositories[i] = item
		for _, candidate := range report.Candidates {
			candidates = append(candidates, radarCandidateToMCP(candidate))
		}
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
	out.Candidates = append(out.Candidates, candidates[:end]...)
	for i := range out.Candidates {
		out.Candidates[i].Rank = i + 1
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
			Ref: work.Ref, Relation: work.Relation, Direction: work.Direction, State: work.State,
		})
	}
	return out
}
func eligibilityRank(v string) int {
	switch radar.Eligibility(v) {
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

// FindPrecedents performs an offline similarity search over stored resolved threads.
// Each source thread is resolved and ranked independently while preserving
// input order and item-level recovery guidance in the bounded batch response.
//
//nolint:gocognit
func (r *MCPReader) FindPrecedents(ctx context.Context, in mcpcontract.FindPrecedentsInput) (mcpcontract.FindPrecedentsOutput, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 20 {
		return mcpcontract.FindPrecedentsOutput{}, errors.New("threads must contain 1 to 20 items")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.FindPrecedentsOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.FindPrecedentsOutput{}, err
	}
	refs := make([]precedent.SourceRef, len(in.Threads))
	for i, input := range in.Threads {
		refs[i] = precedent.SourceRef{Repository: domain.RepoRef{Owner: input.Owner, Repo: input.Repo}, Number: input.Number}
	}
	snapshots, err := c.LoadPrecedentRepositories(ctx, refs, 2000)
	if err != nil {
		return mcpcontract.FindPrecedentsOutput{}, err
	}
	snapshotsByRepo := make(map[string]precedent.RepositorySnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotsByRepo[precedent.RepositoryKey(snapshot.Repository)] = snapshot
	}
	rule := similarity.DefaultPrecedentRule()
	preparedByRepo := make(map[string][]preparedPrecedent, len(snapshots))
	for _, snapshot := range snapshots {
		prepared := make([]preparedPrecedent, len(snapshot.Closed))
		for i, candidate := range snapshot.Closed {
			prepared[i] = preparedPrecedent{thread: candidate, text: rule.Prepare(candidate.Title + " " + candidate.Body)}
		}
		preparedByRepo[precedent.RepositoryKey(snapshot.Repository)] = prepared
	}
	out := mcpcontract.FindPrecedentsOutput{Status: "complete", Items: make([]mcpcontract.BatchItem[mcpcontract.PrecedentSet], len(in.Threads))}
	for i, input := range in.Threads {
		if err := ctx.Err(); err != nil {
			return mcpcontract.FindPrecedentsOutput{}, err
		}
		key := fmt.Sprintf("%s/%s#%d", input.Owner, input.Repo, input.Number)
		item := mcpcontract.BatchItem[mcpcontract.PrecedentSet]{Key: key, Status: "complete"}
		repoKey := precedent.RepositoryKey(refs[i].Repository)
		snapshot := snapshotsByRepo[repoKey]
		if !snapshot.Available {
			item.Status, item.Reason = "unavailable", "repository_not_indexed"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		source, ok := snapshot.Sources[input.Number]
		if !ok {
			item.Status, item.Reason = "unavailable", "thread_not_indexed"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		precedents := make([]mcpcontract.PrecedentOutput, 0, in.Limit)
		preparedSource := rule.Prepare(source.Title + " " + source.Body)
		for candidateIndex, prepared := range preparedByRepo[repoKey] {
			if candidateIndex%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return mcpcontract.FindPrecedentsOutput{}, err
				}
			}
			candidate := prepared.thread
			if candidate.ID == source.ID {
				continue
			}
			score := rule.Compare(preparedSource, prepared.text)
			if score < 0.08 {
				continue
			}
			precedents = append(precedents, precedentToMCP(key, input.Owner, input.Repo, candidate, score))
		}
		qualifying := len(precedents)
		precedents = ranking.TopK(precedents, in.Limit, betterPrecedent)
		item.Value = &mcpcontract.PrecedentSet{Matches: precedents, Population: snapshot.ClosedTotal, Considered: len(snapshot.Closed), Truncated: snapshot.ClosedTruncated || len(precedents) < qualifying}
		if item.Value.Truncated {
			out.Status = "partial"
		}
		out.Total += len(precedents)
		out.Items[i] = item
	}
	return out, nil
}

type preparedPrecedent struct {
	thread precedent.Thread
	text   similarity.PreparedLexical
}

func betterPrecedent(a, b mcpcontract.PrecedentOutput) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Ref < b.Ref
}

func precedentToMCP(source, owner, repo string, t precedent.Thread, score float64) mcpcontract.PrecedentOutput {
	reasons := []string{"similar stored title or body"}
	if t.Merged {
		reasons = append(reasons, "pull request merged")
	}
	if t.StateReason != "" {
		reasons = append(reasons, "GitHub state reason: "+t.StateReason)
	}
	for _, label := range t.Labels {
		lower := strings.ToLower(label)
		if strings.Contains(lower, "duplicate") || strings.Contains(lower, "wontfix") || strings.Contains(lower, "invalid") {
			reasons = append(reasons, "label: "+label)
		}
	}
	return mcpcontract.PrecedentOutput{Source: source, Ref: fmt.Sprintf("%s/%s#%d", owner, repo, t.Number), Kind: t.Kind, State: t.State, StateReason: t.StateReason, Title: t.Title, Score: mcpcontract.SimilarityScore(score), RuleVersion: similarity.PrecedentV1, Reasons: reasons, ClosedAt: formatTime(t.ClosedAt), MergedAt: formatTime(t.MergedAt)}
}

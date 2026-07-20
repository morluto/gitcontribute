package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
	"github.com/morluto/gitcontribute/internal/precedent"
	"github.com/morluto/gitcontribute/internal/radar"
	"github.com/morluto/gitcontribute/internal/ranking"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// GetRepositories performs an offline, input-ordered corpus read and clears
// repository facts when metadata coverage has not been observed.
func (r *MCPReader) GetRepositories(ctx context.Context, in mcpserver.GetRepositoriesInput) (mcpserver.GetRepositoriesOutput, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 100 {
		return mcpserver.GetRepositoriesOutput{}, errors.New("repositories must contain 1 to 100 items")
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpserver.GetRepositoriesOutput{}, err
	}
	out := mcpserver.GetRepositoriesOutput{Status: "complete", Items: make([]mcpserver.BatchItem[mcpserver.TypedRepositoryOutput], len(in.Repositories))}
	for i, input := range in.Repositories {
		key := input.Owner + "/" + input.Repo
		item := mcpserver.BatchItem[mcpserver.TypedRepositoryOutput]{Key: key, Status: "complete"}
		ref := domain.RepoRef{Owner: input.Owner, Repo: input.Repo}
		if err := ref.Validate(); err != nil {
			item.Status, item.Reason, item.Message = "failed", "invalid_reference", err.Error()
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
		if err != nil {
			return mcpserver.GetRepositoriesOutput{}, err
		}
		if repo == nil {
			item.Status, item.Reason, item.Message = "unavailable", "not_indexed", "repository is not present in the local corpus"
			item.NextAction = "Call github.sync_repository_metadata for this repository."
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		value := typedRepository(repo)
		coverage, err := c.GetCoverage(ctx, repo.ID, nil, "metadata")
		if err != nil {
			return mcpserver.GetRepositoriesOutput{}, err
		}
		if coverage == nil {
			value.Metadata = mcpserver.RepositoryMetadataOutput{Status: "missing", NextAction: "Call github.sync_repository_metadata for this repository."}
			clearRepositoryFacts(&value)
		} else {
			status := "complete"
			if !coverage.Complete {
				status = "partial"
			}
			value.Metadata = mcpserver.RepositoryMetadataOutput{Status: status, ObservedAt: formatTime(coverage.UpdatedAt), SourceUpdatedAt: formatTime(coverage.SourceUpdatedAt)}
		}
		item.Value = &value
		out.Items[i] = item
	}
	return out, nil
}

func typedRepository(repo *corpus.Repository) mcpserver.TypedRepositoryOutput {
	return mcpserver.TypedRepositoryOutput{Ref: "repository:" + repo.Owner + "/" + repo.Name, Owner: repo.Owner, Repo: repo.Name, Description: ptr(repo.Description), DefaultBranch: ptr(repo.DefaultBranch), Language: ptr(repo.Language), License: ptr(repo.License), Topics: append([]string(nil), repo.Topics...), Stars: ptr(repo.Stars), Watchers: ptr(repo.Watchers), Forks: ptr(repo.Forks), OpenIssues: ptr(repo.OpenIssues), Archived: ptr(repo.Archived), Fork: ptr(repo.Fork)}
}

func clearRepositoryFacts(v *mcpserver.TypedRepositoryOutput) {
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
func (r *MCPReader) GetThreads(ctx context.Context, in mcpserver.GetThreadsInput) (mcpserver.GetThreadsOutput, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 100 {
		return mcpserver.GetThreadsOutput{}, errors.New("threads must contain 1 to 100 items")
	}
	if in.View == "" {
		in.View = "compact"
	}
	if in.View != "compact" && in.View != "full" {
		return mcpserver.GetThreadsOutput{}, errors.New("view must be compact or full")
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpserver.GetThreadsOutput{}, err
	}
	out := mcpserver.GetThreadsOutput{Status: "complete", Items: make([]mcpserver.BatchItem[mcpserver.ThreadOutput], len(in.Threads))}
	for i, input := range in.Threads {
		key := fmt.Sprintf("%s/%s#%d", input.Owner, input.Repo, input.Number)
		item := mcpserver.BatchItem[mcpserver.ThreadOutput]{Key: key, Status: "complete"}
		ref := domain.RepoRef{Owner: input.Owner, Repo: input.Repo}
		if err := ref.Validate(); err != nil || input.Number < 1 {
			item.Status, item.Reason, item.Message = "failed", "invalid_reference", "invalid thread reference"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
		if err != nil {
			return mcpserver.GetThreadsOutput{}, err
		}
		if repo == nil {
			item.Status, item.Reason, item.Message = "unavailable", "repository_not_indexed", "repository is not present in the local corpus"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		var thread *corpus.Thread
		if input.Kind == "" {
			thread, err = c.GetThreadByNumber(ctx, repo.ID, input.Number)
		} else {
			thread, err = c.GetThread(ctx, repo.ID, input.Kind, input.Number)
		}
		if err != nil {
			return mcpserver.GetThreadsOutput{}, err
		}
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
func (r *MCPReader) GetJobs(ctx context.Context, in mcpserver.GetJobsInput) (mcpserver.GetJobsOutput, error) {
	ids := append([]string(nil), in.IDs...)
	if len(ids) < 1 || len(ids) > 100 {
		return mcpserver.GetJobsOutput{}, errors.New("ids must contain 1 to 100 items")
	}
	out := mcpserver.GetJobsOutput{Status: "complete", Items: make([]mcpserver.BatchItem[mcpserver.GetJobOutput], len(ids))}
	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		item := mcpserver.BatchItem[mcpserver.GetJobOutput]{Key: id, Status: "complete"}
		job, err := r.GetJob(ctx, mcpserver.GetJobInput{ID: id})
		if err != nil {
			var cliErr *cli.CLIError
			if errors.As(err, &cliErr) && cliErr.Code == cli.ExitNotFound {
				item.Status, item.Reason = "unavailable", "not_found"
			} else {
				item.Status, item.Reason = "failed", "read_failed"
			}
			item.Message = err.Error()
			out.Status = "partial"
		} else {
			item.Value = &job
		}
		out.Items[i] = item
	}
	return out, nil
}

// ListPullRequestPortfolio performs an offline projection over stored authored
// PRs and status facets; unsupported health facets remain explicitly unknown.
func (r *MCPReader) ListPullRequestPortfolio(ctx context.Context, in mcpserver.ListPullRequestPortfolioInput) (mcpserver.ListPullRequestPortfolioOutput, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return mcpserver.ListPullRequestPortfolioOutput{}, errors.New("state must be open, closed, or all")
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.ListPullRequestPortfolioOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpserver.ListPullRequestPortfolioOutput{}, err
	}
	stored, err := c.ListPullRequestPortfolio(ctx, strings.TrimSpace(in.Author), in.State, in.Limit)
	if err != nil {
		return mcpserver.ListPullRequestPortfolioOutput{}, err
	}
	out := mcpserver.ListPullRequestPortfolioOutput{Status: "complete", RuleVersion: "portfolio.v2", GeneratedAt: formatTime(r.now()), PullRequests: make([]mcpserver.PullRequestPortfolioItem, 0, len(stored)), Total: len(stored)}
	for _, storedPR := range stored {
		item, err := portfolioItem(ctx, c, storedPR, r.now())
		if err != nil {
			return mcpserver.ListPullRequestPortfolioOutput{}, err
		}
		if item.StatusCoverage != "complete" {
			out.Status = "partial"
		}
		out.PullRequests = append(out.PullRequests, item)
	}
	return out, nil
}

// The projection deliberately keeps coverage, observation decoding, and the
// portfolio.v2 classification together so unknown facets cannot become facts.
//
//nolint:gocognit,cyclop
func portfolioItem(ctx context.Context, c *corpus.Corpus, stored corpus.PortfolioPullRequest, now time.Time) (mcpserver.PullRequestPortfolioItem, error) {
	t := stored.Thread
	out := mcpserver.PullRequestPortfolioItem{Ref: fmt.Sprintf("%s/%s#%d", stored.Owner, stored.Repo, t.Number), Owner: stored.Owner, Repo: stored.Repo, Number: t.Number, Title: t.Title, State: t.State, Author: t.Author, Draft: t.Draft, SourceUpdatedAt: formatTime(t.SourceUpdatedAt), StatusCoverage: "missing"}
	facets := []string{FacetPRDetails, FacetPRReviews, FacetPRChecks, FacetPRReviewThreads, FacetPRMergeState, FacetPRMergeQueue, FacetPRClosingIssues, FacetPRFiles}
	coverage := make(map[string]*corpus.Coverage, len(facets))
	complete, observed := true, 0
	for _, facet := range facets {
		cov, err := c.GetCoverage(ctx, t.RepositoryID, &t.ID, facet)
		if err != nil {
			return out, err
		}
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
		entry := mcpserver.FacetCoverageOutput{Facet: facet, Status: status}
		if cov != nil {
			entry.Complete, entry.UpdatedAt = cov.Complete, formatTime(cov.UpdatedAt)
		}
		out.Facets = append(out.Facets, entry)
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
		observedAt, err := decodeLatestFacet(ctx, c, t, FacetPRDetails, &details)
		if err != nil {
			return out, fmt.Errorf("decode pull-request details for %s: %w", out.Ref, err)
		}
		out.Mergeable, out.HeadRef, out.HeadSHA, out.BaseRef, out.BaseSHA = details.Mergeable, details.HeadRef, details.HeadSHA, details.BaseRef, details.BaseSHA
		out.StatusObservedAt = observedAt
	}
	if reviewCoverage != nil && reviewCoverage.Complete {
		observations, _, err := c.ListFacetObservationsBounded(ctx, t.RepositoryID, &t.ID, FacetPRReviews, 100)
		if err != nil {
			return out, err
		}
		latest := make(map[string]github.Review)
		for _, observation := range observations {
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
		if _, err := decodeLatestFacet(ctx, c, t, FacetPRMergeState, &value); err != nil {
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
		if _, err := decodeLatestFacet(ctx, c, t, FacetPRChecks, &checks); err != nil {
			return out, err
		}
		out.ChecksTotal = len(checks)
		out.ChecksStatus = classifyChecks(checks)
	}
	if cov := coverage[FacetPRReviewThreads]; cov != nil && cov.Complete {
		var threads []github.PullRequestReviewThread
		if _, err := decodeLatestFacet(ctx, c, t, FacetPRReviewThreads, &threads); err != nil {
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
		if _, err := decodeLatestFacet(ctx, c, t, FacetPRMergeQueue, &queue); err != nil {
			return out, err
		}
		if queue != nil {
			out.MergeQueueState, out.MergeQueuePosition = strings.ToLower(queue.State), queue.Position
		}
	}
	if cov := coverage[FacetPRClosingIssues]; cov != nil && cov.Complete {
		var issues []github.PullRequestClosingIssue
		if _, err := decodeLatestFacet(ctx, c, t, FacetPRClosingIssues, &issues); err != nil {
			return out, err
		}
		for _, issue := range issues {
			out.ClosingIssues = append(out.ClosingIssues, fmt.Sprintf("%s#%d", issue.RepositoryFullName, issue.Number))
		}
	}
	if cov := coverage[FacetPRFiles]; cov != nil && cov.Complete {
		var files []github.PullRequestFile
		if _, err := decodeLatestFacet(ctx, c, t, FacetPRFiles, &files); err != nil {
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
	case t.State == "closed":
		out.Attention = "closed_unmerged"
		out.Reasons = append([]string{"pull request is closed without a stored merge"}, out.Reasons...)
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

func decodeLatestFacet(ctx context.Context, c *corpus.Corpus, thread corpus.Thread, facet string, target any) (string, error) {
	observations, _, err := c.ListFacetObservationsBounded(ctx, thread.RepositoryID, &thread.ID, facet, 1)
	if err != nil {
		return "", err
	}
	if len(observations) == 0 {
		return "", fmt.Errorf("complete %s coverage has no observation", facet)
	}
	if err := json.Unmarshal([]byte(observations[0].Payload), target); err != nil {
		return "", err
	}
	return formatTime(observations[0].ObservedAt), nil
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
func (r *MCPReader) RankOpportunities(ctx context.Context, in mcpserver.RankOpportunitiesInput) (mcpserver.RankOpportunitiesOutput, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 50 {
		return mcpserver.RankOpportunitiesOutput{}, errors.New("repositories must contain 1 to 50 items")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.MaxResultsPerRepository == 0 {
		in.MaxResultsPerRepository = 10
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.RankOpportunitiesOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.MaxResultsPerRepository < 1 || in.MaxResultsPerRepository > 100 {
		return mcpserver.RankOpportunitiesOutput{}, errors.New("max_results_per_repository must be between 1 and 100")
	}
	out := mcpserver.RankOpportunitiesOutput{Status: "complete", GeneratedAt: formatTime(r.now()), Repositories: make([]mcpserver.BatchItem[mcpserver.RepositoryOpportunitySummaryOutput], len(in.Repositories))}
	var candidates []mcpserver.OpportunityCandidateOutput
	for i, input := range in.Repositories {
		key := input.Owner + "/" + input.Repo
		item := mcpserver.BatchItem[mcpserver.RepositoryOpportunitySummaryOutput]{Key: key, Status: "complete"}
		report, err := r.ContributionRadar(ctx, cli.RadarOptions{Repo: cli.RepoRef{Owner: input.Owner, Repo: input.Repo}, Limit: in.MaxResultsPerRepository})
		if err != nil {
			item.Status, item.Reason, item.Message = "unavailable", "not_indexed", err.Error()
			item.NextAction = "Sync repository metadata and open issue headers before ranking."
			out.Repositories[i] = item
			out.Status = "partial"
			continue
		}
		summary := mcpserver.RepositoryOpportunitySummaryOutput{Repo: report.Repo, TotalOpenIssues: report.TotalOpenIssues, Considered: report.CandidatePopulation}
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
	if len(candidates) > in.Limit {
		candidates = candidates[:in.Limit]
	}
	for i := range candidates {
		candidates[i].Rank = i + 1
	}
	out.Candidates = candidates
	return out, nil
}

func radarCandidateToMCP(c radar.Candidate) mcpserver.OpportunityCandidateOutput {
	out := mcpserver.OpportunityCandidateOutput{Ref: c.Ref, Repo: c.Repo, Number: c.Number, Title: c.Title, URL: c.URL, Score: c.Score, Eligibility: string(c.Eligibility), Confidence: c.Confidence, SourceUpdatedAt: formatTime(c.SourceUpdatedAt)}
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
		out.RelatedWork = append(out.RelatedWork, mcpserver.OpportunityRelatedWorkOutput{
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
func (r *MCPReader) FindPrecedents(ctx context.Context, in mcpserver.FindPrecedentsInput) (mcpserver.FindPrecedentsOutput, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 20 {
		return mcpserver.FindPrecedentsOutput{}, errors.New("threads must contain 1 to 20 items")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.FindPrecedentsOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpserver.FindPrecedentsOutput{}, err
	}
	refs := make([]precedent.SourceRef, len(in.Threads))
	for i, input := range in.Threads {
		refs[i] = precedent.SourceRef{Repository: domain.RepoRef{Owner: input.Owner, Repo: input.Repo}, Number: input.Number}
	}
	snapshots, err := c.LoadPrecedentRepositories(ctx, refs, 2000)
	if err != nil {
		return mcpserver.FindPrecedentsOutput{}, err
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
	out := mcpserver.FindPrecedentsOutput{Status: "complete", Items: make([]mcpserver.BatchItem[[]mcpserver.PrecedentOutput], len(in.Threads))}
	for i, input := range in.Threads {
		if err := ctx.Err(); err != nil {
			return mcpserver.FindPrecedentsOutput{}, err
		}
		key := fmt.Sprintf("%s/%s#%d", input.Owner, input.Repo, input.Number)
		item := mcpserver.BatchItem[[]mcpserver.PrecedentOutput]{Key: key, Status: "complete"}
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
		precedents := make([]mcpserver.PrecedentOutput, 0, in.Limit)
		preparedSource := rule.Prepare(source.Title + " " + source.Body)
		for candidateIndex, prepared := range preparedByRepo[repoKey] {
			if candidateIndex%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return mcpserver.FindPrecedentsOutput{}, err
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
		precedents = ranking.TopK(precedents, in.Limit, betterPrecedent)
		item.Value = &precedents
		out.Total += len(precedents)
		out.Items[i] = item
	}
	return out, nil
}

type preparedPrecedent struct {
	thread precedent.Thread
	text   similarity.PreparedLexical
}

func betterPrecedent(a, b mcpserver.PrecedentOutput) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Ref < b.Ref
}

func precedentToMCP(source, owner, repo string, t precedent.Thread, score float64) mcpserver.PrecedentOutput {
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
	return mcpserver.PrecedentOutput{Source: source, Ref: fmt.Sprintf("%s/%s#%d", owner, repo, t.Number), Kind: t.Kind, State: t.State, StateReason: t.StateReason, Title: t.Title, Score: score, RuleVersion: similarity.PrecedentV1, Reasons: reasons, ClosedAt: formatTime(t.ClosedAt), MergedAt: formatTime(t.MergedAt)}
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpserver"
	"github.com/morluto/gitcontribute/internal/radar"
)

// GetRepositories performs an offline, input-ordered corpus read and clears
// repository facts when metadata coverage has not been observed.
func (r *MCPReader) GetRepositories(ctx context.Context, in mcpserver.GetRepositoriesInput) (mcpserver.GetRepositoriesOutput, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 100 {
		return mcpserver.GetRepositoriesOutput{}, errors.New("repositories must contain 1 to 100 items")
	}
	c, err := r.Service.openCorpus(ctx)
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
	return mcpserver.TypedRepositoryOutput{Owner: repo.Owner, Repo: repo.Name, Description: ptr(repo.Description), DefaultBranch: ptr(repo.DefaultBranch), Language: ptr(repo.Language), License: ptr(repo.License), Topics: append([]string(nil), repo.Topics...), Stars: ptr(repo.Stars), Watchers: ptr(repo.Watchers), Forks: ptr(repo.Forks), OpenIssues: ptr(repo.OpenIssues), Archived: ptr(repo.Archived), Fork: ptr(repo.Fork)}
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
	c, err := r.Service.openCorpus(ctx)
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
		item := mcpserver.BatchItem[mcpserver.GetJobOutput]{Key: id, Status: "complete"}
		job, err := r.GetJob(ctx, mcpserver.GetJobInput{ID: id})
		if err != nil {
			item.Status, item.Reason, item.Message = "unavailable", "not_found", err.Error()
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
	c, err := r.Service.openCorpus(ctx)
	if err != nil {
		return mcpserver.ListPullRequestPortfolioOutput{}, err
	}
	stored, err := c.ListPullRequestPortfolio(ctx, strings.TrimSpace(in.Author), in.State, in.Limit)
	if err != nil {
		return mcpserver.ListPullRequestPortfolioOutput{}, err
	}
	out := mcpserver.ListPullRequestPortfolioOutput{Status: "complete", RuleVersion: "portfolio.v1", GeneratedAt: formatTime(r.Service.now()), PullRequests: make([]mcpserver.PullRequestPortfolioItem, 0, len(stored)), Total: len(stored)}
	for _, storedPR := range stored {
		item, err := portfolioItem(ctx, c, storedPR, r.Service.now())
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

func portfolioItem(ctx context.Context, c *corpus.Corpus, stored corpus.PortfolioPullRequest, now time.Time) (mcpserver.PullRequestPortfolioItem, error) {
	t := stored.Thread
	out := mcpserver.PullRequestPortfolioItem{Ref: fmt.Sprintf("%s/%s#%d", stored.Owner, stored.Repo, t.Number), Owner: stored.Owner, Repo: stored.Repo, Number: t.Number, Title: t.Title, State: t.State, Author: t.Author, Draft: t.Draft, SourceUpdatedAt: formatTime(t.SourceUpdatedAt), StatusCoverage: "missing"}
	detailCoverage, err := c.GetCoverage(ctx, t.RepositoryID, &t.ID, FacetPRDetails)
	if err != nil {
		return out, err
	}
	reviewCoverage, err := c.GetCoverage(ctx, t.RepositoryID, &t.ID, FacetPRReviews)
	if err != nil {
		return out, err
	}
	var details github.PullRequestDetails
	if detailCoverage != nil {
		observations, _, err := c.ListFacetObservationsBounded(ctx, t.RepositoryID, &t.ID, FacetPRDetails, 1)
		if err != nil {
			return out, err
		}
		if len(observations) > 0 {
			if err := json.Unmarshal([]byte(observations[0].Payload), &details); err != nil {
				return out, fmt.Errorf("decode pull-request details for %s: %w", out.Ref, err)
			}
			out.Mergeable, out.HeadRef, out.HeadSHA, out.BaseRef, out.BaseSHA = details.Mergeable, details.HeadRef, details.HeadSHA, details.BaseRef, details.BaseSHA
			out.StatusObservedAt = formatTime(observations[0].ObservedAt)
			out.StatusCoverage = "partial"
		}
	}
	if reviewCoverage != nil {
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
		if detailCoverage != nil && detailCoverage.Complete && reviewCoverage.Complete {
			out.StatusCoverage = "partial"
		}
	}
	out.Reasons = append(out.Reasons, "check rollup and unresolved review-thread coverage are unavailable")
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
	case out.ReviewDecision == "approved":
		out.Attention = "approved"
		out.Reasons = append([]string{"latest stored reviewer decisions include approval"}, out.Reasons...)
	case now.Sub(t.SourceUpdatedAt) > 14*24*time.Hour:
		out.Attention = "stale"
		out.Reasons = append([]string{"pull request has not been updated for more than 14 days"}, out.Reasons...)
	default:
		out.Attention = "awaiting_review"
		out.Reasons = append([]string{"no approval or change request is stored"}, out.Reasons...)
	}
	return out, nil
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
	out := mcpserver.RankOpportunitiesOutput{Status: "complete", GeneratedAt: formatTime(r.Service.now()), Repositories: make([]mcpserver.BatchItem[mcpserver.RepositoryOpportunitySummaryOutput], len(in.Repositories))}
	var candidates []mcpserver.OpportunityCandidateOutput
	for i, input := range in.Repositories {
		key := input.Owner + "/" + input.Repo
		item := mcpserver.BatchItem[mcpserver.RepositoryOpportunitySummaryOutput]{Key: key, Status: "complete"}
		report, err := r.Service.ContributionRadar(ctx, cli.RadarOptions{Repo: cli.RepoRef{Owner: input.Owner, Repo: input.Repo}, Limit: in.MaxResultsPerRepository})
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
	return out
}
func eligibilityRank(v string) int {
	if v == "eligible" {
		return 0
	}
	if v == "unknown" {
		return 1
	}
	return 2
}

// FindPrecedents performs an offline similarity search over stored resolved threads.
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
	c, err := r.Service.openCorpus(ctx)
	if err != nil {
		return mcpserver.FindPrecedentsOutput{}, err
	}
	out := mcpserver.FindPrecedentsOutput{Status: "complete", Items: make([]mcpserver.BatchItem[[]mcpserver.PrecedentOutput], len(in.Threads))}
	for i, input := range in.Threads {
		key := fmt.Sprintf("%s/%s#%d", input.Owner, input.Repo, input.Number)
		item := mcpserver.BatchItem[[]mcpserver.PrecedentOutput]{Key: key, Status: "complete"}
		repo, err := c.GetRepository(ctx, input.Owner, input.Repo)
		if err != nil {
			return mcpserver.FindPrecedentsOutput{}, err
		}
		if repo == nil {
			item.Status, item.Reason = "unavailable", "repository_not_indexed"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		source, err := c.GetThreadByNumber(ctx, repo.ID, input.Number)
		if err != nil {
			return mcpserver.FindPrecedentsOutput{}, err
		}
		if source == nil {
			item.Status, item.Reason = "unavailable", "thread_not_indexed"
			out.Items[i] = item
			out.Status = "partial"
			continue
		}
		closed, err := c.ListThreadsFiltered(ctx, repo.ID, "", "closed", 2000)
		if err != nil {
			return mcpserver.FindPrecedentsOutput{}, err
		}
		precedents := make([]mcpserver.PrecedentOutput, 0, in.Limit)
		sourceTokens := wordSet(source.Title + " " + source.Body)
		for _, candidate := range closed {
			if candidate.ID == source.ID {
				continue
			}
			score := jaccard(sourceTokens, wordSet(candidate.Title+" "+candidate.Body))
			if score < 0.08 {
				continue
			}
			precedents = append(precedents, precedentToMCP(key, input.Owner, input.Repo, candidate, score))
		}
		sort.SliceStable(precedents, func(i, j int) bool {
			if precedents[i].Score != precedents[j].Score {
				return precedents[i].Score > precedents[j].Score
			}
			return precedents[i].Ref < precedents[j].Ref
		})
		if len(precedents) > in.Limit {
			precedents = precedents[:in.Limit]
		}
		item.Value = &precedents
		out.Total += len(precedents)
		out.Items[i] = item
	}
	return out, nil
}

func precedentToMCP(source, owner, repo string, t corpus.Thread, score float64) mcpserver.PrecedentOutput {
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
	return mcpserver.PrecedentOutput{Source: source, Ref: fmt.Sprintf("%s/%s#%d", owner, repo, t.Number), Kind: t.Kind, State: t.State, StateReason: t.StateReason, Title: t.Title, Score: score, Reasons: reasons, ClosedAt: formatTime(t.ClosedAt), MergedAt: formatTime(t.MergedAt)}
}

func wordSet(text string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := make(map[string]struct{})
	for _, field := range fields {
		if len(field) >= 3 {
			out[field] = struct{}{}
		}
	}
	return out
}
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	union := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		union[k] = struct{}{}
		if _, ok := b[k]; ok {
			intersection++
		}
	}
	for k := range b {
		union[k] = struct{}{}
	}
	return float64(intersection) / float64(len(union))
}

// SyncRepositoryMetadata submits a durable metadata-only GitHub read. It does
// not fetch threads, comments, reviews, or code.
func (r *MCPReader) SyncRepositoryMetadata(ctx context.Context, in mcpserver.SyncRepositoryMetadataInput) (mcpserver.JobReference, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 100 {
		return mcpserver.JobReference{}, errors.New("repositories must contain 1 to 100 items")
	}
	for _, input := range in.Repositories {
		if err := (domain.RepoRef{Owner: input.Owner, Repo: input.Repo}).Validate(); err != nil {
			return mcpserver.JobReference{}, err
		}
	}
	id, err := r.Service.submitJob(ctx, "sync_repository_metadata", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.Service.syncRepositoryMetadata(ctx, in.Repositories, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "sync_repository_metadata", Status: "queued", Message: "repository metadata sync job started"}, nil
}

// SearchGitHubRepositories performs one bounded live repository search and
// persists the returned metadata observations without fetching thread data.
func (r *MCPReader) SearchGitHubRepositories(ctx context.Context, in mcpserver.SearchGitHubRepositoriesInput) (mcpserver.SearchGitHubRepositoriesOutput, error) {
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.Sort != "" && in.Sort != "stars" && in.Sort != "forks" && in.Sort != "help-wanted-issues" && in.Sort != "updated" {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("sort must be stars, forks, help-wanted-issues, or updated")
	}
	if in.Order != "" && in.Order != "asc" && in.Order != "desc" {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("order must be asc or desc")
	}
	reader, err := r.Service.githubReader()
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	searcher, ok := reader.(github.RepositorySearcher)
	if !ok {
		return mcpserver.SearchGitHubRepositoriesOutput{}, errors.New("configured GitHub reader does not support repository search")
	}
	result, err := searcher.SearchRepositories(ctx, github.RepositorySearchOptions{Query: in.Query, Sort: in.Sort, Order: in.Order, PageOptions: github.PageOptions{Page: 1, PerPage: in.Limit}})
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	c, err := r.Service.openCorpus(ctx)
	if err != nil {
		return mcpserver.SearchGitHubRepositoriesOutput{}, err
	}
	out := mcpserver.SearchGitHubRepositoriesOutput{Status: "complete", Query: in.Query, Total: result.Total, Incomplete: result.Incomplete, Items: make([]mcpserver.BatchItem[mcpserver.TypedRepositoryOutput], len(result.Items))}
	if result.Incomplete {
		out.Status = "partial"
	}
	observedAt := r.Service.now()
	for i, remote := range result.Items {
		key := remote.Owner + "/" + remote.Name
		item := mcpserver.BatchItem[mcpserver.TypedRepositoryOutput]{Key: key, Status: "complete"}
		payload, err := json.Marshal(remote)
		if err != nil {
			return mcpserver.SearchGitHubRepositoriesOutput{}, err
		}
		stored, err := c.UpsertRepository(ctx, corpusRepoFromGitHub(remote), string(payload))
		if err == nil {
			err = c.AdvanceFacet(ctx, stored.ID, nil, "metadata", remote.UpdatedAt, true, 0)
		}
		if err != nil {
			return mcpserver.SearchGitHubRepositoriesOutput{}, err
		}
		value := typedRepository(stored)
		value.Metadata = mcpserver.RepositoryMetadataOutput{Status: "complete", ObservedAt: formatTime(observedAt), SourceUpdatedAt: formatTime(remote.UpdatedAt)}
		item.Value = &value
		out.Items[i] = item
	}
	return out, nil
}

// SyncThreads submits a durable bounded GitHub read for thread headers only.
func (r *MCPReader) SyncThreads(ctx context.Context, in mcpserver.SyncThreadsInput) (mcpserver.JobReference, error) {
	if in.Selection != "repositories" && in.Selection != "threads" {
		return mcpserver.JobReference{}, errors.New("selection must be repositories or threads")
	}
	if in.Selection == "repositories" && (len(in.Repositories) < 1 || len(in.Repositories) > 50) {
		return mcpserver.JobReference{}, errors.New("repositories must contain 1 to 50 items")
	}
	if in.Selection == "threads" && (len(in.Threads) < 1 || len(in.Threads) > 100) {
		return mcpserver.JobReference{}, errors.New("threads must contain 1 to 100 items")
	}
	id, err := r.Service.submitJob(ctx, "sync_threads", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.Service.syncThreadsBatch(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "sync_threads", Status: "queued", Message: "thread synchronization job started"}, nil
}

func (s *Service) syncThreadsBatch(ctx context.Context, in mcpserver.SyncThreadsInput, report func(string, string) error) (map[string]any, error) {
	type task struct {
		key     string
		ref     cli.RepoRef
		numbers []int
	}
	var tasks []task
	if in.Selection == "repositories" {
		for _, ref := range in.Repositories {
			tasks = append(tasks, task{key: ref.Owner + "/" + ref.Repo, ref: cli.RepoRef{Owner: ref.Owner, Repo: ref.Repo}})
		}
	} else {
		grouped := make(map[string]int)
		for _, thread := range in.Threads {
			key := thread.Owner + "/" + thread.Repo
			index, ok := grouped[key]
			if !ok {
				grouped[key] = len(tasks)
				tasks = append(tasks, task{key: key, ref: cli.RepoRef{Owner: thread.Owner, Repo: thread.Repo}})
				index = len(tasks) - 1
			}
			tasks[index].numbers = append(tasks[index].numbers, thread.Number)
		}
	}
	state := in.State
	if state == "" {
		state = "open"
	}
	kind := in.Kind
	if kind == "" {
		kind = "both"
	}
	maxPages := 1
	if in.LimitPerRepository > 100 {
		maxPages = (in.LimitPerRepository + 99) / 100
	}
	var since time.Time
	if in.UpdatedAfter != "" {
		parsed, err := time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return nil, errors.New("updated_after must be RFC 3339")
		}
		since = parsed
	}
	results := make([]map[string]any, len(tasks))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 4
	if len(tasks) < workers {
		workers = len(tasks)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := tasks[index]
				opts := SyncOptions{Kind: kind, State: state, Since: since, Numbers: current.numbers, MaxPages: maxPages}
				if len(current.numbers) > 0 {
					opts.State = "all"
					opts.Since = time.Time{}
				}
				res, err := s.SyncWithOptions(ctx, current.ref, opts)
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = map[string]any{"key": current.key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				results[index] = map[string]any{"key": current.key, "status": "complete", "updated": res.Updated, "message": res.Message}
			}
		}()
	}
	for i := range tasks {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	status := "complete"
	completed := 0
	for _, result := range results {
		if result["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	_ = report("100%", fmt.Sprintf("completed=%d total=%d", completed, len(tasks)))
	return map[string]any{"status": status, "items": results, "completed": completed, "total": len(tasks)}, nil
}

// HydrateThreads submits a durable GitHub read for explicit child facets on
// selected threads; an empty facet set is rejected.
func (r *MCPReader) HydrateThreads(ctx context.Context, in mcpserver.HydrateThreadsInput) (mcpserver.JobReference, error) {
	if len(in.Threads) < 1 || len(in.Threads) > 100 {
		return mcpserver.JobReference{}, errors.New("threads must contain 1 to 100 items")
	}
	if len(in.Facets) == 0 {
		return mcpserver.JobReference{}, errors.New("facets must not be empty")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	id, err := r.Service.submitJob(ctx, "hydrate_threads", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.Service.hydrateThreadsBatch(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "hydrate_threads", Status: "queued", Message: "thread hydration job started"}, nil
}

// GetAuthenticatedIdentity reads the GitHub account associated with the active credential.
func (r *MCPReader) GetAuthenticatedIdentity(ctx context.Context) (mcpserver.AuthenticatedIdentityOutput, error) {
	reader, err := r.Service.githubReader()
	if err != nil {
		return mcpserver.AuthenticatedIdentityOutput{}, err
	}
	identityReader, ok := reader.(github.IdentityReader)
	if !ok {
		return mcpserver.AuthenticatedIdentityOutput{}, errors.New("GitHub reader does not support authenticated identity lookup")
	}
	identity, _, err := identityReader.GetAuthenticatedIdentity(ctx)
	if err != nil {
		return mcpserver.AuthenticatedIdentityOutput{}, err
	}
	return mcpserver.AuthenticatedIdentityOutput{Login: identity.Login, ID: identity.ID, NodeID: identity.NodeID, ObservedAt: formatTime(r.Service.now())}, nil
}

// SyncAuthoredPullRequests submits a durable GitHub search and exact-header
// refresh for pull requests authored by the authenticated account.
func (r *MCPReader) SyncAuthoredPullRequests(ctx context.Context, in mcpserver.SyncAuthoredPullRequestsInput) (mcpserver.JobReference, error) {
	if in.State == "" {
		in.State = "open"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return mcpserver.JobReference{}, errors.New("state must be open, closed, or all")
	}
	if in.Limit == 0 {
		in.Limit = 500
	}
	if in.Limit < 1 || in.Limit > 500 {
		return mcpserver.JobReference{}, errors.New("limit must be between 1 and 500")
	}
	id, err := r.Service.submitJob(ctx, "sync_authored_pull_requests", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.Service.syncAuthoredPullRequests(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "sync_authored_pull_requests", Status: "queued", Message: "authored pull-request synchronization job started"}, nil
}

func (s *Service) syncAuthoredPullRequests(ctx context.Context, in mcpserver.SyncAuthoredPullRequestsInput, report func(string, string) error) (map[string]any, error) {
	reader, err := s.githubReader()
	if err != nil {
		return nil, err
	}
	identityReader, ok := reader.(github.IdentityReader)
	if !ok {
		return nil, errors.New("GitHub reader does not support authenticated identity lookup")
	}
	searcher, ok := reader.(github.AuthoredPullRequestSearcher)
	if !ok {
		return nil, errors.New("GitHub reader does not support authored pull-request search")
	}
	identity, _, err := identityReader.GetAuthenticatedIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var updatedAfter time.Time
	if in.UpdatedAfter != "" {
		updatedAfter, err = time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return nil, errors.New("updated_after must be RFC 3339")
		}
	}
	page := 1
	byRepo := make(map[string][]int)
	order := make([]string, 0)
	discovered := 0
	incomplete := false
	for discovered < in.Limit {
		perPage := 100
		if remaining := in.Limit - discovered; remaining < perPage {
			perPage = remaining
		}
		result, err := searcher.SearchAuthoredPullRequests(ctx, github.AuthoredPullRequestSearchOptions{Login: identity.Login, State: in.State, UpdatedAfter: updatedAfter, PageOptions: github.PageOptions{Page: page, PerPage: perPage}})
		if err != nil {
			return nil, err
		}
		incomplete = incomplete || result.Incomplete
		for _, pr := range result.Items {
			if pr.RepositoryOwner == "" || pr.RepositoryName == "" {
				continue
			}
			key := pr.RepositoryOwner + "/" + pr.RepositoryName
			if _, exists := byRepo[key]; !exists {
				order = append(order, key)
			}
			byRepo[key] = append(byRepo[key], pr.Number)
			discovered++
			if discovered >= in.Limit {
				break
			}
		}
		if !result.Page.HasNext || discovered >= in.Limit {
			break
		}
		page = result.Page.NextPage
	}
	type authoredTask struct {
		key, owner, repo string
		numbers          []int
	}
	var tasks []authoredTask
	for _, key := range order {
		owner, repo, _ := strings.Cut(key, "/")
		numbers := byRepo[key]
		for len(numbers) > 0 {
			size := min(100, len(numbers))
			tasks = append(tasks, authoredTask{key: key, owner: owner, repo: repo, numbers: append([]int(nil), numbers[:size]...)})
			numbers = numbers[size:]
		}
	}
	results := make([]map[string]any, len(tasks))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 4
	if len(tasks) < workers {
		workers = len(tasks)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := tasks[index]
				res, err := s.SyncWithOptions(ctx, cli.RepoRef{Owner: current.owner, Repo: current.repo}, SyncOptions{Kind: "pull_request", State: "all", Numbers: current.numbers, MaxPages: 1})
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = map[string]any{"key": current.key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				results[index] = map[string]any{"key": current.key, "status": "complete", "updated": res.Updated}
			}
		}()
	}
	for i := range tasks {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	status := "complete"
	completed := 0
	for _, result := range results {
		if result["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	_ = report("100%", fmt.Sprintf("repositories=%d pull_requests=%d", completed, discovered))
	return map[string]any{"status": status, "login": identity.Login, "pull_requests": discovered, "repositories": results, "search_incomplete": incomplete}, nil
}

// SyncPullRequestStatus submits REST hydration for PR details and reviews.
// Checks, unresolved review threads, and merge-queue state are not fetched.
func (r *MCPReader) SyncPullRequestStatus(ctx context.Context, in mcpserver.SyncPullRequestStatusInput) (mcpserver.JobReference, error) {
	if len(in.PullRequests) < 1 || len(in.PullRequests) > 50 {
		return mcpserver.JobReference{}, errors.New("pull_requests must contain 1 to 50 items")
	}
	if in.MaxPages == 0 {
		in.MaxPages = 3
	}
	hydrate := mcpserver.HydrateThreadsInput{Threads: append([]mcpserver.ThreadRef(nil), in.PullRequests...), Facets: []string{"pr_details", "pr_reviews"}, MaxPages: in.MaxPages}
	for i := range hydrate.Threads {
		hydrate.Threads[i].Kind = "pull_request"
	}
	id, err := r.Service.submitJob(ctx, "sync_pull_request_status", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.Service.hydrateThreadsBatch(ctx, hydrate, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "sync_pull_request_status", Status: "queued", Message: "pull-request status synchronization job started"}, nil
}

// IndexRepositories submits a durable Git acquisition and safe indexing job
// with at most two repositories processed concurrently.
func (r *MCPReader) IndexRepositories(ctx context.Context, in mcpserver.IndexRepositoriesInput) (mcpserver.JobReference, error) {
	if len(in.Repositories) < 1 || len(in.Repositories) > 10 {
		return mcpserver.JobReference{}, errors.New("repositories must contain 1 to 10 items")
	}
	for _, input := range in.Repositories {
		if err := (domain.RepoRef{Owner: input.Owner, Repo: input.Repo}).Validate(); err != nil {
			return mcpserver.JobReference{}, err
		}
	}
	id, err := r.Service.submitJob(ctx, "index_repositories", in, func(ctx context.Context, report func(string, string) error) (any, error) {
		return r.Service.indexRepositoriesBatch(ctx, in, report)
	})
	if err != nil {
		return mcpserver.JobReference{}, err
	}
	return mcpserver.JobReference{ID: id, Kind: "index_repositories", Status: "queued", Message: "repository indexing job started"}, nil
}

// CheckMergeConflicts compares already-fetched OIDs in managed workspaces
// without fetching or modifying refs, indexes, or worktrees.
func (r *MCPReader) CheckMergeConflicts(ctx context.Context, in mcpserver.CheckMergeConflictsInput) (mcpserver.CheckMergeConflictsOutput, error) {
	if len(in.Comparisons) < 1 || len(in.Comparisons) > 50 {
		return mcpserver.CheckMergeConflictsOutput{}, errors.New("comparisons must contain 1 to 50 items")
	}
	c, err := r.Service.openCorpus(ctx)
	if err != nil {
		return mcpserver.CheckMergeConflictsOutput{}, err
	}
	manager, err := r.Service.workspaceManager(ctx)
	if err != nil {
		return mcpserver.CheckMergeConflictsOutput{}, err
	}
	out := mcpserver.CheckMergeConflictsOutput{Status: "complete", Items: make([]mcpserver.BatchItem[mcpserver.MergeConflictOutput], len(in.Comparisons))}
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 4
	if len(in.Comparisons) < workers {
		workers = len(in.Comparisons)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := in.Comparisons[index]
				key := current.WorkspaceID + ":" + current.BaseOID + ".." + current.HeadOID
				item := mcpserver.BatchItem[mcpserver.MergeConflictOutput]{Key: key, Status: "complete"}
				ws, err := c.GetWorkspace(ctx, current.WorkspaceID)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "workspace_not_found", err.Error()
					out.Items[index] = item
					continue
				}
				result, err := manager.CheckMerge(ctx, ws.Path, current.BaseOID, current.HeadOID)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "merge_check_failed", err.Error()
					out.Items[index] = item
					continue
				}
				value := mcpserver.MergeConflictOutput{WorkspaceID: current.WorkspaceID, BaseOID: current.BaseOID, HeadOID: current.HeadOID, MergeBase: result.MergeBase, Conflicted: result.Conflicted, Summary: result.Summary}
				item.Value = &value
				out.Items[index] = item
			}
		}()
	}
	for i := range in.Comparisons {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return out, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	for _, item := range out.Items {
		if item.Status != "complete" {
			out.Status = "partial"
			break
		}
	}
	return out, nil
}

func (s *Service) indexRepositoriesBatch(ctx context.Context, in mcpserver.IndexRepositoriesInput, report func(string, string) error) (map[string]any, error) {
	results := make([]map[string]any, len(in.Repositories))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 2
	if len(in.Repositories) < workers {
		workers = len(in.Repositories)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := in.Repositories[index]
				key := current.Owner + "/" + current.Repo
				result, err := s.Acquire(ctx, cli.RepoRef{Owner: current.Owner, Repo: current.Repo}, current.Remote)
				if err != nil {
					results[index] = map[string]any{"key": key, "status": "failed", "reason": "acquisition_or_index_failed", "message": err.Error()}
					continue
				}
				results[index] = map[string]any{"key": key, "status": "complete", "commit_sha": result.CommitSHA, "files": result.Files, "bytes": result.Bytes, "inserted": result.Inserted}
			}
		}()
	}
	for i := range in.Repositories {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	status := "complete"
	completed := 0
	for _, result := range results {
		if result["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	_ = report("100%", fmt.Sprintf("completed=%d total=%d", completed, len(in.Repositories)))
	return map[string]any{"status": status, "items": results, "completed": completed, "total": len(in.Repositories)}, nil
}

func (s *Service) hydrateThreadsBatch(ctx context.Context, in mcpserver.HydrateThreadsInput, report func(string, string) error) (map[string]any, error) {
	results := make([]map[string]any, len(in.Threads))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := 4
	if len(in.Threads) < workers {
		workers = len(in.Threads)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				current := in.Threads[index]
				key := fmt.Sprintf("%s/%s#%d", current.Owner, current.Repo, current.Number)
				res, err := s.Hydrate(ctx, cli.RepoRef{Owner: current.Owner, Repo: current.Repo}, current.Number, cli.HydrateOptions{Facets: in.Facets, MaxPages: in.MaxPages})
				if err != nil {
					status, reason, message, retry := githubBatchError(err)
					results[index] = map[string]any{"key": key, "status": status, "reason": reason, "message": message, "retry_after_ms": retry}
					continue
				}
				results[index] = map[string]any{"key": key, "status": "complete", "kind": res.Kind, "requests": res.Requests, "facets": res.Facets}
			}
		}()
	}
	for i := range in.Threads {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	status := "complete"
	completed := 0
	for _, result := range results {
		if result["status"] == "complete" {
			completed++
		} else {
			status = "partial"
		}
	}
	_ = report("100%", fmt.Sprintf("completed=%d total=%d", completed, len(in.Threads)))
	return map[string]any{"status": status, "items": results, "completed": completed, "total": len(in.Threads)}, nil
}

func (s *Service) syncRepositoryMetadata(ctx context.Context, refs []mcpserver.RepositoryRef, report func(string, string) error) (mcpserver.GetRepositoriesOutput, error) {
	reader, err := s.githubReader()
	if err != nil {
		return mcpserver.GetRepositoriesOutput{}, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return mcpserver.GetRepositoriesOutput{}, err
	}
	out := mcpserver.GetRepositoriesOutput{Status: "complete", Items: make([]mcpserver.BatchItem[mcpserver.TypedRepositoryOutput], len(refs))}
	type work struct {
		index int
		ref   mcpserver.RepositoryRef
	}
	jobs := make(chan work)
	var wg sync.WaitGroup
	workers := 8
	if len(refs) < workers {
		workers = len(refs)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for current := range jobs {
				if ctx.Err() != nil {
					return
				}
				key := current.ref.Owner + "/" + current.ref.Repo
				item := mcpserver.BatchItem[mcpserver.TypedRepositoryOutput]{Key: key, Status: "complete"}
				remote, _, err := reader.GetRepository(ctx, current.ref.Owner, current.ref.Repo)
				if err != nil {
					item.Status, item.Reason, item.Message, item.RetryAfterMS = githubBatchError(err)
					out.Items[current.index] = item
					continue
				}
				payload, err := json.Marshal(remote)
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "marshal", err.Error()
					out.Items[current.index] = item
					continue
				}
				stored, err := c.UpsertRepository(ctx, corpusRepoFromGitHub(remote), string(payload))
				if err == nil {
					err = c.AdvanceFacet(ctx, stored.ID, nil, "metadata", remote.UpdatedAt, true, 0)
				}
				if err != nil {
					item.Status, item.Reason, item.Message = "failed", "storage", err.Error()
					out.Items[current.index] = item
					continue
				}
				value := typedRepository(stored)
				value.Metadata = mcpserver.RepositoryMetadataOutput{Status: "complete", ObservedAt: formatTime(s.now()), SourceUpdatedAt: formatTime(remote.UpdatedAt)}
				item.Value = &value
				out.Items[current.index] = item
			}
		}()
	}
	for i, ref := range refs {
		select {
		case jobs <- work{i, ref}:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return out, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	completed := 0
	for _, item := range out.Items {
		if item.Status == "complete" {
			completed++
		} else {
			out.Status = "partial"
		}
	}
	_ = report("100%", fmt.Sprintf("completed=%d total=%d", completed, len(refs)))
	return out, nil
}

func githubBatchError(err error) (status, reason, message string, retryMS int) {
	message = err.Error()
	var primary *github.PrimaryRateLimitError
	var secondary *github.SecondaryRateLimitError
	var transient *github.TransientError
	var notFound *github.NotFoundError
	var denied *github.AccessDeniedError
	switch {
	case errors.As(err, &primary):
		return "retryable", "rate_limited", message, int(primary.RetryAfter.Milliseconds())
	case errors.As(err, &secondary):
		return "retryable", "rate_limited", message, int(secondary.RetryAfter.Milliseconds())
	case errors.As(err, &transient):
		return "retryable", "transient", message, 1000
	case errors.As(err, &notFound):
		return "unavailable", "not_found", message, 0
	case errors.As(err, &denied):
		return "unavailable", "access_denied", message, 0
	default:
		return "failed", "request_failed", message, 0
	}
}

// DeepWiki performs one external derived-knowledge read and does not persist its response.
func (r *MCPReader) DeepWiki(ctx context.Context, in mcpserver.DeepWikiInput) (mcpserver.DeepWikiOutput, error) {
	if in.Action != "structure" && in.Action != "contents" && in.Action != "question" {
		return mcpserver.DeepWikiOutput{}, errors.New("action must be structure, contents, or question")
	}
	if (in.Action == "structure" || in.Action == "contents") && strings.TrimSpace(in.Repository) == "" {
		return mcpserver.DeepWikiOutput{}, errors.New("repository is required for structure or contents")
	}
	if in.Action == "question" && (len(in.Repositories) < 1 || strings.TrimSpace(in.Question) == "") {
		return mcpserver.DeepWikiOutput{}, errors.New("repositories and question are required for question")
	}
	repositories := append([]string(nil), in.Repositories...)
	if in.Repository != "" {
		repositories = []string{in.Repository}
	}
	if len(repositories) > 10 {
		return mcpserver.DeepWikiOutput{}, errors.New("DeepWiki supports at most 10 repositories")
	}
	res, err := r.Service.deepWiki().Read(ctx, deepwiki.Request{Action: in.Action, Repository: in.Repository, Repositories: in.Repositories, Question: in.Question})
	if err != nil {
		return mcpserver.DeepWikiOutput{}, err
	}
	out := mcpserver.DeepWikiOutput{Status: "complete", Provider: "deepwiki", Action: in.Action, Repositories: repositories, Question: in.Question, Result: res.Text, SourceURL: res.SourceURL, RetrievedAt: formatTime(r.Service.now()), Provenance: "derived_external"}
	if !res.Available {
		out.Status, out.Reason, out.NextAction = "unavailable", "not_indexed_or_unavailable", "Use GitHub metadata, stored corpus data, or explicit code acquisition instead."
		return out, nil
	}
	maxBytes := in.MaxOutputBytes
	if maxBytes == 0 {
		maxBytes = 131072
	}
	if maxBytes < 1024 || maxBytes > 1048576 {
		return mcpserver.DeepWikiOutput{}, errors.New("max_output_bytes must be between 1024 and 1048576")
	}
	if len(out.Result) > maxBytes {
		out.Result = validUTF8Prefix(out.Result, maxBytes)
		out.Truncated = true
	}
	return out, nil
}

func validUTF8Prefix(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.ValidString(value[:maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

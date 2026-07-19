package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
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
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpserver.ListPullRequestPortfolioOutput{}, err
	}
	stored, err := c.ListPullRequestPortfolio(ctx, strings.TrimSpace(in.Author), in.State, in.Limit)
	if err != nil {
		return mcpserver.ListPullRequestPortfolioOutput{}, err
	}
	out := mcpserver.ListPullRequestPortfolioOutput{Status: "complete", RuleVersion: "portfolio.v1", GeneratedAt: formatTime(r.now()), PullRequests: make([]mcpserver.PullRequestPortfolioItem, 0, len(stored)), Total: len(stored)}
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
// portfolio.v1 classification together so unknown facets cannot become facts.
//
//nolint:gocognit,cyclop
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

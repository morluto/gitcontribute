package app

import (
	"context"
	"errors"
	"strings"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// ListPullRequestPortfolio performs an offline projection over stored authored
// PRs and status facets; unsupported health facets remain explicitly unknown.
func (r *MCPReader) ListPullRequestPortfolio(ctx context.Context, in mcpcontract.ListPullRequestPortfolioInput) (mcpcontract.ListPullRequestPortfolioOutput, error) {
	if len(in.Authors) > 1 {
		return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("authors must contain at most one item")
	}
	if len(in.PullRequests) > 0 {
		if len(in.PullRequests) > 100 {
			return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("pull_requests must contain at most 100 items")
		}
		if in.Repository != nil || len(in.Authors) > 0 || in.State != "" || in.Limit != 0 {
			return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("pull_requests cannot be combined with repository, authors, state, or limit")
		}
		in.PullRequests = canonicalPullRequestRefs(in.PullRequests)
		if err := rejectDuplicateThreadRefs(in.PullRequests); err != nil {
			return mcpcontract.ListPullRequestPortfolioOutput{}, err
		}
		if err := validatePullRequestRefs(in.PullRequests, "pull_requests"); err != nil {
			return mcpcontract.ListPullRequestPortfolioOutput{}, err
		}
	}
	if in.Repository != nil {
		in.Repository.Owner = strings.TrimSpace(in.Repository.Owner)
		in.Repository.Repo = strings.TrimSpace(in.Repository.Repo)
		if err := (domain.RepoRef{Owner: in.Repository.Owner, Repo: in.Repository.Repo}).Validate(); err != nil {
			return mcpcontract.ListPullRequestPortfolioOutput{}, err
		}
	}
	if in.State == "" {
		in.State = "open"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("state must be open, closed, or all")
	}
	if in.View == "" {
		in.View = "compact"
	}
	if in.View != "compact" && in.View != "full" {
		return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("view must be compact or full")
	}
	if in.Limit == 0 && len(in.PullRequests) == 0 {
		in.Limit = 20
	}
	if len(in.PullRequests) == 0 && (in.Limit < 1 || in.Limit > 100) {
		return mcpcontract.ListPullRequestPortfolioOutput{}, errors.New("limit must be between 1 and 100")
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.ListPullRequestPortfolioOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.SnapshotToken)
	if err != nil {
		return mcpcontract.ListPullRequestPortfolioOutput{}, err
	}
	page, unavailable, err := portfolioPage(ctx, c, in)
	if err != nil {
		return mcpcontract.ListPullRequestPortfolioOutput{}, err
	}
	format := portfolioResponseFormat(map[string]string{"compact": "concise", "full": "detailed"}[in.View])
	readSet, err := loadPortfolioReadSet(ctx, c, page.PullRequests, format)
	if err != nil {
		return mcpcontract.ListPullRequestPortfolioOutput{}, err
	}
	out := mcpcontract.ListPullRequestPortfolioOutput{Status: "complete", View: in.View, RuleVersion: "portfolio.v2", GeneratedAt: formatTime(r.now()), PullRequests: make([]mcpcontract.PullRequestPortfolioItem, 0, len(page.PullRequests)), Total: page.Total, Truncated: page.Truncated, UnavailablePullRequests: unavailable, SnapshotToken: snapshotIdentity(in.SnapshotToken, revision)}
	if len(unavailable) > 0 {
		out.Status = "partial"
		out.Recovery = recoveryPlan("portfolio_items_unavailable", "Some exact pull requests are not present in the local corpus. Refresh those exact pull requests, then reread the portfolio.", mcpcontract.RecoveryAction(mcpcontract.SyncPortfolioInput{Selection: "explicit", PullRequests: append([]mcpcontract.ThreadRef(nil), unavailable...)}))
	}
	for _, storedPR := range page.PullRequests {
		item, err := portfolioItem(storedPR, r.now(), readSet, format)
		if err != nil {
			return mcpcontract.ListPullRequestPortfolioOutput{}, err
		}
		if item.StatusCoverage != "complete" {
			out.Status = "partial"
			item.Recovery = recoveryPlan("portfolio_facet_incomplete", "Refresh the incomplete pull-request health facets, then reread this exact portfolio item.", syncPullRequestCalls([]mcpcontract.ThreadRef{{Owner: item.Owner, Repo: item.Repo, Kind: "pull_request", Number: item.Number}})...)
		}
		out.PullRequests = append(out.PullRequests, item)
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.ListPullRequestPortfolioOutput{}, err
	}
	if out.Truncated {
		out.Status = "partial"
		nextLimit := min(100, max(in.Limit*2, in.Limit+1))
		out.Recovery = recoveryPlan("portfolio_truncated", "The portfolio page is bounded. Read the next larger page before treating the returned set as exhaustive.", mcpcontract.RecoveryAction(mcpcontract.ListPullRequestPortfolioInput{Repository: in.Repository, Authors: append([]string(nil), in.Authors...), State: in.State, Limit: nextLimit, View: in.View, SnapshotToken: in.SnapshotToken}))
	}
	return out, nil
}

func portfolioPage(ctx context.Context, c *corpus.Corpus, in mcpcontract.ListPullRequestPortfolioInput) (corpus.PortfolioPage, []mcpcontract.ThreadRef, error) {
	if len(in.PullRequests) == 0 {
		author := ""
		if len(in.Authors) > 0 {
			author = strings.TrimSpace(in.Authors[0])
		}
		var repository *corpus.RepositoryKey
		if in.Repository != nil {
			repository = &corpus.RepositoryKey{Owner: in.Repository.Owner, Name: in.Repository.Repo}
		}
		page, err := c.ListPullRequestPortfolioPage(ctx, author, in.State, repository, in.Limit)
		return page, nil, err
	}
	repositoryKeys := make([]corpus.RepositoryKey, 0, len(in.PullRequests))
	for _, ref := range in.PullRequests {
		repositoryKeys = append(repositoryKeys, corpus.RepositoryKey{Owner: ref.Owner, Name: ref.Repo})
	}
	repositories, err := c.GetRepositoriesBatch(ctx, repositoryKeys)
	if err != nil {
		return corpus.PortfolioPage{}, nil, err
	}
	threadKeys := make([]corpus.ThreadKey, 0, len(in.PullRequests))
	for _, ref := range in.PullRequests {
		if repository := repositories[corpus.RepositoryKey{Owner: ref.Owner, Name: ref.Repo}]; repository != nil {
			threadKeys = append(threadKeys, corpus.ThreadKey{RepositoryID: repository.ID, Kind: corpus.ThreadKindPullRequest, Number: ref.Number})
		}
	}
	threads, err := c.GetThreadsBatch(ctx, threadKeys)
	if err != nil {
		return corpus.PortfolioPage{}, nil, err
	}
	page := corpus.PortfolioPage{PullRequests: make([]corpus.PortfolioPullRequest, 0, len(in.PullRequests)), Total: len(in.PullRequests)}
	unavailable := make([]mcpcontract.ThreadRef, 0)
	for _, ref := range in.PullRequests {
		repository := repositories[corpus.RepositoryKey{Owner: ref.Owner, Name: ref.Repo}]
		if repository == nil {
			unavailable = append(unavailable, ref)
			continue
		}
		thread := threads[corpus.ThreadKey{RepositoryID: repository.ID, Kind: corpus.ThreadKindPullRequest, Number: ref.Number}]
		if thread == nil {
			unavailable = append(unavailable, ref)
			continue
		}
		page.PullRequests = append(page.PullRequests, corpus.PortfolioPullRequest{Owner: repository.Owner, Repo: repository.Name, Thread: *thread})
	}
	return page, unavailable, nil
}

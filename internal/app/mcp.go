package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// MCPReader adapts Service to the mcpserver.Reader interface. It is a thin
// wrapper because Go does not allow two methods named Search on one type.
type MCPReader struct{ *Service }

// MCPReader returns a local, read-only MCP reader backed by this service.
func (s *Service) MCPReader() mcpserver.Reader { return &MCPReader{s} }

// Search performs a local-only corpus search through the MCP interface.
func (r *MCPReader) Search(ctx context.Context, in mcpserver.SearchInput) (mcpserver.SearchOutput, error) {
	if in.Query == "" {
		return mcpserver.SearchOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.SearchOutput{}, errors.New("limit must be between 1 and 100")
	}

	repo := ""
	if (in.Owner == "") != (in.Repo == "") {
		return mcpserver.SearchOutput{}, errors.New("owner and repo must be provided together")
	}
	if in.Owner != "" && in.Repo != "" {
		repo = in.Owner + "/" + in.Repo
	}
	res, err := r.Service.searchCorpus(ctx, in.Query, cli.SearchOptions{
		Kind:  in.Kind,
		Repo:  repo,
		Limit: in.Limit,
	})
	if err != nil {
		return mcpserver.SearchOutput{}, err
	}

	matches := make([]mcpserver.ThreadOutput, len(res.Matches))
	for i, m := range res.Matches {
		updatedAt := ""
		if !m.UpdatedAt.IsZero() {
			updatedAt = m.UpdatedAt.Format(time.RFC3339)
		}
		matches[i] = mcpserver.ThreadOutput{
			Owner:     m.Repo.Owner,
			Repo:      m.Repo.Repo,
			Kind:      m.Kind,
			Number:    m.Number,
			State:     m.State,
			Title:     m.Title,
			Body:      m.Body,
			Author:    m.Author,
			Labels:    m.Labels,
			UpdatedAt: updatedAt,
		}
	}
	return mcpserver.SearchOutput{Query: in.Query, Total: res.Total, Matches: matches}, nil
}

// Repository reads a repository projection from the local corpus.
func (r *MCPReader) Repository(ctx context.Context, in mcpserver.RepoInput) (mcpserver.RepositoryOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.RepositoryOutput{}, err
	}
	c, err := r.Service.openCorpus(ctx)
	if err != nil {
		return mcpserver.RepositoryOutput{}, err
	}
	repo, err := c.GetRepository(ctx, in.Owner, in.Repo)
	if err != nil {
		return mcpserver.RepositoryOutput{}, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpserver.RepositoryOutput{}, mcpserver.ErrNotFound
	}
	return mcpserver.RepositoryOutput{
		Owner:     repo.Owner,
		Repo:      repo.Name,
		UpdatedAt: repo.SourceUpdatedAt.Format(time.RFC3339),
		Fields: map[string]any{
			"description":    repo.Description,
			"default_branch": repo.DefaultBranch,
			"language":       repo.Language,
			"license":        repo.License,
			"topics":         repo.Topics,
			"stars":          repo.Stars,
			"watchers":       repo.Watchers,
			"forks":          repo.Forks,
			"open_issues":    repo.OpenIssues,
			"archived":       repo.Archived,
			"fork":           repo.Fork,
		},
	}, nil
}

// Thread reads one issue or pull request from the local corpus.
func (r *MCPReader) Thread(ctx context.Context, in mcpserver.ThreadInput) (mcpserver.ThreadOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.ThreadOutput{}, err
	}
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return mcpserver.ThreadOutput{}, errors.New("kind must be issue or pull_request")
	}
	if in.Number < 1 {
		return mcpserver.ThreadOutput{}, errors.New("number must be positive")
	}
	c, err := r.Service.openCorpus(ctx)
	if err != nil {
		return mcpserver.ThreadOutput{}, err
	}
	repo, err := c.GetRepository(ctx, in.Owner, in.Repo)
	if err != nil {
		return mcpserver.ThreadOutput{}, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return mcpserver.ThreadOutput{}, mcpserver.ErrNotFound
	}
	thread, err := c.GetThread(ctx, repo.ID, in.Kind, in.Number)
	if err != nil {
		return mcpserver.ThreadOutput{}, fmt.Errorf("get thread: %w", err)
	}
	if thread == nil {
		return mcpserver.ThreadOutput{}, mcpserver.ErrNotFound
	}
	out := corpusThreadToMCPOutput(thread)
	out.Owner = in.Owner
	out.Repo = in.Repo
	return out, nil
}

func corpusThreadToMCPOutput(t *corpus.Thread) mcpserver.ThreadOutput {
	updatedAt := ""
	if !t.UpdatedAt.IsZero() {
		updatedAt = t.UpdatedAt.Format(time.RFC3339)
	}
	return mcpserver.ThreadOutput{
		Owner:     "", // filled by caller
		Repo:      "",
		Kind:      t.Kind,
		Number:    t.Number,
		State:     t.State,
		Title:     t.Title,
		Body:      t.Body,
		Author:    t.Author,
		Labels:    t.Labels,
		UpdatedAt: updatedAt,
	}
}

// Dossier builds a source-backed repository dossier from local corpus data.
func (r *MCPReader) Dossier(ctx context.Context, in mcpserver.RepoInput) (mcpserver.DossierOutput, error) {
	ref := domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
	if err := ref.Validate(); err != nil {
		return mcpserver.DossierOutput{}, err
	}
	if _, err := r.Service.openCorpus(ctx); err != nil {
		return mcpserver.DossierOutput{}, err
	}
	d, err := r.Service.buildDossier(ctx, ref)
	if err != nil {
		return mcpserver.DossierOutput{}, err
	}
	return dossierToMCPOutput(d), nil
}

func dossierToMCPOutput(d *domain.Dossier) mcpserver.DossierOutput {
	return mcpserver.DossierOutput{
		Owner: d.Repo.Owner,
		Repo:  d.Repo.Repo,
		AsOf:  d.AsOf.Format(time.RFC3339),
		Sections: map[string]any{
			"description":                d.Repository.Description,
			"language":                   firstLanguage(d.Repository.Languages),
			"stars":                      d.Repository.Stars,
			"open_issues":                d.OpenIssueCount,
			"closed_issues":              d.ClosedIssueCount,
			"open_prs":                   d.OpenPullRequestCount,
			"merged_prs":                 d.MergedPullRequestCount,
			"closed_unmerged_prs":        d.ClosedUnmergedPullRequestCount,
			"recent_merged_prs":          d.RecentMergedPullRequests,
			"recent_open_prs":            d.RecentOpenPullRequests,
			"recent_closed_unmerged_prs": d.RecentClosedUnmergedPullRequests,
			"recent_issues":              d.RecentIssues,
			"guidance":                   d.ContributionGuidance,
			"coverage":                   coverageNames(d.Coverage),
		},
	}
}

// MCPRunner implements cli.MCPRunner by starting an MCP server over stdio.
type MCPRunner struct{ *Service }

// NewMCPRunner returns a cli.MCPRunner backed by the service.
func (s *Service) NewMCPRunner() cli.MCPRunner { return &MCPRunner{s} }

// Run starts the MCP server using the requested transport.
func (r *MCPRunner) Run(ctx context.Context, opts cli.MCPOptions) error {
	if opts.Transport != "stdio" {
		return fmt.Errorf("unsupported mcp transport %q", opts.Transport)
	}
	server := mcpserver.New(r.Service.MCPReader(), r.Service.version)
	return server.ServeStdio(ctx)
}

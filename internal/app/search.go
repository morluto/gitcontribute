package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

type searchMatch struct {
	Repo      domain.RepoRef
	Kind      string
	Number    int
	State     string
	Title     string
	Body      string
	Author    string
	Labels    []string
	UpdatedAt time.Time
	URL       string
	Score     float64
}

type searchResult struct {
	Query   string
	Total   int
	Matches []searchMatch
}

func (s *Service) searchCorpus(ctx context.Context, query string, opts cli.SearchOptions) (searchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 1000 {
		return searchResult{}, errors.New("search limit cannot exceed 1000")
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return searchResult{}, err
	}

	var repoID int64
	if opts.Repo != "" && opts.Kind != "code" && opts.Kind != "all" && opts.Kind != "repos" {
		ref := domain.RepoRef{}
		parts := strings.Split(opts.Repo, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return searchResult{}, fmt.Errorf("invalid repository filter %q", opts.Repo)
		}
		ref.Owner, ref.Repo = parts[0], parts[1]
		if err := ref.Validate(); err != nil {
			return searchResult{}, fmt.Errorf("invalid repository filter %q: %w", opts.Repo, err)
		}
		repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
		if err != nil {
			return searchResult{}, err
		}
		if repo == nil {
			return searchResult{Query: query, Total: 0, Matches: nil}, nil
		}
		repoID = repo.ID
	}

	kind := ""
	switch opts.Kind {
	case "issue", "issues":
		kind = corpus.ThreadKindIssue
	case "pr", "prs", "pull_request":
		kind = corpus.ThreadKindPullRequest
	case "threads", "":
		kind = ""
	case "repos":
		return s.searchRepositories(ctx, query, opts.Limit)
	case "code":
		return s.searchCode(ctx, query, opts.Repo, opts.Limit)
	case "all":
		return s.searchAll(ctx, query, opts)
	default:
		return searchResult{}, fmt.Errorf("unsupported search kind %q", opts.Kind)
	}

	if query == "" {
		return searchResult{Query: query, Total: 0, Matches: nil}, nil
	}

	threads, err := c.SearchThreadsWithFilter(ctx, query, corpus.SearchFilter{
		RepoID: repoID,
		Kind:   kind,
		Limit:  opts.Limit,
	})
	if err != nil {
		return searchResult{}, fmt.Errorf("search threads: %w", err)
	}

	matches := make([]searchMatch, 0, len(threads))
	repoCache := make(map[int64]*corpus.Repository)
	for _, t := range threads {
		repo, ok := repoCache[t.RepositoryID]
		if !ok {
			repo, err = c.GetRepositoryByID(ctx, t.RepositoryID)
			if err != nil {
				return searchResult{}, err
			}
			repoCache[t.RepositoryID] = repo
		}
		if repo == nil {
			continue
		}
		ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Name}
		matches = append(matches, searchMatch{
			Repo:      ref,
			Kind:      t.Kind,
			Number:    t.Number,
			State:     t.State,
			Title:     t.Title,
			Body:      t.Body,
			Author:    t.Author,
			Labels:    t.Labels,
			UpdatedAt: t.UpdatedAt,
			URL:       threadURL(ref, t.Kind, t.Number),
			Score:     0,
		})
	}
	return searchResult{Query: query, Total: len(matches), Matches: matches}, nil
}

func (s *Service) searchAll(ctx context.Context, query string, opts cli.SearchOptions) (searchResult, error) {
	var combined []searchMatch
	for _, kind := range []string{"threads", "repos", "code"} {
		part := opts
		part.Kind = kind
		result, err := s.searchCorpus(ctx, query, part)
		if err != nil {
			return searchResult{}, err
		}
		combined = append(combined, result.Matches...)
	}
	if len(combined) > opts.Limit {
		combined = combined[:opts.Limit]
	}
	return searchResult{Query: query, Total: len(combined), Matches: combined}, nil
}

func (s *Service) searchCode(ctx context.Context, query, repository string, limit int) (searchResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return searchResult{}, err
	}
	var ref domain.RepoRef
	if repository != "" {
		parts := strings.Split(repository, "/")
		if len(parts) != 2 {
			return searchResult{}, fmt.Errorf("invalid repository filter %q", repository)
		}
		ref = domain.RepoRef{Owner: parts[0], Repo: parts[1]}
	}
	matches, err := c.SearchCode(ctx, query, ref, limit)
	if err != nil {
		return searchResult{}, err
	}
	out := make([]searchMatch, len(matches))
	for i, match := range matches {
		out[i] = searchMatch{
			Repo: match.Repo, Kind: "code", Title: match.Path,
			Body: boundedText(match.Content, 2000),
		}
	}
	return searchResult{Query: query, Total: len(out), Matches: out}, nil
}

func boundedText(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}

func (s *Service) searchRepositories(ctx context.Context, query string, limit int) (searchResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return searchResult{}, err
	}
	repos, err := c.ListRepositories(ctx, query, limit)
	if err != nil {
		return searchResult{}, fmt.Errorf("list repositories: %w", err)
	}
	matches := make([]searchMatch, len(repos))
	for i, r := range repos {
		ref := domain.RepoRef{Owner: r.Owner, Repo: r.Name}
		matches[i] = searchMatch{
			Repo:  ref,
			Kind:  "repo",
			Title: ref.String(),
			URL:   fmt.Sprintf("https://github.com/%s", ref),
			Score: 0,
		}
	}
	return searchResult{Query: query, Total: len(matches), Matches: matches}, nil
}

func threadURL(ref domain.RepoRef, kind string, number int) string {
	path := "issues"
	if kind == corpus.ThreadKindPullRequest {
		path = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%d", ref, path, number)
}

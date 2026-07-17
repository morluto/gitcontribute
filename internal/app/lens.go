package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/lens"
)

// AddLens validates and stores a lens definition.
func (s *Service) AddLens(ctx context.Context, name string, def lens.Definition) (*cli.LensResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("lens name is required")
	}
	def.Name = name
	if err := lens.Validate(def); err != nil {
		return nil, err
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	record, err := c.SaveLens(ctx, def)
	if err != nil {
		return nil, fmt.Errorf("save lens: %w", err)
	}
	return lensResult(record), nil
}

// ListLenses returns all saved lenses in stable order.
func (s *Service) ListLenses(ctx context.Context) (*cli.LensListResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	records, err := c.ListLenses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list lenses: %w", err)
	}
	result := &cli.LensListResult{Lenses: make([]cli.LensResult, len(records))}
	for i, r := range records {
		result.Lenses[i] = *lensResult(&r)
	}
	return result, nil
}

// ShowLens returns a saved lens by name.
func (s *Service) ShowLens(ctx context.Context, name string) (*cli.LensResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("lens name is required")
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	record, err := c.GetLens(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get lens: %w", err)
	}
	if record == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("lens %q not found", name))
	}
	return lensResult(record), nil
}

func lensResult(r *corpus.LensRecord) *cli.LensResult {
	return &cli.LensResult{
		Name:       r.Definition.Name,
		Definition: r.Definition,
		CreatedAt:  formatTime(r.CreatedAt),
		UpdatedAt:  formatTime(r.UpdatedAt),
	}
}

// ExplainLens returns the saved definition, candidate facts, normalized signals,
// weighted contributions, final score, population context, and missing signals
// for a candidate under a saved lens. It performs no network access.
func (s *Service) ExplainLens(ctx context.Context, name, ref string, opts cli.LensExplainOptions) (*cli.LensExplainResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("lens name is required")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("result reference is required")
	}
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, errors.New("original search query is required")
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}

	lensRecord, err := c.GetLens(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("load lens: %w", err)
	}
	if lensRecord == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("lens %q not found", name))
	}
	now := s.now()
	target, inferredKind, err := s.resolveLensExplainTarget(ctx, c, ref)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(opts.Kind)
	if kind == "" {
		kind = inferredKind
	}
	matches, err := s.collectLensMatches(ctx, c, query, cli.SearchOptions{
		Kind: kind, Repo: opts.Repo, State: opts.State, Author: opts.Author,
		Association: opts.Association, Assignee: opts.Assignee,
		Labels: opts.Labels, UpdatedAfter: opts.UpdatedAfter,
	}, now)
	if err != nil {
		return nil, err
	}
	targetID := candidateFromMatch(target, query, now).ID

	candidates := make([]lens.Candidate, 0, len(matches))
	byID := make(map[string]searchMatch, len(matches))
	for _, m := range matches {
		cand := candidateFromMatch(m, query, now)
		candidates = append(candidates, cand)
		byID[cand.ID] = m
	}

	results, err := lens.Rank(lensRecord.Definition, candidates, now)
	if err != nil {
		return nil, fmt.Errorf("rank with lens: %w", err)
	}

	var found *lens.Result
	for i := range results {
		if results[i].Candidate.ID == targetID {
			found = &results[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("result %q does not match lens %q filters", ref, name)
	}
	match, ok := byID[found.Candidate.ID]
	if !ok {
		return nil, fmt.Errorf("result %q is missing from its explanation population", ref)
	}

	scope := "query matches across the local corpus"
	if opts.Repo != "" {
		scope = "query matches in " + opts.Repo
	}
	return buildLensExplainResult(lensRecord, *found, match, len(results), scope+" ("+kind+")", query, now), nil
}

func (s *Service) resolveLensExplainTarget(ctx context.Context, c *corpus.Corpus, ref string) (searchMatch, string, error) {
	prefix, rest := splitLensRef(ref)
	switch prefix {
	case "repo":
		return s.resolveRepoLensTarget(ctx, c, rest)
	case "issue":
		return s.resolveThreadLensTarget(ctx, c, rest, corpus.ThreadKindIssue)
	case "pr", "pull_request":
		return s.resolveThreadLensTarget(ctx, c, rest, corpus.ThreadKindPullRequest)
	case "code":
		return s.resolveCodeLensTarget(ctx, c, rest)
	case "":
		if strings.Contains(ref, "#") {
			return s.resolveThreadLensTarget(ctx, c, ref, "")
		}
		return s.resolveRepoLensTarget(ctx, c, ref)
	default:
		return searchMatch{}, "", fmt.Errorf("unsupported result reference %q", ref)
	}
}

func splitLensRef(ref string) (string, string) {
	idx := strings.IndexByte(ref, ':')
	if idx < 0 {
		return "", ref
	}
	return ref[:idx], ref[idx+1:]
}

func (s *Service) resolveRepoLensTarget(ctx context.Context, c *corpus.Corpus, ref string) (searchMatch, string, error) {
	repoRef, err := parseRepoRef(ref)
	if err != nil {
		return searchMatch{}, "", err
	}
	repo, err := c.GetRepository(ctx, repoRef.Owner, repoRef.Repo)
	if err != nil {
		return searchMatch{}, "", err
	}
	if repo == nil {
		return searchMatch{}, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %q not found", repoRef))
	}
	return searchMatch{
		Repo:      domain.RepoRef{Owner: repo.Owner, Repo: repo.Name},
		Kind:      "repo",
		Title:     repoRef.String(),
		Body:      repo.Description,
		URL:       fmt.Sprintf("https://github.com/%s", repoRef),
		Language:  repo.Language,
		Archived:  repo.Archived,
		Stars:     repo.Stars,
		Watchers:  repo.Watchers,
		Forks:     repo.Forks,
		UpdatedAt: repo.SourceUpdatedAt,
		Freshness: repo.SourceUpdatedAt,
	}, "repos", nil
}

func (s *Service) resolveThreadLensTarget(ctx context.Context, c *corpus.Corpus, ref, kind string) (searchMatch, string, error) {
	repoRef, number, err := parseThreadRef(ref)
	if err != nil {
		return searchMatch{}, "", err
	}
	repo, err := c.GetRepository(ctx, repoRef.Owner, repoRef.Repo)
	if err != nil {
		return searchMatch{}, "", err
	}
	if repo == nil {
		return searchMatch{}, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %q not found", repoRef))
	}

	thread, err := c.GetThreadByNumber(ctx, repo.ID, number)
	if err != nil {
		return searchMatch{}, "", err
	}
	if thread == nil {
		return searchMatch{}, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("thread %q not found", ref))
	}
	if kind != "" && thread.Kind != kind {
		return searchMatch{}, "", fmt.Errorf("thread %q is a %s, not a %s", ref, thread.Kind, kind)
	}

	m := searchMatch{
		Repo:      domain.RepoRef{Owner: repo.Owner, Repo: repo.Name},
		Kind:      thread.Kind,
		Number:    thread.Number,
		State:     thread.State,
		Title:     thread.Title,
		Body:      thread.Body,
		Author:    thread.Author,
		Labels:    thread.Labels,
		Assignees: thread.Assignees,
		Language:  repo.Language,
		Archived:  repo.Archived,
		Stars:     repo.Stars,
		Watchers:  repo.Watchers,
		Forks:     repo.Forks,
		UpdatedAt: thread.SourceUpdatedAt,
		Freshness: thread.SourceUpdatedAt,
		URL:       threadURL(domain.RepoRef{Owner: repo.Owner, Repo: repo.Name}, thread.Kind, thread.Number),
	}
	if thread.Kind == corpus.ThreadKindPullRequest {
		return m, "prs", nil
	}
	return m, "issues", nil
}

func (s *Service) resolveCodeLensTarget(ctx context.Context, c *corpus.Corpus, ref string) (searchMatch, string, error) {
	parts := strings.SplitN(ref, "/", 3)
	if len(parts) < 3 {
		return searchMatch{}, "", fmt.Errorf("invalid code reference %q: expected owner/repo/path", ref)
	}
	repoRef := domain.RepoRef{Owner: parts[0], Repo: parts[1]}
	path := parts[2]
	repo, err := c.GetRepository(ctx, repoRef.Owner, repoRef.Repo)
	if err != nil {
		return searchMatch{}, "", err
	}
	if repo == nil {
		return searchMatch{}, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %q not found", repoRef))
	}

	doc, err := c.GetCodeDocument(ctx, repoRef, path)
	if err != nil {
		return searchMatch{}, "", err
	}
	if doc == nil {
		return searchMatch{}, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("code document %q not found", ref))
	}

	return searchMatch{
		Repo:      repoRef,
		Kind:      "code",
		Title:     doc.Path,
		Body:      doc.Content,
		URL:       fmt.Sprintf("https://github.com/%s/blob/%s/%s", repoRef, doc.Commit, doc.Path),
		Language:  doc.Language,
		Archived:  repo.Archived,
		Stars:     repo.Stars,
		Watchers:  repo.Watchers,
		Forks:     repo.Forks,
		UpdatedAt: doc.SnapshotCreatedAt,
		Freshness: doc.SnapshotCreatedAt,
	}, "code", nil
}

func parseRepoRef(ref string) (domain.RepoRef, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return domain.RepoRef{}, fmt.Errorf("invalid repository reference %q", ref)
	}
	r := domain.RepoRef{Owner: parts[0], Repo: parts[1]}
	if err := r.Validate(); err != nil {
		return domain.RepoRef{}, fmt.Errorf("invalid repository reference %q: %w", ref, err)
	}
	return r, nil
}

func parseThreadRef(ref string) (domain.RepoRef, int, error) {
	parts := strings.Split(ref, "#")
	if len(parts) != 2 {
		return domain.RepoRef{}, 0, fmt.Errorf("invalid thread reference %q", ref)
	}
	repoRef, err := parseRepoRef(parts[0])
	if err != nil {
		return domain.RepoRef{}, 0, err
	}
	number, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || number <= 0 {
		return domain.RepoRef{}, 0, fmt.Errorf("invalid thread number in %q", ref)
	}
	return repoRef, number, nil
}

func buildLensExplainResult(record *corpus.LensRecord, found lens.Result, match searchMatch, populationSize int, scope, query string, now time.Time) *cli.LensExplainResult {
	result := &cli.LensExplainResult{
		Lens:            *lensResult(record),
		Query:           query,
		PopulationSize:  populationSize,
		PopulationScope: scope,
		EvaluatedAt:     now.Format(time.RFC3339),
		Score:           roundScore(found.Score),
		Signals:         make([]cli.LensExplainSignal, 0, len(record.Definition.Weights)),
		MissingSignals:  []string{},
	}

	result.Candidate = cli.LensExplainCandidate{
		Kind:      match.Kind,
		Repo:      cli.RepoRef{Owner: match.Repo.Owner, Repo: match.Repo.Repo},
		Number:    match.Number,
		Title:     match.Title,
		State:     match.State,
		URL:       match.URL,
		UpdatedAt: formatSearchTime(match.UpdatedAt),
	}

	var names []string
	for name := range record.Definition.Weights {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		weight := record.Definition.Weights[name]
		sig := cli.LensExplainSignal{
			Name:         name,
			Weight:       weight,
			Missing:      true,
			Value:        0,
			Normalized:   0,
			Contribution: 0,
		}
		if raw, ok := found.Candidate.Signals[name]; ok {
			sig.Value = raw
			sig.Missing = false
		}
		if norm, ok := found.Normalized[name]; ok {
			sig.Normalized = norm
		}
		if contrib, ok := found.Contributions[name]; ok {
			sig.Contribution = contrib
		}
		if sig.Missing {
			result.MissingSignals = append(result.MissingSignals, name)
		}
		result.Signals = append(result.Signals, sig)
	}
	return result
}

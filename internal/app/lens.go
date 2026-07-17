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
func (s *Service) ExplainLens(ctx context.Context, name, ref string) (*cli.LensExplainResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("lens name is required")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("result reference is required")
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
	def := lensRecord.Definition

	now := s.now()
	candidate, matches, scope, err := s.resolveLensExplainCandidate(ctx, c, ref, def, now)
	if err != nil {
		return nil, err
	}

	candidates := make([]lens.Candidate, 0, len(matches))
	byID := make(map[string]searchMatch, len(matches))
	for _, m := range matches {
		cand := candidateFromMatch(m, "", now)
		candidates = append(candidates, cand)
		byID[cand.ID] = m
	}

	results, err := lens.Rank(def, candidates, now)
	if err != nil {
		return nil, fmt.Errorf("rank with lens: %w", err)
	}

	var found *lens.Result
	for i := range results {
		if results[i].Candidate.ID == candidate.ID {
			found = &results[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("result %q does not match lens %q filters", ref, name)
	}

	return buildLensExplainResult(lensRecord, *found, len(results), scope, now), nil
}

func (s *Service) resolveLensExplainCandidate(ctx context.Context, c *corpus.Corpus, ref string, def lens.Definition, now time.Time) (lens.Candidate, []searchMatch, string, error) {
	prefix, rest := splitLensRef(ref)
	switch prefix {
	case "repo":
		return s.explainRepoCandidate(ctx, c, rest, def, now)
	case "issue":
		return s.explainThreadCandidate(ctx, c, rest, corpus.ThreadKindIssue, def, now)
	case "pr", "pull_request":
		return s.explainThreadCandidate(ctx, c, rest, corpus.ThreadKindPullRequest, def, now)
	case "code":
		return s.explainCodeCandidate(ctx, c, rest, def, now)
	case "":
		if strings.Contains(ref, "#") {
			return s.explainThreadCandidate(ctx, c, ref, "", def, now)
		}
		return s.explainRepoCandidate(ctx, c, ref, def, now)
	default:
		return lens.Candidate{}, nil, "", fmt.Errorf("unsupported result reference %q", ref)
	}
}

func splitLensRef(ref string) (string, string) {
	idx := strings.IndexByte(ref, ':')
	if idx < 0 {
		return "", ref
	}
	return ref[:idx], ref[idx+1:]
}

func (s *Service) explainRepoCandidate(ctx context.Context, c *corpus.Corpus, ref string, def lens.Definition, now time.Time) (lens.Candidate, []searchMatch, string, error) {
	repoRef, err := parseRepoRef(ref)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	repo, err := c.GetRepository(ctx, repoRef.Owner, repoRef.Repo)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	if repo == nil {
		return lens.Candidate{}, nil, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %q not found", repoRef))
	}

	m := searchMatch{
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
	}
	candidate := candidateFromMatch(m, "", now)

	all, err := c.ListRepositoriesWithOptions(ctx, "", corpus.RepositorySearchOptions{Limit: maxLensCandidates})
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	var matches []searchMatch
	for _, r := range all.Repositories {
		matches = append(matches, searchMatch{
			Repo:      domain.RepoRef{Owner: r.Owner, Repo: r.Name},
			Kind:      "repo",
			Title:     domain.RepoRef{Owner: r.Owner, Repo: r.Name}.String(),
			Body:      r.Description,
			URL:       fmt.Sprintf("https://github.com/%s/%s", r.Owner, r.Name),
			Language:  r.Language,
			Archived:  r.Archived,
			Stars:     r.Stars,
			Watchers:  r.Watchers,
			Forks:     r.Forks,
			UpdatedAt: r.SourceUpdatedAt,
			Freshness: r.SourceUpdatedAt,
		})
	}
	return candidate, matches, "all repositories", nil
}

func (s *Service) explainThreadCandidate(ctx context.Context, c *corpus.Corpus, ref, kind string, def lens.Definition, now time.Time) (lens.Candidate, []searchMatch, string, error) {
	repoRef, number, err := parseThreadRef(ref)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	repo, err := c.GetRepository(ctx, repoRef.Owner, repoRef.Repo)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	if repo == nil {
		return lens.Candidate{}, nil, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %q not found", repoRef))
	}

	thread, err := c.GetThreadByNumber(ctx, repo.ID, number)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	if thread == nil {
		return lens.Candidate{}, nil, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("thread %q not found", ref))
	}
	if kind != "" && thread.Kind != kind {
		return lens.Candidate{}, nil, "", fmt.Errorf("thread %q is a %s, not a %s", ref, thread.Kind, kind)
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
	candidate := candidateFromMatch(m, "", now)

	kindFilter := ""
	if len(def.Filter.Kinds) == 1 {
		kindFilter = def.Filter.Kinds[0]
	}
	threads, err := c.ListThreads(ctx, repo.ID, kindFilter, 10000)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	var matches []searchMatch
	for _, t := range threads {
		matches = append(matches, searchMatch{
			Repo:      domain.RepoRef{Owner: repo.Owner, Repo: repo.Name},
			Kind:      t.Kind,
			Number:    t.Number,
			State:     t.State,
			Title:     t.Title,
			Body:      t.Body,
			Author:    t.Author,
			Labels:    t.Labels,
			Assignees: t.Assignees,
			Language:  repo.Language,
			Archived:  repo.Archived,
			Stars:     repo.Stars,
			Watchers:  repo.Watchers,
			Forks:     repo.Forks,
			UpdatedAt: t.SourceUpdatedAt,
			Freshness: t.SourceUpdatedAt,
			URL:       threadURL(domain.RepoRef{Owner: repo.Owner, Repo: repo.Name}, t.Kind, t.Number),
		})
	}
	return candidate, matches, fmt.Sprintf("threads in %s", repoRef), nil
}

func (s *Service) explainCodeCandidate(ctx context.Context, c *corpus.Corpus, ref string, def lens.Definition, now time.Time) (lens.Candidate, []searchMatch, string, error) {
	parts := strings.SplitN(ref, "/", 3)
	if len(parts) < 3 {
		return lens.Candidate{}, nil, "", fmt.Errorf("invalid code reference %q: expected owner/repo/path", ref)
	}
	repoRef := domain.RepoRef{Owner: parts[0], Repo: parts[1]}
	path := parts[2]
	repo, err := c.GetRepository(ctx, repoRef.Owner, repoRef.Repo)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	if repo == nil {
		return lens.Candidate{}, nil, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("repository %q not found", repoRef))
	}

	doc, err := c.GetCodeDocument(ctx, repoRef, path)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	if doc == nil {
		return lens.Candidate{}, nil, "", cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("code document %q not found", ref))
	}

	m := searchMatch{
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
	}
	candidate := candidateFromMatch(m, "", now)

	docs, err := c.ListCodeDocuments(ctx, repoRef)
	if err != nil {
		return lens.Candidate{}, nil, "", err
	}
	var matches []searchMatch
	for _, d := range docs {
		matches = append(matches, searchMatch{
			Repo:      repoRef,
			Kind:      "code",
			Title:     d.Path,
			Body:      d.Content,
			URL:       fmt.Sprintf("https://github.com/%s/blob/%s/%s", repoRef, d.Commit, d.Path),
			Language:  d.Language,
			Archived:  repo.Archived,
			Stars:     repo.Stars,
			Watchers:  repo.Watchers,
			Forks:     repo.Forks,
			UpdatedAt: d.SnapshotCreatedAt,
			Freshness: d.SnapshotCreatedAt,
		})
	}
	return candidate, matches, fmt.Sprintf("code documents in %s", repoRef), nil
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

func buildLensExplainResult(record *corpus.LensRecord, found lens.Result, populationSize int, scope string, now time.Time) *cli.LensExplainResult {
	m := searchMatch{
		Repo:      domain.RepoRef{Owner: strings.Split(found.Candidate.Repository, "/")[0], Repo: strings.Split(found.Candidate.Repository, "/")[1]},
		Kind:      found.Candidate.Kind,
		Number:    0,
		State:     found.Candidate.State,
		Title:     "",
		Body:      "",
		UpdatedAt: found.Candidate.UpdatedAt,
	}
	if idx := strings.IndexByte(found.Candidate.ID, '#'); idx >= 0 {
		if n, err := strconv.Atoi(found.Candidate.ID[idx+1:]); err == nil {
			m.Number = n
		}
	} else if idx := strings.LastIndexByte(found.Candidate.ID, '/'); idx >= 0 {
		m.Title = found.Candidate.ID[idx+1:]
	}

	result := &cli.LensExplainResult{
		Lens:            *lensResult(record),
		PopulationSize:  populationSize,
		PopulationScope: scope,
		EvaluatedAt:     now.Format(time.RFC3339),
		Score:           roundScore(found.Score),
		Signals:         make([]cli.LensExplainSignal, 0, len(record.Definition.Weights)),
		MissingSignals:  []string{},
	}

	result.Candidate = cli.LensExplainCandidate{
		Kind:      found.Candidate.Kind,
		Repo:      cli.RepoRef{Owner: m.Repo.Owner, Repo: m.Repo.Repo},
		Number:    m.Number,
		Title:     m.Title,
		State:     found.Candidate.State,
		UpdatedAt: formatSearchTime(found.Candidate.UpdatedAt),
	}
	if found.Candidate.Kind == corpus.ThreadKindIssue || found.Candidate.Kind == corpus.ThreadKindPullRequest {
		result.Candidate.URL = threadURL(m.Repo, found.Candidate.Kind, m.Number)
		result.Candidate.Title = found.Candidate.ID
	}
	if found.Candidate.Kind == "code" {
		result.Candidate.URL = fmt.Sprintf("https://github.com/%s/blob/HEAD/%s", m.Repo, m.Title)
		result.Candidate.Title = m.Title
	}
	if found.Candidate.Kind == "repo" {
		result.Candidate.URL = fmt.Sprintf("https://github.com/%s", m.Repo)
		result.Candidate.Title = found.Candidate.Repository
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

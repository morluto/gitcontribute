package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/lens"
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
	Assignees []string
	Language  string
	Archived  bool
	Stars     int
	Watchers  int
	Forks     int
	UpdatedAt time.Time
	URL       string
	Score     float64
	Freshness time.Time
	Coverage  []string
	Fields    map[string]any
}

type searchResult struct {
	Query      string
	Total      int
	Matches    []searchMatch
	NextCursor string
}

// ExplainMatchResult exposes the factual signals that contribute to a match
// score. It performs no network access.
type ExplainMatchResult struct {
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
}

const maxLensCandidates = 1000

func (s *Service) searchCorpus(ctx context.Context, query string, opts cli.SearchOptions) (searchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		return searchResult{}, errors.New("search limit cannot exceed 100")
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return searchResult{}, err
	}

	now := s.now()

	if opts.Lens != "" {
		if opts.Cursor != "" {
			return searchResult{}, errors.New("cursor pagination cannot be combined with --lens because lens ranking is not cursor-stable")
		}
		return s.searchWithLens(ctx, c, query, opts, now)
	}

	repoID, repoRef, err := s.resolveRepoFilter(ctx, c, opts)
	if err != nil {
		return searchResult{}, err
	}

	switch opts.Kind {
	case "repos":
		if opts.Repo != "" {
			if opts.Cursor != "" {
				return searchResult{}, errors.New("cursor pagination is not supported for exact repository search")
			}
			ref, err := s.parseRepoRef(opts.Repo)
			if err != nil {
				return searchResult{}, err
			}
			return s.searchRepositoryExact(ctx, c, query, ref, now)
		}
		return s.searchRepositories(ctx, c, query, opts.Limit, opts.Cursor)
	case "code":
		ref, err := s.parseRepoRef(opts.Repo)
		if err != nil {
			return searchResult{}, err
		}
		return s.searchCode(ctx, c, query, ref, opts.Limit, opts.Cursor)
	case "all":
		if opts.Cursor != "" {
			return searchResult{}, errors.New("cursor pagination is not supported for combined search")
		}
		return s.searchAll(ctx, c, query, opts, now)
	default:
		kind := ""
		switch opts.Kind {
		case "issue", "issues":
			kind = corpus.ThreadKindIssue
		case "pr", "prs", "pull_request":
			kind = corpus.ThreadKindPullRequest
		case "threads", "":
			kind = ""
		default:
			return searchResult{}, fmt.Errorf("unsupported search kind %q", opts.Kind)
		}
		if repoRef != (domain.RepoRef{}) && repoID == 0 {
			return searchResult{Query: query, Total: 0, Matches: nil}, nil
		}
		if query == "" {
			return searchResult{Query: query, Total: 0, Matches: nil}, nil
		}
		return s.searchThreads(ctx, c, query, repoID, repoRef, kind, opts, now)
	}
}

func (s *Service) parseRepoRef(repo string) (domain.RepoRef, error) {
	if repo == "" {
		return domain.RepoRef{}, nil
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return domain.RepoRef{}, fmt.Errorf("invalid repository filter %q", repo)
	}
	ref := domain.RepoRef{Owner: parts[0], Repo: parts[1]}
	if err := ref.Validate(); err != nil {
		return domain.RepoRef{}, fmt.Errorf("invalid repository filter %q: %w", repo, err)
	}
	return ref, nil
}

func (s *Service) resolveRepoFilter(ctx context.Context, c *corpus.Corpus, opts cli.SearchOptions) (int64, domain.RepoRef, error) {
	if opts.Repo == "" || opts.Kind == "code" || opts.Kind == "all" || opts.Kind == "repos" {
		return 0, domain.RepoRef{}, nil
	}
	ref, err := s.parseRepoRef(opts.Repo)
	if err != nil {
		return 0, domain.RepoRef{}, err
	}
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return 0, domain.RepoRef{}, err
	}
	if repo == nil {
		return 0, ref, nil
	}
	return repo.ID, ref, nil
}

func (s *Service) searchThreads(ctx context.Context, c *corpus.Corpus, query string, repoID int64, ref domain.RepoRef, kind string, opts cli.SearchOptions, now time.Time) (searchResult, error) {
	filter := corpus.SearchFilter{
		RepoID: repoID, Repo: ref.String(), Kind: kind, State: opts.State, Author: opts.Author,
		Association: opts.Association, Assignee: opts.Assignee,
		Labels: opts.Labels, UpdatedAfter: opts.UpdatedAfter, Limit: opts.Limit, Cursor: opts.Cursor,
	}
	page, err := c.SearchThreadsPage(ctx, query, filter)
	if err != nil {
		return searchResult{}, fmt.Errorf("search threads: %w", err)
	}

	repoCache := make(map[int64]*corpus.Repository)
	repositoryCoverageCache := make(map[int64][]string)

	matches := make([]searchMatch, 0, len(page.Threads))
	for _, t := range page.Threads {
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
		repositoryCoverage, ok := repositoryCoverageCache[t.RepositoryID]
		if !ok {
			repositoryCoverage, err = s.coverageNames(ctx, c, repo.ID, nil)
			if err != nil {
				return searchResult{}, err
			}
			repositoryCoverageCache[t.RepositoryID] = repositoryCoverage
		}
		threadCoverage, err := s.coverageNames(ctx, c, repo.ID, &t.ID)
		if err != nil {
			return searchResult{}, err
		}
		coverage := mergeCoverageNames(repositoryCoverage, threadCoverage)

		ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Name}
		m := searchMatch{
			Repo:      ref,
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
			URL:       threadURL(ref, t.Kind, t.Number),
			Freshness: t.SourceUpdatedAt,
			Coverage:  coverage,
		}
		m.Score, _ = scoreMatch(query, m, m.Freshness, coverage, now)
		matches = append(matches, m)
	}

	return searchResult{
		Query:      query,
		Total:      page.Total,
		Matches:    matches,
		NextCursor: page.NextCursor,
	}, nil
}

func (s *Service) searchRepositories(ctx context.Context, c *corpus.Corpus, query string, limit int, cursor string) (searchResult, error) {
	page, err := c.ListRepositoriesWithOptions(ctx, query, corpus.RepositorySearchOptions{Limit: limit, Cursor: cursor})
	if err != nil {
		return searchResult{}, fmt.Errorf("list repositories: %w", err)
	}

	coverageCache := make(map[int64][]string)
	now := s.now()

	matches := make([]searchMatch, 0, len(page.Repositories))
	for _, r := range page.Repositories {
		coverage, ok := coverageCache[r.ID]
		if !ok {
			coverage, err = s.coverageNames(ctx, c, r.ID, nil)
			if err != nil {
				return searchResult{}, err
			}
			coverageCache[r.ID] = coverage
		}
		m := repositorySearchMatch(r, query, coverage, now)
		matches = append(matches, m)
	}

	return searchResult{
		Query:      query,
		Total:      page.Total,
		Matches:    matches,
		NextCursor: page.NextCursor,
	}, nil
}

func (s *Service) searchRepositoryExact(ctx context.Context, c *corpus.Corpus, query string, ref domain.RepoRef, now time.Time) (searchResult, error) {
	repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return searchResult{}, err
	}
	if repo == nil || !repositoryMatchesQuery(repo, query) {
		return searchResult{Query: query, Matches: []searchMatch{}}, nil
	}
	coverage, err := s.coverageNames(ctx, c, repo.ID, nil)
	if err != nil {
		return searchResult{}, err
	}
	return searchResult{Query: query, Total: 1, Matches: []searchMatch{repositorySearchMatch(*repo, query, coverage, now)}}, nil
}

func repositoryMatchesQuery(repo *corpus.Repository, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return query == "" || strings.Contains(strings.ToLower(repo.Owner+"/"+repo.Name), query) ||
		strings.Contains(strings.ToLower(repo.Description), query)
}

func repositorySearchMatch(r corpus.Repository, query string, coverage []string, now time.Time) searchMatch {
	ref := domain.RepoRef{Owner: r.Owner, Repo: r.Name}
	m := searchMatch{
		Repo: ref, Kind: "repo", Title: ref.String(), Body: r.Description,
		URL: fmt.Sprintf("https://github.com/%s", ref), Language: r.Language,
		Archived: r.Archived, Stars: r.Stars, Watchers: r.Watchers, Forks: r.Forks,
		UpdatedAt: r.SourceUpdatedAt, Freshness: r.SourceUpdatedAt, Coverage: coverage,
		Fields: map[string]any{
			"description": r.Description, "default_branch": r.DefaultBranch,
			"language": r.Language, "license": r.License, "topics": r.Topics,
			"stars": r.Stars, "watchers": r.Watchers, "forks": r.Forks,
			"open_issues": r.OpenIssues, "archived": r.Archived, "fork": r.Fork,
		},
	}
	m.Score, _ = scoreMatch(query, m, m.Freshness, coverage, now)
	return m
}

func (s *Service) searchCode(ctx context.Context, c *corpus.Corpus, query string, ref domain.RepoRef, limit int, cursor string) (searchResult, error) {
	page, err := c.SearchCodeWithOptions(ctx, query, corpus.CodeSearchOptions{Ref: ref, Limit: limit, Cursor: cursor})
	if err != nil {
		return searchResult{}, err
	}

	now := s.now()
	repoCache := make(map[domain.RepoRef]*corpus.Repository)
	matches := make([]searchMatch, 0, len(page.Matches))
	for _, match := range page.Matches {
		coverage := []string{"code"}
		m := searchMatch{
			Repo:      match.Repo,
			Kind:      "code",
			Title:     match.Path,
			Body:      match.Content,
			URL:       fmt.Sprintf("https://github.com/%s/blob/%s/%s", match.Repo, match.Commit, match.Path),
			UpdatedAt: match.SnapshotCreatedAt,
			Freshness: match.SnapshotCreatedAt,
			Coverage:  coverage,
			Language:  match.Language,
		}
		repo, ok := repoCache[match.Repo]
		if !ok {
			repo, err = c.GetRepository(ctx, match.Repo.Owner, match.Repo.Repo)
			if err != nil {
				return searchResult{}, err
			}
			repoCache[match.Repo] = repo
		}
		if repo != nil {
			m.Archived = repo.Archived
			m.Stars = repo.Stars
			m.Watchers = repo.Watchers
			m.Forks = repo.Forks
		}
		m.Score, _ = scoreMatch(query, m, m.Freshness, coverage, now)
		matches = append(matches, m)
	}

	return searchResult{
		Query:      query,
		Total:      page.Total,
		Matches:    matches,
		NextCursor: page.NextCursor,
	}, nil
}

func (s *Service) searchAll(ctx context.Context, c *corpus.Corpus, query string, opts cli.SearchOptions, now time.Time) (searchResult, error) {
	var combined []searchMatch
	total := 0
	for _, kind := range []string{"threads", "repos", "code"} {
		part := opts
		part.Kind = kind
		// Pull a bounded candidate pool per kind before applying the shared
		// cross-kind score. Each underlying search still enforces the hard 100
		// result limit and remains entirely local.
		part.Limit = 100
		result, err := s.searchCorpus(ctx, query, part)
		if err != nil {
			return searchResult{}, err
		}
		total += result.Total
		combined = append(combined, result.Matches...)
	}
	slices.SortStableFunc(combined, func(a, b searchMatch) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		if byRepo := strings.Compare(a.Repo.String(), b.Repo.String()); byRepo != 0 {
			return byRepo
		}
		if byKind := strings.Compare(a.Kind, b.Kind); byKind != 0 {
			return byKind
		}
		return strings.Compare(a.Title, b.Title)
	})
	if len(combined) > opts.Limit {
		combined = combined[:opts.Limit]
	}
	return searchResult{Query: query, Total: total, Matches: combined}, nil
}

func (s *Service) searchWithLens(ctx context.Context, c *corpus.Corpus, query string, opts cli.SearchOptions, now time.Time) (searchResult, error) {
	lensRecord, err := c.GetLens(ctx, opts.Lens)
	if err != nil {
		return searchResult{}, fmt.Errorf("load lens: %w", err)
	}
	if lensRecord == nil {
		return searchResult{}, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("lens %q not found", opts.Lens))
	}
	def := lensRecord.Definition
	matches, err := s.collectLensMatches(ctx, c, query, opts, now)
	if err != nil {
		return searchResult{}, err
	}

	candidates := make([]lens.Candidate, 0, len(matches))
	byID := make(map[string]searchMatch, len(matches))
	for _, m := range matches {
		cand := candidateFromMatch(m, query, now)
		candidates = append(candidates, cand)
		byID[cand.ID] = m
	}

	results, err := lens.Rank(def, candidates, now)
	if err != nil {
		return searchResult{}, fmt.Errorf("rank with lens: %w", err)
	}

	totalEligible := len(results)
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > len(results) {
		limit = len(results)
	}
	results = results[:limit]

	out := make([]searchMatch, 0, len(results))
	for _, r := range results {
		m, ok := byID[r.Candidate.ID]
		if !ok {
			continue
		}
		m.Score = roundScore(r.Score)
		out = append(out, m)
	}
	return searchResult{Query: query, Total: totalEligible, Matches: out, NextCursor: ""}, nil
}

func (s *Service) collectLensMatches(ctx context.Context, c *corpus.Corpus, query string, opts cli.SearchOptions, now time.Time) ([]searchMatch, error) {
	var err error
	var repoRef domain.RepoRef
	var repoID int64
	if opts.Repo != "" {
		ref, err := s.parseRepoRef(opts.Repo)
		if err != nil {
			return nil, err
		}
		repoRef = ref
		repo, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
		if err != nil {
			return nil, err
		}
		if repo == nil {
			return []searchMatch{}, nil
		}
		repoID = repo.ID
	}

	var matches []searchMatch
	switch opts.Kind {
	case "repos":
		if repoRef == (domain.RepoRef{}) {
			matches, err = s.collectRepositoryMatches(ctx, c, query, opts)
		} else {
			var result searchResult
			result, err = s.searchRepositoryExact(ctx, c, query, repoRef, now)
			matches = result.Matches
		}
	case "code":
		matches, err = s.collectCodeMatches(ctx, c, query, repoRef, opts)
	case "all":
		threadMatches, err := s.collectThreadMatches(ctx, c, query, repoID, repoRef, "", opts, now)
		if err != nil {
			return nil, err
		}
		var repoMatches []searchMatch
		if repoRef == (domain.RepoRef{}) {
			repoMatches, err = s.collectRepositoryMatches(ctx, c, query, opts)
		} else {
			var result searchResult
			result, err = s.searchRepositoryExact(ctx, c, query, repoRef, now)
			repoMatches = result.Matches
		}
		if err != nil {
			return nil, err
		}
		codeMatches, err := s.collectCodeMatches(ctx, c, query, repoRef, opts)
		if err != nil {
			return nil, err
		}
		matches = append(threadMatches, repoMatches...)
		matches = append(matches, codeMatches...)
	default:
		kind := threadKindFromSearchKind(opts.Kind)
		if kind == "" && opts.Kind != "" && opts.Kind != "threads" {
			return nil, fmt.Errorf("unsupported search kind %q", opts.Kind)
		}
		matches, err = s.collectThreadMatches(ctx, c, query, repoID, repoRef, kind, opts, now)
	}
	if err != nil {
		return nil, err
	}
	if repoRef != (domain.RepoRef{}) {
		filtered := matches[:0]
		for _, match := range matches {
			if match.Repo == repoRef {
				filtered = append(filtered, match)
			}
		}
		matches = filtered
	}

	return matches, nil
}

func threadKindFromSearchKind(kind string) string {
	switch kind {
	case "issue", "issues":
		return corpus.ThreadKindIssue
	case "pr", "prs", "pull_request":
		return corpus.ThreadKindPullRequest
	case "threads", "":
		return ""
	default:
		return ""
	}
}

func (s *Service) collectThreadMatches(ctx context.Context, c *corpus.Corpus, query string, repoID int64, ref domain.RepoRef, kind string, opts cli.SearchOptions, now time.Time) ([]searchMatch, error) {
	var out []searchMatch
	cursor := ""
	for len(out) < maxLensCandidates {
		collectOpts := opts
		collectOpts.Limit = 100
		collectOpts.Cursor = cursor
		collectOpts.Lens = ""
		res, err := s.searchThreads(ctx, c, query, repoID, ref, kind, collectOpts, now)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Matches...)
		if res.NextCursor == "" || len(res.Matches) == 0 {
			break
		}
		cursor = res.NextCursor
	}
	if len(out) > maxLensCandidates {
		out = out[:maxLensCandidates]
	}
	return out, nil
}

func (s *Service) collectRepositoryMatches(ctx context.Context, c *corpus.Corpus, query string, opts cli.SearchOptions) ([]searchMatch, error) {
	var out []searchMatch
	cursor := ""
	for len(out) < maxLensCandidates {
		res, err := s.searchRepositories(ctx, c, query, 100, cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Matches...)
		if res.NextCursor == "" || len(res.Matches) == 0 {
			break
		}
		cursor = res.NextCursor
	}
	if len(out) > maxLensCandidates {
		out = out[:maxLensCandidates]
	}
	return out, nil
}

func (s *Service) collectCodeMatches(ctx context.Context, c *corpus.Corpus, query string, ref domain.RepoRef, opts cli.SearchOptions) ([]searchMatch, error) {
	var out []searchMatch
	cursor := ""
	for len(out) < maxLensCandidates {
		res, err := s.searchCode(ctx, c, query, ref, 100, cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Matches...)
		if res.NextCursor == "" || len(res.Matches) == 0 {
			break
		}
		cursor = res.NextCursor
	}
	if len(out) > maxLensCandidates {
		out = out[:maxLensCandidates]
	}
	return out, nil
}

func candidateFromMatch(m searchMatch, query string, now time.Time) lens.Candidate {
	id := m.Repo.String()
	switch m.Kind {
	case corpus.ThreadKindIssue, corpus.ThreadKindPullRequest:
		id = fmt.Sprintf("%s#%d", m.Repo, m.Number)
	case "code":
		id = fmt.Sprintf("%s/%s", m.Repo, m.Title)
	}

	cand := lens.Candidate{
		ID:         id,
		Repository: m.Repo.String(),
		Kind:       m.Kind,
		State:      m.State,
		Language:   m.Language,
		Archived:   m.Archived,
		Stars:      m.Stars,
		UpdatedAt:  m.UpdatedAt,
	}
	if m.Kind == corpus.ThreadKindIssue || m.Kind == corpus.ThreadKindPullRequest {
		cand.Assigned = len(m.Assignees) > 0
	}
	cand.Signals = candidateSignals(m, query, now)
	return cand
}

func candidateSignals(m searchMatch, query string, now time.Time) map[string]float64 {
	signals := map[string]float64{
		"text_relevance":      textRelevance(query, m.Title, m.Body),
		"repository_activity": float64(m.Stars + m.Watchers + m.Forks),
	}
	if !m.UpdatedAt.IsZero() && !now.IsZero() {
		signals["freshness"] = freshnessSignal(m.UpdatedAt, now)
	}
	return signals
}

func textRelevance(query, title, body string) float64 {
	terms := uniqueTerms(strings.ToLower(query))
	if len(terms) == 0 {
		return 0
	}
	titleL := strings.ToLower(title)
	bodyL := strings.ToLower(body)

	var score float64
	matched := 0
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(titleL, term) {
			score += 0.25
			matched++
		} else if strings.Contains(bodyL, term) {
			score += 0.10
			matched++
		}
	}
	if matched == len(terms) {
		score += 0.15
	}
	if score > 1 {
		score = 1
	}
	return score
}

func freshnessSignal(updatedAt, now time.Time) float64 {
	if updatedAt.IsZero() || now.IsZero() {
		return 0
	}
	age := now.Sub(updatedAt)
	if age < 0 {
		age = 0
	}
	days := age.Hours() / 24
	score := 1.0 / (1.0 + days/30.0)
	if score > 1 {
		score = 1
	}
	return score
}

func (s *Service) coverageNames(ctx context.Context, c *corpus.Corpus, repoID int64, threadID *int64) ([]string, error) {
	coverage, err := c.ListCoverage(ctx, repoID, threadID)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, cov := range coverage {
		if cov.Complete {
			names = append(names, cov.Facet)
		}
	}
	return names, nil
}

func mergeCoverageNames(groups ...[]string) []string {
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, name := range group {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func boundedText(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}

func threadURL(ref domain.RepoRef, kind string, number int) string {
	path := "issues"
	if kind == corpus.ThreadKindPullRequest {
		path = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%d", ref, path, number)
}

// Search performs a local-only corpus search and supports repo and kind filters.
func (s *Service) Search(ctx context.Context, query string, opts cli.SearchOptions) (*cli.SearchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		return nil, errors.New("search limit cannot exceed 100")
	}
	res, err := s.searchCorpus(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	matches := make([]cli.SearchMatch, len(res.Matches))
	for i, m := range res.Matches {
		matches[i] = cli.SearchMatch{
			Kind:      m.Kind,
			Repo:      cli.RepoRef{Owner: m.Repo.Owner, Repo: m.Repo.Repo},
			Title:     m.Title,
			Number:    m.Number,
			State:     m.State,
			Author:    m.Author,
			Labels:    m.Labels,
			URL:       m.URL,
			Score:     roundScore(m.Score),
			Body:      m.Body,
			Freshness: formatSearchTime(m.Freshness),
			Coverage:  m.Coverage,
		}
	}
	return &cli.SearchResult{
		Query:      query,
		Kind:       opts.Kind,
		Repo:       opts.Repo,
		Limit:      opts.Limit,
		Total:      res.Total,
		Matches:    matches,
		NextCursor: res.NextCursor,
	}, nil
}

// ExplainMatch returns factual score reasons for a search match without network
// access. The returned reasons describe title/body term matches, source
// freshness, and coverage signals.
func (s *Service) ExplainMatch(ctx context.Context, query string, match cli.SearchMatch) (*ExplainMatchResult, error) {
	var freshness time.Time
	if match.Freshness != "" {
		var err error
		freshness, err = time.Parse(time.RFC3339, match.Freshness)
		if err != nil {
			return nil, fmt.Errorf("parse freshness: %w", err)
		}
	}
	m := searchMatch{
		Title: match.Title,
		Body:  match.Body,
		Kind:  match.Kind,
	}
	score, reasons := scoreMatch(query, m, freshness, match.Coverage, s.now())
	return &ExplainMatchResult{Score: roundScore(score), Reasons: reasons}, nil
}

func scoreMatch(query string, m searchMatch, freshness time.Time, coverage []string, now time.Time) (float64, []string) {
	terms := uniqueTerms(strings.ToLower(query))
	title := strings.ToLower(m.Title)
	body := strings.ToLower(m.Body)

	var score float64
	var reasons []string
	matched := 0
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(title, term) {
			score += 0.25
			reasons = append(reasons, fmt.Sprintf("query term %q matched in title", term))
			matched++
		} else if strings.Contains(body, term) {
			score += 0.10
			reasons = append(reasons, fmt.Sprintf("query term %q matched in body", term))
			matched++
		}
	}
	if matched == len(terms) && len(terms) > 0 {
		score += 0.15
		reasons = append(reasons, "all query terms matched")
	}

	if !freshness.IsZero() && !now.IsZero() {
		age := now.Sub(freshness)
		if age < 0 {
			age = 0
		}
		days := age.Hours() / 24
		freshnessScore := 1.0 / (1.0 + days/30.0)
		if freshnessScore > 1 {
			freshnessScore = 1
		}
		score += freshnessScore * 0.20
		reasons = append(reasons, fmt.Sprintf("source updated %s ago at %s", humanDuration(age), freshness.Format(time.RFC3339)))
	}

	if len(coverage) > 0 {
		covScore := float64(len(coverage)) * 0.05
		if covScore > 0.2 {
			covScore = 0.2
		}
		score += covScore
		reasons = append(reasons, fmt.Sprintf("coverage includes %s", strings.Join(coverage, ", ")))
	} else {
		reasons = append(reasons, "no coverage recorded")
	}

	if score > 1 {
		score = 1
	}
	return roundScore(score), reasons
}

func uniqueTerms(query string) []string {
	fields := strings.Fields(query)
	seen := make(map[string]struct{}, len(fields))
	terms := make([]string, 0, len(fields))
	for _, t := range fields {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		terms = append(terms, t)
	}
	return terms
}

func humanDuration(d time.Duration) string {
	if d < time.Hour*24 {
		return "less than a day"
	}
	days := int(d.Hours() / 24)
	if days < 30 {
		return fmt.Sprintf("%d days", days)
	}
	months := days / 30
	if months < 12 {
		return fmt.Sprintf("%d months", months)
	}
	years := months / 12
	months = months % 12
	if months == 0 {
		return fmt.Sprintf("%d years", years)
	}
	return fmt.Sprintf("%d years, %d months", years, months)
}

func roundScore(score float64) float64 {
	return math.Round(score*100) / 100
}

func formatSearchTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/lens"
)

type searchMatchKind uint8

const (
	searchRepositoryMatch searchMatchKind = iota + 1
	searchIssueMatch
	searchPullRequestMatch
	searchCodeMatch
)

func searchMatchKindForThread(kind domain.ThreadKind) (searchMatchKind, error) {
	switch kind {
	case domain.IssueKind:
		return searchIssueMatch, nil
	case domain.PullRequestKind:
		return searchPullRequestMatch, nil
	default:
		return 0, fmt.Errorf("unsupported stored thread kind %q", kind)
	}
}

func (k searchMatchKind) String() string {
	switch k {
	case searchRepositoryMatch:
		return "repo"
	case searchIssueMatch:
		return string(domain.IssueKind)
	case searchPullRequestMatch:
		return string(domain.PullRequestKind)
	case searchCodeMatch:
		return "code"
	default:
		return ""
	}
}

type searchMatch struct {
	Repo              domain.RepoRef
	Kind              searchMatchKind
	Number            int
	State             string
	StateReason       string
	Title             string
	Body              string
	Author            string
	AuthorAssociation string
	Labels            []string
	Assignees         []string
	Draft             bool
	ClosedAt          time.Time
	Merge             domain.MergeStatus
	Language          string
	Archived          bool
	Stars             int
	Watchers          int
	Forks             int
	UpdatedAt         time.Time
	URL               string
	Score             float64
	Freshness         time.Time
	Coverage          []string
	MatchSource       string
	MatchExcerpt      string
	MatchTruncated    bool
}

type searchResult struct {
	Query                string
	Total                int
	Matches              []searchMatch
	NextCursor           string
	UnknownMergeCount    int
	SnapshotToken        string
	ObservationWatermark int64
}

const maxLensCandidates = 1000

func (s *Service) searchCorpus(ctx context.Context, request parsedSearchRequest) (searchResult, error) {
	read := request.read()
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return searchResult{}, err
	}
	revision, err := beginCorpusRead(ctx, c, read.snapshotToken)
	if err != nil {
		return searchResult{}, err
	}
	now := s.now()

	var result searchResult
	switch parsed := request.(type) {
	case repositorySearchRequest:
		if parsed.repo.IsValid() {
			result, err = s.searchRepositoryExact(ctx, c, read.query, parsed.repo)
		} else {
			result, err = s.searchRepositories(ctx, c, read.query, read.page, parsed.order)
		}
	case codeSearchRequest:
		result, err = s.searchCode(ctx, c, read.query, parsed.repo, read.page)
	case threadSearchRequest:
		var scope corpus.ThreadRepositoryScope
		var found bool
		scope, found, err = s.resolveSearchRepository(ctx, c, parsed.repo)
		if err != nil {
			return searchResult{}, err
		}
		if !found || read.query == "" {
			result = searchResult{Query: read.query, Matches: nil}
		} else {
			result, err = s.searchThreads(ctx, c, read.query, scope, parsed.criteria, read.page)
		}
	case lensSearchRequest:
		result, err = s.searchWithLens(ctx, c, parsed, now)
	default:
		return searchResult{}, errors.New("invalid parsed search request")
	}
	if err != nil {
		return searchResult{}, err
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return searchResult{}, err
	}
	result.SnapshotToken = snapshotIdentity(read.snapshotToken, revision)
	result.ObservationWatermark = revision
	return result, nil
}

func (s *Service) resolveSearchRepository(ctx context.Context, c *corpus.Corpus, ref domain.RepoRef) (corpus.ThreadRepositoryScope, bool, error) {
	if !ref.IsValid() {
		return corpus.AllThreadRepositories(), true, nil
	}
	repo, err := c.GetRepository(ctx, ref.Owner(), ref.Repo())
	if err != nil {
		return corpus.ThreadRepositoryScope{}, false, err
	}
	if repo == nil {
		return corpus.ThreadRepositoryScope{}, false, nil
	}
	scope, err := corpus.NewThreadRepositoryScope(ref, repo.ID)
	if err != nil {
		return corpus.ThreadRepositoryScope{}, false, err
	}
	return scope, true, nil
}

func (s *Service) searchThreads(ctx context.Context, c *corpus.Corpus, query string, scope corpus.ThreadRepositoryScope, criteria threadSearchCriteria, pageRequest corpus.SearchPage) (searchResult, error) {
	filter := corpus.SearchFilter{
		Repository: scope, Kind: criteria.kind.corpusThreadKind(), State: criteria.state, StateReason: criteria.stateReason, Merge: criteria.merge, Author: criteria.author,
		Association: criteria.association, Assignee: criteria.assignee,
		Labels: criteria.labels, UpdatedAfter: criteria.updatedAfter, UpdatedBefore: criteria.updatedBefore, Page: pageRequest,
		Order: criteria.order, TermMatch: criteria.match,
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

		ref, err := domain.NewRepoRef(repo.Owner, repo.Name)
		if err != nil {
			return searchResult{}, fmt.Errorf("parse stored repository: %w", err)
		}
		kind, err := searchMatchKindForThread(t.Kind)
		if err != nil {
			return searchResult{}, err
		}
		m := searchMatch{
			Repo:              ref,
			Kind:              kind,
			Number:            t.Number,
			State:             string(t.State),
			StateReason:       t.StateReason,
			Title:             t.Title,
			Body:              t.Body,
			Author:            t.Author,
			AuthorAssociation: t.AuthorAssociation,
			Labels:            t.Labels,
			Assignees:         t.Assignees,
			Draft:             t.Draft,
			ClosedAt:          t.ClosedAt,
			Merge:             t.Merge,
			Language:          repo.Language,
			Archived:          repo.Archived,
			Stars:             repo.Stars,
			Watchers:          repo.Watchers,
			Forks:             repo.Forks,
			UpdatedAt:         t.SourceUpdatedAt,
			URL:               threadURL(ref, t.Kind, t.Number),
			Freshness:         t.SourceUpdatedAt,
			Coverage:          coverage,
		}
		m.MatchSource = t.MatchSource
		m.MatchExcerpt = t.MatchExcerpt
		m.MatchTruncated = t.MatchTruncated
		if !t.MatchUpdatedAt.IsZero() {
			m.Freshness = t.MatchUpdatedAt
		}
		m.Score = bm25Score(t.Rank)
		matches = append(matches, m)
	}

	return searchResult{
		Query:             query,
		Total:             page.Total,
		Matches:           matches,
		NextCursor:        page.NextCursor,
		UnknownMergeCount: page.UnknownMergeCount,
	}, nil
}

func (s *Service) searchRepositories(ctx context.Context, c *corpus.Corpus, query string, pageRequest corpus.SearchPage, order corpus.SearchOrder) (searchResult, error) {
	page, err := c.ListRepositoriesWithOptions(ctx, query, corpus.RepositorySearchOptions{Page: pageRequest, Order: order})
	if err != nil {
		return searchResult{}, fmt.Errorf("list repositories: %w", err)
	}

	coverageCache := make(map[int64][]string)
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
		m, err := repositorySearchMatch(r, coverage)
		if err != nil {
			return searchResult{}, err
		}
		matches = append(matches, m)
	}

	return searchResult{
		Query:      query,
		Total:      page.Total,
		Matches:    matches,
		NextCursor: page.NextCursor,
	}, nil
}

func (s *Service) searchRepositoryExact(ctx context.Context, c *corpus.Corpus, query string, ref domain.RepoRef) (searchResult, error) {
	repo, err := c.GetRepository(ctx, ref.Owner(), ref.Repo())
	if err != nil {
		return searchResult{}, err
	}
	if repo == nil {
		return searchResult{Query: query, Matches: []searchMatch{}}, nil
	}
	hasQuery := strings.TrimSpace(query) != ""
	var rank float64
	if hasQuery {
		evidence, found, err := c.FindRepositorySearchEvidence(ctx, repo.ID, query)
		if err != nil {
			return searchResult{}, err
		}
		if !found {
			return searchResult{Query: query, Matches: []searchMatch{}}, nil
		}
		rank = evidence.Rank
	}
	coverage, err := s.coverageNames(ctx, c, repo.ID, nil)
	if err != nil {
		return searchResult{}, err
	}
	match, err := repositorySearchMatch(*repo, coverage)
	if err != nil {
		return searchResult{}, err
	}
	if hasQuery {
		match.Score = bm25Score(rank)
	}
	return searchResult{Query: query, Total: 1, Matches: []searchMatch{match}}, nil
}

func repositorySearchMatch(r corpus.Repository, coverage []string) (searchMatch, error) {
	ref, err := domain.NewRepoRef(r.Owner, r.Name)
	if err != nil {
		return searchMatch{}, fmt.Errorf("parse stored repository: %w", err)
	}
	m := searchMatch{
		Repo: ref, Kind: searchRepositoryMatch, Title: ref.String(), Body: r.Description,
		URL: fmt.Sprintf("https://github.com/%s", ref), Language: r.Language,
		Archived: r.Archived, Stars: r.Stars, Watchers: r.Watchers, Forks: r.Forks,
		UpdatedAt: r.SourceUpdatedAt, Freshness: r.SourceUpdatedAt, Coverage: coverage,
	}
	m.Score = bm25Score(r.Rank)
	return m, nil
}

func (s *Service) searchCode(ctx context.Context, c *corpus.Corpus, query string, ref domain.RepoRef, pageRequest corpus.SearchPage) (searchResult, error) {
	page, err := c.SearchCodeWithOptions(ctx, query, corpus.CodeSearchOptions{Ref: ref, Page: pageRequest})
	if err != nil {
		return searchResult{}, err
	}

	repoCache := make(map[domain.RepoRef]*corpus.Repository)
	matches := make([]searchMatch, 0, len(page.Matches))
	for _, match := range page.Matches {
		coverage := []string{"code"}
		m := searchMatch{
			Repo:      match.Repo,
			Kind:      searchCodeMatch,
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
			repo, err = c.GetRepository(ctx, match.Repo.Owner(), match.Repo.Repo())
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
		m.Score = bm25Score(match.Rank)
		matches = append(matches, m)
	}

	return searchResult{
		Query:      query,
		Total:      page.Total,
		Matches:    matches,
		NextCursor: page.NextCursor,
	}, nil
}

func (s *Service) searchWithLens(ctx context.Context, c *corpus.Corpus, request lensSearchRequest, now time.Time) (searchResult, error) {
	read := request.read()
	lensRecord, err := c.GetLens(ctx, request.lens)
	if err != nil {
		return searchResult{}, fmt.Errorf("load lens: %w", err)
	}
	if lensRecord == nil {
		return searchResult{}, failure.NotFound(fmt.Errorf("lens %q not found", request.lens))
	}
	def := lensRecord.Definition
	matches, err := s.collectLensMatches(ctx, c, read.query, request.selection)
	if err != nil {
		return searchResult{}, err
	}

	candidates := make([]lens.Candidate, 0, len(matches))
	byID := make(map[string]searchMatch, len(matches))
	for _, m := range matches {
		cand := candidateFromMatch(m, now)
		candidates = append(candidates, cand)
		byID[cand.ID] = m
	}

	results, err := lens.Rank(def, candidates, now)
	if err != nil {
		return searchResult{}, fmt.Errorf("rank with lens: %w", err)
	}

	totalEligible := len(results)
	limit := read.page.Limit()
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
	return searchResult{Query: read.query, Total: totalEligible, Matches: out, NextCursor: ""}, nil
}

func (s *Service) collectLensMatches(ctx context.Context, c *corpus.Corpus, query string, selection lensSearchSelection) ([]searchMatch, error) {
	repoRef := selection.repository()
	scope, found, err := s.resolveSearchRepository(ctx, c, repoRef)
	if err != nil {
		return nil, err
	}
	if !found {
		return []searchMatch{}, nil
	}

	switch selected := selection.(type) {
	case repositoryLensSelection:
		if repoRef.IsValid() {
			result, err := s.searchRepositoryExact(ctx, c, query, repoRef)
			return result.Matches, err
		}
		return s.collectRepositoryMatches(ctx, c, query)
	case codeLensSelection:
		return s.collectCodeMatches(ctx, c, query, repoRef)
	case threadLensSelection:
		return s.collectThreadMatches(ctx, c, query, scope, selected.criteria)
	case allLensSelection:
		threadMatches, err := s.collectThreadMatches(ctx, c, query, scope, selected.criteria)
		if err != nil {
			return nil, err
		}
		var repoMatches []searchMatch
		if repoRef.IsValid() {
			result, searchErr := s.searchRepositoryExact(ctx, c, query, repoRef)
			if searchErr != nil {
				return nil, searchErr
			}
			repoMatches = result.Matches
		} else {
			repoMatches, err = s.collectRepositoryMatches(ctx, c, query)
			if err != nil {
				return nil, err
			}
		}
		codeMatches, err := s.collectCodeMatches(ctx, c, query, repoRef)
		if err != nil {
			return nil, err
		}
		matches := append(threadMatches, repoMatches...)
		return append(matches, codeMatches...), nil
	default:
		return nil, errors.New("invalid parsed lens selection")
	}
}

func (s *Service) collectThreadMatches(ctx context.Context, c *corpus.Corpus, query string, scope corpus.ThreadRepositoryScope, criteria threadSearchCriteria) ([]searchMatch, error) {
	var out []searchMatch
	cursor := ""
	for len(out) < maxLensCandidates {
		res, err := s.searchThreads(ctx, c, query, scope, criteria, corpus.MaximumSearchPage().WithCursor(cursor))
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

func (s *Service) collectRepositoryMatches(ctx context.Context, c *corpus.Corpus, query string) ([]searchMatch, error) {
	var out []searchMatch
	cursor := ""
	for len(out) < maxLensCandidates {
		res, err := s.searchRepositories(ctx, c, query, corpus.MaximumSearchPage().WithCursor(cursor), corpus.RelevanceSearchOrder())
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

func (s *Service) collectCodeMatches(ctx context.Context, c *corpus.Corpus, query string, ref domain.RepoRef) ([]searchMatch, error) {
	var out []searchMatch
	cursor := ""
	for len(out) < maxLensCandidates {
		res, err := s.searchCode(ctx, c, query, ref, corpus.MaximumSearchPage().WithCursor(cursor))
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

func candidateFromMatch(m searchMatch, now time.Time) lens.Candidate {
	id := m.Repo.String()
	switch m.Kind {
	case searchIssueMatch, searchPullRequestMatch:
		id = fmt.Sprintf("%s#%d", m.Repo, m.Number)
	case searchCodeMatch:
		id = fmt.Sprintf("%s/%s", m.Repo, m.Title)
	}

	cand := lens.Candidate{
		ID:         id,
		Repository: m.Repo.String(),
		Kind:       m.Kind.String(),
		State:      m.State,
		Language:   m.Language,
		Archived:   m.Archived,
		Stars:      m.Stars,
		UpdatedAt:  m.UpdatedAt,
	}
	if m.Kind == searchIssueMatch || m.Kind == searchPullRequestMatch {
		cand.Assigned = len(m.Assignees) > 0
	}
	cand.Signals = candidateSignals(m, now)
	return cand
}

func candidateSignals(m searchMatch, now time.Time) map[string]float64 {
	signals := map[string]float64{
		"text_relevance":      m.Score,
		"repository_activity": float64(m.Stars + m.Watchers + m.Forks),
	}
	if !m.UpdatedAt.IsZero() && !now.IsZero() {
		signals["freshness"] = freshnessSignal(m.UpdatedAt, now)
	}
	return signals
}

// SQLite FTS5 returns negative BM25 ranks where smaller values are better.
// Preserve their precision: the values are commonly below 1e-4, so display
// rounding would collapse distinct matches to zero.
func bm25Score(rank float64) float64 { return -rank }

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

func threadURL(ref domain.RepoRef, kind domain.ThreadKind, number int) string {
	path := "issues"
	if kind == domain.PullRequestKind {
		path = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%d", ref, path, number)
}

// Search performs a local-only corpus search and supports repo and kind filters.
func (s *Service) Search(ctx context.Context, query string, opts contracts.SearchOptions) (*contracts.SearchResult, error) {
	request, err := parseServiceSearchRequest(query, opts)
	if err != nil {
		return nil, err
	}
	res, err := s.searchCorpus(ctx, request)
	if err != nil {
		return nil, err
	}
	matches := make([]contracts.SearchMatch, len(res.Matches))
	for i, m := range res.Matches {
		matches[i] = contracts.SearchMatch{
			Kind:           m.Kind.String(),
			Repo:           contracts.RepoRef{Owner: m.Repo.Owner(), Repo: m.Repo.Repo()},
			Title:          m.Title,
			Number:         m.Number,
			State:          m.State,
			Author:         m.Author,
			Labels:         m.Labels,
			URL:            m.URL,
			Score:          m.Score,
			Body:           m.Body,
			Freshness:      formatSearchTime(m.Freshness),
			Coverage:       m.Coverage,
			MatchSource:    m.MatchSource,
			MatchExcerpt:   m.MatchExcerpt,
			MatchTruncated: m.MatchTruncated,
		}
	}
	return &contracts.SearchResult{
		Query:                request.read().query,
		Kind:                 request.kind().String(),
		Repo:                 request.repository().String(),
		Limit:                request.read().page.Limit(),
		Total:                res.Total,
		Matches:              matches,
		NextCursor:           res.NextCursor,
		UnknownMergeCount:    res.UnknownMergeCount,
		SnapshotToken:        res.SnapshotToken,
		ObservationWatermark: res.ObservationWatermark,
	}, nil
}

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/dossier"
)

const maxSeedLimit = 1000

var (
	conventionalCommitRe = regexp.MustCompile(`(?i)^\s*(feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert)(\([^)]+\))?(!)?:\s*`)
	issueRefRe           = regexp.MustCompile(`(?i)(?:^|\s)(?:close[ds]?|fix(?:es|ed)?|resolve[ds]?|relate[ds]?|refs?|references?)?\s*#(\d+)`)
	pathLikeRe           = regexp.MustCompile(`[a-zA-Z0-9_.-]+/[a-zA-Z0-9_./-]+`)
	rejectionRe          = regexp.MustCompile(`(?i)(superseded\s+(?:by)?|closing\s+in\s+favor\s+of|duplicate\s+of|won'?t\s+fix|not\s+planned|rejected|withdrawn)`)
	validationRe         = regexp.MustCompile(`(?i)\b(test|tests|testing|coverage|benchmark|ci|lint|checks?|validate|validation|verify)\b`)
)

var validationKeywords = map[string]struct{}{
	"test": {}, "tests": {}, "testing": {}, "coverage": {}, "benchmark": {},
	"ci": {}, "lint": {}, "check": {}, "checks": {}, "validate": {},
	"validation": {}, "verify": {},
}

var problemLabels = map[string]struct{}{
	"bug": {}, "crash": {}, "regression": {}, "panic": {}, "error": {},
	"flaky": {}, "broken": {}, "defect": {}, "won't fix": {}, "not planned": {},
}

var rejectionLabels = map[string]struct{}{
	"duplicate": {}, "wontfix": {}, "not planned": {}, "superseded": {},
	"invalid": {}, "rejected": {},
}

// BuildRepositoryDossier builds a deterministic dossier from local corpus data,
// persists it safely, and returns the result.
func (s *Service) BuildRepositoryDossier(ctx context.Context, repo cli.RepoRef) (*domain.Dossier, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}

	d, err := s.buildDossier(ctx, ref)
	if err != nil {
		return nil, err
	}

	repoProjection, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	if repoProjection == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}

	snapshot, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("marshal dossier snapshot: %w", err)
	}

	sectionMeta := domain.DossierSectionMetadata{
		RecentLimit:           dossier.DefaultRecentLimit,
		MergedPRCount:         len(d.RecentMergedPullRequests),
		OpenPRCount:           len(d.RecentOpenPullRequests),
		ClosedUnmergedPRCount: len(d.RecentClosedUnmergedPullRequests),
		ClosedUnknownPRCount:  len(d.RecentClosedUnknownPullRequests),
		IssueCount:            len(d.RecentIssues),
		SourceClasses:         []string{"merged_pr", "open_pr", "closed_unmerged_pr", "closed_unknown_merge_pr", "issue"},
	}
	sectionMetaJSON, err := json.Marshal(sectionMeta)
	if err != nil {
		return nil, fmt.Errorf("marshal section metadata: %w", err)
	}

	generatedAt := s.now()
	id, inserted, err := c.RefreshDossier(ctx, repoProjection.ID, ref.Owner, ref.Repo, d.CommitSHA, d.AsOf, string(sectionMetaJSON), string(snapshot), generatedAt, d.SourceRefs)
	if err != nil {
		return nil, fmt.Errorf("refresh dossier: %w", err)
	}
	if inserted {
		return d, nil
	}

	record, sources, err := c.GetDossier(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, fmt.Errorf("get dossier: %w", err)
	}
	if record == nil || record.ID != id {
		return nil, fmt.Errorf("dossier record missing after refresh: %s", ref)
	}
	return dossierFromRecord(record, sources)
}

// GetRepositoryDossier returns the most recently persisted dossier for a repository.
func (s *Service) GetRepositoryDossier(ctx context.Context, repo cli.RepoRef) (*domain.Dossier, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	record, sources, err := c.GetDossier(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, fmt.Errorf("get dossier: %w", err)
	}
	if record == nil {
		return nil, fmt.Errorf("dossier not found for %s", ref)
	}
	return dossierFromRecord(record, sources)
}

func dossierFromRecord(record *corpus.DossierRecord, sources []corpus.DossierSource) (*domain.Dossier, error) {
	var d domain.Dossier
	if err := json.Unmarshal([]byte(record.Snapshot), &d); err != nil {
		return nil, fmt.Errorf("unmarshal dossier snapshot: %w", err)
	}
	if record.CommitSHA != "" {
		d.CommitSHA = record.CommitSHA
	}
	d.SourceRefs = make([]domain.SourceRef, len(sources))
	for i, src := range sources {
		d.SourceRefs[i] = domain.SourceRef{
			Source:     src.Source,
			URL:        src.URL,
			CommitSHA:  src.CommitSHA,
			ObservedAt: src.ObservedAt,
			AsOf:       src.AsOf,
		}
	}
	return &d, nil
}

// ExtractSeeds derives evidence-backed contribution seeds from locally stored
// merged PRs, closed-unmerged PRs, and issues. It performs no network access.
func (s *Service) ExtractSeeds(ctx context.Context, repo cli.RepoRef, opts domain.ExtractSeedsOptions) ([]domain.Seed, error) {
	ref := domain.RepoRef{Owner: repo.Owner, Repo: repo.Repo}
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > maxSeedLimit {
		return nil, fmt.Errorf("seed limit cannot exceed %d", maxSeedLimit)
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}

	repoProjection, err := c.GetRepository(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	if repoProjection == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, ref)
	}

	threads, err := c.ListThreads(ctx, repoProjection.ID, "", 10000)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}

	selected := make(map[domain.SeedSourceClass]struct{})
	if len(opts.Classes) == 0 {
		selected[domain.SeedSourceClassMergedPR] = struct{}{}
		selected[domain.SeedSourceClassClosedUnmergedPR] = struct{}{}
		selected[domain.SeedSourceClassIssue] = struct{}{}
	} else {
		for _, class := range opts.Classes {
			selected[class] = struct{}{}
		}
	}

	var seeds []domain.Seed
	for _, t := range threads {
		class, ok := classForThread(t)
		if !ok {
			continue
		}
		if _, ok := selected[class]; !ok {
			continue
		}
		seed, err := buildSeed(ctx, c, t, class)
		if err != nil {
			return nil, err
		}
		seeds = append(seeds, seed)
	}

	sortSeeds(seeds)
	if len(seeds) > opts.Limit {
		seeds = seeds[:opts.Limit]
	}
	return seeds, nil
}

func classForThread(t corpus.Thread) (domain.SeedSourceClass, bool) {
	switch t.Kind {
	case corpus.ThreadKindPullRequest:
		if t.Merged {
			return domain.SeedSourceClassMergedPR, true
		}
		if t.State == "closed" && t.MergedKnown {
			return domain.SeedSourceClassClosedUnmergedPR, true
		}
		return "", false
	case corpus.ThreadKindIssue:
		return domain.SeedSourceClassIssue, true
	default:
		return "", false
	}
}

func buildSeed(ctx context.Context, c *corpus.Corpus, t corpus.Thread, class domain.SeedSourceClass) (domain.Seed, error) {
	seed := domain.Seed{
		SourceClass: class,
		Number:      t.Number,
		Title:       t.Title,
		Author:      t.Author,
		State:       t.State,
		Labels:      sortedCopy(t.Labels),
		CreatedAt:   t.SourceCreatedAt,
		UpdatedAt:   t.SourceUpdatedAt,
		ClosedAt:    t.ClosedAt,
		MergedAt:    t.MergedAt,
	}

	prPayload, err := latestPRPayload(ctx, c, t)
	if err != nil {
		return domain.Seed{}, err
	}
	seed.Evidence = extractEvidence(t, class, prPayload)
	return seed, nil
}

func latestPRPayload(ctx context.Context, c *corpus.Corpus, t corpus.Thread) (prPayloadFields, error) {
	var out prPayloadFields
	if t.Kind != corpus.ThreadKindPullRequest {
		return out, nil
	}
	obs, err := c.LatestThreadObservation(ctx, t.ID)
	if err != nil {
		return out, fmt.Errorf("latest thread observation: %w", err)
	}
	if obs == nil || obs.Payload == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(obs.Payload), &out); err != nil {
		return out, fmt.Errorf("parse PR observation payload: %w", err)
	}
	return out, nil
}

type prPayloadFields struct {
	ChangedFiles int `json:"ChangedFiles"`
	Additions    int `json:"Additions"`
	Deletions    int `json:"Deletions"`
}

func extractEvidence(t corpus.Thread, class domain.SeedSourceClass, pr prPayloadFields) domain.SeedEvidence {
	text := t.Title
	if t.Body != "" {
		text += " " + t.Body
	}

	ev := domain.SeedEvidence{
		TitleConvention:         extractTitleConvention(t.Title),
		IssueLinkages:           sortedUnique(extractIssueLinkages(text)),
		ValidationIndicators:    sortedUnique(extractValidationIndicators(text, t.Labels)),
		ApproximateScope:        extractApproximateScope(pr),
		ScopeEvidence:           scopeEvidence(pr),
		RejectionOrSupersession: extractRejectionContext(class, t.State, t.Title, t.Body, t.Labels),
		ProblemAreas:            sortedUnique(extractProblemAreas(t.Title, t.Body, t.Labels)),
	}
	return ev
}

func extractTitleConvention(title string) string {
	if m := conventionalCommitRe.FindStringSubmatch(title); m != nil {
		prefix := strings.ToLower(m[1])
		if m[2] != "" {
			return fmt.Sprintf("observed conventional commit prefix: %s%s", prefix, m[2])
		}
		return fmt.Sprintf("observed conventional commit prefix: %s", prefix)
	}
	if issueRefRe.MatchString(title) {
		match := issueRefRe.FindString(title)
		return fmt.Sprintf("observed issue-linked title: %s", strings.TrimSpace(match))
	}
	return "no detectable title convention"
}

func extractIssueLinkages(text string) []string {
	matches := issueRefRe.FindAllString(text, -1)
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = strings.TrimSpace(m)
	}
	return out
}

func extractValidationIndicators(text string, labels []string) []string {
	var out []string
	for _, match := range validationRe.FindAllString(text, -1) {
		out = append(out, strings.ToLower(match))
	}
	for _, label := range labels {
		if _, ok := validationKeywords[strings.ToLower(label)]; ok {
			out = append(out, strings.ToLower(label))
		}
	}
	return out
}

func extractApproximateScope(pr prPayloadFields) string {
	if pr.ChangedFiles == 0 {
		return "unknown"
	}
	switch {
	case pr.ChangedFiles <= 2:
		return "small"
	case pr.ChangedFiles <= 10:
		return "medium"
	default:
		return "large"
	}
}

func scopeEvidence(pr prPayloadFields) string {
	if pr.ChangedFiles == 0 {
		return "no PR file data stored"
	}
	return fmt.Sprintf("observed changed_files=%d additions=%d deletions=%d", pr.ChangedFiles, pr.Additions, pr.Deletions)
}

func extractRejectionContext(class domain.SeedSourceClass, state, title, body string, labels []string) string {
	text := title
	if body != "" {
		text += " " + body
	}

	for _, label := range labels {
		if _, ok := rejectionLabels[strings.ToLower(label)]; ok {
			return fmt.Sprintf("observed rejection/supersession label: %s", label)
		}
	}

	if class == domain.SeedSourceClassClosedUnmergedPR || (class == domain.SeedSourceClassIssue && state == "closed") {
		if m := rejectionRe.FindStringSubmatch(text); m != nil {
			return fmt.Sprintf("observed rejection/supersession phrase: %s", strings.TrimSpace(m[0]))
		}
	}

	return ""
}

func extractProblemAreas(title, body string, labels []string) []string {
	text := title
	if body != "" {
		text += " " + body
	}

	var areas []string
	for _, m := range pathLikeRe.FindAllString(text, -1) {
		m = strings.TrimSpace(m)
		if strings.Contains(m, "://") || strings.HasPrefix(m, "http") {
			continue
		}
		if m != "" {
			areas = append(areas, m)
		}
	}

	if m := conventionalCommitRe.FindStringSubmatch(title); m != nil && m[2] != "" {
		scope := strings.Trim(m[2], "()")
		if scope != "" {
			areas = append(areas, scope)
		}
	}

	for _, label := range labels {
		if _, ok := problemLabels[strings.ToLower(label)]; ok {
			areas = append(areas, label)
		}
	}
	return areas
}

func sortSeeds(seeds []domain.Seed) {
	sort.SliceStable(seeds, func(i, j int) bool {
		a, b := seeds[i], seeds[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		if a.Number != b.Number {
			return a.Number > b.Number
		}
		return a.Title < b.Title
	})
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func sortedUnique(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	sort.Strings(s)
	out := []string{s[0]}
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1] {
			out = append(out, s[i])
		}
	}
	return out
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/morluto/gitcontribute/internal/clustering"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
)

const (
	defaultNeighborsLimit = 10
	defaultDuplicateLimit = 20
	defaultCollisionLimit = 10
	maxResultLimit        = 1000
	maxCandidateLimit     = 10000
	minCandidateLimit     = 200
	candidateLimitFactor  = 20
)

// Neighbors returns a bounded, ranked list of threads most similar to the
// query thread. Results include transparent scores, reasons, and the source
// revision of the candidate population. No network access occurs.
func (s *Service) Neighbors(ctx context.Context, repo contracts.RepoRef, kind string, number int, limit int) (*NeighborsResult, error) {
	target, err := parseSimilarityThread(repo, kind, number)
	if err != nil {
		return nil, err
	}
	return s.neighborsForThread(ctx, target, limit)
}

func (s *Service) neighborsForThread(ctx context.Context, target parsedSimilarityThread, limit int) (*NeighborsResult, error) {
	limit, err := normalizeSimilarityLimit(limit)
	if err != nil {
		return nil, err
	}

	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}

	repository, err := c.GetRepository(ctx, target.repository.Owner(), target.repository.Repo())
	if err != nil {
		return nil, err
	}
	if repository == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, target.repository)
	}

	query, err := c.GetThread(ctx, repository.ID, target.kind, target.number)
	if err != nil {
		return nil, err
	}
	if query == nil {
		return nil, fmt.Errorf("%w: %s", errThreadNotFound, target.member().String())
	}

	threads, err := c.ListThreads(ctx, repository.ID, corpus.AnyThreadKind(), similarityCandidateLimit(limit))
	if err != nil {
		return nil, err
	}

	queryCand := candidateFromThread(target.repository, *query)
	candidates := make([]clustering.Candidate, 0, len(threads))
	for _, t := range threads {
		if t.ID == query.ID {
			continue
		}
		candidates = append(candidates, candidateFromThread(target.repository, t))
	}

	scored, err := clustering.Neighbors(ctx, queryCand, candidates, limit)
	if err != nil {
		return nil, err
	}

	all := make([]clustering.Candidate, 0, len(candidates)+1)
	all = append(all, queryCand)
	all = append(all, candidates...)

	neighbors := make([]Neighbor, len(scored))
	for i, n := range scored {
		neighbors[i] = neighborFromClustering(n)
	}

	return &NeighborsResult{
		Repo:           target.repository.String(),
		Kind:           string(target.kind),
		Number:         target.number,
		Limit:          limit,
		Total:          len(neighbors),
		SourceRevision: clustering.SourceRevision(all),
		RuleVersion:    duplicateRuleVersion,
		Neighbors:      neighbors,
	}, nil
}

// DuplicateCandidates returns the included members of the duplicate-candidate
// cluster containing the query thread, excluding the query itself. It returns a
// deterministic order, transparent scores, the cluster stable id, canonical
// member, and source revision. If the thread is not in a cluster, the result
// is empty.
func (s *Service) DuplicateCandidates(ctx context.Context, repo contracts.RepoRef, kind string, number int, limit int) (*DuplicateCandidatesResult, error) {
	target, err := parseSimilarityThread(repo, kind, number)
	if err != nil {
		return nil, err
	}
	ref := target.member()

	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}

	repository, err := c.GetRepository(ctx, target.repository.Owner(), target.repository.Repo())
	if err != nil {
		return nil, err
	}
	if repository == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, target.repository)
	}

	query, err := c.GetThread(ctx, repository.ID, ref.Kind, ref.Number)
	if err != nil {
		return nil, err
	}
	if query == nil {
		return nil, fmt.Errorf("%w: %s", errThreadNotFound, ref.String())
	}

	cluster, err := c.GetClusterProjectionForMember(ctx, ref)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = defaultDuplicateLimit
	}
	if limit > maxResultLimit {
		return nil, fmt.Errorf("duplicate candidates limit cannot exceed %d", maxResultLimit)
	}

	result := &DuplicateCandidatesResult{
		Repo:           target.repository.String(),
		Kind:           string(ref.Kind),
		Number:         ref.Number,
		Limit:          limit,
		SourceRevision: "",
		Candidates:     []Neighbor{},
	}

	if cluster == nil {
		result.Total = 0
		return result, nil
	}

	result.ClusterID = cluster.ID
	result.StableID = cluster.StableID
	result.Canonical = ThreadRef{
		Kind:   string(cluster.Canonical.Kind),
		Owner:  cluster.Canonical.Owner,
		Repo:   cluster.Canonical.Repo,
		Number: cluster.Canonical.Number,
	}
	result.SourceRevision = cluster.Revision

	for _, m := range cluster.Members {
		if sameRef(m.Ref, ref) || !m.Included {
			continue
		}
		result.Candidates = append(result.Candidates, Neighbor{
			Kind:   string(m.Ref.Kind),
			Owner:  m.Ref.Owner,
			Repo:   m.Ref.Repo,
			Number: m.Ref.Number,
			Title:  m.Title,
			State:  string(m.State),
			Score:  m.Score,
			Reason: m.Reason,
		})
	}

	sortNeighborsByScore(result.Candidates)
	if limit > 0 && len(result.Candidates) > limit {
		result.Candidates = result.Candidates[:limit]
	}
	result.Total = len(result.Candidates)
	return result, nil
}

type parsedSimilarityThread struct {
	repository domain.RepoRef
	kind       domain.ThreadKind
	number     int
}

func parseSimilarityThread(repo contracts.RepoRef, kind string, number int) (parsedSimilarityThread, error) {
	repository, err := domain.NewRepoRef(repo.Owner, repo.Repo)
	if err != nil {
		return parsedSimilarityThread{}, err
	}

	normalized, err := normalizeThreadKind(kind)
	if err != nil {
		return parsedSimilarityThread{}, err
	}
	if number <= 0 {
		return parsedSimilarityThread{}, errors.New("thread number must be positive")
	}
	return parsedSimilarityThread{repository: repository, kind: normalized, number: number}, nil
}

func (t parsedSimilarityThread) member() clustering.MemberRef {
	return clustering.MemberRef{
		Owner: t.repository.Owner(), Repo: t.repository.Repo(), Kind: t.kind, Number: t.number,
	}
}

func normalizeThreadKind(kind string) (domain.ThreadKind, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "issue", "issues":
		return domain.IssueKind, nil
	case "pull_request", "pullrequest", "pr", "pull":
		return domain.PullRequestKind, nil
	}
	return domain.ThreadKind(""), fmt.Errorf("unsupported thread kind %q", kind)
}

func candidateFromThread(repo domain.RepoRef, t corpus.Thread) clustering.Candidate {
	return clustering.Candidate{
		ThreadID:  t.ID,
		Repo:      repo,
		Kind:      t.Kind,
		Number:    t.Number,
		State:     t.State,
		Title:     t.Title,
		Body:      t.Body,
		Author:    t.Author,
		Labels:    t.Labels,
		CreatedAt: t.SourceCreatedAt,
		UpdatedAt: t.SourceUpdatedAt,
	}
}

func neighborFromClustering(n clustering.Neighbor) Neighbor {
	return Neighbor{
		Kind:   string(n.Ref.Kind),
		Owner:  n.Ref.Owner,
		Repo:   n.Ref.Repo,
		Number: n.Ref.Number,
		Title:  n.Title,
		State:  string(n.State),
		Score:  n.Score,
		Reason: n.Reason,
	}
}

func sortNeighborsByScore(n []Neighbor) {
	sort.Slice(n, func(i, j int) bool {
		if n[i].Score > n[j].Score {
			return true
		}
		if n[i].Score < n[j].Score {
			return false
		}
		if n[i].Kind != n[j].Kind {
			return n[i].Kind < n[j].Kind
		}
		if n[i].Owner != n[j].Owner {
			return n[i].Owner < n[j].Owner
		}
		if n[i].Repo != n[j].Repo {
			return n[i].Repo < n[j].Repo
		}
		return n[i].Number < n[j].Number
	})
}

func sameRef(a, b clustering.MemberRef) bool {
	return a.Number == b.Number &&
		a.Kind == b.Kind &&
		strings.EqualFold(a.Owner, b.Owner) &&
		strings.EqualFold(a.Repo, b.Repo)
}

// PullRequestCollision is a competing open pull request with a score and reason.
type PullRequestCollision struct {
	Number  int     `json:"number"`
	Title   string  `json:"title"`
	Author  string  `json:"author"`
	BaseRef string  `json:"base_ref"`
	Score   float64 `json:"score"`
	Reason  string  `json:"reason"`
}

// PullRequestCollisionResult is the response for a focused open-PR collision query.
type PullRequestCollisionResult struct {
	Repo             string                 `json:"repo"`
	Number           int                    `json:"number"`
	Limit            int                    `json:"limit"`
	Total            int                    `json:"total"`
	Truncated        bool                   `json:"truncated"`
	Population       int                    `json:"population"`
	PopulationCapped bool                   `json:"population_capped"`
	SourceRevision   string                 `json:"source_revision"`
	Collisions       []PullRequestCollision `json:"collisions"`
}

// PullRequestCollisions returns a bounded, ranked list of open pull requests
// that may collide with the query PR. It uses only existing local data:
// base branch from the stored PR payload, explicit cross-references in the
// title/body, and shared author. No network access occurs and no semantic file
// overlap is invented.
func (s *Service) PullRequestCollisions(ctx context.Context, repo contracts.RepoRef, number int, limit int) (*PullRequestCollisionResult, error) {
	if number <= 0 {
		return nil, errors.New("pull request number must be positive")
	}
	dref, err := domain.NewRepoRef(repo.Owner, repo.Repo)
	if err != nil {
		return nil, err
	}

	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}

	repository, err := c.GetRepository(ctx, dref.Owner(), dref.Repo())
	if err != nil {
		return nil, err
	}
	if repository == nil {
		return nil, fmt.Errorf("%w: %s", errRepositoryNotFound, dref)
	}

	query, err := c.GetThread(ctx, repository.ID, domain.PullRequestKind, number)
	if err != nil {
		return nil, err
	}
	if query == nil {
		return nil, fmt.Errorf("pull request not found: %s#%d", dref, number)
	}

	queryPayload, err := latestThreadObservationPayload(ctx, c, query.ID)
	if err != nil {
		return nil, err
	}
	queryBase := parsePRBaseRef(queryPayload)
	queryRefs := clustering.ExtractMemberRefs(query.Title+"\n"+query.Body, dref)

	population, err := c.CountThreadsFiltered(ctx, repository.ID, corpus.PullRequestThreadKind(), corpus.OpenThreadState())
	if err != nil {
		return nil, err
	}
	prs, err := c.ListThreadsFiltered(ctx, repository.ID, corpus.PullRequestThreadKind(), corpus.OpenThreadState(), maxCandidateLimit)
	if err != nil {
		return nil, err
	}

	queryCand := candidateFromThread(dref, *query)
	all := []clustering.Candidate{queryCand}
	var collisions []PullRequestCollision

	for _, t := range prs {
		if t.Number == number {
			continue
		}
		all = append(all, candidateFromThread(dref, t))

		otherPayload, err := latestThreadObservationPayload(ctx, c, t.ID)
		if err != nil {
			return nil, err
		}
		otherBase := parsePRBaseRef(otherPayload)
		score, reason := collisionScore(dref, queryBase, queryRefs, *query, otherBase, t)
		if score == 0 {
			continue
		}
		collisions = append(collisions, PullRequestCollision{
			Number:  t.Number,
			Title:   t.Title,
			Author:  t.Author,
			BaseRef: otherBase,
			Score:   score,
			Reason:  reason,
		})
	}

	sortPRCollisions(collisions)
	if limit <= 0 {
		limit = defaultCollisionLimit
	}
	if limit > maxResultLimit {
		return nil, fmt.Errorf("collision limit cannot exceed %d", maxResultLimit)
	}
	total := len(collisions)
	truncated := total > limit
	if truncated {
		collisions = collisions[:limit]
	}

	return &PullRequestCollisionResult{
		Repo:             dref.String(),
		Number:           number,
		Limit:            limit,
		Total:            total,
		Truncated:        truncated,
		Population:       len(prs),
		PopulationCapped: population > len(prs),
		SourceRevision:   clustering.SourceRevision(all),
		Collisions:       collisions,
	}, nil
}

func collisionScore(repo domain.RepoRef, queryBase string, queryRefs []clustering.MemberRef, query corpus.Thread, otherBase string, other corpus.Thread) (float64, string) {
	const (
		sameBaseWeight    = 0.30
		explicitRefWeight = 0.45
		sameAuthorWeight  = 0.15
	)

	score := 0.0
	var reasons []string

	if queryBase != "" && otherBase != "" && strings.EqualFold(queryBase, otherBase) {
		score += sameBaseWeight
		reasons = append(reasons, fmt.Sprintf("same base branch %s", queryBase))
	}

	otherRefs := clustering.ExtractMemberRefs(other.Title+"\n"+other.Body, repo)
	queryRef := clustering.MemberRef{Owner: repo.Owner(), Repo: repo.Repo(), Kind: query.Kind, Number: query.Number}
	otherRef := clustering.MemberRef{Owner: repo.Owner(), Repo: repo.Repo(), Kind: other.Kind, Number: other.Number}

	if referencesThread(otherRefs, queryRef) {
		score += explicitRefWeight
		reasons = append(reasons, fmt.Sprintf("references PR #%d", query.Number))
	} else if referencesThread(queryRefs, otherRef) {
		score += explicitRefWeight
		reasons = append(reasons, fmt.Sprintf("referenced by PR #%d", other.Number))
	}

	if other.Author != "" && other.Author == query.Author {
		score += sameAuthorWeight
		reasons = append(reasons, "same author")
	}

	if score > 1.0 {
		score = 1.0
	}
	if len(reasons) == 0 {
		return 0, ""
	}
	return score, strings.Join(reasons, "; ")
}

func referencesThread(refs []clustering.MemberRef, target clustering.MemberRef) bool {
	for _, r := range refs {
		if r.Number != target.Number {
			continue
		}
		if !strings.EqualFold(r.Owner, target.Owner) || !strings.EqualFold(r.Repo, target.Repo) {
			continue
		}
		if r.Kind != "" && r.Kind != target.Kind {
			continue
		}
		return true
	}
	return false
}

func latestThreadObservationPayload(ctx context.Context, c *corpus.Corpus, threadID int64) (string, error) {
	obs, err := c.ListThreadObservations(ctx, threadID)
	if err != nil {
		return "", err
	}
	if len(obs) == 0 {
		return "", nil
	}
	latest := obs[0]
	for _, o := range obs[1:] {
		if o.SourceUpdatedAt.After(latest.SourceUpdatedAt) || (o.SourceUpdatedAt.Equal(latest.SourceUpdatedAt) && o.ObservationSequence > latest.ObservationSequence) {
			latest = o
		}
	}
	return latest.Payload, nil
}

func parsePRBaseRef(payload string) string {
	var p struct {
		BaseRef string `json:"BaseRef"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return ""
	}
	return p.BaseRef
}

func sortPRCollisions(c []PullRequestCollision) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].Score > c[j].Score {
			return true
		}
		if c[i].Score < c[j].Score {
			return false
		}
		return c[i].Number < c[j].Number
	})
}

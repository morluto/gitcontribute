package clustering

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Config tunes clustering limits and thresholds.
type Config struct {
	Threshold     float64
	MaxCandidates int
	MaxPairs      int
	MaxBodyTokens int
}

// DefaultConfig returns hard, deterministic defaults.
func DefaultConfig() Config {
	return Config{
		Threshold:     0.30,
		MaxCandidates: 5000,
		MaxPairs:      10_000_000,
		MaxBodyTokens: 1000,
	}
}

// ParamsHash returns a stable hash of the configuration used for attribution.
func (c Config) ParamsHash() string {
	s := fmt.Sprintf("threshold=%.4f|candidates=%d|pairs=%d|body=%d", c.Threshold, c.MaxCandidates, c.MaxPairs, c.MaxBodyTokens)
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// Clusterer groups candidates into duplicate-candidate clusters.
type Clusterer struct {
	Config Config
}

// NewClusterer creates a clusterer with the supplied config, falling back to
// defaults for zero or out-of-range values.
func NewClusterer(cfg Config) *Clusterer {
	if cfg.Threshold <= 0 || cfg.Threshold > 1 {
		cfg.Threshold = DefaultConfig().Threshold
	}
	if cfg.MaxCandidates <= 0 {
		cfg.MaxCandidates = DefaultConfig().MaxCandidates
	}
	if cfg.MaxPairs <= 0 {
		cfg.MaxPairs = DefaultConfig().MaxPairs
	}
	if cfg.MaxBodyTokens <= 0 {
		cfg.MaxBodyTokens = DefaultConfig().MaxBodyTokens
	}
	return &Clusterer{Config: cfg}
}

// Cluster groups candidates into duplicate-candidate clusters and applies
// membership overrides deterministically.
func (cl *Clusterer) Cluster(candidates []Candidate, overrides []MembershipOverride) ([]Cluster, error) {
	if len(candidates) > cl.Config.MaxCandidates {
		return nil, fmt.Errorf("too many candidates: %d > %d", len(candidates), cl.Config.MaxCandidates)
	}

	refs := make([][]MemberRef, len(candidates))
	for i, c := range candidates {
		refs[i] = ExtractRefs(c.Title+"\n"+c.Body, c.Repo)
	}

	// Union-find over candidate indices. Edges are added when the duplicate
	// score meets or exceeds the threshold.
	parent := make([]int, len(candidates))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			if ra < rb {
				parent[rb] = ra
			} else {
				parent[ra] = rb
			}
		}
	}

	pairs := 0
	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			pairs++
			if pairs > cl.Config.MaxPairs {
				return nil, fmt.Errorf("pair budget exceeded: %d", cl.Config.MaxPairs)
			}
			sig := signalsBetween(candidates[i], candidates[j], refs[i], refs[j], cl.Config.MaxBodyTokens)
			if sig.Score() >= cl.Config.Threshold {
				union(i, j)
			}
		}
	}

	groups := make(map[int][]int)
	for i := range candidates {
		groups[find(i)] = append(groups[find(i)], i)
	}

	var clusters []Cluster
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		clusters = append(clusters, cl.buildCluster(candidates, refs, members))
	}

	clusters = applyOverrides(clusters, overrides, candidates)
	sortClusters(clusters)
	return clusters, nil
}

func signalsBetween(a, b Candidate, refsA, refsB []MemberRef, maxBodyTokens int) Signals {
	sig := Signals{}

	bRef := b.Ref()
	for _, r := range refsA {
		if refMatches(r, bRef) {
			sig.ExplicitRef = true
			break
		}
	}
	if !sig.ExplicitRef {
		aRef := a.Ref()
		for _, r := range refsB {
			if refMatches(r, aRef) {
				sig.ExplicitRef = true
				break
			}
		}
	}

	sig.TitleJaccard = jaccard(Tokens(a.Title, true), Tokens(b.Title, true))
	sig.BodyJaccard = jaccard(TokensLimited(a.Body, true, maxBodyTokens), TokensLimited(b.Body, true, maxBodyTokens))
	sig.LabelJaccard = labelJaccard(a.Labels, b.Labels)
	sig.SameAuthor = a.Author != "" && a.Author == b.Author

	return sig
}

// refMatches reports whether a reference (which may have an empty kind) matches
// a candidate member. Empty kind in the reference acts as a wildcard because a
// bare #123 may refer to either an issue or a pull request in the same repo.
func refMatches(ref, candidate MemberRef) bool {
	if ref.Number != candidate.Number {
		return false
	}
	if !strings.EqualFold(ref.Owner, candidate.Owner) {
		return false
	}
	if !strings.EqualFold(ref.Repo, candidate.Repo) {
		return false
	}
	if ref.Kind == "" {
		return true
	}
	return strings.EqualFold(ref.Kind, candidate.Kind)
}

func (cl *Clusterer) buildCluster(candidates []Candidate, refs [][]MemberRef, indices []int) Cluster {
	c := Cluster{
		Repo:    candidates[indices[0]].Repo,
		Members: make([]Member, 0, len(indices)),
	}
	for _, i := range indices {
		d := candidates[i]
		c.Members = append(c.Members, Member{
			ThreadID: d.ThreadID,
			Ref:      d.Ref(),
			Title:    d.Title,
			State:    d.State,
			Included: true,
		})
	}
	c.Canonical = chooseCanonical(c.Members)
	c.StableID = StableID(c.Canonical)

	canonicalIdx := 0
	for i, m := range c.Members {
		if m.Ref == c.Canonical {
			canonicalIdx = i
			break
		}
	}
	canonicalCandidate := candidates[indices[canonicalIdx]]

	for i := range c.Members {
		if i == canonicalIdx {
			c.Members[i].Score = 1.0
			c.Members[i].Reason = "canonical member"
			continue
		}
		sig := signalsBetween(
			canonicalCandidate,
			candidates[indices[i]],
			refs[indices[canonicalIdx]],
			refs[indices[i]],
			cl.Config.MaxBodyTokens,
		)
		c.Members[i].Score = sig.Score()
		c.Members[i].Reason = sig.Reason()
	}
	return c
}

func chooseCanonical(members []Member) MemberRef {
	if len(members) == 0 {
		return MemberRef{}
	}
	canonical := members[0].Ref
	for _, m := range members[1:] {
		if m.Ref.Less(canonical) {
			canonical = m.Ref
		}
	}
	return canonical
}

// StableID returns the deterministic stable cluster id derived from a member
// identity. The same canonical member always produces the same stable id.
func StableID(ref MemberRef) string {
	s := strings.ToLower(fmt.Sprintf("%s/%s:%s#%d", ref.Owner, ref.Repo, ref.Kind, ref.Number))
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

func applyOverrides(clusters []Cluster, overrides []MembershipOverride, candidates []Candidate) []Cluster {
	if len(overrides) == 0 {
		return clusters
	}
	candIndex := make(map[MemberRef]Candidate, len(candidates))
	for _, c := range candidates {
		candIndex[c.Ref()] = c
	}
	for i := range clusters {
		cluster := &clusters[i]
		for _, o := range overrides {
			if o.ClusterID != 0 && o.ClusterID != cluster.ID {
				continue
			}
			switch o.Action {
			case OverrideExclude:
				found := false
				for j := range cluster.Members {
					if cluster.Members[j].Ref == o.Ref {
						cluster.Members[j].Included = false
						cluster.Members[j].Reason = "excluded: " + o.Reason
						found = true
					}
				}
				if !found {
					m := Member{Ref: o.Ref, Included: false, Score: 0.0, Reason: "excluded: " + o.Reason}
					enrichMember(&m, candIndex)
					cluster.Members = append(cluster.Members, m)
				}
			case OverrideInclude:
				found := false
				for j := range cluster.Members {
					if cluster.Members[j].Ref == o.Ref {
						cluster.Members[j].Included = true
						cluster.Members[j].Reason = "included: " + o.Reason
						found = true
					}
				}
				if !found {
					m := Member{Ref: o.Ref, Included: true, Score: 0.0, Reason: "included: " + o.Reason}
					enrichMember(&m, candIndex)
					cluster.Members = append(cluster.Members, m)
				}
			case OverrideSetCanonical:
				found := false
				for j := range cluster.Members {
					if cluster.Members[j].Ref == o.Ref {
						cluster.Canonical = o.Ref
						cluster.Members[j].Score = 1.0
						cluster.Members[j].Reason = "canonical override: " + o.Reason
						found = true
					}
				}
				if !found {
					m := Member{Ref: o.Ref, Included: true, Score: 1.0, Reason: "canonical override: " + o.Reason}
					enrichMember(&m, candIndex)
					cluster.Members = append(cluster.Members, m)
					cluster.Canonical = o.Ref
				}
			}
		}
	}
	return clusters
}

// enrichMembers fills thread metadata for cluster members from candidates.
func enrichMembers(candidates []Candidate, cluster *Cluster) {
	candIndex := make(map[MemberRef]Candidate, len(candidates))
	for _, c := range candidates {
		candIndex[c.Ref()] = c
	}
	for i := range cluster.Members {
		enrichMember(&cluster.Members[i], candIndex)
	}
}

func enrichMember(m *Member, candIndex map[MemberRef]Candidate) {
	c, ok := candIndex[m.Ref]
	if !ok {
		return
	}
	if m.ThreadID == 0 {
		m.ThreadID = c.ThreadID
	}
	if m.Title == "" {
		m.Title = c.Title
	}
	if m.State == "" {
		m.State = c.State
	}
}

func sortClusters(clusters []Cluster) {
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].StableID < clusters[j].StableID
	})
}

// SourceRevision computes a stable hash from the candidate source state.
func SourceRevision(candidates []Candidate) string {
	lines := make([]string, len(candidates))
	for i, c := range candidates {
		lines[i] = fmt.Sprintf("%s/%s:%s#%d %d %d", c.Repo.Owner, c.Repo.Repo, c.Kind, c.Number, c.UpdatedAt.UnixNano(), c.ThreadID)
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:8])
}

// SourceWindow returns the minimum and maximum UpdatedAt of the candidates.
func SourceWindow(candidates []Candidate) (time.Time, time.Time) {
	if len(candidates) == 0 {
		return time.Time{}, time.Time{}
	}
	min := candidates[0].UpdatedAt
	max := candidates[0].UpdatedAt
	for _, c := range candidates[1:] {
		if c.UpdatedAt.Before(min) {
			min = c.UpdatedAt
		}
		if c.UpdatedAt.After(max) {
			max = c.UpdatedAt
		}
	}
	return min, max
}

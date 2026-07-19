package clustering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/similarity"
)

func computeClusters(ctx context.Context, candidates []Candidate, overrides []MembershipOverride, rule similarity.DuplicateRule, budget ExactPairBudget, threshold float64) (Computation, error) {
	if err := ctx.Err(); err != nil {
		return Computation{}, err
	}
	required, err := budget.Required(len(candidates))
	if err != nil {
		return Computation{}, err
	}

	prepared := make([]similarity.PreparedDuplicate, len(candidates))
	for i, c := range candidates {
		if err := ctx.Err(); err != nil {
			return Computation{}, err
		}
		prepared[i] = rule.Prepare(duplicateThread(c))
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

	var pairs uint64
	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			if pairs%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return Computation{}, err
				}
			}
			score := rule.Compare(prepared[i], prepared[j])
			pairs++
			if score.Value >= threshold {
				union(i, j)
			}
		}
	}

	groups := make(map[int][]int)
	for i := range candidates {
		groups[find(i)] = append(groups[find(i)], i)
	}

	var clusters []Cluster
	if err := ctx.Err(); err != nil {
		return Computation{}, err
	}
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		clusters = append(clusters, buildCluster(candidates, prepared, members, rule))
	}

	clusters = applyOverrides(clusters, overrides, candidates)
	sortClusters(clusters)
	return Computation{
		Clusters:       clusters,
		CandidateCount: len(candidates),
		RequiredPairs:  required,
		ComparedPairs:  pairs,
		RuleVersion:    rule.Version(),
	}, nil
}

func buildCluster(candidates []Candidate, prepared []similarity.PreparedDuplicate, indices []int, rule similarity.DuplicateRule) Cluster {
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
	c.StableID = stableID(c.Canonical)

	canonicalIdx := 0
	for i, m := range c.Members {
		if m.Ref == c.Canonical {
			canonicalIdx = i
			break
		}
	}
	for i := range c.Members {
		if i == canonicalIdx {
			c.Members[i].Score = 1.0
			c.Members[i].Reason = "canonical member"
			continue
		}
		score := rule.Compare(prepared[indices[canonicalIdx]], prepared[indices[i]])
		c.Members[i].Score = score.Value
		c.Members[i].Reason = score.Reason
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

func stableID(ref MemberRef) string {
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
						cluster.Members[j].Included = true
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

// SourceRevision returns a full SHA-256 digest of every candidate field that
// can affect duplicate scoring or the stored projection. Candidate and label
// ordering do not affect the digest.
func SourceRevision(candidates []Candidate) string {
	lines := make([]string, len(candidates))
	for i, c := range candidates {
		labels := make([]string, 0, len(c.Labels))
		for _, label := range c.Labels {
			label = strings.ToLower(strings.TrimSpace(label))
			if label != "" {
				labels = append(labels, label)
			}
		}
		sort.Strings(labels)
		lines[i] = fmt.Sprintf("%q/%q:%q#%d thread=%d created=%d updated=%d state=%q title=%q body=%q author=%q labels=%q",
			strings.ToLower(c.Repo.Owner), strings.ToLower(c.Repo.Repo), strings.ToLower(c.Kind), c.Number,
			c.ThreadID, c.CreatedAt.UnixNano(), c.UpdatedAt.UnixNano(), c.State, c.Title, c.Body, c.Author, labels)
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:])
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

package clustering

import (
	"context"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// ReconcileProjection applies durable governance and projection metadata to a
// pure engine computation. All existing clusters, members, overrides, and
// candidates must come from the same storage snapshot. StableID remains tied
// to the engine-selected canonical member even when governance changes the
// displayed canonical member, so later refreshes can find the same history.
func ReconcileProjection(
	ctx context.Context,
	raw []Cluster,
	existing []Cluster,
	overridesByStable map[string][]MembershipOverride,
	candidates []Candidate,
	repo domain.RepoRef,
	revision string,
	now time.Time,
) ([]Cluster, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	existingByStable := make(map[string]Cluster, len(existing))
	for _, cluster := range existing {
		existingByStable[cluster.StableID] = cluster
	}
	windowStart, windowEnd := SourceWindow(candidates)
	matched := make(map[string]struct{}, len(raw))
	out := make([]Cluster, 0, len(raw))
	for _, cluster := range raw {
		if current, ok := existingByStable[cluster.StableID]; ok {
			matched[cluster.StableID] = struct{}{}
			cluster.ID = current.ID
			cluster.CreatedAt = current.CreatedAt
			cluster.State = current.State
			if cluster.State == ClusterRetired {
				cluster.State = ClusterOpen
			}
			cluster = applyOverrides([]Cluster{cluster}, overridesByStable[cluster.StableID], candidates)[0]
		}
		enrichMembers(candidates, &cluster)
		stampProjection(&cluster, repo, revision, windowStart, windowEnd, now)
		out = append(out, cluster)
	}
	for _, current := range existing {
		if _, ok := matched[current.StableID]; ok {
			continue
		}
		overrides := overridesByStable[current.StableID]
		if !hasPersistentGovernance(overrides) {
			continue
		}
		preserved := applyOverrides([]Cluster{current}, overrides, candidates)[0]
		enrichMembers(candidates, &preserved)
		stampProjection(&preserved, repo, revision, windowStart, windowEnd, now)
		out = append(out, preserved)
	}
	sortClusters(out)
	return out, nil
}

func hasPersistentGovernance(overrides []MembershipOverride) bool {
	for _, override := range overrides {
		if override.Action == OverrideInclude || override.Action == OverrideSetCanonical {
			return true
		}
	}
	return false
}

func stampProjection(cluster *Cluster, repo domain.RepoRef, revision string, windowStart, windowEnd, now time.Time) {
	cluster.Repo = repo
	cluster.Revision = revision
	cluster.WindowStart = windowStart
	cluster.WindowEnd = windowEnd
	if cluster.State == "" {
		cluster.State = ClusterOpen
	}
	cluster.UpdatedAt = now
	if cluster.CreatedAt.IsZero() {
		cluster.CreatedAt = now
	}
}

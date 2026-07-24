package cli

import (
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
)

func sourceListHuman(r *contracts.SourceListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d sources", len(r.Sources))
	if r.Truncated {
		fmt.Fprintf(&b, " of %d (truncated)", r.Total)
	}
	for _, source := range r.Sources {
		fmt.Fprintf(&b, "\n- %s (%s)", source.Name, source.Kind)
	}
	return b.String()
}

func clusterListHuman(r *contracts.ClusterListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Clusters for %s: %d found", r.Repo, r.Total)
	if r.Truncated {
		fmt.Fprintf(&b, " (%d shown, truncated)", len(r.Clusters))
	}
	for _, cl := range r.Clusters {
		fmt.Fprintf(&b, "\n- %s [%s] %s/%s:%s#%d (%d members)", cl.StableID, cl.State, cl.Canonical.Owner, cl.Canonical.Repo, cl.Canonical.Kind, cl.Canonical.Number, cl.MemberCount)
	}
	return b.String()
}

func lensListHuman(r *contracts.LensListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d lenses", len(r.Lenses))
	if r.Truncated {
		fmt.Fprintf(&b, " of %d (truncated)", r.Total)
	}
	for _, l := range r.Lenses {
		fmt.Fprintf(&b, "\n- %s", l.Name)
	}
	return b.String()
}

func collectionListHuman(r *contracts.CollectionListResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d collections", len(r.Collections))
	if r.Truncated {
		fmt.Fprintf(&b, " of %d (truncated)", r.Total)
	}
	for _, col := range r.Collections {
		fmt.Fprintf(&b, "\n- %s (%d members)", col.Name, col.MemberCount)
	}
	return b.String()
}

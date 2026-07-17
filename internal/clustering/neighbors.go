package clustering

import (
	"fmt"
	"sort"
)

// Neighbor is a scored thread near a query candidate.
type Neighbor struct {
	ThreadID int64
	Ref      MemberRef
	Title    string
	State    string
	Score    float64
	Reason   string
}

// Neighbor defaults and hard limits.
const (
	DefaultNeighborsLimit = 10
	MaxNeighborsLimit     = 1000
)

// Neighbors scores every candidate against the query using deterministic local
// signals and returns the top limit results with stable tie ordering.
//
// The query itself is excluded from the returned set. Scores and reasons are
// produced by the same Signals used for duplicate-candidate clustering.
func Neighbors(query Candidate, candidates []Candidate, cfg Config, limit int) ([]Neighbor, error) {
	cfg = normalizeConfig(cfg)
	if limit <= 0 {
		limit = DefaultNeighborsLimit
	}
	if limit > MaxNeighborsLimit {
		return nil, fmt.Errorf("neighbors limit %d exceeds maximum %d", limit, MaxNeighborsLimit)
	}

	queryRefs := ExtractRefs(query.Title+"\n"+query.Body, query.Repo)

	out := make([]Neighbor, 0, len(candidates))
	for _, c := range candidates {
		if sameMemberRef(c.Ref(), query.Ref()) {
			continue
		}
		otherRefs := ExtractRefs(c.Title+"\n"+c.Body, c.Repo)
		sig := signalsBetween(query, c, queryRefs, otherRefs, cfg.MaxBodyTokens)
		out = append(out, Neighbor{
			ThreadID: c.ThreadID,
			Ref:      c.Ref(),
			Title:    c.Title,
			State:    c.State,
			Score:    sig.Score(),
			Reason:   sig.Reason(),
		})
	}

	sortNeighbors(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func sortNeighbors(n []Neighbor) {
	sort.Slice(n, func(i, j int) bool {
		if n[i].Score > n[j].Score {
			return true
		}
		if n[i].Score < n[j].Score {
			return false
		}
		return n[i].Ref.Less(n[j].Ref)
	})
}

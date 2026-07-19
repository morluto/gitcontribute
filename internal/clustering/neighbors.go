package clustering

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/ranking"
	"github.com/morluto/gitcontribute/internal/similarity"
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
// produced by the same versioned duplicate rule used for clustering.
func Neighbors(ctx context.Context, query Candidate, candidates []Candidate, limit int) ([]Neighbor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultNeighborsLimit
	}
	if limit > MaxNeighborsLimit {
		return nil, fmt.Errorf("neighbors limit %d exceeds maximum %d", limit, MaxNeighborsLimit)
	}

	rule := similarity.DefaultDuplicateRule()
	preparedQuery := rule.Prepare(duplicateThread(query))

	out := make([]Neighbor, 0, len(candidates))
	for i, c := range candidates {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if sameMemberRef(c.Ref(), query.Ref()) {
			continue
		}
		score := rule.Compare(preparedQuery, rule.Prepare(duplicateThread(c)))
		out = append(out, Neighbor{
			ThreadID: c.ThreadID,
			Ref:      c.Ref(),
			Title:    c.Title,
			State:    c.State,
			Score:    score.Value,
			Reason:   score.Reason,
		})
	}

	return ranking.TopK(out, limit, betterNeighbor), nil
}

func betterNeighbor(a, b Neighbor) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Ref.Less(b.Ref)
}

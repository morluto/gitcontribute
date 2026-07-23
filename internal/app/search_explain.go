package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
)

// ExplainMatchResult exposes the factual signals that contribute to a match
// score. It performs no network access.
type ExplainMatchResult struct {
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
}

// ExplainMatch returns factual score reasons for a search match without network
// access. The returned reasons describe stored text matches, source freshness,
// and coverage signals.
func (s *Service) ExplainMatch(_ context.Context, query string, match cli.SearchMatch) (*ExplainMatchResult, error) {
	var freshness time.Time
	if match.Freshness != "" {
		var err error
		freshness, err = time.Parse(time.RFC3339, match.Freshness)
		if err != nil {
			return nil, fmt.Errorf("parse freshness: %w", err)
		}
	}
	reasons := explainMatchReasons(query, match, freshness, s.now())
	return &ExplainMatchResult{Score: match.Score, Reasons: reasons}, nil
}

func explainMatchReasons(query string, match cli.SearchMatch, freshness, now time.Time) []string {
	var reasons []string
	if strings.TrimSpace(query) != "" {
		source := match.MatchSource
		if source == "" {
			source = "weighted search document"
		}
		reasons = append(reasons, "the stored FTS5 index matched the query in "+source)
	}

	if !freshness.IsZero() && !now.IsZero() {
		age := now.Sub(freshness)
		if age < 0 {
			age = 0
		}
		reasons = append(reasons, fmt.Sprintf("source updated %s ago at %s", humanDuration(age), freshness.Format(time.RFC3339)))
	}

	if len(match.Coverage) > 0 {
		reasons = append(reasons, "coverage includes "+strings.Join(match.Coverage, ", "))
	} else {
		reasons = append(reasons, "no coverage recorded")
	}
	reasons = append(reasons, "score is weighted FTS5 BM25 rank converted from lower-is-better to higher-is-better relevance")
	return reasons
}

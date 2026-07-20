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
	m := searchMatch{
		Title:        match.Title,
		Body:         match.Body,
		Kind:         match.Kind,
		MatchSource:  match.MatchSource,
		MatchExcerpt: match.MatchExcerpt,
	}
	score, reasons := scoreMatch(query, m, freshness, match.Coverage, s.now())
	return &ExplainMatchResult{Score: roundScore(score), Reasons: reasons}, nil
}

func scoreMatch(query string, m searchMatch, freshness time.Time, coverage []string, now time.Time) (float64, []string) {
	terms := uniqueTerms(strings.ToLower(query))
	title := strings.ToLower(m.Title)
	body := strings.ToLower(m.Body)

	var score float64
	var reasons []string
	matched := 0
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(title, term) {
			score += 0.25
			reasons = append(reasons, fmt.Sprintf("query term %q matched in title", term))
			matched++
		} else if strings.Contains(body, term) {
			score += 0.10
			reasons = append(reasons, fmt.Sprintf("query term %q matched in body", term))
			matched++
		} else if m.MatchSource != "" {
			score += 0.10
			reasons = append(reasons, fmt.Sprintf("query term %q matched in stored %s", term, m.MatchSource))
			matched++
		}
	}
	if matched == len(terms) && len(terms) > 0 {
		score += 0.15
		reasons = append(reasons, "all query terms matched")
	}

	if !freshness.IsZero() && !now.IsZero() {
		age := now.Sub(freshness)
		if age < 0 {
			age = 0
		}
		days := age.Hours() / 24
		freshnessScore := 1.0 / (1.0 + days/30.0)
		if freshnessScore > 1 {
			freshnessScore = 1
		}
		score += freshnessScore * 0.20
		reasons = append(reasons, fmt.Sprintf("source updated %s ago at %s", humanDuration(age), freshness.Format(time.RFC3339)))
	}

	if len(coverage) > 0 {
		covScore := float64(len(coverage)) * 0.05
		if covScore > 0.2 {
			covScore = 0.2
		}
		score += covScore
		reasons = append(reasons, "coverage includes "+strings.Join(coverage, ", "))
	} else {
		reasons = append(reasons, "no coverage recorded")
	}

	if score > 1 {
		score = 1
	}
	return roundScore(score), reasons
}

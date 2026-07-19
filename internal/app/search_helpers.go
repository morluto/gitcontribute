package app

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func uniqueTerms(query string) []string {
	fields := strings.Fields(query)
	seen := make(map[string]struct{}, len(fields))
	terms := make([]string, 0, len(fields))
	for _, t := range fields {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		terms = append(terms, t)
	}
	return terms
}

func humanDuration(d time.Duration) string {
	if d < time.Hour*24 {
		return "less than a day"
	}
	days := int(d.Hours() / 24)
	if days < 30 {
		return fmt.Sprintf("%d days", days)
	}
	months := days / 30
	if months < 12 {
		return fmt.Sprintf("%d months", months)
	}
	years := months / 12
	months = months % 12
	if months == 0 {
		return fmt.Sprintf("%d years", years)
	}
	return fmt.Sprintf("%d years, %d months", years, months)
}

func roundScore(score float64) float64 {
	return math.Round(score*100) / 100
}

func formatSearchTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

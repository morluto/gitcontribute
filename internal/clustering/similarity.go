package clustering

import (
	"fmt"
	"math"
	"strings"
)

// Signals are the explainable components used to score duplicate candidacy.
// All fields are deterministic and derived from local thread content or metadata.
type Signals struct {
	ExplicitRef  bool
	TitleJaccard float64
	BodyJaccard  float64
	LabelJaccard float64
	SameAuthor   bool
}

const (
	weightExplicitRef = 0.40
	weightTitle       = 0.45
	weightBody        = 0.05
	weightLabels      = 0.05
	weightSameAuthor  = 0.05
)

// Score returns a [0,1] combined score using fixed transparent weights.
func (s Signals) Score() float64 {
	raw := 0.0
	if s.ExplicitRef {
		raw += weightExplicitRef
	}
	raw += s.TitleJaccard * weightTitle
	raw += s.BodyJaccard * weightBody
	raw += s.LabelJaccard * weightLabels
	if s.SameAuthor {
		raw += weightSameAuthor
	}
	return math.Min(1.0, raw)
}

// Reason produces a human-readable explanation of the strongest signals.
func (s Signals) Reason() string {
	var parts []string
	if s.ExplicitRef {
		parts = append(parts, "explicit reference")
	}
	if s.TitleJaccard > 0.3 {
		parts = append(parts, fmt.Sprintf("title similarity %.2f", s.TitleJaccard))
	}
	if s.BodyJaccard > 0.1 {
		parts = append(parts, fmt.Sprintf("body similarity %.2f", s.BodyJaccard))
	}
	if s.LabelJaccard > 0 {
		parts = append(parts, fmt.Sprintf("shared labels %.2f", s.LabelJaccard))
	}
	if s.SameAuthor {
		parts = append(parts, "same author")
	}
	if len(parts) == 0 {
		return "no strong signal"
	}
	return strings.Join(parts, "; ")
}

// jaccard returns the Jaccard similarity of two sorted token sets.
func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	inter := 0
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			inter++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0.0
	}
	return float64(inter) / float64(union)
}

// labelJaccard computes Jaccard similarity of two label sets.
func labelJaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	aSet := make(map[string]struct{}, len(a))
	for _, l := range a {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			aSet[l] = struct{}{}
		}
	}
	bSet := make(map[string]struct{}, len(b))
	for _, l := range b {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			bSet[l] = struct{}{}
		}
	}
	if len(aSet) == 0 || len(bSet) == 0 {
		return 0.0
	}
	inter := 0
	for l := range bSet {
		if _, ok := aSet[l]; ok {
			inter++
		}
	}
	union := len(aSet) + len(bSet) - inter
	if union == 0 {
		return 0.0
	}
	return float64(inter) / float64(union)
}

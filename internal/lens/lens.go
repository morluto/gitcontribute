// Package lens applies transparent, reusable filters and weighted signals to
// local corpus candidates. It does not fetch or infer signal values.
package lens

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Definition is a reusable ranking policy. Negative weights express costs or
// risks; positive weights express desirable signals.
type Definition struct {
	Name              string             `json:"name"`
	Filter            Filter             `json:"filter"`
	Weights           map[string]float64 `json:"weights"`
	MaxResultsPerRepo int                `json:"max_results_per_repo"`
}

// Filter contains hard eligibility rules evaluated before normalization.
type Filter struct {
	Kinds           []string      `json:"kinds,omitempty"`
	States          []string      `json:"states,omitempty"`
	Languages       []string      `json:"languages,omitempty"`
	ExcludeArchived bool          `json:"exclude_archived,omitempty"`
	Unassigned      bool          `json:"unassigned,omitempty"`
	UpdatedWithin   time.Duration `json:"updated_within,omitempty"`
	MinStars        int           `json:"min_stars,omitempty"`
}

// UnmarshalJSON supports JSON lens definitions where updated_within may be
// expressed as a Go duration string (e.g. "720h") or as nanoseconds.
func (d *Definition) UnmarshalJSON(data []byte) error {
	type alias Definition
	var raw struct {
		*alias
	}
	raw.alias = (*alias)(d)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if d.Weights == nil {
		d.Weights = map[string]float64{}
	}
	return nil
}

// MarshalJSON emits updated_within as a Go duration string so JSON lens
// definitions round-trip with the same format used for input.
func (f Filter) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	if len(f.Kinds) > 0 {
		m["kinds"] = f.Kinds
	}
	if len(f.States) > 0 {
		m["states"] = f.States
	}
	if len(f.Languages) > 0 {
		m["languages"] = f.Languages
	}
	if f.ExcludeArchived {
		m["exclude_archived"] = true
	}
	if f.Unassigned {
		m["unassigned"] = true
	}
	if f.UpdatedWithin > 0 {
		m["updated_within"] = f.UpdatedWithin.String()
	}
	if f.MinStars > 0 {
		m["min_stars"] = f.MinStars
	}
	return json.Marshal(m)
}

// UnmarshalJSON supports duration strings for updated_within.
func (f *Filter) UnmarshalJSON(data []byte) error {
	type rawFilter struct {
		Kinds           []string        `json:"kinds"`
		States          []string        `json:"states"`
		Languages       []string        `json:"languages"`
		ExcludeArchived bool            `json:"exclude_archived"`
		Unassigned      bool            `json:"unassigned"`
		UpdatedWithin   json.RawMessage `json:"updated_within"`
		MinStars        int             `json:"min_stars"`
	}
	var raw rawFilter
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Kinds = raw.Kinds
	f.States = raw.States
	f.Languages = raw.Languages
	f.ExcludeArchived = raw.ExcludeArchived
	f.Unassigned = raw.Unassigned
	f.MinStars = raw.MinStars

	if len(raw.UpdatedWithin) > 0 {
		var s string
		if err := json.Unmarshal(raw.UpdatedWithin, &s); err == nil {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("parse updated_within duration: %w", err)
			}
			f.UpdatedWithin = d
		} else {
			var n int64
			if err := json.Unmarshal(raw.UpdatedWithin, &n); err != nil {
				return fmt.Errorf("updated_within must be a duration string or nanoseconds: %w", err)
			}
			f.UpdatedWithin = time.Duration(n)
		}
	}
	return nil
}

// Candidate is a locally derived item and its named, unnormalized signals.
type Candidate struct {
	ID         string
	Repository string
	Kind       string
	State      string
	Language   string
	Archived   bool
	Assigned   bool
	Stars      int
	UpdatedAt  time.Time
	Signals    map[string]float64
}

// Result explains both the normalized values and each weighted contribution.
type Result struct {
	Candidate     Candidate
	Score         float64
	Normalized    map[string]float64
	Contributions map[string]float64
}

// Rank filters candidates, min-max normalizes each configured signal within
// the eligible population, and returns a stable descending ranking. A signal
// with no population variance contributes zero. Missing values also contribute
// zero and remain absent from Normalized, making incomplete evidence visible.
func Rank(def Definition, candidates []Candidate, now time.Time) ([]Result, error) {
	weightTotal, err := validate(def)
	if err != nil {
		return nil, err
	}

	eligible := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !matches(def.Filter, candidate, now) {
			continue
		}
		for name, value := range candidate.Signals {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("candidate %q signal %q is non-finite", candidate.ID, name)
			}
		}
		eligible = append(eligible, candidate)
	}

	bounds := signalBounds(def.Weights, eligible)
	results := make([]Result, 0, len(eligible))
	for _, candidate := range eligible {
		result := Result{
			Candidate:     candidate,
			Normalized:    make(map[string]float64),
			Contributions: make(map[string]float64),
		}
		for name, weight := range def.Weights {
			value, present := candidate.Signals[name]
			if !present {
				continue
			}
			bound := bounds[name]
			normalized := 0.0
			if bound.max > bound.min {
				normalized = (value - bound.min) / (bound.max - bound.min)
			}
			contribution := normalized * weight / weightTotal
			result.Normalized[name] = normalized
			result.Contributions[name] = contribution
			result.Score += contribution
		}
		results = append(results, result)
	}

	slices.SortStableFunc(results, func(a, b Result) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		if byRepo := strings.Compare(a.Candidate.Repository, b.Candidate.Repository); byRepo != 0 {
			return byRepo
		}
		return strings.Compare(a.Candidate.ID, b.Candidate.ID)
	})
	if def.MaxResultsPerRepo > 0 {
		results = capPerRepository(results, def.MaxResultsPerRepo)
	}
	return results, nil
}

// Validate checks whether a lens definition can be stored and applied.
func Validate(def Definition) error {
	_, err := validate(def)
	return err
}

func validate(def Definition) (float64, error) {
	if strings.TrimSpace(def.Name) == "" {
		return 0, errors.New("lens name is required")
	}
	if len(def.Weights) == 0 {
		return 0, errors.New("lens requires at least one signal weight")
	}
	weightTotal := 0.0
	for name, weight := range def.Weights {
		if strings.TrimSpace(name) == "" {
			return 0, errors.New("lens signal name is required")
		}
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			return 0, fmt.Errorf("lens signal %q has non-finite weight", name)
		}
		weightTotal += math.Abs(weight)
	}
	if weightTotal == 0 {
		return 0, errors.New("lens weights cannot all be zero")
	}
	if def.MaxResultsPerRepo < 0 {
		return 0, errors.New("lens per-repository result limit cannot be negative")
	}
	if def.Filter.UpdatedWithin < 0 {
		return 0, errors.New("lens updated-within duration cannot be negative")
	}
	if def.Filter.MinStars < 0 {
		return 0, errors.New("lens minimum stars cannot be negative")
	}
	return weightTotal, nil
}

func matches(filter Filter, candidate Candidate, now time.Time) bool {
	if filter.ExcludeArchived && candidate.Archived {
		return false
	}
	if filter.Unassigned && candidate.Assigned {
		return false
	}
	if candidate.Stars < filter.MinStars {
		return false
	}
	if filter.UpdatedWithin > 0 && (candidate.UpdatedAt.IsZero() || candidate.UpdatedAt.Before(now.Add(-filter.UpdatedWithin))) {
		return false
	}
	return containsFold(filter.Kinds, candidate.Kind) &&
		containsFold(filter.States, candidate.State) &&
		containsFold(filter.Languages, candidate.Language)
}

func containsFold(allowed []string, value string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

type bounds struct {
	min float64
	max float64
}

func signalBounds(weights map[string]float64, candidates []Candidate) map[string]bounds {
	out := make(map[string]bounds, len(weights))
	for name := range weights {
		first := true
		var b bounds
		for _, candidate := range candidates {
			value, ok := candidate.Signals[name]
			if !ok {
				continue
			}
			if first || value < b.min {
				b.min = value
			}
			if first || value > b.max {
				b.max = value
			}
			first = false
		}
		out[name] = b
	}
	return out
}

func capPerRepository(results []Result, limit int) []Result {
	counts := make(map[string]int)
	out := make([]Result, 0, len(results))
	for _, result := range results {
		if counts[result.Candidate.Repository] >= limit {
			continue
		}
		counts[result.Candidate.Repository]++
		out = append(out, result)
	}
	return out
}

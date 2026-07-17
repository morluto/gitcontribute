package lens

import (
	"math"
	"testing"
	"time"
)

func TestRankAppliesHardFiltersBeforeNormalization(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	def := Definition{
		Name: "active-go",
		Filter: Filter{
			Kinds: []string{"issue"}, Languages: []string{"go"},
			ExcludeArchived: true, Unassigned: true, UpdatedWithin: 30 * 24 * time.Hour,
			MinStars: 20,
		},
		Weights: map[string]float64{"relevance": 3, "collision_risk": -1},
	}
	candidates := []Candidate{
		{ID: "a", Repository: "o/a", Kind: "issue", State: "open", Language: "Go", Stars: 50, UpdatedAt: now, Signals: map[string]float64{"relevance": 10, "collision_risk": 0}},
		{ID: "b", Repository: "o/b", Kind: "issue", State: "open", Language: "Go", Stars: 50, UpdatedAt: now, Signals: map[string]float64{"relevance": 5, "collision_risk": 10}},
		{ID: "filtered", Repository: "o/c", Kind: "issue", Language: "Go", Archived: true, Stars: 50, UpdatedAt: now, Signals: map[string]float64{"relevance": 1_000}},
	}
	results, err := Rank(def, candidates, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Candidate.ID != "a" || results[1].Candidate.ID != "b" {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Score != 0.75 || results[1].Score != -0.25 {
		t.Fatalf("scores = %v, %v", results[0].Score, results[1].Score)
	}
}

func TestRankExplainsMissingAndConstantSignals(t *testing.T) {
	def := Definition{Name: "explain", Weights: map[string]float64{"constant": 1, "missing": 1}}
	results, err := Rank(def, []Candidate{
		{ID: "a", Repository: "o/r", Signals: map[string]float64{"constant": 7}},
		{ID: "b", Repository: "o/r", Signals: map[string]float64{"constant": 7, "missing": 4}},
	}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Normalized["constant"] != 0 || result.Contributions["constant"] != 0 {
			t.Fatalf("constant signal contributed: %+v", result)
		}
	}
	if _, present := results[0].Normalized["missing"]; present && results[0].Candidate.ID == "a" {
		t.Fatalf("missing signal was presented as observed: %+v", results[0])
	}
}

func TestRankCapsRepositoriesAfterScoring(t *testing.T) {
	def := Definition{Name: "diverse", Weights: map[string]float64{"value": 1}, MaxResultsPerRepo: 1}
	results, err := Rank(def, []Candidate{
		{ID: "a1", Repository: "o/a", Signals: map[string]float64{"value": 10}},
		{ID: "a2", Repository: "o/a", Signals: map[string]float64{"value": 9}},
		{ID: "b1", Repository: "o/b", Signals: map[string]float64{"value": 8}},
	}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Candidate.ID != "a1" || results[1].Candidate.ID != "b1" {
		t.Fatalf("results = %+v", results)
	}
}

func TestRankRejectsNonFiniteInputs(t *testing.T) {
	_, err := Rank(Definition{Name: "bad", Weights: map[string]float64{"value": 1}}, []Candidate{
		{ID: "bad", Signals: map[string]float64{"value": math.NaN()}},
	}, time.Time{})
	if err == nil {
		t.Fatal("Rank accepted a non-finite signal")
	}
}

func TestValidateRejectsImpossibleFiltersAndCaps(t *testing.T) {
	tests := []Definition{
		{Name: "bad", Weights: map[string]float64{"value": 1}, MaxResultsPerRepo: -1},
		{Name: "bad", Weights: map[string]float64{"value": 1}, Filter: Filter{UpdatedWithin: -time.Second}},
		{Name: "bad", Weights: map[string]float64{"value": 1}, Filter: Filter{MinStars: -1}},
	}
	for _, def := range tests {
		if err := Validate(def); err == nil {
			t.Fatalf("Validate accepted %+v", def)
		}
	}
}

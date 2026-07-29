package contribution

import (
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func sampleOpportunity() *investigation.Opportunity {
	return &investigation.Opportunity{
		ID:               "opp-1",
		Title:            "Fix data race in pkg/foo",
		ProblemStatement: "A data race under load causes intermittent panics.",
		Scope:            "pkg/foo",
		Impact:           "Improves reliability under concurrent access.",
		Category:         investigation.CategoryBug,
		Confidence:       0.8,
		CollisionStatus:  investigation.CollisionNone,
	}
}

func sampleEvidence() []*evidence.Evidence {
	return []*evidence.Evidence{
		{
			ID:          "ev-2",
			Type:        evidence.EvidenceTypeMinimalReproduction,
			Relation:    evidence.RelationSupporting,
			Description: "Reproduced the panic with go test -race.",
		},
		{
			ID:              "ev-1",
			Type:            evidence.EvidenceTypeBaseFailingRegression,
			Relation:        evidence.RelationSupporting,
			Description:     "Base branch fails the regression test.",
			ValidationRunID: "run-base",
		},
	}
}

func TestRenderIssue(t *testing.T) {
	r := NewRenderer()
	draft, err := r.RenderIssue(IssueInput{
		Opportunity: sampleOpportunity(),
		Evidence:    sampleEvidence(),
		Guidance:    "Include a minimal reproducer and run go test -race.",
		Success:     "go test -race passes and no races are reported.",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if draft.Title != "Fix data race in pkg/foo" {
		t.Fatalf("title: got %q", draft.Title)
	}
	body := draft.Body
	for _, want := range []string{
		"data race",
		"pkg/foo",
		"supporting",
		"Base branch fails",
		"Reproduced the panic",
		"Repository Guidance",
		"Success Criteria",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestRenderIssueRejectsMissingOpportunity(t *testing.T) {
	r := NewRenderer()
	if _, err := r.RenderIssue(IssueInput{}); err == nil {
		t.Fatal("expected error for missing opportunity")
	}
}

func TestRenderPullRequest(t *testing.T) {
	r := NewRenderer()
	draft, err := r.RenderPullRequest(PullRequestInput{
		Opportunity:   sampleOpportunity(),
		Evidence:      sampleEvidence(),
		Guidance:      "Keep changes focused and update tests.",
		Approach:      "Guard the shared map with a mutex during iteration.",
		Changes:       "pkg/foo: add RWMutex around map access.",
		Compatibility: "No public API changes.",
		Limitations:   "Does not address other unrelated races.",
		LinkedIssue:   "Closes owner/repo#42.",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"Motivation",
		"Approach",
		"Focused Changes",
		"Validation",
		"Compatibility",
		"Limitations",
		"Issue Linkage",
		"Repository Guidance",
	} {
		if !strings.Contains(draft.Body, want) {
			t.Fatalf("PR body missing %q:\n%s", want, draft.Body)
		}
	}
}

func TestRenderPullRequestIncludesCompatibleBeforeAfterProof(t *testing.T) {
	def := &evidence.ValidationDefinition{ID: "def", Command: []string{"go", "test", "./pkg/foo", "-run", "TestRace"}}
	base := &evidence.ValidationRun{
		ID: "base", DefinitionID: def.ID, Kind: evidence.RunKindBase,
		Classification: evidence.RunClassificationFailing, ObservationStatus: evidence.ObservationMatched,
		WorkspaceSnapshotAfter: "base-sha",
		Observations: []evidence.ObservationResult{{
			ExpectedObservation: evidence.ExpectedObservation{Name: "race reproduced"},
			Status:              evidence.ObservationMatched, Excerpt: "DATA RACE",
		}},
	}
	candidate := &evidence.ValidationRun{
		ID: "candidate", DefinitionID: def.ID, Kind: evidence.RunKindCandidate,
		Classification: evidence.RunClassificationPassing, ObservationStatus: evidence.ObservationMatched,
		WorkspaceSnapshotAfter: "candidate-sha",
	}
	draft, err := NewRenderer().RenderPullRequest(PullRequestInput{
		Opportunity: sampleOpportunity(), Approach: "Synchronize access.", Evidence: []*evidence.Evidence{
			{ID: "base-evidence", Type: evidence.EvidenceTypeBaseFailingRegression, Relation: evidence.RelationSupporting, ValidationRun: base, ValidationDefinition: def},
			{ID: "candidate-evidence", Type: evidence.EvidenceTypeCandidatePassingRegression, Relation: evidence.RelationSupporting, ValidationRun: candidate, ValidationDefinition: def},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Before/after regression proof", "base-sha", "candidate-sha", "race reproduced", `["go","test","./pkg/foo","-run","TestRace"]`, "sha256:"} {
		if !strings.Contains(draft.Body, want) {
			t.Fatalf("proof missing %q:\n%s", want, draft.Body)
		}
	}
}

func TestRenderPullRequestLabelsCandidateOnlyValidation(t *testing.T) {
	def := &evidence.ValidationDefinition{ID: "def", Command: []string{"go", "test", "./..."}}
	run := &evidence.ValidationRun{
		ID: "candidate", DefinitionID: def.ID, Kind: evidence.RunKindCandidate,
		Classification: evidence.RunClassificationPassing,
	}
	draft, err := NewRenderer().RenderPullRequest(PullRequestInput{
		Opportunity: sampleOpportunity(), Approach: "Change behavior.", Evidence: []*evidence.Evidence{{
			ID: "candidate-evidence", Relation: evidence.RelationSupporting, ValidationRun: run, ValidationDefinition: def,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(draft.Body, "Candidate validation only") || !strings.Contains(draft.Body, "not causal regression proof") {
		t.Fatalf("candidate-only proof mislabeled:\n%s", draft.Body)
	}
}

func TestRenderPullRequestRequiresApproach(t *testing.T) {
	r := NewRenderer()
	if _, err := r.RenderPullRequest(PullRequestInput{Opportunity: sampleOpportunity()}); err == nil {
		t.Fatal("expected error for missing approach")
	}
}

func TestDeterministicIssueDraft(t *testing.T) {
	r := NewRenderer()
	in := IssueInput{
		Opportunity: sampleOpportunity(),
		Evidence:    sampleEvidence(),
		Guidance:    "Guide.",
		Success:     "Success.",
	}
	d1, err := r.RenderIssue(in)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	d2, err := r.RenderIssue(in)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if d1.Body != d2.Body {
		t.Fatalf("drafts differ:\n%s\n---\n%s", d1.Body, d2.Body)
	}
}

func TestRenderPullRequestNoInventedClaims(t *testing.T) {
	r := NewRenderer()
	draft, err := r.RenderPullRequest(PullRequestInput{
		Opportunity: sampleOpportunity(),
		Evidence:    sampleEvidence(),
		Approach:    "Use a mutex.",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Sections that were not supplied should not appear.
	if strings.Contains(draft.Body, "Compatibility") {
		t.Fatalf("PR body invented Compatibility section:\n%s", draft.Body)
	}
	if strings.Contains(draft.Body, "Limitations") {
		t.Fatalf("PR body invented Limitations section:\n%s", draft.Body)
	}
	if strings.Contains(draft.Body, "Issue Linkage") {
		t.Fatalf("PR body invented Issue Linkage section:\n%s", draft.Body)
	}
}

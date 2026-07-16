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

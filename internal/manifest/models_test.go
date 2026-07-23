package manifest

import (
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/workspace"
)

func TestFinalizeUsesDeterministicContentIdentity(t *testing.T) {
	predicate := Predicate{
		GeneratedAt: time.Unix(100, 0).UTC(),
		Repository:  RepositoryIdentity{Owner: "owner", Repo: "repo", CommitSHA: "commit"},
		Opportunity: OpportunityRecord{ID: "opp", InvestigationID: "inv", ProblemStatement: "problem"},
		Readiness:   ReadinessRecord{Status: "unknown", EvaluatedAt: "2025-01-01T00:00:00Z"},
		Status:      "incomplete",
	}
	first, err := Finalize(predicate)
	if err != nil {
		t.Fatal(err)
	}
	predicate.GeneratedAt = time.Unix(200, 0).UTC()
	predicate.Readiness.EvaluatedAt = "2026-01-01T00:00:00Z"
	second, err := Finalize(predicate)
	if err != nil {
		t.Fatal(err)
	}
	if first.Predicate.ManifestID != second.Predicate.ManifestID || first.Predicate.ContentSHA256 != second.Predicate.ContentSHA256 {
		t.Fatalf("generation timestamps changed content identity: %q != %q", first.Predicate.ManifestID, second.Predicate.ManifestID)
	}
}

func TestFinalizeBindsSubjectToWorkspaceSnapshot(t *testing.T) {
	predicate := Predicate{
		Repository:  RepositoryIdentity{Owner: "owner", Repo: "repo"},
		Opportunity: OpportunityRecord{ID: "opp", InvestigationID: "inv"},
		Workspace:   &workspace.Snapshot{SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Status:      "incomplete",
	}
	statement, err := Finalize(predicate)
	if err != nil {
		t.Fatal(err)
	}
	if got := statement.Subject[0].Digest["sha256"]; got != predicate.Workspace.SHA256 {
		t.Fatalf("subject digest = %q, want workspace digest", got)
	}
	if err := statement.Validate(); err != nil {
		t.Fatalf("validate finalized statement: %v", err)
	}
}

func TestValidateRejectsTamperedPredicate(t *testing.T) {
	statement, err := Finalize(Predicate{
		Repository:  RepositoryIdentity{Owner: "owner", Repo: "repo"},
		Opportunity: OpportunityRecord{ID: "opp", InvestigationID: "inv", ProblemStatement: "original"},
		Status:      "incomplete",
	})
	if err != nil {
		t.Fatal(err)
	}
	statement.Predicate.Opportunity.ProblemStatement = "tampered"
	if err := statement.Validate(); err == nil {
		t.Fatal("tampered predicate passed validation")
	}
}

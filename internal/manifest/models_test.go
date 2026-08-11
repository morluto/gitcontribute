package manifest

import (
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
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

func TestFinalizeDerivesStatusFromGaps(t *testing.T) {
	predicate := Predicate{
		Repository:  RepositoryIdentity{Owner: "owner", Repo: "repo"},
		Opportunity: OpportunityRecord{ID: "opp", InvestigationID: "inv"},
		Status:      StatusIncomplete,
	}
	complete, err := Finalize(predicate)
	if err != nil {
		t.Fatal(err)
	}
	if complete.Predicate.Status != StatusComplete {
		t.Fatalf("status without gaps = %q", complete.Predicate.Status)
	}

	predicate.Status = StatusComplete
	predicate.Gaps = []Gap{{Code: "missing", Facet: "tests", Reason: "not run"}}
	incomplete, err := Finalize(predicate)
	if err != nil {
		t.Fatal(err)
	}
	if incomplete.Predicate.Status != StatusIncomplete {
		t.Fatalf("status with gaps = %q", incomplete.Predicate.Status)
	}
}

func TestFinalizeRejectsUnknownDomainDiscriminators(t *testing.T) {
	base := func() Predicate {
		return Predicate{
			Repository:  RepositoryIdentity{Owner: "owner", Repo: "repo"},
			Opportunity: OpportunityRecord{ID: "opp", InvestigationID: "inv"},
		}
	}
	tests := map[string]func(*Predicate){
		"opportunity status": func(predicate *Predicate) {
			predicate.Opportunity.Status = investigation.OpportunityStatus("mystery")
		},
		"validation kind": func(predicate *Predicate) {
			predicate.Validations = []ValidationRecord{{
				DefinitionID: "definition", RunID: "run", Kind: evidence.RunKind("mystery"),
				Classification: evidence.RunClassificationPassing,
			}}
		},
		"JUnit status": func(predicate *Predicate) {
			predicate.Validations = []ValidationRecord{{
				DefinitionID: "definition", RunID: "run", Kind: evidence.RunKindCandidate,
				Classification: evidence.RunClassificationPassing,
				JUnitReport: &JUnitReportRecord{
					SchemaVersion: evidence.JUnitReportSchemaV1,
					Counts:        evidence.JUnitCounts{Total: 1, Unknown: 1},
					TestCases:     []evidence.JUnitTestCase{{Status: evidence.JUnitTestStatus("mystery")}},
				},
			}}
		},
		"evidence freshness": func(predicate *Predicate) {
			predicate.Evidence = []EvidenceRecord{{
				ID: "evidence", Type: evidence.EvidenceTypeManualObservation,
				Relation: evidence.RelationSupporting, Freshness: evidence.FreshnessStatus("mystery"),
			}}
		},
		"pull request state": func(predicate *Predicate) {
			predicate.PullRequest = &PullRequestRecord{State: domain.ThreadState("mystery")}
		},
		"draft kind": func(predicate *Predicate) {
			predicate.Drafts = []DraftRecord{{Kind: domain.ThreadKind("mystery")}}
		},
		"completeness": func(predicate *Predicate) {
			predicate.Completeness = []CompletenessFacet{{Status: CompletenessStatus("mystery")}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			predicate := base()
			mutate(&predicate)
			if _, err := Finalize(predicate); err == nil {
				t.Fatal("unknown manifest discriminator was accepted")
			}
		})
	}
}

func TestValidateRejectsStatusGapContradiction(t *testing.T) {
	statement, err := Finalize(Predicate{
		Repository:  RepositoryIdentity{Owner: "owner", Repo: "repo"},
		Opportunity: OpportunityRecord{ID: "opp", InvestigationID: "inv"},
	})
	if err != nil {
		t.Fatal(err)
	}
	statement.Predicate.Status = StatusIncomplete
	if err := statement.Validate(); err == nil {
		t.Fatal("manifest status contradiction was accepted")
	}
}

package app

import (
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/evidence"
)

func TestManifestValidationPreservesExternalReceiptProvenance(t *testing.T) {
	now := time.Now().UTC()
	definition := &evidence.ValidationDefinition{ID: "external-definition", Command: []string{"go", "test", "./..."}, CreatedAt: now}
	run := &evidence.ValidationRun{
		ID: "external-run", DefinitionID: definition.ID, Kind: evidence.RunKindCandidate,
		Classification: evidence.RunClassificationPassing, StartedAt: now, CompletedAt: now,
		ExecutionOrigin: "external", External: &evidence.ExternalReceiptProvenance{
			SchemaVersion: evidence.ExternalReceiptSchemaV1, Producer: "crabbox", ValidationID: "focused-tests",
			ReceiptSHA256: "receipt-digest", Repository: "owner/repo", Revision: "candidate",
			ArtifactSHA256: "artifact-digest",
		},
	}
	record, gaps, err := buildManifestValidation(definition, run, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if record.ExecutionOrigin != "external" || record.External == nil || record.External.ReceiptSHA256 != "receipt-digest" {
		t.Fatalf("record = %+v", record)
	}
	if len(gaps) == 0 || record.WorkspaceCompatibility != "external_unverified" {
		t.Fatalf("external trust boundary missing: record=%+v gaps=%+v", record, gaps)
	}
}

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestFlameoxReceiptCrossesPublicAttachAndEvidenceBoundaries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "acme", Repo: "rocket"}, "0123456789abcdef0123456789abcdef01234567", "")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "evidence", "testdata", "flameox-profiler-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt evidence.ExternalReceipt
	if err := json.Unmarshal(fixture, &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.InvestigationID = inv.ID
	receipt.ReceiptSHA256 = ""
	receipt.ReceiptSHA256, err = evidence.DigestExternalReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{Service: svc}).AttachValidationReceipt(ctx, mcpcontract.AttachValidationReceiptInput{ReceiptJSON: string(payload)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Producer != "flameox-profiler" || out.Provider != "flameox" || out.ExternalRunID != receipt.ExternalRunID || out.SourceRevision != receipt.Revision || out.ArtifactSHA256 != receipt.ArtifactSHA256 || len(out.Limitations) != 1 || !out.Incomplete || out.ReceiptSHA256 != receipt.ReceiptSHA256 {
		t.Fatalf("attach output = %+v", out)
	}
	stored, err := svc.corpus.GetValidationRun(ctx, out.RunID)
	if err != nil || stored == nil || stored.External == nil {
		t.Fatalf("stored run = %+v, %v", stored, err)
	}
	external := stored.External
	if external.Provider != "flameox" || external.ExternalRunID != "flameox-run-20260731-001" || external.Revision != receipt.Revision || external.ArtifactSHA256 != receipt.ArtifactSHA256 || !external.Incomplete || len(external.Limitations) != 1 || external.ReceiptSHA256 != receipt.ReceiptSHA256 {
		t.Fatalf("external provenance was not preserved: %+v", external)
	}
	evidenceSummary, err := svc.ShowEvidence(ctx, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidenceSummary.Evidence) != 1 || evidenceSummary.Evidence[0].ValidationRunID != out.RunID {
		t.Fatalf("evidence summary = %+v", evidenceSummary)
	}
}

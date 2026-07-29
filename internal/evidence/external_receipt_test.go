package evidence

import (
	"context"
	"strings"
	"testing"
	"time"
)

func validExternalReceipt(t *testing.T) ExternalReceipt {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	receipt := ExternalReceipt{
		SchemaVersion: ExternalReceiptSchemaV1, Producer: "crabbox", InvestigationID: "inv-1",
		ValidationID:  "cuda-focused-tests",
		OpportunityID: "opp-1", Kind: RunKindCandidate, Repository: "owner/repo", Revision: "candidate-sha",
		ArtifactSHA256: strings.Repeat("a", 64), Provider: "digitalocean", ExternalRunID: "lease-1",
		Command: []string{"go", "test", "./..."}, StartedAt: now, CompletedAt: now.Add(time.Second),
		Classification: RunClassificationPassing, Limitations: []string{"CUDA used a stub"},
	}
	digest, err := DigestExternalReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receipt.ReceiptSHA256 = digest
	return receipt
}

func TestAttachExternalReceipt(t *testing.T) {
	repo := newFakeRepo()
	run, err := NewService(repo, nil).AttachExternalReceipt(context.Background(), validExternalReceipt(t))
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if run.ExecutionOrigin != "external" || run.External == nil || run.External.Producer != "crabbox" {
		t.Fatalf("run provenance = %+v", run)
	}
	if len(repo.defs) != 1 || len(repo.runs) != 1 {
		t.Fatalf("stored defs=%d runs=%d", len(repo.defs), len(repo.runs))
	}
}

func TestExternalReceiptPairSharesValidationIdentity(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	candidateReceipt := validExternalReceipt(t)
	candidate, err := service.AttachExternalReceipt(context.Background(), candidateReceipt)
	if err != nil {
		t.Fatal(err)
	}
	baseReceipt := validExternalReceipt(t)
	baseReceipt.Kind = RunKindBase
	baseReceipt.Revision = "base-sha"
	baseReceipt.ArtifactSHA256 = strings.Repeat("b", 64)
	baseReceipt.Classification = RunClassificationFailing
	baseReceipt.ReceiptSHA256 = ""
	baseReceipt.ReceiptSHA256, err = DigestExternalReceipt(baseReceipt)
	if err != nil {
		t.Fatal(err)
	}
	base, err := service.AttachExternalReceipt(context.Background(), baseReceipt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Compare(base, candidate)
	if err != nil || result.Classification != ComparisonFixed {
		t.Fatalf("paired receipt comparison = %+v, %v", result, err)
	}
}

func TestAttachExternalReceiptRejectsSchemaAndDigestMismatch(t *testing.T) {
	for _, mutate := range []func(*ExternalReceipt){
		func(receipt *ExternalReceipt) { receipt.SchemaVersion = "v2" },
		func(receipt *ExternalReceipt) { receipt.Stdout = "tampered" },
	} {
		receipt := validExternalReceipt(t)
		mutate(&receipt)
		if _, err := NewService(newFakeRepo(), nil).AttachExternalReceipt(context.Background(), receipt); err == nil {
			t.Fatal("invalid receipt was accepted")
		}
	}
}

func TestExternalComparisonRequiresCompatibleCompleteIdentity(t *testing.T) {
	receipt := validExternalReceipt(t)
	base := &ValidationRun{
		DefinitionID: "pair", Kind: RunKindBase, Classification: RunClassificationFailing,
		ExecutionOrigin: "external", External: &ExternalReceiptProvenance{
			Repository: receipt.Repository, Revision: "base", ArtifactSHA256: strings.Repeat("b", 64),
		},
	}
	candidate := &ValidationRun{
		DefinitionID: "pair", Kind: RunKindCandidate, Classification: RunClassificationPassing,
		ExecutionOrigin: "external", External: &ExternalReceiptProvenance{
			Repository: receipt.Repository, Revision: "candidate", ArtifactSHA256: strings.Repeat("c", 64),
		},
	}
	result, err := Compare(base, candidate)
	if err != nil || result.Classification != ComparisonFixed {
		t.Fatalf("compatible comparison = %+v, %v", result, err)
	}
	candidate.External.Repository = "other/repo"
	result, err = Compare(base, candidate)
	if err != nil || result.Classification != ComparisonInconclusive {
		t.Fatalf("incompatible comparison = %+v, %v", result, err)
	}
}

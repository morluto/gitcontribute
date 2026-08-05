package evidence

import (
	"context"
	"testing"
	"time"
)

func TestImportExternalEvidenceManifestIsDigestBoundAndIdempotent(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, nil)
	item := ExternalEvidenceManifest{
		SchemaVersion: ExternalEvidenceManifestSchemaV1, Producer: "analysis-tool", InvestigationID: "inv-1",
		Repository: "octo/project", Revision: "abcdef1234567", ObservedAt: time.Unix(100, 0).UTC(),
		Completeness: "complete", Integrity: "verified", Claims: []ExternalEvidenceClaim{{ID: "hotspot-1", Type: EvidenceTypeProfiler, Relation: RelationSupporting, Description: "bounded hotspot", SourceRefs: []string{"https://example.test/report"}, Measurements: map[string]any{"samples": 3}}},
	}
	digest, err := DigestExternalEvidenceManifest(item)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	item.ManifestSHA256 = digest
	first, err := service.ImportExternalEvidenceManifest(context.Background(), item)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, err := service.ImportExternalEvidenceManifest(context.Background(), item)
	if err != nil {
		t.Fatalf("retry import: %v", err)
	}
	if first.ManifestSHA256 != second.ManifestSHA256 || first.ClaimCount != 1 || second.ClaimCount != 1 {
		t.Fatalf("imports = %+v, %+v", first, second)
	}
	if len(repo.evidence) != 1 {
		t.Fatalf("stored evidence = %d, want idempotent one", len(repo.evidence))
	}
	var stored *Evidence
	for _, value := range repo.evidence {
		stored = value
	}
	samples, _ := stored.Measurements["samples"].(int)
	if stored.External == nil || stored.External.Revision != item.Revision || len(stored.SourceRefs) != 1 || stored.SourceRefs[0].URL != "https://example.test/report" || samples != 3 {
		t.Fatalf("stored provenance = %+v", stored)
	}
}

func TestImportExternalEvidenceManifestRejectsDigestAndInvalidIntegrity(t *testing.T) {
	item := ExternalEvidenceManifest{SchemaVersion: ExternalEvidenceManifestSchemaV1, Producer: "tool", InvestigationID: "inv", Repository: "octo/project", Revision: "abc", ObservedAt: time.Now().UTC(), Completeness: "complete", Integrity: "invalid", Claims: []ExternalEvidenceClaim{{ID: "x", Type: EvidenceTypeStaticAnalysis, Relation: RelationInconclusive, Description: "claim"}}, ManifestSHA256: "bad"}
	if err := validateExternalEvidenceManifest(item); err == nil {
		t.Fatal("invalid integrity accepted")
	}
	item.Integrity = "verified"
	if _, err := NewService(newFakeRepo(), nil).ImportExternalEvidenceManifest(context.Background(), item); err == nil {
		t.Fatal("unsigned manifest accepted")
	}
}

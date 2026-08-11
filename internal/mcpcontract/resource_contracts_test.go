package mcpcontract

import (
	"encoding/json"
	"testing"
)

func TestStoredFacetResourceOwnsEffectiveCoverage(t *testing.T) {
	t.Parallel()
	resource, err := NewStoredFacetResource(
		json.RawMessage(`{"head_sha":"abc","items":[]}`),
		&ResourceCoverage{Complete: true, SourceUpdatedAt: "2026-08-11T00:00:00Z"},
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"head_sha":"abc","items":[],"effective_coverage":{"complete":true,"source_updated_at":"2026-08-11T00:00:00Z"}}` {
		t.Fatalf("resource = %s", payload)
	}
	if _, err := NewStoredFacetResource(json.RawMessage(`[]`), nil); err == nil {
		t.Fatal("non-object facet payload was accepted")
	}
	if _, err := NewStoredFacetResource(json.RawMessage(`null`), nil); err == nil {
		t.Fatal("null facet payload was accepted")
	}
	if _, err := NewStoredFacetResource(json.RawMessage(`{"effective_coverage":null}`), nil); err == nil {
		t.Fatal("payload-owned effective coverage was accepted")
	}
	if _, err := json.Marshal(StoredFacetResource{}); err == nil {
		t.Fatal("unparsed facet resource was encoded")
	}
}

func TestActorFacetResourceOwnsParsedValue(t *testing.T) {
	t.Parallel()
	resource, err := NewActorFacetResource(
		"github:U_1", "organizations", false,
		"2026-08-11T01:00:00Z", "2026-08-11T00:00:00Z", "viewer",
		json.RawMessage(`{"organizations":[{"login":"acme"}]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":"gitcontribute.actor-facet.v1","actor_id":"github:U_1","facet":"organizations","complete":false,"observed_at":"2026-08-11T01:00:00Z","source_updated_at":"2026-08-11T00:00:00Z","authorization_scope":"viewer","value":{"organizations":[{"login":"acme"}]}}`
	if string(payload) != want {
		t.Fatalf("resource = %s, want %s", payload, want)
	}
	if _, err := NewActorFacetResource("github:U_1", "organizations", true, "", "", "", json.RawMessage(`not-json`)); err == nil {
		t.Fatal("invalid actor facet JSON was accepted")
	}
	if _, err := json.Marshal(ActorFacetResource{}); err == nil {
		t.Fatal("unparsed actor facet resource was encoded")
	}
}

func TestCIFailureResourcePreservesSparseStoredPayload(t *testing.T) {
	t.Parallel()
	resource, err := NewCIFailureResource(
		json.RawMessage(` { "head_sha": "abc", "workflow_runs": null } `),
		"acme", "rocket", 7,
		&ResourceCoverage{Complete: false, SourceUpdatedAt: "2026-08-11T00:00:00Z"},
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"head_sha":"abc","workflow_runs":null,"schema_version":"gitcontribute.ci-failure-report.v1","owner":"acme","repo":"rocket","number":7,"effective_coverage":{"complete":false,"source_updated_at":"2026-08-11T00:00:00Z"}}`
	if string(payload) != want {
		t.Fatalf("resource = %s, want %s", payload, want)
	}

	for _, invalid := range []json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`[]`),
		json.RawMessage(`{"owner":"stored"}`),
		json.RawMessage(`{"effective_coverage":null}`),
	} {
		if _, err := NewCIFailureResource(invalid, "acme", "rocket", 7, nil); err == nil {
			t.Fatalf("invalid CI payload %s was accepted", invalid)
		}
	}
	if _, err := json.Marshal(CIFailureResource{}); err == nil {
		t.Fatal("unparsed CI failure resource was encoded")
	}
}

func TestCIJobLogResourceRequiresParsedIdentity(t *testing.T) {
	t.Parallel()
	resource, err := NewCIJobLogResource(31, "failure", true)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"schema_version":"gitcontribute.ci-job-log.v1","job_id":31,"body":"failure","truncated":true}` {
		t.Fatalf("resource = %s", payload)
	}
	if _, err := NewCIJobLogResource(0, "failure", false); err == nil {
		t.Fatal("non-positive CI job ID was accepted")
	}
	if _, err := json.Marshal(CIJobLogResource{}); err == nil {
		t.Fatal("unparsed CI job log resource was encoded")
	}
}

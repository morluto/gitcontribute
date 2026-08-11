package research

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCoverageFactParsesOneStructuralState(t *testing.T) {
	t.Parallel()
	fact := observedCoverageFact("thread", "comments", false, true, time.Unix(1, 0).UTC(), 10)
	payload, err := json.Marshal(fact)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CoverageFact
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Present() || decoded.Complete() || !decoded.Truncated() {
		t.Fatalf("decoded coverage = %+v", decoded)
	}
	for _, payload := range []string{
		`{"scope":"thread","facet":"comments","present":false,"complete":true,"truncated":false,"count":0}`,
		`{"scope":"thread","facet":"comments","present":true,"complete":true,"truncated":true,"count":0}`,
		`{"scope":"thread","facet":"comments","present":false,"complete":false,"truncated":false,"count":1}`,
	} {
		if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
			t.Fatalf("contradictory coverage %s was accepted", payload)
		}
	}
}

func TestParseSectionStatusRejectsUnknownValue(t *testing.T) {
	t.Parallel()
	if status, err := ParseSectionStatus(" partial "); err != nil || status != StatusPartial {
		t.Fatalf("status = %q, %v", status, err)
	}
	if _, err := ParseSectionStatus("invented"); err == nil {
		t.Fatal("unknown section status was accepted")
	}
}

func TestReadProvenanceRejectsContradictoryCoverage(t *testing.T) {
	t.Parallel()
	provenance := NewReadProvenance("ephemeral:abc", false, 1, "digest", false, true)
	if provenance.Complete() || provenance.Truncated() || !provenance.UnknownCoverage() {
		t.Fatalf("provenance coverage = complete:%t truncated:%t unknown:%t", provenance.Complete(), provenance.Truncated(), provenance.UnknownCoverage())
	}
	var decoded ReadProvenance
	if err := json.Unmarshal([]byte(`{"snapshot_token":"ephemeral:abc","durable":false,"observation_watermark":1,"query_digest_sha256":"digest","complete":true,"truncated":false,"unknown_coverage":true}`), &decoded); err == nil {
		t.Fatal("contradictory research provenance was accepted")
	}
}

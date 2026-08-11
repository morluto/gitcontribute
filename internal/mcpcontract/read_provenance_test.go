package mcpcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCorpusReadProvenanceDerivesAndParsesCoverage(t *testing.T) {
	t.Parallel()
	provenance := NewCorpusReadProvenance("ephemeral:abc", false, 7, strings.Repeat("a", 64), true, true)
	if provenance.Complete() || !provenance.Truncated() || !provenance.UnknownCoverage() {
		t.Fatalf("provenance coverage = complete:%t truncated:%t unknown:%t", provenance.Complete(), provenance.Truncated(), provenance.UnknownCoverage())
	}
	payload, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CorpusReadProvenance
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Complete() || !decoded.Truncated() || !decoded.UnknownCoverage() {
		t.Fatalf("decoded provenance = %+v", decoded)
	}
	if err := json.Unmarshal([]byte(`{"snapshot_token":"ephemeral:abc","durable":false,"observation_watermark":7,"query_digest_sha256":"abc","complete":true,"truncated":true,"unknown_coverage":false}`), &decoded); err == nil {
		t.Fatal("contradictory provenance coverage was accepted")
	}
}

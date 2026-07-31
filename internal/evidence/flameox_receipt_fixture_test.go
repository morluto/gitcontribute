package evidence

import (
	"bytes"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// This fixture is producer-neutral: the test validates the repository receipt
// contract and never invokes Flameox or installs a profiler dependency.
//
//go:embed testdata/flameox-profiler-receipt.json
var flameoxProfilerReceiptFixture []byte

func TestFlameoxProfilerReceiptFixtureRoundTripsThroughExternalContract(t *testing.T) {
	receipt := decodeExternalReceiptFixture(t, flameoxProfilerReceiptFixture)
	if receipt.SchemaVersion != ExternalReceiptSchemaV1 || receipt.Producer != "flameox-profiler" || receipt.Provider != "flameox" {
		t.Fatalf("fixture provenance = %+v", receipt)
	}
	if receipt.Kind != RunKindBase || !receipt.Incomplete || len(receipt.Limitations) != 1 {
		t.Fatalf("fixture completeness = %+v", receipt)
	}
	if _, err := hex.DecodeString(receipt.ArtifactSHA256); err != nil || len(receipt.ArtifactSHA256) != 64 {
		t.Fatalf("artifact digest = %q: %v", receipt.ArtifactSHA256, err)
	}
	if len(receipt.Artifacts) != 1 || len(receipt.Artifacts["profile.json"]) != 64 {
		t.Fatalf("artifact manifest = %+v", receipt.Artifacts)
	}
	for name, digest := range receipt.Artifacts {
		if raw, err := hex.DecodeString(digest); err != nil || len(raw) != 32 {
			t.Fatalf("artifact %q digest = %q: %v", name, digest, err)
		}
	}
	digest, err := DigestExternalReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ReceiptSHA256 != digest {
		t.Fatalf("receipt digest = %q, want %q", receipt.ReceiptSHA256, digest)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip := decodeExternalReceiptFixture(t, encoded)
	if !reflect.DeepEqual(receipt, roundTrip) {
		t.Fatalf("round trip changed receipt:\n%+v\n%+v", receipt, roundTrip)
	}
}

func TestFlameoxProfilerReceiptFixtureRejectsMalformedAndUnknownFields(t *testing.T) {
	if _, err := decodeExternalReceipt([]byte("{")); err == nil {
		t.Fatal("malformed receipt was accepted")
	}
	var value map[string]any
	if err := json.Unmarshal(flameoxProfilerReceiptFixture, &value); err != nil {
		t.Fatal(err)
	}
	value["unexpected"] = true
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeExternalReceipt(encoded); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("unknown field error = %v", err)
	}
	receipt := decodeExternalReceiptFixture(t, flameoxProfilerReceiptFixture)
	receipt.ArtifactSHA256 = strings.Repeat("z", 64)
	if _, err := DigestExternalReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Service{}).AttachExternalReceipt(nil, receipt); err == nil {
		t.Fatal("wrong artifact digest was accepted")
	}
}

func decodeExternalReceiptFixture(t *testing.T, data []byte) ExternalReceipt {
	t.Helper()
	receipt, err := decodeExternalReceipt(data)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func decodeExternalReceipt(data []byte) (ExternalReceipt, error) {
	var receipt ExternalReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return ExternalReceipt{}, err
	}
	return receipt, nil
}

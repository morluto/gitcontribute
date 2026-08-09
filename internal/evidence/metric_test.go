package evidence

import (
	"encoding/json"
	"testing"
)

func TestMetricJSONRoundTripPreservesAvailableAndUnavailableVariants(t *testing.T) {
	tests := []struct {
		name   string
		metric Int64Metric
		value  int64
		known  bool
		reason string
	}{
		{name: "available zero", metric: AvailableInt64Metric(0), value: 0, known: true},
		{name: "unavailable", metric: UnavailableInt64Metric("not supported"), reason: "not supported"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := json.Marshal(testCase.metric)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Int64Metric
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			value, known := decoded.Value()
			if value != testCase.value || known != testCase.known || decoded.UnavailableReason() != testCase.reason {
				t.Fatalf("decoded metric = value %d known %t reason %q", value, known, decoded.UnavailableReason())
			}
		})
	}
}

func TestMetricJSONRejectsValueUnavailableContradiction(t *testing.T) {
	for name, payload := range map[string]string{
		"signed":   `{"Value":1,"UnavailableReason":"not supported"}`,
		"unsigned": `{"Value":1,"UnavailableReason":"not supported"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			if name == "signed" {
				err = json.Unmarshal([]byte(payload), &Int64Metric{})
			} else {
				err = json.Unmarshal([]byte(payload), &Uint64Metric{})
			}
			if err == nil {
				t.Fatal("contradictory metric was accepted")
			}
		})
	}
}

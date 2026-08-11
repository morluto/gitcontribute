package mcpcontract

import (
	"encoding/json"
	"testing"
)

func TestBatchItemStatusParsesDurableJSON(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		encoded string
		want    BatchItemStatus
	}{
		{`" complete "`, BatchItemComplete},
		{`" PARTIAL "`, BatchItemPartial},
		{`" Retryable "`, BatchItemRetryable},
		{`" unavailable "`, BatchItemUnavailable},
		{`" failed "`, BatchItemFailed},
	} {
		var status BatchItemStatus
		if err := json.Unmarshal([]byte(test.encoded), &status); err != nil {
			t.Fatal(err)
		}
		if status != test.want {
			t.Fatalf("status = %q, want %q", status, test.want)
		}
	}
	var status BatchItemStatus
	if err := json.Unmarshal([]byte(`"impossible"`), &status); err == nil {
		t.Fatal("invalid batch item status was accepted")
	}
}

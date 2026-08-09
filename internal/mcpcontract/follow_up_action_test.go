package mcpcontract

import (
	"encoding/json"
	"testing"
)

func TestFollowUpActionRoundTripsTypedVariant(t *testing.T) {
	t.Parallel()
	want := FollowUpActionFor(ResourceReadAction{URI: "gitcontribute://artifact/1"})
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got FollowUpAction
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	input, ok := RecoveryInput[ResourceReadAction](got)
	if !ok || got.Type() != "read_resource" || input.URI != "gitcontribute://artifact/1" {
		t.Fatalf("follow-up action = type %q input %+v", got.Type(), input)
	}
}

func TestFollowUpActionRejectsMismatchedOrEmptyVariant(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		`{"type":"read_resource","poll_job":{"ids":["job-1"]}}`,
		`{"type":"read_resource","read_resource":{"uri":"x"},"poll_job":{"ids":["job-1"]}}`,
		`{"type":"sync_threads","sync_threads":{"selection":"repositories"}}`,
		`{"type":"read_resource","read_resource":null}`,
	} {
		var action FollowUpAction
		if err := json.Unmarshal([]byte(payload), &action); err == nil {
			t.Fatalf("decoded invalid follow-up %s", payload)
		}
	}
	if _, err := json.Marshal(FollowUpAction{}); err == nil {
		t.Fatal("empty follow-up action encoded")
	}
}

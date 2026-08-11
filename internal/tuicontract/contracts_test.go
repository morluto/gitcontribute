package tuicontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestActionDerivesConfirmationFromCapability(t *testing.T) {
	t.Parallel()
	write := Action{ID: "write", Label: "Write", Capability: CapabilityLocalWrite}
	if !write.RequiresConfirmation() {
		t.Fatal("local write action did not require confirmation")
	}
	payload, err := json.Marshal(write)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"requires_confirmation":true`) {
		t.Fatalf("action JSON = %s", payload)
	}
	var decoded Action
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != write {
		t.Fatalf("decoded action = %+v, want %+v", decoded, write)
	}
	if err := json.Unmarshal([]byte(`{"id":"write","label":"Write","capability":"local_write","requires_confirmation":false}`), &decoded); err == nil {
		t.Fatal("contradictory action confirmation was accepted")
	}
}

func TestParseActionsRejectsInvalidOrAmbiguousProviderMenus(t *testing.T) {
	t.Parallel()
	if _, err := ParseActions([]Action{{ID: "read", Label: "Read", Capability: Capability("network")}}); err == nil {
		t.Fatal("unknown action capability was accepted")
	}
	if _, err := ParseActions([]Action{
		{ID: "read", Label: "First", Capability: CapabilityOfflineRead},
		{ID: " read ", Label: "Second", Capability: CapabilityOfflineRead},
	}); err == nil {
		t.Fatal("duplicate canonical action ids were accepted")
	}
}

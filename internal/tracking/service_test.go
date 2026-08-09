package tracking

import (
	"fmt"
	"testing"
)

func TestParseBundleRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	valid := fmt.Sprintf(`{"schema_version":%d}`, CurrentBundleSchemaVersion)
	if _, err := ParseBundle([]byte(valid)); err != nil {
		t.Fatalf("parse minimal bundle: %v", err)
	}
	for _, payload := range []string{
		fmt.Sprintf(`{"schema_version":%d,"unexpected":true}`, CurrentBundleSchemaVersion),
		valid + `{}`,
	} {
		if _, err := ParseBundle([]byte(payload)); err == nil {
			t.Fatalf("invalid bundle was accepted: %s", payload)
		}
	}
}

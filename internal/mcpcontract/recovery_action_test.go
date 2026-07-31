package mcpcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecoveryActionOwnsArgumentsForItsDiscriminator(t *testing.T) {
	t.Parallel()
	action := RecoveryAction(HydrateThreadsInput{Threads: []ThreadRef{{Owner: "acme", Repo: "rocket", Kind: "issue", Number: 7}}, Facets: []string{"issue_comments"}})
	payload, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	if action.Type != "hydrate_threads" || action.HydrateThreads == nil || !strings.Contains(encoded, `"hydrate_threads"`) || strings.Contains(encoded, `"arguments"`) || strings.Contains(encoded, `"tool"`) {
		t.Fatalf("recovery action = %s (%+v)", encoded, action)
	}
}

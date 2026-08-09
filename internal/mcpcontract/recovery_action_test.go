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
	input, ok := RecoveryInput[HydrateThreadsInput](action)
	if action.Type() != "hydrate_threads" || !ok || len(input.Threads) != 1 || !strings.Contains(encoded, `"hydrate_threads"`) || strings.Contains(encoded, `"arguments"`) || strings.Contains(encoded, `"tool"`) {
		t.Fatalf("recovery action = %s (%+v)", encoded, action)
	}
}

func TestRecoveryActionSupportsManifestReplay(t *testing.T) {
	action := RecoveryAction(ExportManifestInput{OpportunityID: "opp-1", WorkspaceID: "ws-1"})
	input, ok := RecoveryInput[ExportManifestInput](action)
	if action.Type() != "export_manifest" || !ok || input.OpportunityID != "opp-1" {
		t.Fatalf("manifest recovery action = %+v", action)
	}
}

func TestRecoveryActionCatalogRoundTripsEveryParserVariant(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	for _, action := range RecoveryActionPrototypes() {
		if _, duplicate := seen[action.Type()]; duplicate {
			t.Fatalf("duplicate recovery action type %q", action.Type())
		}
		seen[action.Type()] = struct{}{}
		data, err := json.Marshal(action)
		if err != nil {
			t.Fatalf("marshal %s: %v", action.Type(), err)
		}
		parsed, err := parseRecoveryAction(data)
		if err != nil {
			t.Fatalf("parse %s: %v", action.Type(), err)
		}
		if parsed.Type() != action.Type() {
			t.Fatalf("parsed type = %q, want %q", parsed.Type(), action.Type())
		}
	}
}

func TestRecoveryPlanParsesTypedActions(t *testing.T) {
	plan := RecoveryPlan{Version: RecoveryPlanVersion, Reason: "partial", Then: []ToolCall{
		RecoveryAction(SyncThreadsInput{Selection: "repositories", LimitPerRepository: 20}),
	}}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RecoveryPlan
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Then) != 1 || decoded.Then[0].Type() != "sync_threads" {
		t.Fatalf("decoded recovery plan = %+v", decoded)
	}
	input, ok := RecoveryInput[SyncThreadsInput](decoded.Then[0])
	if !ok || input.Selection != "repositories" || input.LimitPerRepository != 20 {
		t.Fatalf("decoded recovery input = %+v", decoded.Then[0].Input())
	}
}

func TestRecoveryPlanRejectsImpossibleActions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		json string
	}{
		{name: "unsupported plan version", json: `{"version":"invented","then":[{"type":"sync_threads","sync_threads":{}}]}`},
		{name: "mismatched input", json: `{"version":"gitcontribute.recovery.v1","then":[{"type":"sync_threads","hydrate_threads":{}}]}`},
		{name: "multiple inputs", json: `{"version":"gitcontribute.recovery.v1","then":[{"type":"sync_threads","sync_threads":{},"hydrate_threads":{}}]}`},
		{name: "unknown type", json: `{"version":"gitcontribute.recovery.v1","then":[{"type":"invented","invented":{}}]}`},
		{name: "unknown input field", json: `{"version":"gitcontribute.recovery.v1","then":[{"type":"list_concerns","list_concerns":{"invented":true}}]}`},
		{name: "null input", json: `{"version":"gitcontribute.recovery.v1","then":[{"type":"sync_threads","sync_threads":null}]}`},
		{name: "unknown plan field", json: `{"version":"gitcontribute.recovery.v1","invented":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var plan RecoveryPlan
			if err := json.Unmarshal([]byte(test.json), &plan); err == nil {
				t.Fatalf("decoded impossible recovery action: %+v", plan)
			}
		})
	}
}

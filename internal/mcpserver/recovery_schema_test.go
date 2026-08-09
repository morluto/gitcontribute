package mcpserver

import "testing"

func TestToolCallSchemaUsesExclusiveVariants(t *testing.T) {
	t.Parallel()
	schema, err := recoveryToolCallSchema()
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.OneOf) != 32 {
		t.Fatalf("tool-call variants = %d, want 32", len(schema.OneOf))
	}
	seen := make(map[string]bool, len(schema.OneOf))
	for _, variant := range schema.OneOf {
		if variant.Type != "object" || len(variant.Required) != 2 || variant.AdditionalProperties == nil || variant.AdditionalProperties.Not == nil {
			t.Fatalf("non-exclusive recovery action schema: %+v", variant)
		}
		discriminator := variant.Properties["type"]
		if discriminator == nil || discriminator.Const == nil {
			t.Fatalf("recovery action has no constant discriminator: %+v", variant)
		}
		name, ok := (*discriminator.Const).(string)
		if !ok || name == "" || variant.Properties[name] == nil {
			t.Fatalf("recovery action discriminator does not own input: %+v", variant)
		}
		if seen[name] {
			t.Fatalf("duplicate recovery action schema %q", name)
		}
		seen[name] = true
	}
}

func TestFollowUpSchemaExcludesMutatingRecoveryActions(t *testing.T) {
	t.Parallel()
	schema, err := followUpToolCallSchema()
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.OneOf) != 9 {
		t.Fatalf("follow-up variants = %d, want 9", len(schema.OneOf))
	}
	for _, variant := range schema.OneOf {
		name, _ := (*variant.Properties["type"].Const).(string)
		if name == "sync_threads" || name == "export_manifest" {
			t.Fatalf("mutating recovery action %q appears in follow-up schema", name)
		}
	}
}

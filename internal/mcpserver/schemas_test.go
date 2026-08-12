package mcpserver

import (
	"testing"

	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestInferredSchemaIsFreshByType(t *testing.T) {
	first := inferredSchema[mcpcontract.RepoInput]()
	second := inferredSchema[mcpcontract.RepoInput]()
	if first.err != nil || second.err != nil || first.schema == second.schema {
		t.Fatalf("schema definitions share mutable state: first=%p/%v second=%p/%v", first.schema, first.err, second.schema, second.err)
	}
}

func TestCustomizedSchemasDoNotShareMutableState(t *testing.T) {
	first := inputSchema[mcpcontract.SearchCodeBatchInput](func(schema *schemaBuilder) {
		setRange(schema, "limit", 1, 7)
		requireTogether(schema, "owner", "repo")
	})
	second := inputSchema[mcpcontract.SearchCodeBatchInput](func(schema *schemaBuilder) {
		setRange(schema, "limit", 1, 99)
	})
	if first.err != nil || second.err != nil {
		t.Fatalf("schema customization failed: %v / %v", first.err, second.err)
	}
	if got := *first.schema.Properties["limit"].Maximum; got != 7 {
		t.Fatalf("first maximum = %v, want 7", got)
	}
	if got := *second.schema.Properties["limit"].Maximum; got != 99 {
		t.Fatalf("second maximum = %v, want 99", got)
	}
	if len(second.schema.DependentRequired) != 0 {
		t.Fatalf("dependent required state leaked into second schema: %#v", second.schema.DependentRequired)
	}
}

func TestNestedDefinitionsAndArrayItemsRemainCustomizable(t *testing.T) {
	definition := inputSchema[mcpcontract.GetCoverageInput](func(schema *schemaBuilder) {
		setArrayBounds(schema, "targets", 1, 7)
		configureCoverageTargetFields(schema)
	})
	if definition.err != nil {
		t.Fatal(definition.err)
	}
	targets := definition.schema.Properties["targets"]
	if targets == nil || targets.Items == nil || targets.MaxItems == nil || *targets.MaxItems != 7 {
		t.Fatalf("targets schema lost array customization: %#v", targets)
	}
	target := definition.schema.Defs["CoverageTarget"]
	if target == nil {
		target = targets.Items
	}
	if target == nil || len(target.Properties["type"].Enum) != 2 {
		t.Fatalf("nested target definition lost enum customization: %#v", target)
	}
}

func TestFixPatternSchemaAdvertisesNestedHandlerConstraints(t *testing.T) {
	definition := inputSchema[mcpcontract.AnalyzeFixPatternsInput](configureFixPatternAnalysisSchema)
	if definition.err != nil {
		t.Fatal(definition.err)
	}
	symptom := definition.schema.Defs["FixPatternSymptom"]
	if symptom == nil {
		symptom = definition.schema.Properties["symptom_taxonomy"].Items
	}
	name, terms := symptom.Properties["name"], symptom.Properties["terms"]
	if name == nil || name.Pattern != nonWhitespacePattern || terms == nil || terms.MinItems == nil || *terms.MinItems != 1 || terms.MaxItems == nil || *terms.MaxItems != 12 || !terms.UniqueItems || terms.Items == nil || terms.Items.Pattern != nonWhitespacePattern {
		t.Fatalf("symptom schema = %#v", symptom)
	}
	window := definition.schema.Defs["FixPatternTimeWindow"]
	if window == nil {
		window = definition.schema.Properties["time_window"]
	}
	if window.Properties["updated_after"].Format != "date-time" || window.Properties["updated_before"].Format != "date-time" {
		t.Fatalf("time-window schema = %#v", window)
	}
}

func TestServerConstructionProducesStableCatalog(t *testing.T) {
	first, err := New(&fakeReader{}, "test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(&fakeReader{}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := second.catalogFingerprint(), first.catalogFingerprint(); got == "" || got != want {
		t.Fatalf("catalog fingerprint = %q, want %q", got, want)
	}
}

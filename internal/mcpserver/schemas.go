package mcpserver

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type schemaDefinition struct {
	schema *jsonschema.Schema
	err    error
}

type schemaBuilder struct {
	schema *jsonschema.Schema
	err    *error
}

func inferredSchema[T any]() schemaDefinition {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			reflect.TypeFor[mcpcontract.Probability](): {
				Type:        "number",
				Description: "Numeric confidence from 0 to 1.",
				Minimum:     jsonschema.Ptr(0.0),
				Maximum:     jsonschema.Ptr(1.0),
			},
			reflect.TypeFor[mcpcontract.SimilarityScore](): {
				Type:        "number",
				Description: "Normalized similarity score from 0 to 1.",
				Minimum:     jsonschema.Ptr(0.0),
				Maximum:     jsonschema.Ptr(1.0),
			},
			reflect.TypeFor[mcpcontract.RadarScore](): {
				Type:        "integer",
				Description: "Deterministic Contribution Radar score from 0 to 100.",
				Minimum:     jsonschema.Ptr(0.0),
				Maximum:     jsonschema.Ptr(100.0),
			},
			reflect.TypeFor[mcpcontract.ProgressPercent](): {
				Type:        "integer",
				Description: "Integer completion percentage from 0 to 100.",
				Minimum:     jsonschema.Ptr(0.0),
				Maximum:     jsonschema.Ptr(100.0),
			},
			reflect.TypeFor[mcpcontract.NonNegativeInt](): {
				Type:        "integer",
				Description: "Non-negative integer count or delay.",
				Minimum:     jsonschema.Ptr(0.0),
			},
			reflect.TypeFor[mcpcontract.BatchItemStatus](): {
				Type:        "string",
				Description: "Per-item batch outcome.",
				Enum:        []any{"complete", "retryable", "unavailable", "failed"},
			},
			reflect.TypeFor[mcpcontract.JobStatus](): {
				Type:        "string",
				Description: "Durable job lifecycle status.",
				Enum:        []any{"queued", "running", "succeeded", "failed", "cancelled"},
			},
			reflect.TypeFor[mcpcontract.JobExecutionState](): {
				Type:        "string",
				Description: "Whether a durable job is queued, running, or terminal.",
				Enum:        []any{"queued", "running", "terminal"},
			},
			reflect.TypeFor[mcpcontract.JobOutcome](): {
				Type:        "string",
				Description: "Terminal job outcome; omitted until execution is terminal.",
				Enum:        []any{"succeeded", "partial", "failed", "cancelled"},
			},
			reflect.TypeFor[mcpcontract.FixPatternOutcome](): {
				Type:        "string",
				Description: "Pull-request outcome; merged state comes from GitHub and superseded requires an explicit replacement relationship.",
				Enum:        []any{"merged", "closed_unmerged", "superseded", "open", "unknown"},
			},
			reflect.TypeFor[mcpcontract.FixPatternRelationship](): {
				Type:        "string",
				Description: "Evidence connecting a pull request to an issue.",
				Enum:        []any{"closes", "references", "explicit_replacement", "similarity_only"},
			},
			reflect.TypeFor[mcpcontract.FixPatternReportStatus](): {
				Type:        "string",
				Description: "Whether the bounded report is complete or retains coverage limits or failures.",
				Enum:        []any{"complete", "partial"},
			},
			reflect.TypeFor[mcpcontract.FixPatternProofStyle](): {
				Type:        "string",
				Description: "Evidence style detected in stored pull-request text.",
				Enum:        []any{"regression_test", "reproduction", "benchmark", "before_after", "screenshot"},
			},
			reflect.TypeFor[mcpcontract.FixPatternRelatedKind](): {
				Type:        "string",
				Description: "Stored thread kind of a related target.",
				Enum:        []any{"issue", "pull_request"},
			},
		},
	})
	if err != nil {
		return schemaDefinition{err: fmt.Errorf("infer MCP schema: %w", err)}
	}
	return schemaDefinition{schema: schema}
}

func inputSchema[T any](customize func(*schemaBuilder)) schemaDefinition {
	definition := inferredSchema[T]()
	if definition.err != nil {
		return definition
	}
	var buildErr error
	builder := &schemaBuilder{schema: definition.schema, err: &buildErr}
	if customize != nil {
		customize(builder)
	}
	definition.err = buildErr
	return definition
}

func outputSchema[T any](description string) schemaDefinition {
	definition := inferredSchema[T]()
	if definition.err == nil {
		definition.schema.Description = description
	}
	return definition
}

func property(builder *schemaBuilder, name string) *jsonschema.Schema {
	if *builder.err != nil {
		return nil
	}
	p := builder.schema.Properties[name]
	if p == nil {
		*builder.err = fmt.Errorf("MCP schema property %q not found", name)
		return nil
	}
	return p
}

func setEnum(schema *schemaBuilder, name string, values ...string) {
	p := property(schema, name)
	if p == nil {
		return
	}
	p.Enum = make([]any, len(values))
	for i, value := range values {
		p.Enum[i] = value
	}
}

func setArrayEnum(schema *schemaBuilder, name string, values ...string) {
	p := property(schema, name)
	if p == nil {
		return
	}
	if p.Items == nil {
		*schema.err = fmt.Errorf("MCP schema array property %q has no items schema", name)
		return
	}
	p.Items.Enum = make([]any, len(values))
	for i, value := range values {
		p.Items.Enum[i] = value
	}
}

func setRange(schema *schemaBuilder, name string, minimum, maximum float64) {
	p := property(schema, name)
	if p == nil {
		return
	}
	p.Minimum = jsonschema.Ptr(minimum)
	p.Maximum = jsonschema.Ptr(maximum)
}

func setMinimum(schema *schemaBuilder, name string, minimum float64) {
	p := property(schema, name)
	if p != nil {
		p.Minimum = jsonschema.Ptr(minimum)
	}
}

func setDefault(schema *schemaBuilder, name string, value any) {
	b, err := json.Marshal(value)
	if err != nil {
		*schema.err = fmt.Errorf("marshal MCP schema default for %q: %w", name, err)
		return
	}
	p := property(schema, name)
	if p != nil {
		p.Default = b
	}
}

func constrainPullRequestRefs(builder *schemaBuilder, name string) {
	array := property(builder, name)
	if array == nil || array.Items == nil {
		return
	}
	thread := builder.schema.Defs["ThreadRef"]
	if thread == nil {
		return
	}
	threadBuilder := &schemaBuilder{schema: thread, err: builder.err}
	setEnum(threadBuilder, "kind", "pull_request")
	setMinimum(threadBuilder, "number", 1)
	for _, field := range []string{"owner", "repo"} {
		value := property(threadBuilder, field)
		if value != nil {
			value.MinLength = jsonschema.Ptr(1)
			value.Pattern = nonWhitespacePattern
		}
	}
}

func setConst(schema *schemaBuilder, name string, value any) {
	p := property(schema, name)
	if p != nil {
		p.Const = &value
	}
}

// configureCoverageTargetFields constrains the fields in the explicit
// discriminated contract without rebuilding the SDK-inferred schema.
func configureCoverageTargetFields(builder *schemaBuilder) {
	target := builder.schema.Defs["CoverageTarget"]
	if target == nil {
		if targets := property(builder, "targets"); targets != nil {
			target = targets.Items
		}
	}
	if target == nil {
		target = property(builder, "target")
	}
	if target == nil {
		*builder.err = fmt.Errorf("MCP schema definition %q not found", "CoverageTarget")
		return
	}
	targetBuilder := &schemaBuilder{schema: target, err: builder.err}
	setEnum(targetBuilder, "type", "repository", "exact_thread")
	if thread := builder.schema.Defs["ExactCoverageThread"]; thread != nil {
		threadBuilder := &schemaBuilder{schema: thread, err: builder.err}
		setEnum(threadBuilder, "kind", "issue", "pull_request")
		setMinimum(threadBuilder, "number", 1)
	}
}

func requireTogether(builder *schemaBuilder, names ...string) {
	schema := builder.schema
	if schema.DependentRequired == nil {
		schema.DependentRequired = make(map[string][]string)
	}
	for _, name := range names {
		others := make([]string, 0, len(names)-1)
		for _, other := range names {
			if other != name {
				others = append(others, other)
			}
		}
		schema.DependentRequired[name] = others
	}
}

func configureValidationObservationSchema(builder *schemaBuilder) {
	observation := property(builder, "observation")
	if observation == nil {
		return
	}
	contract := &schemaBuilder{schema: observation, err: builder.err}
	observations := property(contract, "observations")
	if observations == nil {
		return
	}
	if observations.Items == nil {
		*builder.err = fmt.Errorf("MCP schema array property %q has no items schema", "observations")
		return
	}
	observations.MinItems = jsonschema.Ptr(2)
	observations.MaxItems = jsonschema.Ptr(2 * maxObservationsPerValidationRun)
	item := &schemaBuilder{schema: observations.Items, err: builder.err}
	setEnum(item, "run", "base", "candidate")
	setEnum(item, "source", "stdout", "stderr", "artifact")
	setEnum(item, "matcher", "exact", "regexp")
	setEnum(item, "occurrence", "present", "absent")
	setDefault(item, "occurrence", "present")
	observations.AllOf = []*jsonschema.Schema{
		observationRunBounds("base"),
		observationRunBounds("candidate"),
	}
}

const maxObservationsPerValidationRun = 8

func observationRunBounds(run string) *jsonschema.Schema {
	value := any(run)
	return &jsonschema.Schema{
		Contains: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"run": {Const: &value},
			},
			Required: []string{"run"},
		},
		MinContains: jsonschema.Ptr(1),
		MaxContains: jsonschema.Ptr(maxObservationsPerValidationRun),
	}
}

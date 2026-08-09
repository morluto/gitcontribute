package mcpserver

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

var recoverySchemaCache struct {
	once   sync.Once
	schema *jsonschema.Schema
	err    error
}

var followUpSchemaCache struct {
	once   sync.Once
	schema *jsonschema.Schema
	err    error
}

func recoveryToolCallSchema() (*jsonschema.Schema, error) {
	recoverySchemaCache.once.Do(func() {
		recoverySchemaCache.schema, recoverySchemaCache.err = buildRecoveryToolCallSchema()
	})
	if recoverySchemaCache.err != nil {
		return nil, recoverySchemaCache.err
	}
	return recoverySchemaCache.schema.CloneSchemas(), nil
}

func buildRecoveryToolCallSchema() (*jsonschema.Schema, error) {
	return buildToolCallSchema(mcpcontract.RecoveryActionPrototypes(), "One replayable recovery action whose discriminator owns exactly one typed input.")
}

func followUpToolCallSchema() (*jsonschema.Schema, error) {
	followUpSchemaCache.once.Do(func() {
		followUpSchemaCache.schema, followUpSchemaCache.err = buildToolCallSchema(mcpcontract.FollowUpActionPrototypes(), "One read or poll action that follows a durable job.")
	})
	if followUpSchemaCache.err != nil {
		return nil, followUpSchemaCache.err
	}
	return followUpSchemaCache.schema.CloneSchemas(), nil
}

func buildToolCallSchema(actions []mcpcontract.ToolCall, description string) (*jsonschema.Schema, error) {
	root := &jsonschema.Schema{
		Description: description,
		OneOf:       make([]*jsonschema.Schema, 0, len(actions)),
	}
	for _, action := range actions {
		input, err := jsonschema.ForType(reflect.TypeOf(action.Input()), nil)
		if err != nil {
			return nil, fmt.Errorf("%s input: %w", action.Type(), err)
		}
		input.Schema = ""
		discriminator := any(action.Type())
		root.OneOf = append(root.OneOf, &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"type":        {Type: "string", Const: &discriminator},
				action.Type(): input,
			},
			Required:             []string{"type", action.Type()},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		})
	}
	return root, nil
}

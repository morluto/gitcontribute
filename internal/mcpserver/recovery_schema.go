package mcpserver

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

var recoveryToolCallSchema = sync.OnceValues(buildRecoveryToolCallSchema)

func buildRecoveryToolCallSchema() (*jsonschema.Schema, error) {
	return buildToolCallSchema(mcpcontract.RecoveryActionPrototypes(), "One replayable recovery action whose discriminator owns exactly one typed input.")
}

var followUpToolCallSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return buildToolCallSchema(mcpcontract.FollowUpActionPrototypes(), "One read or poll action that follows a durable job.")
})

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

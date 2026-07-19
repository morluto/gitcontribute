package mcpserver

import "encoding/json"

// ToolError is the stable, actionable shape used for agent-correctable errors.
// Error keeps the code, field, and example visible when an MCP client only
// transports the error message rather than structured error data.
type ToolError struct {
	Code             string            `json:"code"`
	Message          string            `json:"message"`
	Field            string            `json:"field,omitempty"`
	Retryable        bool              `json:"retryable"`
	Example          map[string]any    `json:"example,omitempty"`
	SuggestedActions []SuggestedAction `json:"suggested_actions,omitempty"`
}

func (e *ToolError) Error() string {
	payload, err := json.Marshal(e)
	if err != nil {
		return `{"code":"tool_error_encoding_failed","message":"The tool error could not be encoded.","retryable":false}`
	}
	return string(payload)
}

// InvalidArgument reports one agent-correctable request error.
func InvalidArgument(field, message string, example map[string]any) error {
	return &ToolError{Code: "invalid_argument", Field: field, Message: message, Example: example}
}

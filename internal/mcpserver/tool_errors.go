package mcpserver

import (
	"errors"

	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type ToolError = mcpcontract.ToolError

// InvalidArgument reports one agent-correctable request error.
func InvalidArgument(field, message string, example map[string]any) error {
	return mcpcontract.InvalidArgument(field, message, example)
}

func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || failure.Is(err, failure.KindNotFound)
}

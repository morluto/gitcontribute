package mcpserver

import (
	"errors"

	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func isNotFound(err error) bool {
	return errors.Is(err, mcpcontract.ErrNotFound) || failure.Is(err, failure.KindNotFound)
}

package cli

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/morluto/gitcontribute/internal/contracts"
)

// RuntimeContractService reports only immutable executable compatibility
// metadata. Implementations must not inspect configuration or the corpus.

// RuntimeContractResult is immutable executable compatibility metadata.

func (c *CLI) runRuntimeContract(ctx context.Context) error {
	service, ok := c.svc.(contracts.RuntimeContractService)
	if !ok {
		return NewCLIError(ExitNotWired, errors.New("runtime contract service is not available"))
	}
	contract, err := service.RuntimeContract(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return json.NewEncoder(c.stdout).Encode(contract)
}

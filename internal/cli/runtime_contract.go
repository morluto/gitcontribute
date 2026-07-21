package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
)

// RuntimeContractService reports only immutable executable compatibility
// metadata. Implementations must not inspect configuration or the corpus.
type RuntimeContractService interface {
	RuntimeContract(ctx context.Context) (*RuntimeContractResult, error)
}

// RuntimeContractResult is immutable executable compatibility metadata.
type RuntimeContractResult struct {
	Name                   string `json:"name"`
	Version                string `json:"version"`
	SupportedSchemaVersion int64  `json:"supported_schema_version"`
}

func (c *CLI) runRuntimeContract(ctx context.Context) error {
	service, ok := c.svc.(RuntimeContractService)
	if !ok {
		return NewCLIError(ExitNotWired, errors.New("runtime contract service is not available"))
	}
	contract, err := service.RuntimeContract(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return WriteRuntimeContract(c.stdout, contract)
}

// WriteRuntimeContract emits exactly one machine-readable JSON value.
func WriteRuntimeContract(output io.Writer, contract *RuntimeContractResult) error {
	return json.NewEncoder(output).Encode(contract)
}

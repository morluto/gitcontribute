package app

import (
	"errors"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
)

// NewRuntimeContract constructs immutable executable compatibility metadata.
// It does not resolve configuration, inspect a corpus, access the network, or
// write local state.
func NewRuntimeContract(version string) (*contracts.RuntimeContractResult, error) {
	if strings.TrimSpace(version) == "" {
		return nil, errors.New("runtime version is empty")
	}
	schema, err := corpus.SupportedSchemaVersion()
	if err != nil {
		return nil, err
	}
	return &contracts.RuntimeContractResult{
		Name:                   "gitcontribute",
		Version:                version,
		SupportedSchemaLineage: corpus.SupportedSchemaLineage(),
		SupportedSchemaVersion: schema,
	}, nil
}

package app

import (
	"context"
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
		Name: "gitcontribute", Version: version, SupportedSchemaVersion: schema,
	}, nil
}

// RuntimeContract reports immutable executable compatibility metadata.
func (s *Service) RuntimeContract(ctx context.Context) (*contracts.RuntimeContractResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return NewRuntimeContract(s.version)
}

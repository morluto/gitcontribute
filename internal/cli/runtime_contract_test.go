package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/contracts"
)

type runtimeContractService struct {
	*fakeService
	called bool
}

func (s *runtimeContractService) RuntimeContract(context.Context) (*contracts.RuntimeContractResult, error) {
	s.called = true
	return &contracts.RuntimeContractResult{Name: "gitcontribute", Version: "1.2.4", SupportedSchemaLineage: "canonical-v1", SupportedSchemaVersion: 28}, nil
}

func TestRuntimeContractCommandIsAlwaysMachineReadable(t *testing.T) {
	t.Parallel()
	service := &runtimeContractService{fakeService: &fakeService{}}
	command, stdout, _ := newTestCLI(service, nil)
	requireNoErr(t, command.Run(context.Background(), []string{"runtime-contract"}))
	if !service.called || !strings.Contains(stdout.String(), `"supported_schema_lineage":"canonical-v1"`) || !strings.Contains(stdout.String(), `"supported_schema_version":28`) {
		t.Fatalf("called=%t output=%q", service.called, stdout.String())
	}
}

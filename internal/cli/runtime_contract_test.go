package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

type runtimeContractService struct {
	*fakeService
	called bool
}

func (s *runtimeContractService) RuntimeContract(context.Context) (*cli.RuntimeContractResult, error) {
	s.called = true
	return &cli.RuntimeContractResult{Name: "gitcontribute", Version: "1.2.4", SupportedSchemaVersion: 28}, nil
}

func TestRuntimeContractCommandIsAlwaysMachineReadable(t *testing.T) {
	service := &runtimeContractService{fakeService: &fakeService{}}
	command, stdout, _ := newTestCLI(service, nil)
	requireNoErr(t, command.Run(context.Background(), []string{"runtime-contract"}))
	if !service.called || !strings.Contains(stdout.String(), `"supported_schema_version":28`) {
		t.Fatalf("called=%t output=%q", service.called, stdout.String())
	}
}

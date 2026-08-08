package cli_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contracts"
)

type fakeUpgradeService struct {
	*fakeService
	calls int
	opts  contracts.UpgradeOptions
}

func (s *fakeUpgradeService) Upgrade(_ context.Context, opts contracts.UpgradeOptions) (*contracts.UpgradeReport, error) {
	s.calls++
	s.opts = opts
	return &contracts.UpgradeReport{}, nil
}

func TestUpgradeDoesNotPromptWhenStandardOutputIsRedirected(t *testing.T) {
	redirected, err := os.CreateTemp(t.TempDir(), "upgrade-stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = redirected.Close() }()

	service := &fakeUpgradeService{fakeService: &fakeService{}}
	var stderr bytes.Buffer
	c := cli.New(service, &fakeMCPRunner{}, redirected, &stderr)
	c.SetInput(strings.NewReader(""))
	err = c.Run(context.Background(), []string{"upgrade"})
	if err == nil || !strings.Contains(err.Error(), "terminal input and visible output") {
		t.Fatalf("error = %v", err)
	}
	if service.calls != 0 {
		t.Fatalf("upgrade service was called %d times", service.calls)
	}
}

func TestUpgradeConsentDescribesCheckAndEligibleManagedUpdate(t *testing.T) {
	service := &fakeUpgradeService{fakeService: &fakeService{}}
	var stdout, stderr bytes.Buffer
	c := cli.New(service, &fakeMCPRunner{}, &stdout, &stderr)
	c.SetInput(strings.NewReader("n\n"))
	if err := c.Run(context.Background(), []string{"upgrade"}); err != nil {
		t.Fatal(err)
	}
	if service.calls != 0 {
		t.Fatalf("upgrade service was called %d times", service.calls)
	}
	output := stderr.String()
	if !strings.Contains(output, "Check npm for the latest GitContribute release and apply an eligible managed update?") || strings.Contains(output, "Install the latest global npm release") {
		t.Fatalf("consent prompt = %q", output)
	}
}

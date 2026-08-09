package cli_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/contracts"
)

type fakeUpgradeService struct {
	*fakeService
	calls  int
	opts   contracts.UpgradeOptions
	report *contracts.UpgradeReport
}

func (s *fakeUpgradeService) Upgrade(_ context.Context, opts contracts.UpgradeOptions) (*contracts.UpgradeReport, error) {
	s.calls++
	s.opts = opts
	if s.report != nil {
		return s.report, nil
	}
	return &contracts.UpgradeReport{}, nil
}

type failFirstWriter struct {
	bytes.Buffer
	err    error
	writes int
}

func (w *failFirstWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.writes == 1 {
		return 0, w.err
	}
	return w.Buffer.Write(data)
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

func TestUpgradeDoesNotLoseAnEarlierOutputFailure(t *testing.T) {
	want := errors.New("broken stdout")
	service := &fakeUpgradeService{fakeService: &fakeService{}, report: &contracts.UpgradeReport{
		Context: "managed", Status: "updated", Latest: "1.2.3", Current: "1.2.2", Command: "npm install",
	}}
	stdout := &failFirstWriter{err: want}
	var stderr bytes.Buffer
	c := cli.New(service, nil, stdout, &stderr)
	err := c.Run(context.Background(), []string{"upgrade", "--yes"})
	if !errors.Is(err, want) {
		t.Fatalf("upgrade error = %v, want %v", err, want)
	}
	if stdout.writes != 1 {
		t.Fatalf("upgrade wrote %d times after output failure, want 1", stdout.writes)
	}
}

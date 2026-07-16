package evidence

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func findOrSkip(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not found in PATH", name)
	}
	return p
}

func TestExecRunnerRejectsUnboundedCapture(t *testing.T) {
	r := NewExecRunner()
	_, err := r.Run(context.Background(), RunRequest{Args: []string{"ignored"}, Dir: t.TempDir(), MaxOutputBytes: maxOutputBytes + 1})
	if !errors.Is(err, ErrInvalidOutputLimit) {
		t.Fatalf("runner error = %v, want ErrInvalidOutputLimit", err)
	}
}

func TestExecRunnerEcho(t *testing.T) {
	echo := findOrSkip(t, "echo")
	r := NewExecRunner()
	ctx := context.Background()
	dir := t.TempDir()

	res, err := r.Run(ctx, RunRequest{
		Args: []string{echo, "hello", "world"},
		Dir:  dir,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code: got %d, want 0", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "hello world") {
		t.Fatalf("stdout: got %q", res.Stdout)
	}
	if res.Classification != RunClassificationPassing {
		t.Fatalf("classification: got %q, want passing", res.Classification)
	}
}

func TestExecRunnerNoShell(t *testing.T) {
	echo := findOrSkip(t, "echo")
	r := NewExecRunner()
	ctx := context.Background()
	dir := t.TempDir()

	// Pass a single argument that a shell would interpret as two words.
	res, err := r.Run(ctx, RunRequest{
		Args: []string{echo, "hello; world"},
		Dir:  dir,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code: got %d, want 0", res.ExitCode)
	}
	got := strings.TrimSpace(res.Stdout)
	if got != "hello; world" {
		t.Fatalf("no shell expansion: got %q", got)
	}
}

func TestExecRunnerFailingExit(t *testing.T) {
	falseCmd := findOrSkip(t, "false")
	r := NewExecRunner()
	ctx := context.Background()
	dir := t.TempDir()

	res, err := r.Run(ctx, RunRequest{
		Args: []string{falseCmd},
		Dir:  dir,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if res.Classification != RunClassificationFailing {
		t.Fatalf("classification: got %q, want failing", res.Classification)
	}
}

func TestExecRunnerTruncation(t *testing.T) {
	printf := findOrSkip(t, "printf")
	r := NewExecRunner()
	ctx := context.Background()
	dir := t.TempDir()

	big := strings.Repeat("x", 2048)
	res, err := r.Run(ctx, RunRequest{
		Args:           []string{printf, "%s", big},
		Dir:            dir,
		MaxOutputBytes: 256,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.Truncated {
		t.Fatal("expected Truncated to be true")
	}
	if len(res.Stdout) != 256 {
		t.Fatalf("stdout length: got %d, want 256", len(res.Stdout))
	}
}

func TestExecRunnerCancellation(t *testing.T) {
	sleep := findOrSkip(t, "sleep")
	r := NewExecRunner()
	dir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	res, err := r.Run(ctx, RunRequest{
		Args: []string{sleep, "5"},
		Dir:  dir,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Classification != RunClassificationCancelled {
		t.Fatalf("classification: got %q, want cancelled", res.Classification)
	}
}

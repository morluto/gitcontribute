//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package evidence

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecRunnerCancellationKillsProcessGroup(t *testing.T) {
	t.Parallel()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type runOutcome struct {
		result *RunResult
		err    error
	}
	done := make(chan runOutcome, 1)
	go func() {
		result, err := NewExecRunner().Run(ctx, RunRequest{
			Args: []string{sh, "-c", "sleep 30 & child=$!; echo $child > child.pid; wait"},
			Dir:  dir,
		})
		done <- runOutcome{result: result, err: err}
	}()

	var rawPID []byte
	deadline := time.Now().Add(2 * time.Second)
	for {
		rawPID, err = os.ReadFile(pidFile)
		if err == nil && strings.TrimSpace(string(rawPID)) != "" {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			outcome := <-done
			t.Fatalf("child pid was not recorded before cancellation: err=%v run_err=%v", err, outcome.err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	outcome := <-done
	if outcome.err != nil {
		t.Fatal(outcome.err)
	}
	result := outcome.result
	if result.Classification != RunClassificationCancelled {
		t.Fatalf("classification = %q, want cancelled", result.Classification)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived cancellation: %v", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

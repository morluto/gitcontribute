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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := NewExecRunner().Run(ctx, RunRequest{
		Args: []string{sh, "-c", "sleep 30 & child=$!; echo $child > child.pid; wait"},
		Dir:  dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Classification != RunClassificationCancelled {
		t.Fatalf("classification = %q, want cancelled", result.Classification)
	}
	rawPID, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
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

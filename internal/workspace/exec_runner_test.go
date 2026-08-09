package workspace

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestExecRunnerRedactsCredentialLikeStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell to produce controlled stderr")
	}
	secret := "github_pat_" + strings.Repeat("a", 22)
	_, err := (execRunner{}).Run(context.Background(), "sh", "-c", "printf '%s\\n' \"token=$1\" >&2; exit 1", "sh", secret)
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("runner error exposed credential-like stderr: %v", err)
	}
}

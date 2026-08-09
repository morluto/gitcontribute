package terminalinstall

import (
	"errors"
	"strings"
	"testing"
)

func TestGitContributePackageSpec(t *testing.T) {
	valid := []string{
		"gitcontribute@latest",
		"gitcontribute@1.2.3",
		"gitcontribute@1.2.3-rc.1",
		"gitcontribute@1.2.3+build.4",
	}
	for _, packageSpec := range valid {
		if err := validatePackageSpec(packageSpec); err != nil {
			t.Errorf("valid package spec rejected: %q: %v", packageSpec, err)
		}
	}

	invalid := []string{
		"",
		"gitcontribute",
		"gitcontribute@v1.2.3",
		"gitcontribute@1.2",
		"gitcontribute@--ignore-scripts",
		"other-package@1.2.3",
	}
	for _, packageSpec := range invalid {
		if err := validatePackageSpec(packageSpec); err == nil {
			t.Errorf("invalid package spec accepted: %q", packageSpec)
		}
	}
}

func TestCommandFailureIncludesOutputWithoutDroppingCause(t *testing.T) {
	cause := errors.New("exit status 1")
	err := commandFailure("install persistent CLI", []byte("permission denied\n"), cause)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("command failure = %v", err)
	}
}

func TestCommandFailureOmitsEmptyOutput(t *testing.T) {
	err := commandFailure("resolve prefix", nil, errors.New("failed"))
	if err.Error() != "resolve prefix: failed" {
		t.Fatalf("command failure = %q", err)
	}
}

func TestCommandFailureRedactsCredentialLikeOutput(t *testing.T) {
	secret := "github_pat_" + strings.Repeat("a", 22)
	err := commandFailure("install persistent CLI", []byte("npm ERR! token="+secret), errors.New("failed"))
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("command failure exposed credential-like output: %q", err)
	}
}

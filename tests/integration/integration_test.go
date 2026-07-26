// Package integration exercises the application end-to-end through the
// product-owned Service contracts. Tests use temporary directories and
// databases; no external network access is performed.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/morluto/gitcontribute/internal/app"
	"github.com/morluto/gitcontribute/internal/config"
	gitlog "github.com/morluto/gitcontribute/internal/log"
)

// TestInitAndStatus exercises corpus creation and health reporting without
// touching the network.
func TestInitAndStatus(t *testing.T) {
	logger := gitlog.New("integration-test")
	paths := testPaths(t)

	svc, err := app.NewWithContext(context.Background(), paths, "test", logger)
	if err != nil {
		t.Fatalf("failed to create application service: %v", err)
	}
	defer svc.Close()

	result, err := svc.Init(context.Background())
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if result.Path == "" {
		t.Error("Init returned empty path")
	}

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !status.Healthy {
		t.Error("Status should report healthy after init")
	}
}

// TestMetadataConfirmsVersion verifies the metadata command returns the
// expected version and does not require network access.
func TestMetadataConfirmsVersion(t *testing.T) {
	logger := gitlog.New("integration-test")
	paths := testPaths(t)

	svc, err := app.NewWithContext(context.Background(), paths, "1.0.0-test", logger)
	if err != nil {
		t.Fatalf("failed to create application service: %v", err)
	}
	defer svc.Close()

	metadata, err := svc.Metadata(context.Background())
	if err != nil {
		t.Fatalf("Metadata failed: %v", err)
	}
	if metadata.Version != "1.0.0-test" {
		t.Errorf("Metadata version = %q, want %q", metadata.Version, "1.0.0-test")
	}
	if metadata.Name != "gitcontribute" {
		t.Errorf("Metadata name = %q, want %q", metadata.Name, "gitcontribute")
	}
}

// TestDoctorWithoutCorpus verifies the doctor command handles a missing
// corpus gracefully.
func TestDoctorWithoutCorpus(t *testing.T) {
	logger := gitlog.New("integration-test")
	paths := testPaths(t)

	svc, err := app.NewWithContext(context.Background(), paths, "test", logger)
	if err != nil {
		t.Fatalf("failed to create application service: %v", err)
	}
	defer svc.Close()

	result, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	if result == nil {
		t.Fatal("Doctor returned nil result")
	}
	if len(result.Checks) == 0 {
		t.Error("Doctor should report at least one check")
	}
}

func testPaths(t *testing.T) *config.Paths {
	t.Helper()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("GITCONTRIBUTE_TEST", "1")

	return config.NewPaths(&config.Env{
		Home: home,
		Vars: map[string]string{
			"XDG_CONFIG_HOME":      filepath.Join(home, ".config"),
			"XDG_DATA_HOME":        filepath.Join(home, ".local", "share"),
			"GITCONTRIBUTE_TEST":   "1",
		},
	})
}

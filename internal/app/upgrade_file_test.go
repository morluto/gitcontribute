package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadUpgradeFileBoundsPackageMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxUpgradePackageBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readUpgradeFile(path); err == nil || !strings.Contains(err.Error(), "upgrade metadata exceeds") {
		t.Fatalf("oversized package metadata error = %v", err)
	}
}

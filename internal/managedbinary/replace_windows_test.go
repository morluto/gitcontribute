//go:build windows

package managedbinary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFileFailurePreservesDestination(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "gitcontribute.exe")
	if err := os.WriteFile(destination, []byte("working"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := replaceFile(filepath.Join(t.TempDir(), "missing.exe"), destination)
	if err == nil {
		t.Fatal("replaceFile accepted a missing source")
	}
	contents, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatalf("read destination after failed replacement: %v", readErr)
	}
	if string(contents) != "working" {
		t.Fatalf("destination contents = %q, want working", contents)
	}
}

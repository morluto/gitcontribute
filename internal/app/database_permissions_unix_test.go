//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDatabaseDirPreservesExistingCustomParent(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := ensureDatabaseDir(filepath.Join(parent, "corpus.db")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0755 {
		t.Fatalf("existing parent mode = %o, want 755", got)
	}
}

func TestEnsureDatabaseDirCreatesPrivateParent(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "gitcontribute", "data")
	if err := ensureDatabaseDir(filepath.Join(parent, "corpus.db")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Fatalf("created parent mode = %o, want 700", got)
	}
}

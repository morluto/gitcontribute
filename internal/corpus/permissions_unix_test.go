//go:build !windows

package corpus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenProtectsDatabaseFile(t *testing.T) {
	t.Parallel()
	_, path := openTestCorpus(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("database mode = %o, want 600", got)
	}
}

func TestOpenProtectsDatabaseFileURI(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "corpus.db")
	c, err := Open(context.Background(), "file:"+filepath.ToSlash(path)+"?cache=private")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("database mode = %o, want 600", got)
	}
}

func TestOpenRepairsDatabaseFileURIPermissions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "corpus.db")
	uri := "file:" + filepath.ToSlash(path) + "?cache=private"
	c, err := Open(context.Background(), uri)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	c, err = Open(context.Background(), uri)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("database mode = %o, want 600", got)
	}
}

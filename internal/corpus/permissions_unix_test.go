//go:build !windows

package corpus

import (
	"os"
	"testing"
)

func TestOpenProtectsDatabaseFile(t *testing.T) {
	_, path := openTestCorpus(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("database mode = %o, want 600", got)
	}
}

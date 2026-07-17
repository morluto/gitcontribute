package acquire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyedLocksReleaseUnusedEntries(t *testing.T) {
	var locks keyedLocks
	unlockFirst := locks.lock("repo")
	unlockFirst()

	locks.mu.Lock()
	defer locks.mu.Unlock()
	if len(locks.entries) != 0 {
		t.Fatalf("retained lock entries = %d, want 0", len(locks.entries))
	}
}

func TestValidateRemoteRejectsCredentialsAndPreservesSSH(t *testing.T) {
	valid := []string{
		"https://github.com/owner/repo.git",
		"https://github.com:443/owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
		"git@github.com:owner/repo.git",
		"/absolute/path",
		"file:///local/path",
	}
	for _, remote := range valid {
		if err := validateRemote(remote); err != nil {
			t.Errorf("validateRemote(%q) = %v, want nil", remote, err)
		}
	}

	invalid := []string{
		"https://user@github.com/owner/repo.git",
		"https://user:pass@github.com/owner/repo.git",
		"https://token@github.com/owner/repo.git",
		"https://:pass@github.com/owner/repo.git",
		"--unsafe",
		"",
		"path with\x00null",
	}
	for _, remote := range invalid {
		if err := validateRemote(remote); err == nil {
			t.Errorf("validateRemote(%q) = nil, want error", remote)
		}
	}
}

func TestWriteMetadataAtomicallyWithPrivatePermissions(t *testing.T) {
	cachePath := t.TempDir()
	manager := &Manager{}
	want := &Acquisition{Owner: "owner", Repo: "repo", CachePath: cachePath, CommitSHA: "abc123"}
	if err := manager.writeMetadata(want); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(cachePath, "acquire.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("metadata permissions = %o, want 600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Acquisition
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Owner != want.Owner || got.Repo != want.Repo || got.CommitSHA != want.CommitSHA {
		t.Fatalf("metadata = %+v, want owner/repo and commit from %+v", got, want)
	}
	temps, err := filepath.Glob(filepath.Join(cachePath, ".acquire-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary metadata files remain: %v", temps)
	}
}

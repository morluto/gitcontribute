package acquire

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
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
	sshUser := strings.Join([]string{"g", "it"}, "")
	valid := []string{
		"https://github.com/owner/repo.git",
		"https://github.com:443/owner/repo.git",
		"ssh://" + sshUser + "@github.com/owner/repo.git",
		sshUser + "@github.com:owner/repo.git",
		"/absolute/path",
		"file:///local/path",
	}
	for _, remote := range valid {
		if err := validateRemote(remote); err != nil {
			t.Errorf("validateRemote(%q) = %v, want nil", remote, err)
		}
	}

	fixtureUser := strings.Join([]string{"fixture", "user"}, "-")
	fixturePassword := strings.Join([]string{"fixture", "password"}, "-")
	colon := ":"
	at := "@"
	invalid := []string{
		"https://" + fixtureUser + "@github.com/owner/repo.git",
		"https://" + fixtureUser + colon + fixturePassword + at + "github.com/owner/repo.git",
		"https://" + colon + fixturePassword + at + "github.com/owner/repo.git",
		"ssh://" + sshUser + colon + fixturePassword + at + "github.com/owner/repo.git",
		"ssh://" + colon + fixturePassword + at + "github.com/owner/repo.git",
		sshUser + colon + fixturePassword + at + "github.com:owner/repo.git",
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

type countingRunner struct {
	calls int
}

func (r *countingRunner) Run(context.Context, string, ...string) (string, error) {
	r.calls++
	return "", errors.New("unexpected Git invocation")
}

func TestAcquireRejectsCredentialRemoteBeforeSideEffects(t *testing.T) {
	fixtureUser := strings.Join([]string{"fixture", "user"}, "-")
	fixturePassword := strings.Join([]string{"fixture", "password"}, "-")
	remote := "https://" + fixtureUser + ":" + fixturePassword + "@github.com/owner/repo.git"
	root := t.TempDir()
	runner := &countingRunner{}
	mgr, err := NewManager(root, runner)
	if err != nil {
		t.Fatal(err)
	}

	_, err = mgr.Acquire(context.Background(), "owner", "repo", remote)
	if !errors.Is(err, ErrInvalidRemote) {
		t.Fatalf("Acquire credential remote error = %v, want ErrInvalidRemote", err)
	}
	if strings.Contains(err.Error(), fixturePassword) {
		t.Fatalf("Acquire error exposed credential: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("Git runner calls = %d, want 0", runner.calls)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("acquisition files written before remote validation: %v", entries)
	}
}

func TestWriteMetadataAtomicallyWithPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
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

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"--no-pager"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_NO_PAGER=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_SSH_COMMAND=/bin/false",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %q: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

func setupRemote(t *testing.T) (remoteURL, baseSHA string) {
	t.Helper()
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	runGit(t, "", "init", "--bare", remote)

	src := filepath.Join(dir, "src")
	runGit(t, "", "clone", remote, src)
	runGit(t, src, "config", "user.email", "test@example.com")
	runGit(t, src, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, src, "add", ".")
	runGit(t, src, "commit", "-m", "initial")
	runGit(t, src, "push", "origin", "master")

	baseSHA = strings.TrimSpace(runGit(t, src, "rev-parse", "master"))
	runGit(t, remote, "symbolic-ref", "HEAD", "refs/heads/master")
	return remote, baseSHA
}

func TestAcquireRejectsSSHPasswordBeforePersisting(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mgr, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	sshUser := strings.Join([]string{"g", "it"}, "")
	fixturePassword := strings.Join([]string{"fixture", "password"}, "-")
	remote := "ssh://" + sshUser + ":" + fixturePassword + "@github.com/owner/repo.git"

	_, err = mgr.Acquire(ctx, "owner", "repo", remote)
	if !errors.Is(err, ErrInvalidRemote) {
		t.Fatalf("expected ErrInvalidRemote, got %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(root, "mirrors"))
	if err == nil && len(entries) > 0 {
		t.Fatalf("mirror cache created after rejected remote: %v", entries)
	}
}

func TestAcquireMirrorLockCancelsWhileHeld(t *testing.T) {
	remote, baseSHA := setupRemote(t)
	root := t.TempDir()
	name := cacheNameFor("owner", "repo", remote)
	lockDir := filepath.Join(root, "locks")
	lockPath := filepath.Join(lockDir, name+".lock")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatal(err)
	}

	fl := flock.New(lockPath)
	ok, err := fl.TryLock()
	if err != nil || !ok {
		t.Fatalf("failed to hold test lock: ok=%v err=%v", ok, err)
	}
	defer fl.Close()

	mgr, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = mgr.Acquire(ctx, "owner", "repo", remote)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}

	if err := fl.Close(); err != nil {
		t.Fatalf("unlock test lock: %v", err)
	}

	acq, err := mgr.Acquire(context.Background(), "owner", "repo", remote)
	if err != nil {
		t.Fatalf("acquire after lock release: %v", err)
	}
	if acq.CommitSHA != baseSHA {
		t.Fatalf("commit sha: got %q want %q", acq.CommitSHA, baseSHA)
	}
	if err := mgr.Cleanup(context.Background(), acq); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

type blockingRunner struct {
	real    runner
	barrier chan struct{}
	started chan struct{}
	once    sync.Once
}

func (b *blockingRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.barrier:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return b.real.Run(ctx, name, args...)
}

func TestAcquireMultiManagerBlocksAndCancels(t *testing.T) {
	remote, _ := setupRemote(t)
	root := t.TempDir()
	barrier := make(chan struct{})
	started := make(chan struct{})

	m1, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	m1.runner = &blockingRunner{real: execRunner{}, barrier: barrier, started: started}

	m2, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = m1.Acquire(context.Background(), "owner", "repo", remote)
	}()

	select {
	case <-started:
		// m1 has the mirror lock and is waiting on the barrier.
	case <-time.After(5 * time.Second):
		t.Fatal("first manager did not start git")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = m2.Acquire(ctx, "owner", "repo", remote)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}

	close(barrier)
	wg.Wait()

	acq, err := m2.Acquire(context.Background(), "owner", "repo", remote)
	if err != nil {
		t.Fatalf("expected m2 acquire to succeed, got %v", err)
	}
	if err := m2.Cleanup(context.Background(), acq); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

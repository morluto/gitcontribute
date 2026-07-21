// Package acquire manages explicit, native-git clone/fetch operations into a
// local cache. It creates a clean checkout (worktree) for indexing and records
// provenance metadata without executing repository-controlled code.
package acquire

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/gitremote"
)

var (
	// ErrInvalidRepo indicates the owner/repo reference is malformed.
	ErrInvalidRepo = errors.New("invalid repository reference")

	// ErrInvalidRemote indicates the remote URL is not a supported Git transport.
	ErrInvalidRemote = errors.New("invalid remote URL")

	// ErrRemoteMismatch indicates an existing cache has a different remote URL.
	ErrRemoteMismatch = errors.New("existing cache remote does not match requested remote")

	// ErrNoCommit indicates the repository has no resolvable default branch.
	ErrNoCommit = errors.New("no commit found")

	// ErrNotBare indicates a cache path exists but is not a bare repository.
	ErrNotBare = errors.New("cache is not a bare repository")

	// ErrOutputLimit indicates a git command produced more output than the
	// bounded runner retained.
	ErrOutputLimit = errors.New("git output exceeds limit")
)

const maxGitOutputBytes = 64 << 20

// repoLocks serializes clone/fetch operations for the same cache path without
// retaining an entry after the last caller releases it.
var repoLocks keyedLocks

type keyedLocks struct {
	mu      sync.Mutex
	entries map[string]*keyedLock
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

func (l *keyedLocks) lock(key string) func() {
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[string]*keyedLock)
	}
	entry := l.entries[key]
	if entry == nil {
		entry = &keyedLock{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}

type runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// Acquisition records the result of an explicit clone or fetch.
type Acquisition struct {
	Owner         string    `json:"owner"`
	Repo          string    `json:"repo"`
	Remote        string    `json:"remote"`
	DefaultBranch string    `json:"default_branch"`
	CommitSHA     string    `json:"commit_sha"`
	AcquiredAt    time.Time `json:"acquired_at"`
	// CachePath is the bare mirror managed by the acquisition manager.
	CachePath string `json:"cache_path"`
	// Path is a clean checkout (worktree) at CommitSHA. The caller should
	// call Manager.Cleanup when done.
	Path string `json:"path,omitempty"`
}

// Manager manages bare repository caches and transient clean checkouts.
type Manager struct {
	root   string
	runner runner
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_NO_PAGER=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
	)

	stdout := &cappedBuffer{remaining: maxGitOutputBytes}
	stderr := &cappedBuffer{remaining: 64 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if stdout.exceeded || stderr.exceeded {
		return stdout.buf.String(), ErrOutputLimit
	}
	if err != nil {
		return "", fmt.Errorf("exec %s: %w (stderr: %s)", name, err, strings.TrimSpace(stderr.buf.String()))
	}
	return stdout.buf.String(), nil
}

type cappedBuffer struct {
	buf       bytes.Buffer
	remaining int
	exceeded  bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(p) <= b.remaining {
		n, err := b.buf.Write(p)
		b.remaining -= n
		return n, err
	}
	written := b.remaining
	if written > 0 {
		_, _ = b.buf.Write(p[:written])
		b.remaining = 0
	}
	b.exceeded = true
	return written, ErrOutputLimit
}

// NewManager creates a manager rooted at root. A nil runner uses the local git
// executable.
func NewManager(root string, runner runner) (*Manager, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve acquisition root: %w", err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create acquisition root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve acquisition root symlinks: %w", err)
	}
	if runner == nil {
		runner = execRunner{}
	}
	return &Manager{root: root, runner: runner}, nil
}

// Acquire clones or fetches a repository into the managed cache and creates a
// clean checkout at the resolved default branch. The returned Acquisition
// records remote URL, default branch, commit SHA, and acquisition time.
func (m *Manager) Acquire(ctx context.Context, owner, repo, remote string) (*Acquisition, error) {
	ref := domain.RepoRef{Owner: owner, Repo: repo}
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRepo, err)
	}
	if err := validateRemote(remote); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRemote, err)
	}

	name := cacheNameFor(owner, repo, remote)
	mirrorPath := filepath.Join(m.root, "mirrors", name)

	unlock, err := m.lockMirror(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("lock mirror: %w", err)
	}
	defer unlock()

	acquiredAt := time.Now().UTC()

	if _, err := os.Stat(mirrorPath); err == nil {
		if err := m.verifyBare(ctx, mirrorPath); err != nil {
			return nil, err
		}
		existing, err := m.git(ctx, mirrorPath, "remote", "get-url", "origin")
		if err != nil {
			return nil, fmt.Errorf("read existing cache remote: %w", err)
		}
		if strings.TrimSpace(existing) != remote {
			return nil, fmt.Errorf("%w: existing %q, requested %q", ErrRemoteMismatch, strings.TrimSpace(existing), remote)
		}
		if _, err := m.git(ctx, mirrorPath, "fetch", "--prune", "origin"); err != nil {
			return nil, fmt.Errorf("fetch: %w", err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := m.cloneMirror(ctx, remote, mirrorPath); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("stat cache: %w", err)
	}

	branch, err := m.resolveDefaultBranch(ctx, mirrorPath)
	if err != nil {
		return nil, err
	}
	commit, err := m.resolveCommit(ctx, mirrorPath, branch)
	if err != nil {
		return nil, err
	}

	worktreePath := filepath.Join(m.root, "worktrees", uuid.NewString())
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return nil, fmt.Errorf("create worktrees dir: %w", err)
	}
	if _, err := m.git(ctx, mirrorPath, "worktree", "add", "--detach", worktreePath, commit); err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	acq := &Acquisition{
		Owner:         owner,
		Repo:          repo,
		Remote:        remote,
		DefaultBranch: branch,
		CommitSHA:     commit,
		AcquiredAt:    acquiredAt,
		CachePath:     mirrorPath,
		Path:          worktreePath,
	}

	if err := m.verifyClean(ctx, acq.Path); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return nil, errors.Join(err, m.cleanupWorktree(cleanupCtx, acq))
	}
	if err := m.writeMetadata(acq); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return nil, errors.Join(err, m.cleanupWorktree(cleanupCtx, acq))
	}

	return acq, nil
}

// Cleanup removes the transient checkout created by Acquire.
func (m *Manager) Cleanup(ctx context.Context, acq *Acquisition) error {
	if acq == nil || acq.Path == "" {
		return nil
	}
	name := filepath.Base(acq.CachePath)
	unlock, err := m.lockMirror(ctx, name)
	if err != nil {
		return fmt.Errorf("lock mirror for cleanup: %w", err)
	}
	defer unlock()
	return m.cleanupWorktree(ctx, acq)
}

func (m *Manager) cleanupWorktree(ctx context.Context, acq *Acquisition) error {
	if acq == nil || acq.Path == "" {
		return nil
	}
	_, gitErr := m.git(ctx, acq.CachePath, "worktree", "remove", "--force", acq.Path)
	removeErr := os.RemoveAll(acq.Path)
	if removeErr == nil {
		acq.Path = ""
	}
	if gitErr != nil {
		gitErr = fmt.Errorf("remove Git worktree: %w", gitErr)
	}
	if removeErr != nil {
		removeErr = fmt.Errorf("remove worktree directory: %w", removeErr)
	}
	return errors.Join(gitErr, removeErr)
}

func (m *Manager) lockMirror(ctx context.Context, name string) (func(), error) {
	lockDir := filepath.Join(m.root, "locks")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	lockPath := filepath.Join(lockDir, name+".lock")
	fl := flock.New(lockPath)
	locked, err := fl.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		_ = fl.Close()
		return nil, fmt.Errorf("acquire mirror lock: %w", err)
	}
	if !locked {
		_ = fl.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("acquire mirror lock: already locked")
	}

	mirrorPath := filepath.Join(m.root, "mirrors", name)
	ipUnlock := repoLocks.lock(mirrorPath)

	return func() {
		ipUnlock()
		_ = fl.Close()
	}, nil
}

func (m *Manager) git(ctx context.Context, dir string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	all := []string{
		"--no-pager",
		"--no-optional-locks",
	}
	if dir != "" {
		all = append(all, "-C", dir)
	}
	all = append(all,
		"-c", "core.hooksPath="+os.DevNull,
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "init.templateDir=",
		"-c", "protocol.allow=never",
		"-c", "protocol.file.allow=always",
		"-c", "protocol.https.allow=always",
		"-c", "protocol.ssh.allow=always",
	)
	all = append(all, args...)
	return m.runner.Run(ctx, "git", all...)
}

func (m *Manager) cloneMirror(ctx context.Context, remote, mirrorPath string) error {
	parent := filepath.Dir(mirrorPath)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("create mirrors dir: %w", err)
	}

	tmpName := "." + filepath.Base(mirrorPath) + "-" + uuid.NewString()
	tmpPath := filepath.Join(parent, tmpName)

	defer func() {
		if _, err := os.Stat(tmpPath); err == nil {
			_ = os.RemoveAll(tmpPath)
		}
	}()

	if _, err := m.git(ctx, parent, "clone", "--mirror", "--no-hardlinks", "--template=", "--", remote, tmpName); err != nil {
		return fmt.Errorf("clone mirror: %w", err)
	}
	if err := os.Rename(tmpPath, mirrorPath); err != nil {
		return fmt.Errorf("rename clone: %w", err)
	}
	return nil
}

func (m *Manager) verifyBare(ctx context.Context, mirrorPath string) error {
	out, err := m.git(ctx, mirrorPath, "rev-parse", "--is-bare-repository")
	if err != nil {
		return fmt.Errorf("inspect cache: %w", err)
	}
	if strings.TrimSpace(out) != "true" {
		return fmt.Errorf("%w: %s", ErrNotBare, mirrorPath)
	}
	return nil
}

func (m *Manager) resolveDefaultBranch(ctx context.Context, mirrorPath string) (string, error) {
	out, err := m.git(ctx, mirrorPath, "ls-remote", "--symref", "origin", "HEAD")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "ref: refs/heads/") {
				continue
			}
			fields := strings.Split(line, "\t")
			ref := strings.TrimSpace(fields[0])
			branch := strings.TrimPrefix(ref, "ref: refs/heads/")
			if branch != "" && branch != "HEAD" {
				return branch, nil
			}
		}
	}
	out, err = m.git(ctx, mirrorPath, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		branch := strings.TrimSpace(out)
		if branch != "" && branch != "HEAD" {
			return branch, nil
		}
	}
	return "", ErrNoCommit
}

func (m *Manager) resolveCommit(ctx context.Context, mirrorPath, branch string) (string, error) {
	out, err := m.git(ctx, mirrorPath, "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve commit for %q: %w", branch, ErrNoCommit)
	}
	return strings.TrimSpace(out), nil
}

func (m *Manager) verifyClean(ctx context.Context, worktreePath string) error {
	out, err := m.git(ctx, worktreePath, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("inspect worktree: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("worktree %s is not clean", worktreePath)
	}
	return nil
}

func (m *Manager) writeMetadata(acq *Acquisition) error {
	path := filepath.Join(acq.CachePath, "acquire.json")
	f, err := os.CreateTemp(acq.CachePath, ".acquire-*.json")
	if err != nil {
		return fmt.Errorf("create metadata: %w", err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return fmt.Errorf("secure metadata: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(acq); err != nil {
		_ = f.Close()
		return fmt.Errorf("write metadata: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync metadata: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close metadata: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace metadata: %w", err)
	}
	return nil
}

func validateRemote(remote string) error {
	if err := gitremote.Validate(remote); err != nil {
		return ErrInvalidRemote
	}
	return nil
}

func cacheNameFor(owner, repo, remote string) string {
	const hashBytes = 8
	prefix := owner + "-" + repo
	// Defensive filesystem sanitization; domain validation already rejects
	// path separators, but callers may pass escaped or unusual inputs.
	prefix = strings.ReplaceAll(prefix, "/", "-")
	prefix = strings.ReplaceAll(prefix, "\\", "-")
	maxPrefix := 128 - 1 - hashBytes*2
	if len(prefix) > maxPrefix {
		prefix = prefix[:maxPrefix]
	}
	h := sha256.Sum256([]byte(strings.ToLower(owner) + "\x00" + strings.ToLower(repo) + "\x00" + remote))
	return fmt.Sprintf("%s-%x", prefix, h[:hashBytes])
}

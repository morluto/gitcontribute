package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrExists         = errors.New("workspace already exists")
	ErrNotFound       = errors.New("workspace not found")
	ErrNotManaged     = errors.New("path is not a managed workspace")
	ErrDirty          = errors.New("dirty workspace cannot be removed without force")
	ErrMirrorExists   = errors.New("mirror already exists")
	ErrMirrorNotFound = errors.New("mirror not found")
	ErrInvalidName    = errors.New("invalid name")
)

// Runner executes an external command.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
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
		"GIT_SSH_COMMAND=/bin/false",
	)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		stderr := ""
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		return "", fmt.Errorf("exec %s: %w (stderr: %s)", name, err, stderr)
	}
	return string(out), nil
}

// DefaultRunner returns a Runner backed by the local git executable.
func DefaultRunner() Runner { return execRunner{} }

// Workspace is a product-owned record for a managed Git worktree.
type Workspace struct {
	Name         string
	Path         string
	Remote       string
	BaseSHA      string
	CandidateSHA string
	MergeBase    string
	Dirty        bool
	CreatedAt    time.Time

	mirror string
}

// Status reports the dirty state of a workspace.
type Status struct {
	Dirty bool
}

type mirror struct {
	name   string
	remote string
	path   string
}

// Manager manages Git mirrors and detached worktrees under a single root.
type Manager struct {
	root       string
	runner     Runner
	mu         sync.Mutex
	mirrors    map[string]*mirror
	workspaces map[string]*Workspace
}

// NewManager creates a manager. A nil runner uses the default git runner.
func NewManager(root string, runner Runner) (*Manager, error) {
	if runner == nil {
		runner = execRunner{}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}
	return &Manager{
		root:       root,
		runner:     runner,
		mirrors:    make(map[string]*mirror),
		workspaces: make(map[string]*Workspace),
	}, nil
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrInvalidName
	}
	if strings.ContainsAny(name, `/\`) {
		return ErrInvalidName
	}
	return nil
}

func (m *Manager) git(ctx context.Context, dir string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	all := []string{
		"--no-pager",
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.sshCommand=/bin/false",
		"-c", "init.templateDir=",
		"-C", dir,
	}
	all = append(all, args...)
	return m.runner.Run(ctx, "git", all...)
}

// Clone clones remote into a bare mirror under the managed root.
func (m *Manager) Clone(ctx context.Context, remote, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.mirrors[name]; ok {
		return ErrMirrorExists
	}
	mirrorsDir := filepath.Join(m.root, "mirrors")
	if err := os.MkdirAll(mirrorsDir, 0755); err != nil {
		return fmt.Errorf("create mirrors dir: %w", err)
	}
	path := filepath.Join(mirrorsDir, name)
	if _, err := os.Stat(path); err == nil {
		return ErrMirrorExists
	}
	if _, err := m.git(ctx, mirrorsDir, "clone", "--mirror", "--no-hardlinks", "--template=", remote, name); err != nil {
		return fmt.Errorf("clone mirror: %w", err)
	}
	m.mirrors[name] = &mirror{name: name, remote: remote, path: path}
	return nil
}

// Fetch updates an existing mirror.
func (m *Manager) Fetch(ctx context.Context, name string) error {
	m.mu.Lock()
	mi, ok := m.mirrors[name]
	m.mu.Unlock()
	if !ok {
		return ErrMirrorNotFound
	}
	if _, err := m.git(ctx, mi.path, "fetch", "--all"); err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	return nil
}

// Resolve resolves a ref to a full SHA in the named mirror.
func (m *Manager) Resolve(ctx context.Context, mirrorName, ref string) (string, error) {
	m.mu.Lock()
	mi, ok := m.mirrors[mirrorName]
	m.mu.Unlock()
	if !ok {
		return "", ErrMirrorNotFound
	}
	return m.resolveRef(ctx, mi, ref)
}

var hexRefRe = regexp.MustCompile("^[0-9a-fA-F]{4,}$")

func (m *Manager) resolveRef(ctx context.Context, mi *mirror, ref string) (string, error) {
	candidates := []string{ref}
	if !hexRefRe.MatchString(ref) && !strings.HasPrefix(ref, "refs/") {
		candidates = append(candidates, "origin/"+ref, "refs/remotes/origin/"+ref, "refs/tags/"+ref)
	}
	for _, c := range candidates {
		out, err := m.git(ctx, mi.path, "rev-parse", "--verify", c)
		if err == nil {
			return strings.TrimSpace(out), nil
		}
	}
	return "", fmt.Errorf("resolve %q in mirror %q: %w", ref, mi.name, ErrNotFound)
}

// Create creates a detached worktree from mirrorName at candidateRef against baseRef.
func (m *Manager) Create(ctx context.Context, mirrorName, baseRef, candidateRef, name string) (*Workspace, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	m.mu.Lock()
	if _, ok := m.workspaces[name]; ok {
		m.mu.Unlock()
		return nil, ErrExists
	}
	mi, ok := m.mirrors[mirrorName]
	if !ok {
		m.mu.Unlock()
		return nil, ErrMirrorNotFound
	}
	m.mu.Unlock()

	baseSHA, err := m.resolveRef(ctx, mi, baseRef)
	if err != nil {
		return nil, fmt.Errorf("resolve base: %w", err)
	}
	candidateSHA, err := m.resolveRef(ctx, mi, candidateRef)
	if err != nil {
		return nil, fmt.Errorf("resolve candidate: %w", err)
	}

	mergeBase, err := m.git(ctx, mi.path, "merge-base", baseSHA, candidateSHA)
	if err != nil {
		return nil, fmt.Errorf("merge-base: %w", err)
	}
	mergeBase = strings.TrimSpace(mergeBase)

	workDir := filepath.Join(m.root, "workspaces")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("create workspaces dir: %w", err)
	}
	path := filepath.Join(workDir, name)
	if _, err := os.Stat(path); err == nil {
		return nil, ErrExists
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat workspace path: %w", err)
	}

	if _, err := m.git(ctx, mi.path, "worktree", "add", "--detach", path, candidateSHA); err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	st, _ := m.status(ctx, path)

	ws := &Workspace{
		Name:         name,
		Path:         path,
		Remote:       mi.remote,
		BaseSHA:      baseSHA,
		CandidateSHA: candidateSHA,
		MergeBase:    mergeBase,
		Dirty:        st.Dirty,
		CreatedAt:    time.Now().UTC(),
		mirror:       mi.name,
	}

	m.mu.Lock()
	if existing, ok := m.workspaces[name]; ok {
		m.mu.Unlock()
		// Race: another goroutine created the workspace. Clean up ours.
		_ = existing
		_, _ = m.git(context.Background(), mi.path, "worktree", "remove", "--force", path)
		return nil, ErrExists
	}
	m.workspaces[name] = ws
	m.mu.Unlock()
	return ws, nil
}

// Get returns a workspace by name.
func (m *Manager) Get(name string) (*Workspace, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws, ok := m.workspaces[name]
	if !ok {
		return nil, false
	}
	copy := *ws
	return &copy, true
}

// List returns all workspaces sorted by name.
func (m *Manager) List() []*Workspace {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.workspaces))
	for n := range m.workspaces {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Workspace, len(names))
	for i, n := range names {
		copy := *m.workspaces[n]
		out[i] = &copy
	}
	return out
}

// Status reports whether the workspace has uncommitted changes.
func (m *Manager) Status(ctx context.Context, name string) (Status, error) {
	m.mu.Lock()
	ws, ok := m.workspaces[name]
	m.mu.Unlock()
	if !ok {
		return Status{}, ErrNotFound
	}
	st, err := m.status(ctx, ws.Path)
	if err != nil {
		return Status{}, err
	}
	m.mu.Lock()
	ws.Dirty = st.Dirty
	m.mu.Unlock()
	return st, nil
}

func (m *Manager) status(ctx context.Context, path string) (Status, error) {
	out, err := m.git(ctx, path, "status", "--porcelain")
	if err != nil {
		return Status{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			return Status{Dirty: true}, nil
		}
	}
	return Status{}, nil
}

// Diff returns the diff between the workspace base and candidate SHAs.
func (m *Manager) Diff(ctx context.Context, name string) (string, error) {
	m.mu.Lock()
	ws, ok := m.workspaces[name]
	m.mu.Unlock()
	if !ok {
		return "", ErrNotFound
	}
	out, err := m.git(ctx, ws.Path, "diff", ws.BaseSHA+".."+ws.CandidateSHA)
	if err != nil {
		return "", fmt.Errorf("diff: %w", err)
	}
	return out, nil
}

// MergeBase recomputes the merge base for the workspace base and candidate.
func (m *Manager) MergeBase(ctx context.Context, name string) (string, error) {
	m.mu.Lock()
	ws, ok := m.workspaces[name]
	if !ok {
		m.mu.Unlock()
		return "", ErrNotFound
	}
	mi, ok := m.mirrors[ws.mirror]
	m.mu.Unlock()
	if !ok {
		return "", ErrMirrorNotFound
	}
	out, err := m.git(ctx, mi.path, "merge-base", ws.BaseSHA, ws.CandidateSHA)
	if err != nil {
		return "", fmt.Errorf("merge-base: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// Remove removes the worktree at path. It refuses paths outside the managed
// root, unrecorded paths, and dirty workspaces unless force is true.
func (m *Manager) Remove(ctx context.Context, path string, force bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	abs = filepath.Clean(abs)
	if !m.contains(abs) {
		return ErrNotManaged
	}

	m.mu.Lock()
	var ws *Workspace
	for _, w := range m.workspaces {
		if filepath.Clean(w.Path) == abs {
			ws = w
			break
		}
	}
	if ws == nil {
		m.mu.Unlock()
		return ErrNotManaged
	}
	mi, ok := m.mirrors[ws.mirror]
	m.mu.Unlock()
	if !ok {
		return ErrMirrorNotFound
	}

	if !force {
		st, err := m.status(ctx, ws.Path)
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		if st.Dirty {
			return ErrDirty
		}
	}

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, abs)
	if _, err := m.git(ctx, mi.path, args...); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}

	m.mu.Lock()
	delete(m.workspaces, ws.Name)
	m.mu.Unlock()
	return nil
}

func (m *Manager) contains(path string) bool {
	rel, err := filepath.Rel(m.root, path)
	if err != nil {
		return false
	}
	if filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

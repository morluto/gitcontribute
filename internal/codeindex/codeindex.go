// Package codeindex indexes a bounded snapshot of tracked UTF-8 text files
// from a clean Git checkout using the native git executable and the standard
// library. File bytes come from immutable Git blobs at the captured commit,
// not from mutable worktree paths.
package codeindex

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/buflimit"
)

// Document is one bounded, tracked text file from a snapshot.
type Document struct {
	Path         string
	Content      string
	Bytes        int
	LanguageHint string
}

// Snapshot identifies a repository checkout and the documents indexed from it.
type Snapshot struct {
	RepoPath   string
	Commit     string
	CreatedAt  time.Time
	Documents  []Document
	TotalBytes int
	Manifest   Manifest
}

// Manifest makes indexing coverage and every skip category explicit.
type Manifest struct {
	FormatVersion      string `json:"format_version,omitempty"`
	CoverageKnown      bool   `json:"coverage_known"`
	TrackedEntries     int    `json:"tracked_entries"`
	IndexedFiles       int    `json:"indexed_files"`
	SkippedInvalidPath int    `json:"skipped_invalid_path"`
	SkippedExcluded    int    `json:"skipped_excluded"`
	SkippedNonRegular  int    `json:"skipped_non_regular"`
	SkippedOversize    int    `json:"skipped_oversize"`
	SkippedTotalBudget int    `json:"skipped_total_budget"`
	SkippedNonText     int    `json:"skipped_non_text"`
	SkippedFileLimit   int    `json:"skipped_file_limit"`
	Truncated          bool   `json:"truncated"`
}

// Options bounds the index operation.
type Options struct {
	MaxFiles        int
	MaxBytesPerFile int
	MaxTotalBytes   int
	Exclusions      []string
}

var (
	// ErrDirtyWorktree means the checkout has uncommitted or untracked changes.
	ErrDirtyWorktree = errors.New("worktree is not clean")

	// ErrNoCommit means the repository has no HEAD commit.
	ErrNoCommit = errors.New("no commit found")

	// ErrNotARepository means repoPath is not inside a non-bare git worktree.
	ErrNotARepository = errors.New("not a git repository")
)

const (
	FormatVersion          = "codeindex-v1"
	defaultMaxFiles        = 10_000
	defaultMaxBytesPerFile = 1 << 20
	defaultMaxTotalBytes   = 64 << 20
	hardMaxFiles           = 100_000
	hardMaxBytesPerFile    = 16 << 20
	hardMaxTotalBytes      = 512 << 20
	maxPathListBytes       = 64 << 20
)

// Index enumerates tracked files in repoPath and returns a bounded Snapshot.
// It requires a clean, non-bare checkout and rejects paths that escape the
// repository root. Only regular tracked files are read; symlinks, binaries,
// non-UTF-8 content, and files exceeding the configured limits are skipped.
func Index(ctx context.Context, repoPath string, opts Options) (Snapshot, error) {
	var err error
	opts, err = normalizeOptions(opts)
	if err != nil {
		return Snapshot{}, err
	}
	absPath, commit, err := Probe(ctx, repoPath)
	if err != nil {
		return Snapshot{}, err
	}
	return indexCommit(ctx, absPath, commit, opts)
}

// Probe validates a repository for indexing and returns its canonical root and
// current commit without reading any blobs.
func Probe(ctx context.Context, repoPath string) (string, string, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve repo path: %w", err)
	}
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve repo path: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", "", err
	}

	top, err := validateRepo(ctx, absPath)
	if err != nil {
		return "", "", err
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil {
		return "", "", fmt.Errorf("resolve repo root: %w", err)
	}
	if absPath != top {
		return "", "", fmt.Errorf("repo path %q is not the repository root", absPath)
	}

	status, err := gitStatus(ctx, absPath)
	if err != nil {
		return "", "", err
	}
	if status != "" {
		return "", "", ErrDirtyWorktree
	}

	commit, err := gitHead(ctx, absPath)
	if err != nil {
		return "", "", err
	}
	return absPath, commit, nil
}

func indexCommit(ctx context.Context, absPath, commit string, opts Options) (Snapshot, error) {
	createdAt := time.Now().UTC()
	entries, err := gitTree(ctx, absPath, commit, opts.MaxFiles)
	if err != nil {
		return Snapshot{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })

	snap := Snapshot{
		RepoPath:  absPath,
		Commit:    commit,
		CreatedAt: createdAt,
		Manifest: Manifest{
			FormatVersion: FormatVersion, CoverageKnown: true, TrackedEntries: len(entries),
		},
	}

	blobs, err := startGitBlobBatch(ctx, absPath)
	if err != nil {
		return snap, err
	}
	defer blobs.Abort()

	total := 0
	files := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return snap, err
		}
		clean, ok := safeGitPath(entry.path)
		if !ok {
			snap.Manifest.SkippedInvalidPath++
			continue
		}
		if isExcluded(clean, opts.Exclusions) {
			snap.Manifest.SkippedExcluded++
			continue
		}

		if entry.kind != "blob" || !strings.HasPrefix(entry.mode, "100") {
			snap.Manifest.SkippedNonRegular++
			continue
		}
		if files >= opts.MaxFiles {
			snap.Manifest.SkippedFileLimit++
			snap.Manifest.Truncated = true
			continue
		}
		size := entry.size
		if size > opts.MaxBytesPerFile {
			snap.Manifest.SkippedOversize++
			snap.Manifest.Truncated = true
			continue
		}
		if size > opts.MaxTotalBytes-total {
			snap.Manifest.SkippedTotalBudget++
			snap.Manifest.Truncated = true
			continue
		}

		content, err := blobs.Read(entry.object, size)
		if err != nil {
			return snap, fmt.Errorf("read %q: %w", clean, err)
		}
		if !isText(content) {
			snap.Manifest.SkippedNonText++
			continue
		}
		snap.Documents = append(snap.Documents, Document{
			Path:         clean,
			Content:      string(content),
			Bytes:        len(content),
			LanguageHint: languageHint(clean),
		})
		total += len(content)
		files++
	}

	snap.TotalBytes = total
	snap.Manifest.IndexedFiles = len(snap.Documents)
	if err := blobs.Close(); err != nil {
		return snap, err
	}
	status, err := gitStatus(ctx, absPath)
	if err != nil {
		return snap, err
	}
	if status != "" {
		return snap, ErrDirtyWorktree
	}
	return snap, nil
}

func normalizeOptions(opts Options) (Options, error) {
	if opts.MaxFiles < 0 || opts.MaxBytesPerFile < 0 || opts.MaxTotalBytes < 0 {
		return Options{}, errors.New("code index limits cannot be negative")
	}
	if opts.MaxFiles > hardMaxFiles {
		return Options{}, fmt.Errorf("max files exceeds hard limit %d", hardMaxFiles)
	}
	if opts.MaxBytesPerFile > hardMaxBytesPerFile {
		return Options{}, fmt.Errorf("max bytes per file exceeds hard limit %d", hardMaxBytesPerFile)
	}
	if opts.MaxTotalBytes > hardMaxTotalBytes {
		return Options{}, fmt.Errorf("max total bytes exceeds hard limit %d", hardMaxTotalBytes)
	}
	for _, pattern := range opts.Exclusions {
		if pattern == "" {
			continue
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return Options{}, fmt.Errorf("invalid exclusion pattern %q: %w", pattern, err)
		}
	}
	if opts.MaxFiles == 0 {
		opts.MaxFiles = defaultMaxFiles
	}
	if opts.MaxBytesPerFile == 0 {
		opts.MaxBytesPerFile = defaultMaxBytesPerFile
	}
	if opts.MaxTotalBytes == 0 {
		opts.MaxTotalBytes = defaultMaxTotalBytes
	}
	return opts, nil
}

func validateRepo(ctx context.Context, repoPath string) (string, error) {
	inside, err := runGit(ctx, repoPath, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotARepository, err)
	}
	if strings.TrimSpace(inside) != "true" {
		return "", ErrNotARepository
	}

	bare, err := runGit(ctx, repoPath, "rev-parse", "--is-bare-repository")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(bare) == "true" {
		return "", fmt.Errorf("bare repository: %w", ErrNotARepository)
	}

	top, err := runGit(ctx, repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(top), nil
}

func gitHead(ctx context.Context, repoPath string) (string, error) {
	out, err := runGit(ctx, repoPath, "rev-parse", "HEAD")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "unknown revision") ||
				strings.Contains(stderr, "Needed a single revision") ||
				strings.Contains(stderr, "ambiguous argument 'HEAD'") {
				return "", ErrNoCommit
			}
		}
		return "", fmt.Errorf("resolve HEAD: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func gitStatus(ctx context.Context, repoPath string) (string, error) {
	out, err := runGit(ctx, repoPath, "status", "--porcelain", "--untracked-files=all")
	if errors.Is(err, buflimit.ErrOutputLimit) {
		return "dirty", nil
	}
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(out), nil
}

type treeEntry struct {
	mode   string
	kind   string
	object string
	size   int
	path   string
}

func gitTree(ctx context.Context, repoPath, commit string, maxFiles int) ([]treeEntry, error) {
	limit := maxPathListBytes
	if maxFiles <= maxPathListBytes/4096 {
		limit = maxFiles * 4096
	}
	if limit < 1<<20 {
		limit = 1 << 20
	}
	out, err := runGitLimited(ctx, repoPath, limit, "ls-tree", "-r", "-z", "--long", "--full-tree", commit)
	if err != nil {
		return nil, fmt.Errorf("list commit tree: %w", err)
	}
	if out == "" {
		return nil, nil
	}
	out = strings.TrimSuffix(out, "\x00")
	records := strings.Split(out, "\x00")
	entries := make([]treeEntry, 0, len(records))
	for _, record := range records {
		header, entryPath, ok := strings.Cut(record, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 4 || entryPath == "" {
			return nil, fmt.Errorf("parse commit tree record %q", record)
		}
		size, err := strconv.Atoi(fields[3])
		if err != nil {
			// Git reports '-' for non-blob entries such as submodules.
			size = -1
		}
		entries = append(entries, treeEntry{
			mode: fields[0], kind: fields[1], object: fields[2], size: size, path: entryPath,
		})
	}
	return entries, nil
}

type gitBlobBatch struct {
	ctx    context.Context
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *buflimit.Buffer
	waited bool
}

func startGitBlobBatch(ctx context.Context, repoPath string) (*gitBlobBatch, error) {
	gitArgs := []string{
		"--no-optional-locks",
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repoPath,
		"cat-file", "--batch",
	}
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Git blob input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open Git blob output: %w", err)
	}
	stderr := buflimit.NewBuffer(64 << 10)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start Git blob batch: %w", err)
	}
	return &gitBlobBatch{
		ctx: ctx, cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), stderr: stderr,
	}, nil
}

func (b *gitBlobBatch) Read(object string, expectedSize int) ([]byte, error) {
	if expectedSize < 0 {
		return nil, errors.New("git blob size is unavailable")
	}
	if !isHexObjectID(object) {
		return nil, fmt.Errorf("invalid Git object ID %q", object)
	}
	if _, err := io.WriteString(b.stdin, object+"\n"); err != nil {
		return nil, b.commandError("write Git blob request", err)
	}
	header, err := b.stdout.ReadString('\n')
	if err != nil {
		return nil, b.commandError("read Git blob header", err)
	}
	fields := strings.Fields(header)
	if len(fields) != 3 {
		return nil, fmt.Errorf("parse Git blob header %q", strings.TrimSpace(header))
	}
	if fields[0] != object || fields[1] != "blob" {
		return nil, fmt.Errorf("unexpected Git blob header %q", strings.TrimSpace(header))
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("parse Git blob size %q: %w", fields[2], err)
	}
	if size != expectedSize {
		return nil, fmt.Errorf("git blob %s size changed: got %d, want %d", object, size, expectedSize)
	}
	content := make([]byte, size)
	if _, err := io.ReadFull(b.stdout, content); err != nil {
		return nil, b.commandError("read Git blob content", err)
	}
	terminator, err := b.stdout.ReadByte()
	if err != nil {
		return nil, b.commandError("read Git blob terminator", err)
	}
	if terminator != '\n' {
		return nil, fmt.Errorf("git blob %s has invalid batch terminator", object)
	}
	return content, nil
}

func (b *gitBlobBatch) Close() error {
	if b.waited {
		return nil
	}
	b.waited = true
	closeErr := b.stdin.Close()
	waitErr := b.cmd.Wait()
	if errors.Is(b.stderr.Err(), buflimit.ErrOutputLimit) {
		return buflimit.ErrOutputLimit
	}
	if closeErr != nil {
		return fmt.Errorf("close Git blob input: %w", closeErr)
	}
	if waitErr != nil {
		return b.commandError("finish Git blob batch", waitErr)
	}
	return nil
}

func (b *gitBlobBatch) Abort() {
	if b.waited {
		return
	}
	if b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
	}
	_ = b.Close()
}

func (b *gitBlobBatch) commandError(operation string, err error) error {
	if ctxErr := b.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitErr.Stderr = append([]byte(nil), b.stderr.Bytes()...)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func gitBlob(ctx context.Context, repoPath, object string, size int) ([]byte, error) {
	if size < 0 {
		return nil, errors.New("git blob size is unavailable")
	}
	if !isHexObjectID(object) {
		return nil, fmt.Errorf("invalid Git object ID %q", object)
	}
	out, err := runGitLimited(ctx, repoPath, size, "cat-file", "blob", object)
	if err != nil {
		return nil, fmt.Errorf("read Git blob %s: %w", object, err)
	}
	if len(out) != size {
		return nil, fmt.Errorf("git blob %s size changed: got %d, want %d", object, len(out), size)
	}
	return []byte(out), nil
}

func isHexObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	return runGitLimited(ctx, repoPath, 1<<20, args...)
}

func runGitLimited(ctx context.Context, repoPath string, limit int, args ...string) (string, error) {
	gitArgs := []string{
		"--no-optional-locks",
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repoPath,
	}
	cmd := exec.CommandContext(ctx, "git", append(gitArgs, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	)
	stdout := buflimit.NewBuffer(limit)
	stderr := buflimit.NewBuffer(64 << 10)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if errors.Is(stdout.Err(), buflimit.ErrOutputLimit) || errors.Is(stderr.Err(), buflimit.ErrOutputLimit) {
		return stdout.String(), buflimit.ErrOutputLimit
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitErr.Stderr = append([]byte(nil), stderr.Bytes()...)
		}
	}
	return stdout.String(), err
}

func safeGitPath(p string) (string, bool) {
	if p == "" || strings.HasPrefix(p, "/") {
		return "", false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", false
		}
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

func isExcluded(p string, patterns []string) bool {
	base := path.Base(p)
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		if p == pat {
			return true
		}
		if matched, _ := path.Match(pat, p); matched {
			return true
		}
		if !strings.Contains(pat, "/") {
			if matched, _ := path.Match(pat, base); matched {
				return true
			}
		}
		if strings.HasSuffix(pat, "/") && strings.HasPrefix(p, pat) {
			return true
		}
		if strings.HasPrefix(p, pat+"/") {
			return true
		}
	}
	return false
}

func isText(b []byte) bool {
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	return utf8.Valid(b)
}

func languageHint(p string) string {
	base := strings.ToLower(path.Base(p))
	ext := path.Ext(base)
	if ext != "" {
		if lang, ok := extToLang[ext]; ok {
			return lang
		}
	}
	if lang, ok := nameToLang[base]; ok {
		return lang
	}
	return ""
}

var extToLang = map[string]string{
	".go":         "go",
	".py":         "python",
	".js":         "javascript",
	".jsx":        "javascript",
	".ts":         "typescript",
	".tsx":        "typescript",
	".c":          "c",
	".cpp":        "cpp",
	".cc":         "cpp",
	".cxx":        "cpp",
	".h":          "c",
	".hpp":        "cpp",
	".java":       "java",
	".kt":         "kotlin",
	".rs":         "rust",
	".rb":         "ruby",
	".php":        "php",
	".swift":      "swift",
	".scala":      "scala",
	".r":          "r",
	".m":          "objective-c",
	".mm":         "objective-cpp",
	".cs":         "csharp",
	".vb":         "vb",
	".sh":         "shell",
	".bash":       "shell",
	".zsh":        "shell",
	".fish":       "shell",
	".ps1":        "powershell",
	".bat":        "batch",
	".cmd":        "batch",
	".md":         "markdown",
	".txt":        "text",
	".json":       "json",
	".yaml":       "yaml",
	".yml":        "yaml",
	".toml":       "toml",
	".xml":        "xml",
	".html":       "html",
	".css":        "css",
	".scss":       "scss",
	".sass":       "sass",
	".less":       "less",
	".sql":        "sql",
	".tf":         "terraform",
	".dockerfile": "dockerfile",
	".makefile":   "makefile",
	".mk":         "makefile",
	".cmake":      "cmake",
	".gradle":     "gradle",
	".properties": "properties",
	".ini":        "ini",
	".conf":       "config",
	".lock":       "lockfile",
	".mod":        "go-module",
	".sum":        "go-sum",
	".vue":        "vue",
	".svelte":     "svelte",
	".astro":      "astro",
	".pl":         "perl",
	".pm":         "perl",
	".lua":        "lua",
	".vim":        "vim",
}

var nameToLang = map[string]string{
	"dockerfile":     "dockerfile",
	"makefile":       "makefile",
	"gnumakefile":    "makefile",
	"cmakelists.txt": "cmake",
	"readme":         "markdown",
	"license":        "text",
	".gitignore":     "gitignore",
}

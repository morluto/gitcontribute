// Package codeindex indexes a bounded snapshot of tracked UTF-8 text files
// from a clean Git checkout using the native git executable and the standard
// library. It reads files through os.OpenRoot so paths cannot escape the
// repository root.
package codeindex

import (
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
	"strings"
	"time"
	"unicode/utf8"
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

	// ErrOutputLimit means git produced more metadata than the bounded indexer
	// was willing to retain in memory.
	ErrOutputLimit = errors.New("git output exceeds limit")
)

const (
	defaultMaxFiles        = 10_000
	defaultMaxBytesPerFile = 1 << 20
	defaultMaxTotalBytes   = 64 << 20
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
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve repo path: %w", err)
	}
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve repo path: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	top, err := validateRepo(ctx, absPath)
	if err != nil {
		return Snapshot{}, err
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve repo root: %w", err)
	}
	if absPath != top {
		return Snapshot{}, fmt.Errorf("repo path %q is not the repository root", absPath)
	}

	status, err := gitStatus(ctx, absPath)
	if err != nil {
		return Snapshot{}, err
	}
	if status != "" {
		return Snapshot{}, ErrDirtyWorktree
	}

	commit, err := gitHead(ctx, absPath)
	if err != nil {
		return Snapshot{}, err
	}

	root, err := os.OpenRoot(absPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open repo root: %w", err)
	}
	defer root.Close()

	createdAt := time.Now().UTC()
	paths, err := gitLsFiles(ctx, absPath, opts.MaxFiles)
	if err != nil {
		return Snapshot{}, err
	}
	sort.Strings(paths)

	snap := Snapshot{
		RepoPath:  absPath,
		Commit:    commit,
		CreatedAt: createdAt,
	}

	total := 0
	files := 0
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return snap, err
		}
		if files >= opts.MaxFiles {
			break
		}

		clean, ok := safeGitPath(p)
		if !ok {
			continue
		}
		if isExcluded(clean, opts.Exclusions) {
			continue
		}

		localName := filepath.FromSlash(clean)
		info, err := root.Lstat(localName)
		if err != nil {
			return snap, fmt.Errorf("stat %q: %w", clean, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		size := int(info.Size())
		if size > opts.MaxBytesPerFile {
			continue
		}
		if size > opts.MaxTotalBytes-total {
			continue
		}

		content, err := readFileContext(ctx, root, localName, opts.MaxBytesPerFile)
		if err != nil {
			return snap, fmt.Errorf("read %q: %w", clean, err)
		}
		if !isText(content) {
			continue
		}
		if total+len(content) > opts.MaxTotalBytes {
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
	status, err = gitStatus(ctx, absPath)
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
	if errors.Is(err, ErrOutputLimit) {
		return "dirty", nil
	}
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func gitLsFiles(ctx context.Context, repoPath string, maxFiles int) ([]string, error) {
	limit := maxPathListBytes
	if maxFiles <= maxPathListBytes/4096 {
		limit = maxFiles * 4096
	}
	if limit < 1<<20 {
		limit = 1 << 20
	}
	out, err := runGitLimited(ctx, repoPath, limit, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("list tracked files: %w", err)
	}
	if out == "" {
		return nil, nil
	}
	// git ls-files -z terminates the list with a NUL.
	out = strings.TrimSuffix(out, "\x00")
	parts := strings.Split(out, "\x00")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	return runGitLimited(ctx, repoPath, 1<<20, args...)
}

func runGitLimited(ctx context.Context, repoPath string, limit int, args ...string) (string, error) {
	gitArgs := []string{
		"--no-optional-locks",
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.hooksPath=/dev/null",
		"-C", repoPath,
	}
	cmd := exec.CommandContext(ctx, "git", append(gitArgs, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	)
	var stdout limitedBuffer
	stdout.remaining = limit
	cmd.Stdout = &stdout
	var stderr limitedBuffer
	stderr.remaining = 64 << 10
	cmd.Stderr = &stderr
	err := cmd.Run()
	if errors.Is(stdout.err, ErrOutputLimit) || errors.Is(stderr.err, ErrOutputLimit) {
		return stdout.buf.String(), ErrOutputLimit
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitErr.Stderr = append([]byte(nil), stderr.buf.Bytes()...)
		}
	}
	return stdout.buf.String(), err
}

type limitedBuffer struct {
	buf       bytes.Buffer
	remaining int
	err       error
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.remaining {
		written := b.remaining
		if b.remaining > 0 {
			_, _ = b.buf.Write(p[:b.remaining])
			b.remaining = 0
		}
		b.err = ErrOutputLimit
		return written, b.err
	}
	n, err := b.buf.Write(p)
	b.remaining -= n
	return n, err
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

func readFileContext(ctx context.Context, root *os.Root, name string, limit int) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	chunk := 8192
	if limit > 0 && limit < chunk {
		chunk = limit
	}
	buf := make([]byte, chunk)
	var out []byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		toRead := chunk
		if limit > 0 && limit-len(out) < toRead {
			toRead = limit - len(out)
		}
		if toRead <= 0 {
			break
		}

		n, err := f.Read(buf[:toRead])
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
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

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	// WorkspaceSnapshotVersion identifies the composite candidate identity contract.
	WorkspaceSnapshotVersion = "workspace-snapshot.v1"
	maxSnapshotUntracked     = 1000
	maxSnapshotFileBytes     = 16 << 20
	maxSnapshotTotalBytes    = 64 << 20
	maxSnapshotCommits       = 100
	maxSnapshotSubmodules    = 100
)

// ContentDigest identifies bounded content without embedding it.
type ContentDigest struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// UntrackedResource identifies one untracked path and its content when bounded.
type UntrackedResource struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

// SubmoduleIdentity binds the index and checked-out identities of a submodule.
type SubmoduleIdentity struct {
	Path     string `json:"path"`
	IndexSHA string `json:"index_sha"`
	HeadSHA  string `json:"head_sha,omitempty"`
	Dirty    *bool  `json:"dirty,omitempty"`
}

// CommitSummary is one bounded commit header between base and HEAD.
type CommitSummary struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// SnapshotGap explicitly records candidate content that could not be bound.
type SnapshotGap struct {
	Code   string `json:"code"`
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason"`
}

// Snapshot is a deterministic composite identity for a managed worktree.
type Snapshot struct {
	Version          string              `json:"version"`
	Ownership        string              `json:"ownership"`
	BaseSHA          string              `json:"base_sha,omitempty"`
	HeadSHA          string              `json:"head_sha"`
	MergeBase        string              `json:"merge_base,omitempty"`
	Staged           ContentDigest       `json:"staged"`
	Unstaged         ContentDigest       `json:"unstaged"`
	Untracked        []UntrackedResource `json:"untracked"`
	Submodules       []SubmoduleIdentity `json:"submodules"`
	ChangedFiles     []string            `json:"changed_files"`
	Commits          []CommitSummary     `json:"commits"`
	CommitTotal      int                 `json:"commit_total"`
	CommitsTruncated bool                `json:"commits_truncated"`
	Complete         bool                `json:"complete"`
	Gaps             []SnapshotGap       `json:"gaps"`
	SHA256           string              `json:"sha256"`
}

// SnapshotByPath derives a bounded, no-hook identity for a managed worktree.
func (m *Manager) SnapshotByPath(ctx context.Context, path, baseSHA, mergeBase string) (Snapshot, error) {
	managed, err := m.managedPath(path)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Version: WorkspaceSnapshotVersion, Ownership: "managed", BaseSHA: strings.TrimSpace(baseSHA), MergeBase: strings.TrimSpace(mergeBase), Complete: true}
	if snapshot.HeadSHA, err = trimmedGit(m.git(ctx, managed, "rev-parse", "HEAD")); err != nil {
		return Snapshot{}, fmt.Errorf("resolve workspace HEAD: %w", err)
	}
	staged, err := m.git(ctx, managed, "diff", "--cached", "--binary", "--full-index", "--no-ext-diff", "--no-textconv", "--")
	if err != nil {
		return Snapshot{}, fmt.Errorf("read staged diff: %w", err)
	}
	unstaged, err := m.git(ctx, managed, "diff", "--binary", "--full-index", "--no-ext-diff", "--no-textconv", "--")
	if err != nil {
		return Snapshot{}, fmt.Errorf("read unstaged diff: %w", err)
	}
	snapshot.Staged, snapshot.Unstaged = digestString(staged), digestString(unstaged)
	if snapshot.ChangedFiles, err = m.ChangedFilesByPath(ctx, managed, snapshot.BaseSHA); err != nil {
		return Snapshot{}, err
	}
	sort.Strings(snapshot.ChangedFiles)
	if err := m.addUntrackedSnapshot(ctx, managed, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if err := m.addSubmoduleSnapshot(ctx, managed, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if err := m.addCommitSnapshot(ctx, managed, &snapshot); err != nil {
		return Snapshot{}, err
	}
	sort.Slice(snapshot.Gaps, func(i, j int) bool {
		if snapshot.Gaps[i].Code != snapshot.Gaps[j].Code {
			return snapshot.Gaps[i].Code < snapshot.Gaps[j].Code
		}
		return snapshot.Gaps[i].Path < snapshot.Gaps[j].Path
	})
	snapshot.Complete = len(snapshot.Gaps) == 0
	snapshot.SHA256, err = snapshotDigest(snapshot)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) addUntrackedSnapshot(ctx context.Context, managed string, snapshot *Snapshot) error {
	ignored, err := m.git(ctx, managed, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return fmt.Errorf("list ignored files: %w", err)
	}
	if ignoredPaths := splitNUL(ignored); len(ignoredPaths) > 0 {
		snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "ignored_content_unbound", Reason: fmt.Sprintf("%d ignored paths are present and may affect validation", len(ignoredPaths))})
	}
	out, err := m.git(ctx, managed, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return fmt.Errorf("list untracked files: %w", err)
	}
	paths := splitNUL(out)
	sort.Strings(paths)
	if len(paths) > maxSnapshotUntracked {
		snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "untracked_paths_truncated", Reason: fmt.Sprintf("%d paths exceed the %d-path bound", len(paths), maxSnapshotUntracked)})
		paths = paths[:maxSnapshotUntracked]
	}
	root, err := os.OpenRoot(managed)
	if err != nil {
		return fmt.Errorf("open workspace root: %w", err)
	}
	var total int64
	for _, gitPath := range paths {
		localPath := filepath.FromSlash(gitPath)
		if !filepath.IsLocal(localPath) {
			snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "untracked_path_invalid", Path: gitPath, Reason: "Git returned a path outside the workspace"})
			continue
		}
		info, err := root.Lstat(localPath)
		if err != nil {
			snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "untracked_unavailable", Path: gitPath, Reason: err.Error()})
			continue
		}
		entry := UntrackedResource{Path: gitPath, Mode: uint32(info.Mode().Perm()), Bytes: info.Size()}
		switch {
		case info.Mode().IsRegular():
			entry.Kind = "file"
			if info.Size() > maxSnapshotFileBytes || total+info.Size() > maxSnapshotTotalBytes {
				snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "untracked_content_omitted", Path: gitPath, Reason: "content exceeds the snapshot byte bound"})
				break
			}
			digest, bytesRead, err := digestRootFile(root, localPath, info.Size())
			if err != nil {
				snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "untracked_content_changed", Path: gitPath, Reason: err.Error()})
				break
			}
			entry.SHA256, entry.Bytes, total = digest, bytesRead, total+bytesRead
		case info.Mode()&os.ModeSymlink != 0:
			entry.Kind = "symlink"
			target, err := root.Readlink(localPath)
			if err != nil {
				snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "untracked_symlink_unavailable", Path: gitPath, Reason: err.Error()})
				break
			}
			entry.SHA256 = digestBytes([]byte(target))
			entry.Bytes = int64(len(target))
		default:
			entry.Kind = "unsupported"
			snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "untracked_type_unsupported", Path: gitPath, Reason: info.Mode().String()})
		}
		snapshot.Untracked = append(snapshot.Untracked, entry)
	}
	if err := root.Close(); err != nil {
		return fmt.Errorf("close workspace root: %w", err)
	}
	return nil
}

func (m *Manager) addSubmoduleSnapshot(ctx context.Context, managed string, snapshot *Snapshot) error {
	out, err := m.git(ctx, managed, "ls-files", "--stage", "-z")
	if err != nil {
		return fmt.Errorf("list index entries: %w", err)
	}
	for _, record := range splitNUL(out) {
		metadata, gitPath, ok := strings.Cut(record, "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 3 || fields[0] != "160000" {
			continue
		}
		if len(snapshot.Submodules) == maxSnapshotSubmodules {
			snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "submodules_truncated", Reason: fmt.Sprintf("more than %d submodules are present", maxSnapshotSubmodules)})
			break
		}
		entry := SubmoduleIdentity{Path: gitPath, IndexSHA: fields[1]}
		localPath := filepath.FromSlash(gitPath)
		if !filepath.IsLocal(localPath) {
			snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "submodule_path_invalid", Path: gitPath, Reason: "Git returned a path outside the workspace"})
			snapshot.Submodules = append(snapshot.Submodules, entry)
			continue
		}
		submodulePath := filepath.Join(managed, localPath)
		head, headErr := trimmedGit(m.git(ctx, submodulePath, "rev-parse", "HEAD"))
		if headErr != nil {
			snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "submodule_head_unavailable", Path: gitPath, Reason: headErr.Error()})
		} else {
			entry.HeadSHA = head
			status, statusErr := m.git(ctx, submodulePath, "status", "--porcelain=v2", "--untracked-files=normal")
			if statusErr != nil {
				snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "submodule_status_unavailable", Path: gitPath, Reason: statusErr.Error()})
			} else {
				dirty := status != ""
				entry.Dirty = &dirty
				if dirty {
					snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "submodule_content_unbound", Path: gitPath, Reason: "dirty submodule content is not included in the parent snapshot"})
				}
			}
		}
		snapshot.Submodules = append(snapshot.Submodules, entry)
	}
	sort.Slice(snapshot.Submodules, func(i, j int) bool { return snapshot.Submodules[i].Path < snapshot.Submodules[j].Path })
	return nil
}

func (m *Manager) addCommitSnapshot(ctx context.Context, managed string, snapshot *Snapshot) error {
	if snapshot.BaseSHA == "" {
		return nil
	}
	countText, err := trimmedGit(m.git(ctx, managed, "rev-list", "--count", snapshot.BaseSHA+"..HEAD"))
	if err != nil {
		return fmt.Errorf("count workspace commits: %w", err)
	}
	snapshot.CommitTotal, err = strconv.Atoi(countText)
	if err != nil {
		return fmt.Errorf("parse workspace commit count %q: %w", countText, err)
	}
	out, err := m.git(ctx, managed, "log", "-z", "--max-count="+strconv.Itoa(maxSnapshotCommits), "--format=%H%x00%s", snapshot.BaseSHA+"..HEAD")
	if err != nil {
		return fmt.Errorf("read workspace commits: %w", err)
	}
	parts := splitNUL(out)
	if len(parts)%2 != 0 {
		return errors.New("parse workspace commits: Git returned an incomplete record")
	}
	for i := 0; i < len(parts); i += 2 {
		snapshot.Commits = append(snapshot.Commits, CommitSummary{SHA: parts[i], Subject: parts[i+1]})
	}
	snapshot.CommitsTruncated = snapshot.CommitTotal > len(snapshot.Commits)
	if snapshot.CommitsTruncated {
		snapshot.Gaps = append(snapshot.Gaps, SnapshotGap{Code: "commits_truncated", Reason: fmt.Sprintf("%d commits exceed the %d-commit metadata bound", snapshot.CommitTotal, maxSnapshotCommits)})
	}
	return nil
}

func digestRootFile(root *os.Root, path string, expected int64) (digest string, bytesRead int64, err error) {
	file, err := root.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, expected+1))
	if err != nil {
		return "", n, err
	}
	if n != expected {
		return "", n, fmt.Errorf("file size changed while hashing: got %d bytes, expected %d", n, expected)
	}
	return hex.EncodeToString(hash.Sum(nil)), n, nil
}

func snapshotDigest(snapshot Snapshot) (string, error) {
	snapshot.SHA256 = ""
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("encode workspace snapshot: %w", err)
	}
	return digestBytes(payload), nil
}

func digestString(value string) ContentDigest {
	return ContentDigest{SHA256: digestBytes([]byte(value)), Bytes: int64(len(value))}
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func splitNUL(value string) []string {
	parts := strings.Split(value, "\x00")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func trimmedGit(value string, err error) (string, error) {
	return strings.TrimSpace(value), err
}

package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/workspace"
)

func (s *Service) workspaceManager(ctx context.Context) (*workspace.Manager, error) {
	dataDir, err := s.paths.DataDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(dataDir, "workspaces")
	if err := ensurePrivateDir(root); err != nil {
		return nil, fmt.Errorf("ensure workspace root: %w", err)
	}
	return workspace.NewManager(root, nil)
}

// CreateWorkspace creates a managed worktree for an investigation.
func (s *Service) CreateWorkspace(ctx context.Context, investigationID string, opts cli.WorkspaceCreateOptions) (*cli.WorkspaceResult, error) {
	invSvc, err := s.investigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := invSvc.GetInvestigation(ctx, investigationID)
	if err != nil {
		return nil, mapInvestigationError(err)
	}

	remote := strings.TrimSpace(opts.Remote)
	if remote == "" {
		remote = fmt.Sprintf("https://github.com/%s/%s.git", inv.Repo.Owner, inv.Repo.Repo)
	}

	baseRef := strings.TrimSpace(opts.BaseRef)
	if baseRef == "" {
		baseRef = "main"
	}

	candidateRef := strings.TrimSpace(opts.CandidateRef)
	if candidateRef == "" {
		candidateRef = inv.CommitSHA
	}
	if candidateRef == "" {
		return nil, errors.New("candidate ref is required when investigation has no commit")
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = uuid.NewString()
	}

	mgr, err := s.workspaceManager(ctx)
	if err != nil {
		return nil, err
	}

	mirrorName := mirrorNameFor(inv.Repo.Owner, inv.Repo.Repo, remote)
	if err := mgr.Clone(ctx, remote, mirrorName); err != nil {
		return nil, fmt.Errorf("clone repository: %w", err)
	}

	ws, err := mgr.Create(ctx, mirrorName, baseRef, candidateRef, name)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}
	persisted := false
	defer func() {
		if persisted {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = mgr.Remove(cleanup, ws.Path, true)
	}()

	ws.InvestigationID = inv.ID
	ws.RepoOwner = inv.Repo.Owner
	ws.RepoName = inv.Repo.Repo

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.SaveWorkspace(ctx, ws); err != nil {
		return nil, fmt.Errorf("save workspace metadata: %w", err)
	}
	persisted = true

	return workspaceResult(ws), nil
}

// ShowWorkspace returns a workspace by ID with its current dirty state.
func (s *Service) ShowWorkspace(ctx context.Context, id string) (*cli.WorkspaceResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := c.GetWorkspace(ctx, id)
	if err != nil {
		return nil, mapWorkspaceError(err)
	}

	mgr, err := s.workspaceManager(ctx)
	if err == nil {
		if st, err := mgr.StatusByPath(ctx, ws.Path); err == nil {
			ws.Dirty = st.Dirty
		}
	}

	return workspaceResult(ws), nil
}

// ReviewStep is one suggested review item with a stable priority.
type ReviewStep struct {
	Path      string `json:"path"`
	Priority  int    `json:"priority"`
	Rationale string `json:"rationale"`
}

// WorkspaceDiffResult is the complete diff metadata for a workspace.
type WorkspaceDiffResult struct {
	ID               string       `json:"id"`
	Repo             cli.RepoRef  `json:"repo"`
	BaseSHA          string       `json:"base_sha"`
	CandidateSHA     string       `json:"candidate_sha"`
	MergeBase        string       `json:"merge_base"`
	Dirty            bool         `json:"dirty"`
	HasUntracked     bool         `json:"has_untracked"`
	Diff             string       `json:"diff"`
	ChangedFiles     []string     `json:"changed_files"`
	ChangedFileCount int          `json:"changed_file_count"`
	DiffBytes        int          `json:"diff_bytes"`
	ReviewOrder      []ReviewStep `json:"review_order"`
}

// WorkspaceDiff returns the current diff of a workspace against its recorded
// base, including changed files and a suggested review order.
func (s *Service) WorkspaceDiff(ctx context.Context, id string) (*WorkspaceDiffResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := c.GetWorkspace(ctx, id)
	if err != nil {
		return nil, mapWorkspaceError(err)
	}

	mgr, err := s.workspaceManager(ctx)
	if err != nil {
		return nil, err
	}

	st, err := mgr.StatusByPath(ctx, ws.Path)
	if err != nil {
		return nil, err
	}

	hasUntracked, err := mgr.HasUntrackedByPath(ctx, ws.Path)
	if err != nil {
		return nil, err
	}

	diff, err := mgr.DiffByPath(ctx, ws.Path, ws.BaseSHA)
	if err != nil {
		return nil, err
	}

	files := changedFilesFromDiff(diff)
	result := &WorkspaceDiffResult{
		ID:               ws.Name,
		Repo:             cli.RepoRef{Owner: ws.RepoOwner, Repo: ws.RepoName},
		BaseSHA:          ws.BaseSHA,
		CandidateSHA:     ws.CandidateSHA,
		MergeBase:        ws.MergeBase,
		Dirty:            st.Dirty,
		HasUntracked:     hasUntracked,
		Diff:             diff,
		ChangedFiles:     files,
		ChangedFileCount: len(files),
		DiffBytes:        len(diff),
		ReviewOrder:      reviewOrderFromFiles(files),
	}
	return result, nil
}

var diffPathRe = regexp.MustCompile(`\n\+\+\+ b/([^\t\n]+)`)

func changedFilesFromDiff(diff string) []string {
	seen := make(map[string]struct{})
	var files []string
	for _, m := range diffPathRe.FindAllStringSubmatch(diff, -1) {
		path := m[1]
		if path == "" || path == "dev/null" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	return files
}

func reviewOrderFromFiles(files []string) []ReviewStep {
	steps := make([]ReviewStep, 0, len(files))
	for _, f := range files {
		steps = append(steps, ReviewStep{Path: f, Priority: reviewPriority(f), Rationale: reviewRationale(f)})
	}
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].Priority != steps[j].Priority {
			return steps[i].Priority < steps[j].Priority
		}
		return steps[i].Path < steps[j].Path
	})
	return steps
}

func reviewPriority(path string) int {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "readme") || strings.Contains(lower, "contributing") ||
		strings.Contains(lower, "docs/") || strings.HasSuffix(lower, ".md") {
		return 0
	}
	if strings.Contains(lower, "_test.go") || strings.Contains(lower, "/test") ||
		strings.Contains(lower, "tests/") {
		return 1
	}
	return 2
}

func reviewRationale(path string) string {
	switch reviewPriority(path) {
	case 0:
		return "documentation and contribution guidance first"
	case 1:
		return "tests and validation next"
	default:
		return "implementation changes last"
	}
}

func safeMirrorName(s string) string {
	// Mirror names cannot contain path separators or control characters.
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}

func mirrorNameFor(owner, repo, remote string) string {
	const hashBytes = 8
	prefix := safeMirrorName(owner + "-" + repo)
	maxPrefix := 128 - 1 - hashBytes*2
	if len(prefix) > maxPrefix {
		prefix = prefix[:maxPrefix]
	}
	sum := sha256.Sum256([]byte(strings.ToLower(owner) + "\x00" + strings.ToLower(repo) + "\x00" + remote))
	return fmt.Sprintf("%s-%x", prefix, sum[:hashBytes])
}

func workspaceResult(ws *workspace.Workspace) *cli.WorkspaceResult {
	return &cli.WorkspaceResult{
		ID:              ws.Name,
		InvestigationID: ws.InvestigationID,
		Repo:            cli.RepoRef{Owner: ws.RepoOwner, Repo: ws.RepoName},
		Path:            ws.Path,
		Remote:          ws.Remote,
		BaseSHA:         ws.BaseSHA,
		CandidateSHA:    ws.CandidateSHA,
		MergeBase:       ws.MergeBase,
		Dirty:           ws.Dirty,
		CreatedAt:       formatTime(ws.CreatedAt),
	}
}

func mapWorkspaceError(err error) error {
	if errors.Is(err, workspace.ErrNotFound) {
		return cli.NewCLIError(cli.ExitNotFound, err)
	}
	return err
}

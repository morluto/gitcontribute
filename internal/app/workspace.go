package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

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

	mirrorName := safeMirrorName(inv.Repo.Owner + "-" + inv.Repo.Repo)
	if err := mgr.Clone(ctx, remote, mirrorName); err != nil {
		return nil, fmt.Errorf("clone repository: %w", err)
	}

	ws, err := mgr.Create(ctx, mirrorName, baseRef, candidateRef, name)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

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

func safeMirrorName(s string) string {
	// Mirror names cannot contain path separators or control characters.
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	if len(s) > 128 {
		s = s[:128]
	}
	return s
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

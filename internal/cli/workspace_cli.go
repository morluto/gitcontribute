package cli

import (
	"context"
	"fmt"
)

func (c *CLI) workspaceService() (WorkspaceService, error) {
	service, ok := c.svc.(WorkspaceService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runWorkspace(ctx context.Context, command string, cmd *workspaceCmd) error {
	service, err := c.workspaceService()
	if err != nil {
		return err
	}
	switch command {
	case "workspace create":
		if _, err := fmt.Fprintf(c.stderr, "creating workspace for investigation %s...\n", cmd.Create.InvestigationID); err != nil {
			return err
		}
		result, err := service.CreateWorkspace(ctx, cmd.Create.InvestigationID, WorkspaceCreateOptions{
			Remote:       cmd.Create.Remote,
			BaseRef:      cmd.Create.Base,
			CandidateRef: cmd.Create.Candidate,
			Name:         cmd.Create.Name,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Create.JSON, result)
	case "workspace adopt":
		if _, err := fmt.Fprintf(c.stderr, "adopting local worktree for investigation %s...\n", cmd.Adopt.InvestigationID); err != nil {
			return err
		}
		result, err := service.AdoptWorkspace(ctx, cmd.Adopt.InvestigationID, WorkspaceAdoptOptions{
			Path: cmd.Adopt.Path, BaseRef: cmd.Adopt.Base, Name: cmd.Adopt.Name,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Adopt.JSON, result)
	case "workspace show":
		result, err := service.ShowWorkspace(ctx, cmd.Show.ID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Show.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown workspace command: %s", command))
	}
}

func (c *CLI) runDiff(ctx context.Context, cmd *diffCmd) error {
	service, err := c.workflowExtensionService()
	if err != nil {
		return err
	}
	result, err := service.WorkspaceDiffForCLI(ctx, cmd.WorkspaceID)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

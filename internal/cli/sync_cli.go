package cli

import (
	"context"
	"errors"
	"fmt"
)

func (c *CLI) runSync(ctx context.Context, cmd *syncCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	if planner, ok := c.svc.(SyncPlanningService); ok {
		plan, err := planner.PlanArchiveSync(ctx, repo, ArchiveSyncOptions{State: "all"})
		if err != nil {
			return c.mapError(err)
		}
		if err := c.printSyncPlan(plan); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.stderr, "syncing %s...\n", repo); err != nil {
		return err
	}
	res, err := c.svc.Sync(ctx, repo)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runArchiveSync(ctx context.Context, cmd *archiveSyncCmd) error {
	service, err := c.archiveService()
	if err != nil {
		return err
	}
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	numbers, err := parseNumberList(cmd.Numbers)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	if cmd.Since < 0 {
		return NewCLIError(ExitUsage, errors.New("since duration cannot be negative"))
	}
	if len(numbers) > 0 && (cmd.State != "all" || cmd.Since != 0) {
		return NewCLIError(ExitUsage, errors.New("state and since filters cannot be combined with exact thread numbers"))
	}
	opts := ArchiveSyncOptions{
		State: cmd.State, Since: cmd.Since, Numbers: numbers, MaxPages: cmd.MaxPages, MaxRequests: cmd.MaxRequests,
	}
	plan, err := service.PlanArchiveSync(ctx, repo, opts)
	if err != nil {
		return c.mapError(err)
	}
	if err := c.printSyncPlan(plan); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.stderr, "syncing archive for %s...\n", repo); err != nil {
		return err
	}
	result, err := service.ArchiveSync(ctx, repo, opts)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runArchiveRefresh(ctx context.Context, cmd *archiveRefreshCmd) error {
	service, err := c.archiveService()
	if err != nil {
		return err
	}
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	opts := ArchiveSyncOptions{State: "all", MaxPages: cmd.MaxPages}
	plan, err := service.PlanArchiveSync(ctx, repo, opts)
	if err != nil {
		return c.mapError(err)
	}
	if err := c.printSyncPlan(plan); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.stderr, "refreshing archive for %s...\n", repo); err != nil {
		return err
	}
	result, err := service.ArchiveSync(ctx, repo, opts)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) printSyncPlan(plan *SyncPlanResult) error {
	if plan == nil {
		return nil
	}
	_, err := fmt.Fprintf(c.stderr, "planned sync for %s: up to %d requests (%d fixed + up to %d thread requests; budget %d)\n",
		plan.Repo, plan.PlannedRequests, plan.FixedRequests, plan.ThreadRequestCeiling, plan.RequestBudget)
	return err
}

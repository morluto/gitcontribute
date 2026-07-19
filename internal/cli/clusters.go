package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type clustersCmd struct {
	List    clustersListCmd    `cmd:"" help:"List the stored repository cluster projection"`
	Refresh clustersRefreshCmd `cmd:"" help:"Compute and persist the repository cluster projection"`
}

type clustersListCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Limit     int    `name:"limit" default:"50" help:"Maximum clusters to return"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type clustersRefreshCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the refreshed projection as JSON"`
}

type clusterCmd struct {
	Show clusterShowCmd `cmd:"" help:"Show a cluster by stable id"`
}

type clusterShowCmd struct {
	ID    string `arg:"" help:"Cluster stable id"`
	Limit int    `name:"limit" default:"100" help:"Maximum members to show"`
	JSON  bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) runClusters(ctx context.Context, command string, cmd *clustersCmd) error {
	if command == "clusters refresh" {
		return c.runClusterRefresh(ctx, cmd.Refresh)
	}
	if command != "clusters list" {
		return NewCLIError(ExitUsage, fmt.Errorf("unknown clusters command: %s", command))
	}
	return c.runClusterList(ctx, cmd.List)
}

func (c *CLI) runClusterRefresh(ctx context.Context, cmd clustersRefreshCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	service, err := c.clusteringService()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.stderr, "refreshing clusters for %s...\n", repo); err != nil {
		return err
	}
	res, err := service.RefreshClusters(ctx, repo)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runClusterList(ctx context.Context, cmd clustersListCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	if cmd.Limit <= 0 || cmd.Limit > 1000 {
		return NewCLIError(ExitUsage, errors.New("limit must be between 1 and 1000"))
	}
	service, err := c.clusteringService()
	if err != nil {
		return err
	}
	res, err := service.ListClusters(ctx, repo, cmd.Limit)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runCluster(ctx context.Context, command string, cmd *clusterCmd) error {
	if command != "cluster show" {
		return NewCLIError(ExitUsage, fmt.Errorf("unknown cluster command: %s", command))
	}
	show := &cmd.Show
	if strings.TrimSpace(show.ID) == "" {
		return NewCLIError(ExitUsage, errors.New("cluster id is required"))
	}
	if show.Limit <= 0 || show.Limit > 1000 {
		return NewCLIError(ExitUsage, errors.New("limit must be between 1 and 1000"))
	}
	service, err := c.clusteringService()
	if err != nil {
		return err
	}
	res, err := service.Cluster(ctx, show.ID, show.Limit)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(show.JSON, res)
}

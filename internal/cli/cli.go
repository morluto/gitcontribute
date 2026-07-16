package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"
)

const maxSearchLimit = 100

// CLI is a Kong-based adapter that parses arguments and dispatches to product-
// owned application services. It owns no domain logic.
type CLI struct {
	svc    Service
	runner MCPRunner
	stdout io.Writer
	stderr io.Writer
}

// New constructs a CLI that writes results to stdout and progress to stderr.
func New(service Service, runner MCPRunner, stdout, stderr io.Writer) *CLI {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &CLI{svc: service, runner: runner, stdout: stdout, stderr: stderr}
}

type rootCmd struct {
	Init    initCmd    `cmd:"" help:"Initialize the local corpus"`
	Status  statusCmd  `cmd:"" help:"Show corpus status"`
	Sync    syncCmd    `cmd:"" help:"Sync a repository into the corpus"`
	Search  searchCmd  `cmd:"" help:"Search the local corpus"`
	Dossier dossierCmd `cmd:"" help:"Show repository dossier"`
	MCP     mcpCmd     `cmd:"" name:"mcp" help:"Run the MCP server"`
}

type initCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type statusCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type syncCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type searchCmd struct {
	Query string `arg:"" name:"query" help:"Search query"`
	Kind  string `name:"kind" short:"k" default:"all" enum:"repos,issues,prs,threads,code,all" help:"Search kind"`
	Repo  string `name:"repo" help:"Restrict to repository OWNER/REPO"`
	Limit int    `name:"limit" default:"20" help:"Maximum number of results"`
	JSON  bool   `name:"json" help:"Print the result as JSON"`
}

type dossierCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type mcpCmd struct {
	Transport string `name:"transport" default:"stdio" enum:"stdio" help:"MCP transport protocol"`
}

// Run parses arguments and dispatches to the appropriate Service or MCPRunner
// method. It respects context cancellation.
func (c *CLI) Run(ctx context.Context, args []string) error {
	var cli rootCmd
	parser, err := kong.New(&cli,
		kong.Name("gitcontribute"),
		kong.Description("GitHub contribution research workbench"),
		kong.UsageOnError(),
	)
	if err != nil {
		return NewCLIError(ExitGeneral, err)
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}

	cmd := strings.Fields(kctx.Command())[0]
	switch cmd {
	case "init":
		return c.runInit(ctx, &cli.Init)
	case "status":
		return c.runStatus(ctx, &cli.Status)
	case "sync":
		return c.runSync(ctx, &cli.Sync)
	case "search":
		return c.runSearch(ctx, &cli.Search)
	case "dossier":
		return c.runDossier(ctx, &cli.Dossier)
	case "mcp":
		return c.runMCP(ctx, &cli.MCP)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown command: %s", cmd))
	}
}

func (c *CLI) runInit(ctx context.Context, cmd *initCmd) error {
	fmt.Fprintln(c.stderr, "initializing...")
	res, err := c.svc.Init(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runStatus(ctx context.Context, cmd *statusCmd) error {
	res, err := c.svc.Status(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runSync(ctx context.Context, cmd *syncCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stderr, "syncing %s...\n", repo)
	res, err := c.svc.Sync(ctx, repo)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runSearch(ctx context.Context, cmd *searchCmd) error {
	opts := SearchOptions{Kind: cmd.Kind, Repo: cmd.Repo, Limit: cmd.Limit}
	if opts.Limit <= 0 || opts.Limit > maxSearchLimit {
		return NewCLIError(ExitUsage, fmt.Errorf("limit must be between 1 and %d", maxSearchLimit))
	}
	if opts.Repo != "" {
		if _, err := parseRepo(opts.Repo); err != nil {
			return NewCLIError(ExitUsage, fmt.Errorf("invalid --repo value: %w", err))
		}
	}
	res, err := c.svc.Search(ctx, cmd.Query, opts)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runDossier(ctx context.Context, cmd *dossierCmd) error {
	repo, err := parseRepo(cmd.OwnerRepo)
	if err != nil {
		return err
	}
	res, err := c.svc.Dossier(ctx, repo)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, res)
}

func (c *CLI) runMCP(ctx context.Context, cmd *mcpCmd) error {
	fmt.Fprintf(c.stderr, "starting mcp server (transport=%s)...\n", cmd.Transport)
	return c.mapError(c.runner.Run(ctx, MCPOptions{Transport: cmd.Transport}))
}

func (c *CLI) render(json bool, v any) error {
	if json {
		return writeJSON(c.stdout, v)
	}
	s, err := humanOutput(v)
	if err != nil {
		return NewCLIError(ExitGeneral, err)
	}
	_, err = fmt.Fprintln(c.stdout, s)
	return err
}

func (c *CLI) mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NewCLIError(ExitCancelled, err)
	}
	var ce *CLIError
	if errors.As(err, &ce) {
		return ce
	}
	return NewCLIError(ExitGeneral, err)
}

func parseRepo(s string) (RepoRef, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RepoRef{}, NewCLIError(ExitUsage, fmt.Errorf("invalid repository %q: expected OWNER/REPO", s))
	}
	return RepoRef{Owner: parts[0], Repo: parts[1]}, nil
}

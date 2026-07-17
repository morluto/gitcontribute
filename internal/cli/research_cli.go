package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/research"
)

type researchCmd struct {
	Brief researchBriefCmd `cmd:"" help:"Build a deterministic brief for one stored issue or pull request"`
}

type researchBriefCmd struct {
	Thread string `arg:"" name:"thread" help:"Thread as OWNER/REPO#NUMBER, issue:OWNER/REPO#NUMBER, or pr:OWNER/REPO#NUMBER"`
	Format string `name:"format" default:"markdown" enum:"markdown,md,json" help:"Output format"`
	JSON   bool   `name:"json" help:"Print JSON (shorthand for --format json)"`
}

func (c *CLI) researchService() (ResearchService, error) {
	service, ok := c.svc.(ResearchService)
	if !ok {
		return nil, NewCLIError(ExitNotWired, ErrNotWired)
	}
	return service, nil
}

func (c *CLI) runResearch(ctx context.Context, command string, cmd *researchCmd) error {
	if command != "research brief" {
		return NewCLIError(ExitUsage, fmt.Errorf("unknown research command: %s", command))
	}
	ref, err := research.ParseThreadRef(cmd.Brief.Thread)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	format := cmd.Brief.Format
	if cmd.Brief.JSON {
		format = "json"
	}
	service, err := c.researchService()
	if err != nil {
		return err
	}
	brief, err := service.ThreadResearchBrief(ctx, ref)
	if err != nil {
		return c.mapError(err)
	}
	if format == "json" {
		return writeJSON(c.stdout, brief)
	}
	if format != "markdown" && format != "md" {
		return NewCLIError(ExitUsage, fmt.Errorf("unsupported research format %q", format))
	}
	var rendered strings.Builder
	if err := research.RenderMarkdown(&rendered, brief); err != nil {
		return c.mapError(err)
	}
	if _, err := fmt.Fprint(c.stdout, rendered.String()); err != nil {
		return c.mapError(err)
	}
	if !strings.HasSuffix(rendered.String(), "\n") {
		_, err = fmt.Fprintln(c.stdout)
	}
	return c.mapError(err)
}

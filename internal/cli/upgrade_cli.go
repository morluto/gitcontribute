package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
)

func (c *CLI) runUpgrade(ctx context.Context, cmd *upgradeCmd) error {
	service, ok := c.svc.(contracts.UpgradeService)
	if !ok {
		return NewCLIError(ExitNotWired, ErrNotWired)
	}
	if !cmd.Check && !cmd.Yes {
		if !c.interactiveInput() || !c.interactiveOutput() || !c.interactivePromptOutput() {
			return NewCLIError(ExitUsage, errors.New("interactive upgrade requires terminal input and visible output; pass --check or --yes"))
		}
		confirmed, err := c.confirmSetup("Check npm for the latest GitContribute release and apply an eligible managed update")
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		if !confirmed {
			_, _ = fmt.Fprintln(c.stderr, "Upgrade cancelled.")
			return nil
		}
		cmd.Yes = true
	}
	report, err := service.Upgrade(ctx, contracts.UpgradeOptions{Check: cmd.Check, Yes: cmd.Yes})
	if err != nil {
		return NewCLIError(ExitGeneral, err)
	}
	if cmd.JSON {
		return writeJSON(c.stdout, report)
	}
	_, err = fmt.Fprintf(c.stdout, "Upgrade [%s]: %s", report.Context, report.Status)
	if report.Latest != "" {
		_, err = fmt.Fprintf(c.stdout, " (current %s, latest %s)", report.Current, report.Latest)
	}
	if report.Command != "" {
		_, err = fmt.Fprintf(c.stdout, "\n%s", report.Command)
	}
	for _, stage := range report.Stages {
		_, err = fmt.Fprintf(c.stdout, "\n- %s: %s", stage.Name, stage.Status)
		if stage.Message != "" {
			_, err = fmt.Fprintf(c.stdout, " — %s", stage.Message)
		}
	}
	if report.Action != "" {
		_, err = fmt.Fprintf(c.stdout, "\nNext: %s", report.Action)
	}
	if report.Rollback != "" {
		_, err = fmt.Fprintf(c.stdout, "\nRollback: %s", report.Rollback)
	}
	if err == nil {
		_, err = fmt.Fprintln(c.stdout)
	}
	return err
}

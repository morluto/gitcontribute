package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
			return c.writeProgressf("Upgrade cancelled.\n")
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
	var output strings.Builder
	fmt.Fprintf(&output, "Upgrade [%s]: %s", report.Context, report.Status)
	if report.Latest != "" {
		fmt.Fprintf(&output, " (current %s, latest %s)", report.Current, report.Latest)
	}
	if report.Command != "" {
		fmt.Fprintf(&output, "\n%s", report.Command)
	}
	for _, stage := range report.Stages {
		fmt.Fprintf(&output, "\n- %s: %s", stage.Name, stage.Status)
		if stage.Message != "" {
			fmt.Fprintf(&output, " — %s", stage.Message)
		}
	}
	if report.Action != "" {
		fmt.Fprintf(&output, "\nNext: %s", report.Action)
	}
	if report.Rollback != "" {
		fmt.Fprintf(&output, "\nRollback: %s", report.Rollback)
	}
	_, err = fmt.Fprintln(c.stdout, output.String())
	return err
}

package cli

import (
	"context"
	"errors"
	"os"
)

func (c *CLI) runDefault(ctx context.Context) error {
	if !c.interactiveInput() || !c.interactiveOutput() {
		return NewCLIError(ExitUsage, errors.New("interactive interface requires a terminal; run gitcontribute --help for commands"))
	}
	return c.runTUI(ctx, &tuiCmd{})
}

func (c *CLI) interactiveInput() bool {
	file, ok := c.stdin.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (c *CLI) interactiveOutput() bool {
	file, ok := c.stdout.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (c *CLI) interactivePromptOutput() bool {
	file, ok := c.stderr.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

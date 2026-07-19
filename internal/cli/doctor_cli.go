package cli

import (
	"context"
	"errors"
)

func (c *CLI) runDoctor(ctx context.Context, cmd *doctorCmd) error {
	service, err := c.controlService()
	if err != nil {
		return err
	}
	result, err := service.Doctor(ctx)
	if err != nil {
		return c.mapError(err)
	}
	if err := c.render(cmd.JSON, result); err != nil {
		return err
	}
	if cmd.Strict && !result.Healthy {
		return NewCLIError(ExitGeneral, errors.New("required diagnostic checks failed"))
	}
	return nil
}

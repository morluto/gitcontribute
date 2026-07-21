package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (c *CLI) runValidation(ctx context.Context, command string, cmd *validationCmd) error {
	service, err := c.validationService()
	if err != nil {
		return err
	}
	switch command {
	case "validation define":
		return c.defineValidation(ctx, service, &cmd.Define)
	case "validation run":
		return c.executeValidation(ctx, service, &cmd.Run)
	case "validation compare":
		return c.compareValidation(ctx, service, &cmd.Compare)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown validation command: %s", command))
	}
}

func (c *CLI) defineValidation(ctx context.Context, service ValidationService, cmd *defineValidationCmd) error {
	if _, err := fmt.Fprintf(c.stderr, "defining validation for investigation %s...\n", cmd.InvestigationID); err != nil {
		return err
	}
	var observation *ValidationObservationContract
	if strings.TrimSpace(cmd.Observation) != "" {
		observation = &ValidationObservationContract{}
		if err := json.Unmarshal([]byte(cmd.Observation), observation); err != nil {
			return c.mapError(fmt.Errorf("parse observation contract: %w", err))
		}
	}
	result, err := service.DefineValidation(ctx, cmd.InvestigationID, DefineValidationOptions{
		Kind: cmd.Kind, Command: cmd.Command, WorkingDir: cmd.WorkingDir,
		BaseWorkingDir: cmd.BaseWorkingDir, CandidateDir: cmd.CandidateDir,
		Env: cmd.Env, Timeout: cmd.Timeout, MaxOutputBytes: cmd.MaxOutput,
		Observation: observation,
	})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) executeValidation(ctx context.Context, service ValidationService, cmd *runValidationCmd) error {
	definition, err := service.ShowValidation(ctx, cmd.ID)
	if err != nil {
		return c.mapError(err)
	}
	dir := definition.WorkingDir
	if cmd.Kind == "base" && definition.BaseWorkingDir != "" {
		dir = definition.BaseWorkingDir
	}
	if cmd.Kind == "candidate" && definition.CandidateDir != "" {
		dir = definition.CandidateDir
	}
	visible := formatCommand(definition.Command)
	if !cmd.Execute {
		return NewCLIError(ExitUsage, fmt.Errorf("host execution requires --execute; command: %s (directory: %s)", visible, dir))
	}
	if _, err := fmt.Fprintf(c.stderr, "executing in %s: %s\n", dir, visible); err != nil {
		return err
	}
	result, err := service.RunValidation(ctx, cmd.ID, RunValidationOptions{Kind: cmd.Kind, Execute: true})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) compareValidation(ctx context.Context, service ValidationService, cmd *compareValidationCmd) error {
	result, err := service.CompareValidation(ctx, cmd.BaseRunID, cmd.CandidateRunID)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

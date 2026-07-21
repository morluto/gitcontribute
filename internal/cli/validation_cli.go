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
		if _, err := fmt.Fprintf(c.stderr, "defining validation for investigation %s...\n", cmd.Define.InvestigationID); err != nil {
			return err
		}
		var observation *ValidationObservationContract
		if strings.TrimSpace(cmd.Define.Observation) != "" {
			observation = &ValidationObservationContract{}
			if err := json.Unmarshal([]byte(cmd.Define.Observation), observation); err != nil {
				return c.mapError(fmt.Errorf("parse observation contract: %w", err))
			}
		}
		result, err := service.DefineValidation(ctx, cmd.Define.InvestigationID, DefineValidationOptions{
			Kind: cmd.Define.Kind, Command: cmd.Define.Command, WorkingDir: cmd.Define.WorkingDir,
			BaseWorkingDir: cmd.Define.BaseWorkingDir, CandidateDir: cmd.Define.CandidateDir,
			Env: cmd.Define.Env, Timeout: cmd.Define.Timeout, MaxOutputBytes: cmd.Define.MaxOutput,
			Observation: observation,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Define.JSON, result)
	case "validation run":
		definition, err := service.ShowValidation(ctx, cmd.Run.ID)
		if err != nil {
			return c.mapError(err)
		}
		dir := definition.WorkingDir
		if cmd.Run.Kind == "base" && definition.BaseWorkingDir != "" {
			dir = definition.BaseWorkingDir
		}
		if cmd.Run.Kind == "candidate" && definition.CandidateDir != "" {
			dir = definition.CandidateDir
		}
		visible := formatCommand(definition.Command)
		if !cmd.Run.Execute {
			return NewCLIError(ExitUsage, fmt.Errorf("host execution requires --execute; command: %s (directory: %s)", visible, dir))
		}
		if _, err := fmt.Fprintf(c.stderr, "executing in %s: %s\n", dir, visible); err != nil {
			return err
		}
		result, err := service.RunValidation(ctx, cmd.Run.ID, RunValidationOptions{Kind: cmd.Run.Kind, Execute: true})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Run.JSON, result)
	case "validation compare":
		result, err := service.CompareValidation(ctx, cmd.Compare.BaseRunID, cmd.Compare.CandidateRunID)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Compare.JSON, result)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown validation command: %s", command))
	}
}

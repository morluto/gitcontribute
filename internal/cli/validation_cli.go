package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
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
	case "validation repeat":
		return c.executeRepeatedValidation(ctx, service, &cmd.Repeat)
	case "validation compare":
		return c.compareValidation(ctx, service, &cmd.Compare)
	case "validation attach":
		receiptService, ok := any(service).(contracts.ValidationReceiptService)
		if !ok {
			return errors.New("external validation receipt import is unavailable")
		}
		return c.attachValidationReceipt(ctx, receiptService, &cmd.Attach)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown validation command: %s", command))
	}
}

func (c *CLI) attachValidationReceipt(ctx context.Context, service contracts.ValidationReceiptService, cmd *attachValidationReceiptCmd) error {
	file, err := os.Open(cmd.File)
	if err != nil {
		return fmt.Errorf("open external validation receipt: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 2<<20))
	decoder.DisallowUnknownFields()
	var receipt contracts.ExternalValidationReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return fmt.Errorf("decode external validation receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("external validation receipt must contain one JSON value")
	}
	result, err := service.AttachValidationReceipt(ctx, receipt)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) executeRepeatedValidation(ctx context.Context, service contracts.ValidationService, cmd *repeatValidationCmd) error {
	definition, err := service.ShowValidation(ctx, cmd.ID)
	if err != nil {
		return c.mapError(err)
	}
	if !cmd.Execute {
		return NewCLIError(ExitUsage, fmt.Errorf("host execution requires --execute; command: %s", formatCommand(definition.Command)))
	}
	kinds := []string{cmd.Kind}
	if cmd.Kind == "both" {
		kinds = []string{"base", "candidate"}
	}
	if _, err := fmt.Fprintf(c.stderr, "executing %d validation attempt(s) per target: %s\n", cmd.Runs, formatCommand(definition.Command)); err != nil {
		return err
	}
	result, err := service.RunValidationGroup(ctx, cmd.ID, contracts.RepeatValidationOptions{
		Kinds: kinds, RunCount: cmd.Runs, Concurrency: cmd.Concurrency,
		PerRunTimeout: cmd.PerRunTimeout, OverallTimeout: cmd.OverallTimeout, SampleInterval: cmd.SampleInterval,
		Execute: true,
	})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) defineValidation(ctx context.Context, service contracts.ValidationService, cmd *defineValidationCmd) error {
	if _, err := fmt.Fprintf(c.stderr, "defining validation for investigation %s...\n", cmd.InvestigationID); err != nil {
		return err
	}
	var observation *contracts.ValidationObservationContract
	if strings.TrimSpace(cmd.Observation) != "" {
		observation = &contracts.ValidationObservationContract{}
		if err := json.Unmarshal([]byte(cmd.Observation), observation); err != nil {
			return c.mapError(fmt.Errorf("parse observation contract: %w", err))
		}
	}
	result, err := service.DefineValidation(ctx, cmd.InvestigationID, contracts.DefineValidationOptions{
		Kind: cmd.Kind, Command: cmd.Command, WorkingDir: cmd.WorkingDir,
		BaseWorkingDir: cmd.BaseWorkingDir, CandidateDir: cmd.CandidateDir,
		WorkspaceID: cmd.WorkspaceID, BaseWorkspaceID: cmd.BaseWorkspaceID, CandidateWorkspaceID: cmd.CandidateWorkspaceID,
		Env: cmd.Env, Timeout: cmd.Timeout, MaxOutputBytes: cmd.MaxOutput,
		Observation: observation, Protocol: cmd.Protocol, ReadinessTimeout: cmd.ReadinessTimeout,
	})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) executeValidation(ctx context.Context, service contracts.ValidationService, cmd *runValidationCmd) error {
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
	result, err := service.RunValidation(ctx, cmd.ID, contracts.RunValidationOptions{Kind: cmd.Kind, Execute: true})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) compareValidation(ctx context.Context, service contracts.ValidationService, cmd *compareValidationCmd) error {
	result, err := service.CompareValidation(ctx, cmd.BaseRunID, cmd.CandidateRunID)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

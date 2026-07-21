package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/managedbinary"
	clientsetup "github.com/morluto/gitcontribute/internal/setup"
)

type installDetails struct {
	context    string
	executable string
	npmRoot    string
}

func (s *Service) validateNewerCorpusTarget(ctx context.Context, report *cli.UpgradeReport, candidate, target string) bool {
	fail := func(err error) bool {
		stage := upgradeStage(report, "activation")
		stage.Status = "target_validation_failed"
		stage.Message = err.Error()
		setStage(report, stage)
		report.Status = "target validation failed"
		report.Action = "install a release whose runtime contract supports the configured corpus"
		return false
	}
	if candidate == "" {
		return fail(errors.New("installed target executable is unavailable for schema validation"))
	}
	if err := verifySetupExecutable(candidate); err != nil {
		return fail(fmt.Errorf("installed target executable is not usable: %w", err))
	}
	contract, err := readRuntimeContract(ctx, candidate)
	if err != nil {
		return fail(fmt.Errorf("installed target runtime contract is unreadable: %w", err))
	}
	if normalizeVersion(contract.Version) != normalizeVersion(target) {
		return fail(fmt.Errorf("installed target reports version %s, not target %s", contract.Version, target))
	}
	if contract.SupportedSchemaVersion <= 0 {
		return fail(errors.New("installed target does not report a supported schema version"))
	}

	currentSchema, corpusExists, err := corpus.InspectSchemaVersion(ctx, s.databasePath())
	if err != nil {
		return fail(fmt.Errorf("inspect configured corpus schema: %w", err))
	}
	if !corpusExists {
		return fail(errors.New("configured corpus disappeared during target validation"))
	}
	stage := cli.UpgradeStage{
		Name:    "corpus-schema",
		Path:    s.databasePath(),
		Version: strconv.FormatInt(currentSchema, 10),
		Target:  strconv.FormatInt(contract.SupportedSchemaVersion, 10),
	}
	switch {
	case currentSchema < contract.SupportedSchemaVersion:
		stage.Status = "migration_required"
		stage.Message = fmt.Sprintf("corpus schema %d is older than installed target schema %d", currentSchema, contract.SupportedSchemaVersion)
		setStage(report, stage)
		report.Status = "schema migration required"
		report.Action = "migrate the corpus before running the new version"
		return false
	case currentSchema > contract.SupportedSchemaVersion:
		stage.Status = "newer"
		stage.Message = fmt.Sprintf("corpus schema %d is newer than installed target schema %d", currentSchema, contract.SupportedSchemaVersion)
		setStage(report, stage)
		report.Status = "corpus newer than binary"
		report.Action = "upgrade to a release that supports the current corpus schema"
		return false
	default:
		stage.Status = "current"
		stage.Message = fmt.Sprintf("installed target supports corpus schema %d", currentSchema)
		setStage(report, stage)
		activation := upgradeStage(report, "activation")
		activation.Status = "compatible"
		activation.Message = "installed target runtime is compatible with the configured corpus"
		setStage(report, activation)
		return true
	}
}

func (s *Service) activatePrivateRuntime(ctx context.Context, report *cli.UpgradeReport, details installDetails) {
	clients := outdatedPrivateRuntimeClients(report)
	if len(clients) == 0 {
		return
	}

	target := report.Latest
	candidate := details.executable
	if candidate == "" {
		stage := upgradeStage(report, "activation")
		stage.Status = "target_runtime_unavailable"
		stage.Message = "no staged target executable is available to evaluate"
		setStage(report, stage)
		report.Status = "target runtime unavailable"
		report.Action = stage.Message
		report.RestartClients = nil
		return
	}

	if err := verifySetupExecutable(candidate); err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("staged target executable is not usable: %w", err))
		return
	}

	contract, err := readRuntimeContract(ctx, candidate)
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("runtime contract is unreadable: %w", err))
		return
	}
	if normalizeVersion(contract.Version) != normalizeVersion(target) {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("staged executable reports version %s, not target %s", contract.Version, target))
		return
	}
	if contract.SupportedSchemaVersion <= 0 {
		s.setPrivateActivationFailure(report, len(clients), errors.New("staged executable does not report a supported schema version"))
		return
	}

	currentSchema, corpusExists, err := corpus.InspectSchemaVersion(ctx, s.databasePath())
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("inspect configured corpus schema: %w", err))
		return
	}
	if corpusExists {
		switch {
		case currentSchema < contract.SupportedSchemaVersion:
			msg := fmt.Sprintf("corpus schema %d is older than target schema %d; migrate the corpus before activating", currentSchema, contract.SupportedSchemaVersion)
			s.setPrivateActivationFailure(report, len(clients), errors.New(msg))
			setStage(report, cli.UpgradeStage{
				Name:    "corpus-schema",
				Status:  "migration_required",
				Version: strconv.FormatInt(currentSchema, 10),
				Target:  strconv.FormatInt(contract.SupportedSchemaVersion, 10),
				Message: msg,
			})
			report.Status = "schema migration required"
			report.Action = "migrate the corpus before running the new version"
			report.Rollback = "prior client registrations remain unchanged"
			return
		case currentSchema > contract.SupportedSchemaVersion:
			msg := fmt.Sprintf("corpus schema %d is newer than target schema %d", currentSchema, contract.SupportedSchemaVersion)
			s.setPrivateActivationFailure(report, len(clients), errors.New(msg))
			setStage(report, cli.UpgradeStage{
				Name:    "corpus-schema",
				Status:  "newer",
				Version: strconv.FormatInt(currentSchema, 10),
				Target:  strconv.FormatInt(contract.SupportedSchemaVersion, 10),
				Message: msg,
			})
			report.Status = "corpus newer than binary"
			report.Action = "upgrade to a release that supports the current corpus schema"
			report.Rollback = "prior client registrations remain unchanged"
			return
		}
	}

	dataDir, err := s.paths.DataDir()
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), err)
		return
	}
	destination, err := managedbinary.Destination(dataDir, target)
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), err)
		return
	}
	if _, err := managedbinary.Install(candidate, destination); err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("stage private MCP runtime: %w", err))
		return
	}
	if err := verifySetupExecutable(destination); err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("verify staged private MCP runtime: %w", err))
		return
	}
	destinationContract, err := readRuntimeContract(ctx, destination)
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("installed runtime contract is unreadable: %w", err))
		return
	}
	if *destinationContract != *contract {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("installed runtime contract disagrees with source candidate: source=%+v destination=%+v", *contract, *destinationContract))
		return
	}
	setStage(report, cli.UpgradeStage{
		Name: "private-mcp-runtime", Status: "staged", Path: destination,
		Version: target, Target: target, Message: fmt.Sprintf("private MCP runtime %s is staged; client activation is pending", target),
	})
	s.activateConfiguredClients(ctx, report, clients, destination, target)
}

func (s *Service) activateConfiguredClients(ctx context.Context, report *cli.UpgradeReport, clients []string, destination, target string) {
	setupClients := make([]clientsetup.Client, 0, len(clients))
	for _, name := range clients {
		setupClients = append(setupClients, clientsetup.Client(name))
	}
	_, err := clientsetup.ActivateExistingAndVerify(ctx, clientsetup.Options{
		Clients: setupClients, Home: s.paths.HomeDir(), Executable: destination,
	}, func() error { return s.verifyPrivateActivation(ctx, report, setupClients, destination, target) })
	if err != nil {
		s.setPrivateActivationFailure(report, len(clients), fmt.Errorf("activate private MCP runtime: %w", err))
		return
	}

	setStage(report, cli.UpgradeStage{
		Name: "private-mcp-runtime", Status: "verified", Path: destination,
		Version: target, Target: target, Message: fmt.Sprintf("private MCP runtime %s is staged and verified", target),
	})
	setStage(report, cli.UpgradeStage{
		Name: "configured-runtime", Status: "activated",
		Message: fmt.Sprintf("%d registered client(s) now reference runtime %s", len(clients), target),
	})
	setStage(report, cli.UpgradeStage{
		Name: "activation", Status: "restart_required",
		Message: "activation is verified; restart the configured clients to replace their running MCP processes",
	})
	report.Status = "restart required"
	report.Action = "restart the configured clients to activate the verified MCP runtime"
	report.RestartClients = append([]string(nil), clients...)
	report.Rollback = "the previous versioned runtime remains installed; rerun setup with that compatible release to reactivate it"
	setStage(report, cli.UpgradeStage{Name: "rollback", Status: "available", Message: report.Rollback})
}

func readRuntimeContract(ctx context.Context, path string) (*cli.RuntimeContractResult, error) {
	out, err := runtimeContractCommand(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("execute %s runtime-contract: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var meta cli.RuntimeContractResult
	if err := dec.Decode(&meta); err != nil {
		return nil, fmt.Errorf("parse runtime contract output: %w", err)
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, errors.New("trailing data after runtime contract")
	}
	if meta.Name != "gitcontribute" {
		return nil, fmt.Errorf("runtime contract name is %q, want %q", meta.Name, "gitcontribute")
	}
	if meta.Version == "" {
		return nil, errors.New("runtime contract is missing version")
	}
	return &meta, nil
}

func (s *Service) verifyPrivateActivation(ctx context.Context, report *cli.UpgradeReport, clients []clientsetup.Client, destination, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, client := range clients {
		configured, err := inspectConfiguredClient(s.paths.HomeDir(), client, target, target)
		if err != nil {
			return fmt.Errorf("verify %s registration: %w", client, err)
		}
		if configured.Status != "target" || filepath.Clean(configured.Path) != filepath.Clean(destination) {
			return fmt.Errorf("verify %s registration: configured runtime does not match %s", client, destination)
		}
		for i := range report.ConfiguredClients {
			if report.ConfiguredClients[i].Name == string(client) {
				report.ConfiguredClients[i] = configured
			}
		}
	}
	return nil
}

func (s *Service) setPrivateActivationFailure(report *cli.UpgradeReport, clientCount int, err error) {
	stage := upgradeStage(report, "activation")
	var rollbackFailure *clientsetup.ActivationRollbackError
	if errors.As(err, &rollbackFailure) {
		stage.Status = "rollback_failed"
		stage.Message = fmt.Sprintf("%s; one or more of %d client registration(s) may require manual repair", err, clientCount)
		setStage(report, stage)
		report.Status = "activation rollback failed"
		report.Action = "inspect configured client registrations before retrying upgrade"
		report.Rollback = "automatic registration rollback was incomplete"
		report.RestartClients = nil
		return
	}
	stage.Status = "failed"
	stage.Message = fmt.Sprintf("%s; prior registrations for %d client(s) were preserved", err, clientCount)
	setStage(report, stage)
	report.Status = "activation failed"
	report.Action = "resolve the activation error and run upgrade --yes again"
	report.RestartClients = nil
}

func upgradeStage(report *cli.UpgradeReport, name string) cli.UpgradeStage {
	for _, stage := range report.Stages {
		if stage.Name == name {
			return stage
		}
	}
	return cli.UpgradeStage{Name: name}
}

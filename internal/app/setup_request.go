package app

import (
	"errors"
	"strings"

	"github.com/morluto/gitcontribute/internal/contracts"
	clientsetup "github.com/morluto/gitcontribute/internal/setup"
)

type setupKind uint8

const (
	setupKindInvalid setupKind = iota
	setupMCP
	setupCLI
	setupBoth
	setupRemove
)

func (k setupKind) clientOperation() clientsetup.Operation {
	if k == setupRemove {
		return clientsetup.Remove
	}
	return clientsetup.Configure
}

func (k setupKind) configuresClients() bool {
	return k == setupMCP || k == setupBoth || k == setupRemove
}

func (k setupKind) installsCLI() bool {
	return k == setupCLI || k == setupBoth
}

func (k setupKind) configuresProduct() bool {
	return k != setupRemove
}

type setupExecution uint8

const (
	setupApply setupExecution = iota
	setupPlan
)

func (e setupExecution) dryRun() bool { return e == setupPlan }

type setupRequest struct {
	kind           setupKind
	execution      setupExecution
	clients        []clientsetup.Client
	tokenSource    string
	tokenSourceKey string
	repository     *contracts.RepoRef
	version        string
}

func parseSetupRequest(opts contracts.SetupOptions, defaultVersion string) (setupRequest, error) {
	kind, err := parseSetupKind(opts)
	if err != nil {
		return setupRequest{}, err
	}
	execution := setupApply
	if opts.DryRun {
		execution = setupPlan
	}
	request := setupRequest{
		kind: kind, execution: execution,
		tokenSource: opts.TokenSource, tokenSourceKey: opts.TokenSourceKey,
		version: opts.Version,
	}
	if request.version == "" {
		request.version = defaultVersion
	}
	if kind.configuresClients() {
		if len(opts.Clients) == 0 && !opts.AllClients {
			return setupRequest{}, errors.New("no coding-agent targets selected; pass --codex, --claude, --devin, or --all-clients")
		}
		if opts.AllClients {
			request.clients = clientsetup.SupportedClients()
		} else {
			request.clients, err = clientsetup.ParseClients(opts.Clients)
			if err != nil {
				return setupRequest{}, err
			}
		}
	}
	if strings.TrimSpace(opts.Repository) != "" {
		ref, err := setupRepoRef(opts.Repository)
		if err != nil {
			return setupRequest{}, err
		}
		request.repository = &ref
	}
	return request, nil
}

func parseSetupKind(opts contracts.SetupOptions) (setupKind, error) {
	if opts.Remove {
		if opts.Mode != "" {
			return setupKindInvalid, errors.New("an access mode is not supported by remove")
		}
		return setupRemove, nil
	}
	selectedClients := len(opts.Clients) > 0 || opts.AllClients
	switch opts.Mode {
	case contracts.SetupModeMCP:
		return setupMCP, nil
	case contracts.SetupModeCLI:
		if selectedClients {
			return setupKindInvalid, errors.New("CLI mode cannot configure MCP clients")
		}
		return setupCLI, nil
	case contracts.SetupModeBoth:
		return setupBoth, nil
	default:
		return setupKindInvalid, errors.New("setup has no selected access mode")
	}
}

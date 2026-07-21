package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
)

func (s *Service) schemaStage(ctx context.Context) cli.UpgradeStage {
	stage := cli.UpgradeStage{Name: "corpus-schema"}
	path := s.databasePath()
	if path == "" {
		stage.Status = "not_configured"
		stage.Message = "corpus database path is not configured"
		return stage
	}
	stage.Path = path
	inspection, err := corpus.InspectSchema(ctx, path)
	if err != nil {
		stage.Status = "failed"
		stage.Message = err.Error()
		return stage
	}
	stage.Version = strconv.FormatInt(inspection.Current, 10)
	stage.Target = strconv.FormatInt(inspection.Target, 10)
	switch inspection.State {
	case corpus.SchemaMissing:
		stage.Status = "missing"
		stage.Message = fmt.Sprintf("corpus has not been created (target schema %d)", inspection.Target)
	case corpus.SchemaCurrent:
		stage.Status = "current"
		stage.Message = fmt.Sprintf("corpus schema is current (%d)", inspection.Current)
	case corpus.SchemaMigrationRequired:
		stage.Status = "migration_required"
		stage.Message = fmt.Sprintf("corpus migration required: current %d, target %d", inspection.Current, inspection.Target)
	case corpus.SchemaNewer:
		stage.Status = "newer"
		stage.Message = fmt.Sprintf("corpus schema %d is newer than this binary supports (%d)", inspection.Current, inspection.Target)
	}
	return stage
}

func activationStage(report *cli.UpgradeReport, opts cli.UpgradeOptions) cli.UpgradeStage {
	stage := cli.UpgradeStage{Name: "activation"}
	switch stageStatus(report, "corpus-schema") {
	case "migration_required":
		stage.Status = "migrate_first"
		stage.Message = "migrate the corpus before running the new version"
		report.Action = stage.Message
		return stage
	case "newer":
		stage.Status = "rollback_or_upgrade"
		stage.Message = "corpus was written by a newer binary; upgrade to a matching release first"
		report.Action = stage.Message
		return stage
	}
	if clients := outdatedPrivateRuntimeClients(report); len(clients) > 0 {
		stage.Status = "setup_required"
		stage.Message = "versioned private MCP runtimes are not replaced by npm launcher upgrade; rerun setup with the target release, then restart configured clients"
		report.Action = stage.Message
		report.RestartClients = clients
		return stage
	}
	switch report.Context {
	case "npx":
		stage.Status = "not_required"
		stage.Message = "npx resolves versions on demand; no activation needed"
	case "other":
		stage.Status = "manual"
		stage.Message = "installation method is not managed automatically"
	case "project-npm":
		stage.Status = "manual"
		stage.Message = "project npm installation; update with npm install --save-dev"
	case "global-npm":
		switch reportVersionDisposition(report) {
		case versionUnavailable:
			stage.Status = "awaiting_confirmation"
			stage.Message = "pass --check or --yes to evaluate the latest release"
		case versionCurrent:
			stage.Status = "current"
			stage.Message = "global npm installation is current"
		case versionDowngrade:
			stage.Status = "downgrade_blocked"
			stage.Message = "installed version is newer than the registry target; automatic downgrade is blocked"
		case versionInvalid:
			stage.Status = "manual"
			stage.Message = "installed and registry versions cannot be compared safely"
		case versionUpgrade, versionPrerelease:
			switch {
			case opts.Yes:
				if upgradeGOOS == "windows" {
					stage.Status = "manual"
					stage.Message = "close running GitContribute processes, then run the displayed command"
				} else {
					stage.Status = "install_and_restart"
					stage.Message = "install the latest release, then restart configured clients"
					report.RestartClients = registeredClients(report)
				}
			case opts.Check:
				stage.Status = "review"
				if reportVersionDisposition(report) == versionPrerelease {
					stage.Message = "a newer prerelease is available; pass --yes to install"
				} else {
					stage.Message = "latest release is available; pass --yes to install"
				}
			default:
				stage.Status = "awaiting_confirmation"
				stage.Message = "pass --yes to install the latest release and restart clients"
			}
		}
	}
	report.Action = stage.Message
	return stage
}

func outdatedPrivateRuntimeClients(report *cli.UpgradeReport) []string {
	var clients []string
	for _, client := range report.ConfiguredClients {
		if client.Status != "outdated" || strings.Contains(filepath.ToSlash(client.Path), "/node_modules/gitcontribute/") {
			continue
		}
		clients = append(clients, client.Name)
	}
	return clients
}

func registeredClients(report *cli.UpgradeReport) []string {
	var names []string
	for _, client := range report.ConfiguredClients {
		if client.Status != "not_configured" && client.Status != "failed" {
			names = append(names, client.Name)
		}
	}
	return names
}

func rollbackStage(report *cli.UpgradeReport) cli.UpgradeStage {
	stage := cli.UpgradeStage{Name: "rollback"}
	switch report.Context {
	case "npx":
		stage.Status = "not_applicable"
		stage.Message = "no persistent installation to roll back"
	case "global-npm":
		stage.Status = "limited"
		stage.Message = "npm global installs cannot be rolled back automatically; reinstall the previous version with npm if needed"
	case "project-npm":
		stage.Status = "manual"
		stage.Message = "roll back by reinstalling the previous version in the project"
	default:
		stage.Status = "manual"
		stage.Message = "roll back by replacing the executable or re-running setup with the previous version"
	}
	if stageStatus(report, "corpus-schema") == "migration_required" {
		stage.Message += "; migrating the corpus without a backup limits rollback"
	}
	report.Rollback = stage.Message
	return stage
}

func stageStatus(report *cli.UpgradeReport, name string) string {
	for _, stage := range report.Stages {
		if stage.Name == name {
			return stage.Status
		}
	}
	return ""
}

func stagePath(report *cli.UpgradeReport, name string) string {
	for _, stage := range report.Stages {
		if stage.Name == name {
			return stage.Path
		}
	}
	return ""
}

func stageVersion(report *cli.UpgradeReport, name string) string {
	for _, stage := range report.Stages {
		if stage.Name == name {
			return stage.Version
		}
	}
	return ""
}

func setStage(report *cli.UpgradeReport, stage cli.UpgradeStage) {
	for i := range report.Stages {
		if report.Stages[i].Name == stage.Name {
			report.Stages[i] = stage
			return
		}
	}
	report.Stages = append(report.Stages, stage)
}

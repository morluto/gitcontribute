package cli

import (
	"context"
	"fmt"
)

type dossierCmd struct {
	Build  dossierBuildCmd  `cmd:"" help:"Build and persist a repository dossier"`
	Show   dossierShowCmd   `cmd:"" help:"Show the latest persisted dossier"`
	Export dossierExportCmd `cmd:"" help:"Export a repository dossier"`
}

type dossierBuildCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type dossierShowCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type dossierExportCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Format    string `name:"format" default:"markdown" enum:"json,markdown,md" help:"Export format"`
	Output    string `name:"output" help:"Write to a file instead of stdout"`
}

func (c *CLI) runDossier(ctx context.Context, command string, cmd *dossierCmd) error {
	service, err := c.dossierExtensionService()
	if err != nil {
		// Preserve the original dossier command for lightweight implementations.
		if command == "dossier show" {
			repo, parseErr := parseRepo(cmd.Show.OwnerRepo)
			if parseErr != nil {
				return parseErr
			}
			res, callErr := c.svc.Dossier(ctx, repo)
			if callErr != nil {
				return c.mapError(callErr)
			}
			return c.render(cmd.Show.JSON, res)
		}
		return err
	}
	var result any
	var jsonOutput bool
	switch command {
	case "dossier build":
		repo, parseErr := parseRepo(cmd.Build.OwnerRepo)
		if parseErr != nil {
			return parseErr
		}
		result, err = service.BuildDossierForCLI(ctx, repo)
		jsonOutput = cmd.Build.JSON
	case "dossier show":
		repo, parseErr := parseRepo(cmd.Show.OwnerRepo)
		if parseErr != nil {
			return parseErr
		}
		result, err = service.GetDossierForCLI(ctx, repo)
		jsonOutput = cmd.Show.JSON
	case "dossier export":
		return c.runExport(ctx, "export dossier", &exportCmd{Dossier: exportDossierCmd{
			OwnerRepo: cmd.Export.OwnerRepo, Format: cmd.Export.Format, Output: cmd.Export.Output,
		}})
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown dossier command: %s", command))
	}
	if err != nil {
		return c.mapError(err)
	}
	return c.render(jsonOutput, result)
}

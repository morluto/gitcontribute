package cli

import (
	"context"
	"errors"
	"fmt"
)

type initCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type corpusCmd struct {
	Inspect           corpusInspectCmd           `cmd:"" help:"Inspect schema compatibility without mutation"`
	Migrate           corpusMigrateCmd           `cmd:"" help:"Back up and explicitly migrate the corpus"`
	Backup            corpusBackupCmd            `cmd:"" help:"Create a verified online SQLite backup"`
	Restore           corpusRestoreCmd           `cmd:"" help:"Restore the corpus with an automatic safety backup"`
	Inventory         corpusInventoryCmd         `cmd:"" help:"Show stored and derived data for one repository"`
	List              corpusListCmd              `cmd:"" help:"List bounded storage and freshness summaries for all repositories"`
	PruneCode         corpusPruneCodeCmd         `cmd:"" name:"prune-code" help:"Plan or prune derived code snapshots for one repository"`
	RemoveRepository  corpusRemoveRepositoryCmd  `cmd:"" name:"remove-repository" help:"Preview or remove one repository from the corpus"`
	Projections       corpusProjectionsCmd       `cmd:"" help:"List derived projection versions and status"`
	RebuildProjection corpusRebuildProjectionCmd `cmd:"" name:"rebuild-projection" help:"Explicitly rebuild one derived search projection"`
}

type corpusProjectionsCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type corpusRebuildProjectionCmd struct {
	Name string `arg:"" name:"name" enum:"threads_fts,facet_observations_fts,code_documents_fts" help:"Projection name"`
	Yes  bool   `name:"yes" short:"y" help:"Confirm the local derived-index rebuild"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type corpusInventoryCmd struct {
	Repo string `arg:"" name:"repository" help:"Repository as OWNER/REPO"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type corpusListCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type corpusPruneCodeCmd struct {
	Repo       string `arg:"" name:"repository" help:"Repository as OWNER/REPO"`
	KeepLatest int    `name:"keep-latest" default:"1" help:"Number of newest code snapshots to retain"`
	Yes        bool   `name:"yes" short:"y" help:"Apply the displayed derived-data prune plan"`
	JSON       bool   `name:"json" help:"Print the result as JSON"`
}

type corpusRemoveRepositoryCmd struct {
	Repo string `arg:"" name:"repository" help:"Repository as OWNER/REPO"`
	Yes  bool   `name:"yes" short:"y" help:"Apply the exact displayed repository-removal plan"`
	JSON bool   `name:"json" help:"Print the result as JSON"`
}

type corpusInspectCmd struct {
	JSON bool `name:"json" help:"Print the result as JSON"`
}

type corpusMigrateCmd struct {
	BackupPath string `name:"backup" help:"Backup destination (defaults beside the corpus)"`
	NoBackup   bool   `name:"no-backup" help:"Migrate without creating a backup"`
	Yes        bool   `name:"yes" short:"y" help:"Apply the migration without prompting"`
	JSON       bool   `name:"json" help:"Print the result as JSON"`
}

type corpusBackupCmd struct {
	Destination string `arg:"" name:"destination" help:"New backup file path"`
	JSON        bool   `name:"json" help:"Print the result as JSON"`
}

type corpusRestoreCmd struct {
	Source       string `arg:"" name:"source" help:"Verified SQLite backup to restore"`
	SafetyBackup string `name:"safety-backup" help:"Safety-backup destination (defaults beside the corpus)"`
	Yes          bool   `name:"yes" short:"y" help:"Confirm replacement of the configured corpus"`
	JSON         bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) runCorpus(ctx context.Context, command string, cmd *corpusCmd) error {
	service, ok := c.svc.(CorpusLifecycleService)
	if !ok {
		return NewCLIError(ExitNotWired, ErrNotWired)
	}
	switch command {
	case "corpus inspect":
		return c.runCorpusInspect(ctx, service, cmd.Inspect.JSON)
	case "corpus backup":
		return c.runCorpusBackup(ctx, service, &cmd.Backup)
	case "corpus migrate":
		return c.runCorpusMigrate(ctx, service, &cmd.Migrate)
	case "corpus restore":
		return c.runCorpusRestore(ctx, service, &cmd.Restore)
	case "corpus inventory":
		return c.runCorpusInventory(ctx, service, &cmd.Inventory)
	case "corpus list":
		return c.runCorpusList(ctx, service, cmd.List.JSON)
	case "corpus prune-code":
		return c.runCorpusPrune(ctx, service, &cmd.PruneCode)
	case "corpus remove-repository":
		return c.runCorpusRemoveRepository(ctx, service, &cmd.RemoveRepository)
	case "corpus projections":
		return c.runCorpusProjections(ctx, service, cmd.Projections.JSON)
	case "corpus rebuild-projection":
		return c.runCorpusRebuildProjection(ctx, service, &cmd.RebuildProjection)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown corpus command: %s", command))
	}
}

func (c *CLI) runCorpusInspect(ctx context.Context, service CorpusLifecycleService, json bool) error {
	result, err := service.InspectCorpus(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(json, result)
}

func (c *CLI) runCorpusBackup(ctx context.Context, service CorpusLifecycleService, cmd *corpusBackupCmd) error {
	result, err := service.BackupCorpus(ctx, cmd.Destination)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) confirmCorpusAction(yes bool, nonInteractiveError, prompt, cancelled string) (bool, error) {
	if yes {
		return true, nil
	}
	if !c.interactiveInput() {
		return false, NewCLIError(ExitUsage, errors.New(nonInteractiveError))
	}
	confirmed, err := c.confirmSetup(prompt)
	if err != nil {
		return false, NewCLIError(ExitUsage, err)
	}
	if !confirmed {
		_, err = fmt.Fprintln(c.stderr, cancelled)
	}
	return confirmed, err
}

func (c *CLI) runCorpusMigrate(ctx context.Context, service CorpusLifecycleService, cmd *corpusMigrateCmd) error {
	confirmed, err := c.confirmCorpusAction(cmd.Yes, "corpus migration requires --yes in non-interactive use", "Back up and migrate the configured corpus", "Corpus migration cancelled.")
	if err != nil || !confirmed {
		return err
	}
	result, err := service.MigrateCorpus(ctx, CorpusMigrateOptions{BackupPath: cmd.BackupPath, NoBackup: cmd.NoBackup})
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runCorpusRestore(ctx context.Context, service CorpusLifecycleService, cmd *corpusRestoreCmd) error {
	confirmed, err := c.confirmCorpusAction(cmd.Yes, "corpus restore requires --yes in non-interactive use", "Replace the configured corpus after creating a safety backup", "Corpus restore cancelled.")
	if err != nil || !confirmed {
		return err
	}
	result, err := service.RestoreCorpus(ctx, cmd.Source, cmd.SafetyBackup)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runCorpusInventory(ctx context.Context, service CorpusLifecycleService, cmd *corpusInventoryCmd) error {
	result, err := service.InventoryCorpus(ctx, cmd.Repo)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runCorpusList(ctx context.Context, service CorpusLifecycleService, json bool) error {
	result, err := service.ListCorpusInventory(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(json, result)
}

func (c *CLI) runCorpusPrune(ctx context.Context, service CorpusLifecycleService, cmd *corpusPruneCodeCmd) error {
	plan, err := service.PlanCodePrune(ctx, cmd.Repo, cmd.KeepLatest)
	if err != nil {
		return c.mapError(err)
	}
	if !cmd.Yes {
		return c.render(cmd.JSON, plan)
	}
	expected := make([]string, len(plan.Delete))
	for i := range plan.Delete {
		expected[i] = plan.Delete[i].CommitSHA
	}
	result, err := service.ApplyCodePrune(ctx, cmd.Repo, cmd.KeepLatest, expected)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runCorpusRemoveRepository(ctx context.Context, service CorpusLifecycleService, cmd *corpusRemoveRepositoryCmd) error {
	plan, err := service.PlanRepositoryRemoval(ctx, cmd.Repo)
	if err != nil {
		return c.mapError(err)
	}
	if !cmd.Yes {
		return c.render(cmd.JSON, plan)
	}
	result, err := service.ApplyRepositoryRemoval(ctx, cmd.Repo, plan.Revision)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

func (c *CLI) runCorpusProjections(ctx context.Context, service CorpusLifecycleService, json bool) error {
	result, err := service.ListCorpusProjections(ctx)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(json, result)
}

func (c *CLI) runCorpusRebuildProjection(ctx context.Context, service CorpusLifecycleService, cmd *corpusRebuildProjectionCmd) error {
	if !cmd.Yes {
		return NewCLIError(ExitUsage, errors.New("projection rebuild requires --yes"))
	}
	result, err := service.RebuildCorpusProjection(ctx, cmd.Name)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

package cli

import (
	"context"
	"fmt"
)

// dispatchCommand is the narrow routing layer between Kong's parsed command
// tree and the capability-specific handlers. Keeping the table here means the
// top-level CLI owns parsing, logging, and error boundaries while each command
// group owns its own behavior.
func (c *CLI) dispatchCommand(ctx context.Context, cmd, command string, parsed *rootCmd) error {
	handlers := map[string]func() error{
		"setup":            func() error { return c.runSetupCommand(ctx, &parsed.Setup) },
		"remove":           func() error { return c.runRemoveCommand(ctx, &parsed.Remove) },
		"upgrade":          func() error { return c.runUpgrade(ctx, &parsed.Upgrade) },
		"runtime-contract": func() error { return c.runRuntimeContract(ctx) },
		"init":             func() error { return c.runInit(ctx, &parsed.Init) },
		"corpus":           func() error { return c.runCorpus(ctx, command, &parsed.Corpus) },
		"configure":        func() error { return c.runConfigure(ctx, &parsed.Configure) },
		"metadata":         func() error { return c.runMetadata(ctx, &parsed.Metadata) },
		"status":           func() error { return c.runStatus(ctx, &parsed.Status) },
		"doctor":           func() error { return c.runDoctor(ctx, &parsed.Doctor) },
		"health":           func() error { return c.runHealth(ctx, &parsed.Health) },
		"radar":            func() error { return c.runRadar(ctx, &parsed.Radar) },
		"search":           func() error { return c.runSearch(ctx, command, &parsed.Search) },
		"dossier":          func() error { return c.runDossier(ctx, command, &parsed.Dossier) },
		"research":         func() error { return c.runResearch(ctx, command, &parsed.Research) },
		"seeds":            func() error { return c.runSeeds(ctx, &parsed.Seeds) },
		"index":            func() error { return c.runIndex(ctx, &parsed.Index) },
		"acquire":          func() error { return c.runAcquire(ctx, &parsed.Acquire) },
		"source":           func() error { return c.runSource(ctx, command, &parsed.Source) },
		"crawl":            func() error { return c.runCrawl(ctx, &parsed.Crawl) },
		"tail":             func() error { return c.runTail(ctx, &parsed.Tail) },
		"investigation":    func() error { return c.runInvestigation(ctx, command, &parsed.Investigation) },
		"hypothesis":       func() error { return c.runHypothesis(ctx, command, &parsed.Hypothesis) },
		"duplicates":       func() error { return c.runCheck(ctx, command, "duplicates", &parsed.Duplicates) },
		"collisions":       func() error { return c.runCheck(ctx, command, "collisions", &parsed.Collisions) },
		"opportunity":      func() error { return c.runOpportunity(ctx, command, &parsed.Opportunity) },
		"concern":          func() error { return c.runConcern(ctx, command, &parsed.Concern) },
		"workspace":        func() error { return c.runWorkspace(ctx, command, &parsed.Workspace) },
		"diff":             func() error { return c.runDiff(ctx, &parsed.Diff) },
		"validation":       func() error { return c.runValidation(ctx, command, &parsed.Validation) },
		"evidence":         func() error { return c.runEvidence(ctx, command, &parsed.Evidence) },
		"readiness":        func() error { return c.runReadiness(ctx, command, &parsed.Readiness) },
		"prepare":          func() error { return c.runPrepare(ctx, command, &parsed.Prepare) },
		"archive":          func() error { return c.runArchive(ctx, command, &parsed.Archive) },
		"coverage":         func() error { return c.runCoverage(ctx, &parsed.Coverage) },
		"runs":             func() error { return c.runRuns(ctx, &parsed.Runs) },
		"jobs":             func() error { return c.runJobs(ctx, command, &parsed.Jobs) },
		"neighbors":        func() error { return c.runNeighbors(ctx, &parsed.Neighbors) },
		"export":           func() error { return c.runExport(ctx, command, &parsed.Export) },
		"clusters":         func() error { return c.runClusters(ctx, command, &parsed.Clusters) },
		"cluster":          func() error { return c.runCluster(ctx, command, &parsed.Cluster) },
		"lens":             func() error { return c.runLens(ctx, command, &parsed.Lens) },
		"collection":       func() error { return c.runCollection(ctx, command, &parsed.Collection) },
		"triage":           func() error { return c.runTriage(ctx, command, &parsed.Triage) },
		"contribution":     func() error { return c.runContribution(ctx, command, &parsed.Contribution) },
		"tracking":         func() error { return c.runTracking(ctx, command, &parsed.Tracking) },
		"mcp":              func() error { return c.runMCP(ctx, &parsed.MCP) },
		"tui":              func() error { return c.runTUI(ctx, &parsed.TUI) },
	}
	if handler, ok := handlers[cmd]; ok {
		return handler()
	}
	return NewCLIError(ExitUsage, fmt.Errorf("unknown command: %s", cmd))
}

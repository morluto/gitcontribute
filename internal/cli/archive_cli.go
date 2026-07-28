package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
)

type archiveCmd struct {
	SyncContext archiveContextSyncCmd `cmd:"" name:"sync-context" help:"Synchronize repository metadata and contribution guidance"`
	Sync        archiveSyncCmd        `cmd:"" help:"Synchronize repository threads"`
	Hydrate     archiveHydrateCmd     `cmd:"" help:"Hydrate one issue or pull request"`
	Refresh     archiveRefreshCmd     `cmd:"" help:"Refresh all repository threads"`
	Threads     archiveThreadsCmd     `cmd:"" help:"List archived repository threads"`
	Coverage    coverageCmd           `cmd:"" help:"Show repository facet coverage"`
}

type archiveContextSyncCmd struct {
	OwnerRepo   string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	MaxRequests int    `name:"max-requests" help:"Maximum total GitHub requests (defaults to the repository-context plan)"`
	JSON        bool   `name:"json" help:"Print the result as JSON"`
}

type archiveSyncCmd struct {
	OwnerRepo   string        `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	State       string        `name:"state" default:"all" enum:"open,closed,all" help:"Thread state"`
	Since       time.Duration `name:"since" help:"Only threads updated within this duration"`
	Numbers     string        `name:"numbers" help:"Comma-separated exact thread numbers"`
	MaxPages    int           `name:"max-pages" default:"1000" help:"Maximum issue-list pages"`
	MaxRequests int           `name:"max-requests" default:"100" help:"Maximum total GitHub requests"`
	JSON        bool          `name:"json" help:"Print the result as JSON"`
}

type archiveHydrateCmd struct {
	Thread   string `arg:"" name:"owner/repo#number" help:"Thread as OWNER/REPO#NUMBER"`
	With     string `name:"with" help:"Comma-separated facets: issue_comments, issue_timeline, pr_details, pr_reviews, pr_review_comments (defaults to applicable non-timeline facets)"`
	MaxPages int    `name:"max-pages" default:"50" help:"Maximum pages per facet"`
	JSON     bool   `name:"json" help:"Print the result as JSON"`
}

type archiveRefreshCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	MaxPages  int    `name:"max-pages" default:"1000" help:"Maximum issue-list pages"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

type archiveThreadsCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	Kind      string `name:"kind" default:"all" enum:"all,issue,pr,pull_request" help:"Restrict by thread kind"`
	State     string `name:"state" default:"all" enum:"open,closed,all" help:"Restrict by state"`
	Limit     int    `name:"limit" default:"100" help:"Maximum threads to return"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

func (c *CLI) runArchive(ctx context.Context, command string, cmd *archiveCmd) error {
	switch command {
	case "archive sync-context":
		return c.runArchiveContextSync(ctx, &cmd.SyncContext)
	case "archive sync":
		return c.runArchiveSync(ctx, &cmd.Sync)
	case "archive hydrate":
		service, err := c.archiveService()
		if err != nil {
			return err
		}
		repo, number, err := parseThreadRef(cmd.Hydrate.Thread)
		if err != nil {
			return NewCLIError(ExitUsage, err)
		}
		_, _ = fmt.Fprintf(c.stderr, "hydrating %s#%d...\n", repo, number)
		result, err := service.Hydrate(ctx, repo, number, contracts.HydrateOptions{
			Facets: splitCSV(cmd.Hydrate.With), MaxPages: cmd.Hydrate.MaxPages,
		})
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Hydrate.JSON, result)
	case "archive refresh":
		return c.runArchiveRefresh(ctx, &cmd.Refresh)
	case "archive threads":
		repo, err := parseRepo(cmd.Threads.OwnerRepo)
		if err != nil {
			return err
		}
		if cmd.Threads.Limit <= 0 || cmd.Threads.Limit > 1000 {
			return NewCLIError(ExitUsage, errors.New("limit must be between 1 and 1000"))
		}
		service, err := c.archiveThreadService()
		if err != nil {
			return err
		}
		result, err := service.ArchiveThreads(ctx, repo, cmd.Threads.Kind, cmd.Threads.State, cmd.Threads.Limit)
		if err != nil {
			return c.mapError(err)
		}
		return c.render(cmd.Threads.JSON, result)
	case "archive coverage":
		return c.runCoverage(ctx, &cmd.Coverage)
	default:
		return NewCLIError(ExitUsage, fmt.Errorf("unknown archive command: %s", command))
	}
}

type coverageCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

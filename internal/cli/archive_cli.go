package cli

import "time"

type archiveCmd struct {
	Sync     archiveSyncCmd    `cmd:"" help:"Synchronize repository threads"`
	Hydrate  archiveHydrateCmd `cmd:"" help:"Hydrate one issue or pull request"`
	Refresh  archiveRefreshCmd `cmd:"" help:"Refresh all repository threads"`
	Threads  archiveThreadsCmd `cmd:"" help:"List archived repository threads"`
	Coverage coverageCmd       `cmd:"" help:"Show repository facet coverage"`
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

type coverageCmd struct {
	OwnerRepo string `arg:"" name:"owner/repo" help:"Repository as OWNER/REPO"`
	JSON      bool   `name:"json" help:"Print the result as JSON"`
}

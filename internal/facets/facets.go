// Package facets owns the names and selection policy for stored thread facets.
// Protocol and application adapters should project this catalog rather than
// repeating facet names or default sets.
package facets

// Thread kinds used by facet selection policy.
const (
	IssueKind       = "issue"
	PullRequestKind = "pull_request"
)

// Facet names are stable corpus keys. Health-only facets are included here so
// every stored facet has one owner, even when it is not selectable hydration.
const (
	IssueComments    = "issue_comments"
	PRDetails        = "pr_details"
	PRReviews        = "pr_reviews"
	PRReviewComments = "pr_review_comments"
	PRChecks         = "pr_checks"
	PRReviewThreads  = "pr_review_threads"
	PRMergeState     = "pr_merge_state"
	PRMergeQueue     = "pr_merge_queue"
	PRClosingIssues  = "pr_closing_issues"
	PRFiles          = "pr_files"
	IssueTimeline    = "issue_timeline"
)

type definition struct {
	name                  string
	hydratable            bool
	defaultForIssue       bool
	defaultForPullRequest bool
	explicitOnly          bool
}

var catalog = [...]definition{
	{name: IssueComments, hydratable: true, defaultForIssue: true, defaultForPullRequest: true},
	{name: PRDetails, hydratable: true, defaultForPullRequest: true},
	{name: PRReviews, hydratable: true, defaultForPullRequest: true},
	{name: PRReviewComments, hydratable: true, defaultForPullRequest: true},
	{name: PRChecks},
	{name: PRReviewThreads},
	{name: PRMergeState},
	{name: PRMergeQueue},
	{name: PRClosingIssues},
	{name: PRFiles},
	{name: IssueTimeline, hydratable: true, explicitOnly: true},
}

// DefaultFor returns the default hydration facets for a thread kind.
func DefaultFor(kind string) []string {
	result := make([]string, 0, len(catalog))
	for _, facet := range catalog {
		if (kind == IssueKind && facet.defaultForIssue) || (kind == PullRequestKind && facet.defaultForPullRequest) {
			result = append(result, facet.name)
		}
	}
	return result
}

// SelectableFor returns facets accepted for explicit hydration of a thread
// kind. Timeline is intentionally explicit-only because it may be large.
func SelectableFor(kind string) []string {
	result := DefaultFor(kind)
	if len(result) == 0 {
		return nil
	}
	for _, facet := range catalog {
		if facet.hydratable && facet.explicitOnly {
			result = append(result, facet.name)
		}
	}
	return result
}

// SelectableNames returns the union used by protocol schemas. Per-thread
// applicability remains enforced by the application service.
func SelectableNames() []string {
	seen := make(map[string]struct{}, len(catalog))
	result := make([]string, 0, len(catalog))
	for _, kind := range []string{IssueKind, PullRequestKind} {
		for _, facet := range SelectableFor(kind) {
			if _, ok := seen[facet]; ok {
				continue
			}
			seen[facet] = struct{}{}
			result = append(result, facet)
		}
	}
	return result
}

// AllNames returns every stored facet key, including health-only projections.
func AllNames() []string {
	result := make([]string, 0, len(catalog))
	for _, facet := range catalog {
		result = append(result, facet.name)
	}
	return result
}

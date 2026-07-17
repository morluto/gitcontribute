// Package health computes deterministic repository health and community metrics
// from already stored public corpus facts. It performs no network access and does
// not infer maintainer identity beyond what author-association metadata supports.
package health

import (
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// RepoRef is the shared repository identifier.
type RepoRef = domain.RepoRef

// Window describes the time bounds and label for a metric group.
type Window struct {
	Start time.Time
	End   time.Time
	Label string
}

// Options controls the analysis window and thresholds.
type Options struct {
	// Now is the reference time for staleness and age calculations.
	// Zero defaults to time.Now().UTC().
	Now time.Time
	// Start and End bound the activity window. Zero Start means all observed
	// history; zero End means Now.
	Start, End time.Time
	// StaleThreshold marks an open PR stale after this duration without activity.
	// Zero defaults to 14 days.
	StaleThreshold time.Duration
}

// Report is a deterministic, coverage-aware health snapshot.
type Report struct {
	Repo         domain.RepoRef
	GeneratedAt  time.Time
	Window       Window
	Repository   RepositoryMetrics
	Issues       IssueMetrics
	PullRequests PullRequestMetrics
	External     ExternalContributorMetrics
	Congestion   CongestionMetrics
	Stale        StaleMetrics
	Response     ResponseTimeDistributions
	Coverage     CoverageSummary
}

// RepositoryMetrics surfaces the stored repository projection counts.
type RepositoryMetrics struct {
	Present       bool
	Stars         int
	Watchers      int
	Forks         int
	OpenIssues    int
	Archived      bool
	Fork          bool
	DefaultBranch string
	License       string
	Coverage      string
}

// IssueMetrics counts issues by state.
type IssueMetrics struct {
	Window     Window
	SampleSize int
	Open       int
	Closed     int
	Coverage   string
}

// PullRequestMetrics counts pull requests by state.
type PullRequestMetrics struct {
	Window         Window
	SampleSize     int
	Open           int
	Merged         int
	ClosedUnmerged int
	Coverage       string
}

// ExternalContributorMetrics reports PR outcomes for authors whose public
// author association is not owner/member/collaborator/mannequin.
type ExternalContributorMetrics struct {
	Window         Window
	SampleSize     int // PRs considered within the window
	Known          int // PRs with a non-empty author association
	External       int // PRs classified as external contributors
	Open           int
	Merged         int
	ClosedUnmerged int
	MergeRate      float64
	Coverage       string
}

// CongestionMetrics describes the current open PR backlog and age distribution.
type CongestionMetrics struct {
	Window     Window
	SampleSize int
	OpenPRs    int
	MedianAge  float64 // hours
	P90Age     float64 // hours
	MaxAge     float64 // hours
	AgeBuckets []AgeBucket
	Coverage   string
}

// AgeBucket is a labeled open-PR age group.
type AgeBucket struct {
	Label string
	Count int
}

// StaleMetrics reports how many open PRs lack recent review or update activity.
type StaleMetrics struct {
	Window                 Window
	SampleSize             int
	StaleCount             int     // latest activity older than the threshold
	ActiveCount            int     // recent activity within the threshold
	NoReviewOrCommentCount int     // no review/comment events at all
	MissingCoverageCount   int     // cannot determine activity from stored facts
	Threshold              float64 // hours
	Coverage               string
}

// ResponseTimeDistributions reports first-response times for issues and PRs.
type ResponseTimeDistributions struct {
	Issues       ResponseTimeMetric
	PullRequests ResponseTimeMetric
}

// ResponseTimeMetric is a first-response-time distribution.
type ResponseTimeMetric struct {
	Window     Window
	SampleSize int
	Median     float64 // hours
	P90        float64 // hours
	Mean       float64 // hours
	Min        float64 // hours
	Max        float64 // hours
	Coverage   string
}

// CoverageSummary reports top-level data-availability notes.
type CoverageSummary struct {
	ThreadsLimit         int
	ThreadsTruncated     bool
	ThreadsSampleSize    int
	RepositoryProjection bool
}

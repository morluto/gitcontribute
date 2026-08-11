// Package health computes deterministic repository health and community metrics
// from already stored public corpus facts. It performs no network access and does
// not infer maintainer identity beyond what author-association metadata supports.
package health

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// Window describes the time bounds and label for a metric group.
type Window struct {
	Start time.Time `json:"start,omitempty"`
	End   time.Time `json:"end,omitempty"`
	Label string    `json:"label"`
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
	Repo         domain.RepoRef             `json:"repo"`
	GeneratedAt  time.Time                  `json:"generated_at"`
	Window       Window                     `json:"window"`
	Repository   RepositoryMetrics          `json:"repository"`
	Issues       IssueMetrics               `json:"issues"`
	PullRequests PullRequestMetrics         `json:"pull_requests"`
	External     ExternalContributorMetrics `json:"external_contributors"`
	Congestion   CongestionMetrics          `json:"congestion"`
	Stale        StaleMetrics               `json:"stale"`
	Response     ResponseTimeDistributions  `json:"response_times"`
	Coverage     CoverageSummary            `json:"coverage"`
}

// RepositoryMetrics surfaces the stored repository projection counts.
type RepositoryMetrics struct {
	Present       bool   `json:"present"`
	Stars         int    `json:"stars"`
	Watchers      int    `json:"watchers"`
	Forks         int    `json:"forks"`
	OpenIssues    int    `json:"open_issues"`
	Archived      bool   `json:"archived"`
	Fork          bool   `json:"fork"`
	DefaultBranch string `json:"default_branch,omitempty"`
	License       string `json:"license,omitempty"`
	Coverage      string `json:"coverage"`
}

// IssueMetrics counts issues by state.
type IssueMetrics struct {
	Window     Window `json:"window"`
	SampleSize int    `json:"sample_size"`
	Open       int    `json:"open"`
	Closed     int    `json:"closed"`
	Coverage   string `json:"coverage"`
}

// PullRequestMetrics counts pull requests by state.
type PullRequestMetrics struct {
	Window             Window `json:"window"`
	SampleSize         int    `json:"sample_size"`
	Open               int    `json:"open"`
	Merged             int    `json:"merged"`
	ClosedUnmerged     int    `json:"closed_unmerged"`
	ClosedUnknownMerge int    `json:"closed_unknown_merge"`
	Coverage           string `json:"coverage"`
}

// ExternalContributorMetrics reports PR outcomes for authors whose public
// author association is not owner/member/collaborator/mannequin.
type ExternalContributorMetrics struct {
	Window             Window   `json:"window"`
	SampleSize         int      `json:"sample_size"`       // PRs considered within the window
	Known              int      `json:"known_association"` // PRs with a non-empty author association
	External           int      `json:"external"`          // PRs classified as external contributors
	Open               int      `json:"open"`
	Merged             int      `json:"merged"`
	ClosedUnmerged     int      `json:"closed_unmerged"`
	ClosedUnknownMerge int      `json:"closed_unknown_merge"`
	MergeRate          *float64 `json:"merge_rate"`
	Coverage           string   `json:"coverage"`
}

// CongestionMetrics describes the current open PR backlog and age distribution.
type CongestionMetrics struct {
	Window     Window      `json:"window"`
	SampleSize int         `json:"sample_size"`
	OpenPRs    int         `json:"open_prs"`
	MedianAge  float64     `json:"median_age_hours"`
	P90Age     float64     `json:"p90_age_hours"`
	MaxAge     float64     `json:"max_age_hours"`
	AgeBuckets []AgeBucket `json:"age_buckets"`
	Coverage   string      `json:"coverage"`
}

// AgeBucket is a labeled open-PR age group.
type AgeBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// StaleMetrics reports how many open PRs lack recent review or update activity.
type StaleMetrics struct {
	Window                 Window  `json:"window"`
	SampleSize             int     `json:"sample_size"`
	StaleCount             int     `json:"stale"`
	ActiveCount            int     `json:"active"`
	NoReviewOrCommentCount int     `json:"without_review_or_comment"`
	MissingCoverageCount   int     `json:"missing_coverage"`
	Threshold              float64 `json:"threshold_hours"`
	Coverage               string  `json:"coverage"`
}

// ResponseTimeDistributions reports first-response times for issues and PRs.
type ResponseTimeDistributions struct {
	Issues       ResponseTimeMetric `json:"issues"`
	PullRequests ResponseTimeMetric `json:"pull_requests"`
}

// ResponseTimeMetric is a first-response-time distribution.
type ResponseTimeMetric struct {
	Window     Window  `json:"window"`
	SampleSize int     `json:"sample_size"`
	Source     string  `json:"source"`
	Median     float64 `json:"median_hours"`
	P90        float64 `json:"p90_hours"`
	Mean       float64 `json:"mean_hours"`
	Min        float64 `json:"min_hours"`
	Max        float64 `json:"max_hours"`
	Coverage   string  `json:"coverage"`
}

// CoverageSummary reports top-level data-availability notes.
type CoverageSummary struct {
	ThreadsLimit         int  `json:"threads_limit"`
	ThreadsSampleSize    int  `json:"threads_sample_size"`
	RepositoryProjection bool `json:"repository_projection"`
	threads              ThreadCoverage
}

// ThreadCoverage is the mutually exclusive completeness state of the bounded
// repository thread sample. The zero value represents an uncomputed report.
type ThreadCoverage string

const (
	ThreadCoverageComplete  ThreadCoverage = "complete"
	ThreadCoverageTruncated ThreadCoverage = "truncated"
)

// ThreadsComplete reports whether every stored thread was included.
func (s CoverageSummary) ThreadsComplete() bool { return s.threads == ThreadCoverageComplete }

// ThreadsTruncated reports whether the bounded sample omitted stored threads.
func (s CoverageSummary) ThreadsTruncated() bool { return s.threads == ThreadCoverageTruncated }

func newCoverageSummary(limit, sampleSize int, complete, repositoryProjection bool) CoverageSummary {
	coverage := ThreadCoverageTruncated
	if complete {
		coverage = ThreadCoverageComplete
	}
	return CoverageSummary{
		ThreadsLimit: limit, ThreadsSampleSize: sampleSize,
		RepositoryProjection: repositoryProjection, threads: coverage,
	}
}

// MarshalJSON preserves the public complementary boolean representation while
// keeping one authoritative state in memory.
func (s CoverageSummary) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ThreadsLimit         int  `json:"threads_limit"`
		ThreadsComplete      bool `json:"threads_complete"`
		ThreadsTruncated     bool `json:"threads_truncated"`
		ThreadsSampleSize    int  `json:"threads_sample_size"`
		RepositoryProjection bool `json:"repository_projection"`
	}{
		ThreadsLimit: s.ThreadsLimit, ThreadsComplete: s.ThreadsComplete(),
		ThreadsTruncated: s.ThreadsTruncated(), ThreadsSampleSize: s.ThreadsSampleSize,
		RepositoryProjection: s.RepositoryProjection,
	})
}

// UnmarshalJSON parses the legacy public booleans into one coverage state.
func (s *CoverageSummary) UnmarshalJSON(data []byte) error {
	var raw struct {
		ThreadsLimit         int  `json:"threads_limit"`
		ThreadsComplete      bool `json:"threads_complete"`
		ThreadsTruncated     bool `json:"threads_truncated"`
		ThreadsSampleSize    int  `json:"threads_sample_size"`
		RepositoryProjection bool `json:"repository_projection"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ThreadsComplete && raw.ThreadsTruncated {
		return fmt.Errorf("thread coverage cannot be both complete and truncated")
	}
	coverage := ThreadCoverage("")
	if raw.ThreadsComplete {
		coverage = ThreadCoverageComplete
	} else if raw.ThreadsTruncated {
		coverage = ThreadCoverageTruncated
	}
	*s = CoverageSummary{
		ThreadsLimit: raw.ThreadsLimit, ThreadsSampleSize: raw.ThreadsSampleSize,
		RepositoryProjection: raw.RepositoryProjection, threads: coverage,
	}
	return nil
}

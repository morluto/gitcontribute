package corpus

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// Repository is the current projection of a GitHub repository.
type Repository struct {
	ID                  int64
	Owner               string
	Name                string
	ExternalID          string
	Description         string
	DefaultBranch       string
	Language            string
	License             string
	Topics              []string
	Stars               int
	Watchers            int
	Forks               int
	OpenIssues          int
	Archived            bool
	Fork                bool
	SourceCreatedAt     time.Time
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	// Rank is query-specific and populated only by full-text search results.
	Rank float64
}

// RepositoryObservation is an immutable snapshot received from a source.
type RepositoryObservation struct {
	ID                  int64
	RepositoryID        int64
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	Payload             string
	ObservedAt          time.Time
}

// Thread is the current projection of an issue or pull request.
type Thread struct {
	ID                  int64
	RepositoryID        int64
	Kind                domain.ThreadKind
	Number              int
	State               domain.ThreadState
	StateReason         string
	Title               string
	Body                string
	Author              string
	AuthorAssociation   string
	Labels              []string
	Assignees           []string
	Draft               bool
	Locked              bool
	Milestone           string
	ClosedAt            time.Time
	Merge               domain.MergeStatus
	SourceCreatedAt     time.Time
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	// Rank and match fields are query-specific and populated only by search results.
	Rank           float64
	MatchTitle     bool
	MatchSource    string
	MatchExcerpt   string
	MatchUpdatedAt time.Time
	MatchTruncated bool
}

// ThreadSearchEvidence is the stored document that made an exact thread match.
type ThreadSearchEvidence struct {
	Source          string
	Text            string
	Excerpt         string
	SourceUpdatedAt time.Time
	Rank            float64
	Truncated       bool
}

// PortfolioPullRequest identifies a pull request together with the repository
// that owns it. It is returned by global, offline portfolio reads.
type PortfolioPullRequest struct {
	Owner  string
	Repo   string
	Thread Thread
}

// PortfolioPage reports the complete matching population separately from the
// bounded returned items.
type PortfolioPage struct {
	PullRequests []PortfolioPullRequest
	Total        int
	Truncated    bool
}

// ThreadObservation is an immutable snapshot received from a source.
type ThreadObservation struct {
	ID                  int64
	ThreadID            int64
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	Payload             string
	ObservedAt          time.Time
}

// FacetObservation is an immutable snapshot of a thread facet (comments,
// reviews, review comments, or PR details) received from a source.
type FacetObservation struct {
	ID                  int64
	RepositoryID        int64
	ThreadID            *int64
	Facet               string
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	Payload             string
	ObservedAt          time.Time
}

// Coverage records which hydration facet has been fetched for a repository or
// thread and whether it is complete. Each facet advances independently under
// the same source_updated_at/observation_sequence ordering as projections.
type Coverage struct {
	ID                  int64
	RepositoryID        int64
	ThreadID            *int64
	Facet               string
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	Complete            bool
	RunID               *int64
	UpdatedAt           time.Time
}

// Run records a crawl, hydration, indexing, or validation attempt.
type Run struct {
	ID        int64
	Kind      string
	State     RunState
	StartedAt time.Time
	Stats     string
	Error     string
}

// RunEvent is a durable log line emitted during a run.
type RunEvent struct {
	ID         int64
	RunID      int64
	Level      string
	Message    string
	RecordedAt time.Time
}

// RunStatus is a persisted run lifecycle value.
type RunStatus string

// RunStatus values.
const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusPartial   RunStatus = "partial"
	RunStatusFailed    RunStatus = "failed"
)

// RunState binds a run status to the completion time required by terminal
// states. Its zero value is invalid.
type RunState struct {
	status      RunStatus
	completedAt time.Time
}

func parseRunState(status string, completedAt *time.Time) (RunState, error) {
	parsed := RunStatus(status)
	switch parsed {
	case RunStatusRunning:
		if completedAt != nil {
			return RunState{}, errors.New("running run cannot have a completion time")
		}
		return RunState{status: parsed}, nil
	case RunStatusCompleted, RunStatusPartial, RunStatusFailed:
		if completedAt == nil || completedAt.IsZero() {
			return RunState{}, fmt.Errorf("%s run requires a completion time", parsed)
		}
		return RunState{status: parsed, completedAt: *completedAt}, nil
	default:
		return RunState{}, fmt.Errorf("unknown run status %q", status)
	}
}

func (s RunState) Status() RunStatus { return s.status }

func (s RunState) CompletedAt() (time.Time, bool) {
	return s.completedAt, !s.completedAt.IsZero()
}

// JobStatus is a parsed durable job lifecycle value. The zero value means no
// status filter; it is never a persisted job state.
type JobStatus string

// JobStatus values for the durable job lifecycle.
const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// ParseJobStatus parses a persisted or boundary job status.
func ParseJobStatus(value string) (JobStatus, error) {
	status := JobStatus(strings.TrimSpace(value))
	switch status {
	case JobStatusQueued, JobStatusRunning, JobStatusSucceeded, JobStatusFailed, JobStatusCancelled:
		return status, nil
	default:
		return "", fmt.Errorf("unknown job status %q", value)
	}
}

// ParseJobStatusFilter parses an optional boundary filter. Its zero value
// selects jobs in every status.
func ParseJobStatusFilter(value string) (JobStatus, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return ParseJobStatus(value)
}

// String returns the stable persisted and boundary spelling.
func (s JobStatus) String() string { return string(s) }

// Terminal reports whether no further lifecycle transition is valid.
func (s JobStatus) Terminal() bool {
	return s == JobStatusSucceeded || s == JobStatusFailed || s == JobStatusCancelled
}

// JobTransition is one structurally valid durable lifecycle transition.
type JobTransition uint8

// Valid transitions performed by TransitionJob. Queued-to-running is owned by
// StartJobAs because it also claims an executor owner.
const (
	JobQueuedToCancelled JobTransition = iota + 1
	JobQueuedToFailed
	JobRunningToSucceeded
	JobRunningToFailed
	JobRunningToCancelled
)

// From returns the required current status, or the zero value for an invalid
// transition representation.
func (t JobTransition) From() JobStatus {
	switch t {
	case JobQueuedToCancelled, JobQueuedToFailed:
		return JobStatusQueued
	case JobRunningToSucceeded, JobRunningToFailed, JobRunningToCancelled:
		return JobStatusRunning
	default:
		return ""
	}
}

// To returns the terminal target status, or the zero value for an invalid
// transition representation.
func (t JobTransition) To() JobStatus {
	switch t {
	case JobQueuedToCancelled, JobRunningToCancelled:
		return JobStatusCancelled
	case JobQueuedToFailed, JobRunningToFailed:
		return JobStatusFailed
	case JobRunningToSucceeded:
		return JobStatusSucceeded
	default:
		return ""
	}
}

// Job is a durable, cancellable unit of work.
type Job struct {
	ID         string
	Kind       string
	State      JobState
	Request    string
	Result     string
	Error      string
	Progress   string
	Statistics string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// JobState binds lifecycle timestamps and cancellation requests to the statuses
// in which they are meaningful. Its zero value is invalid.
type JobState struct {
	status      JobStatus
	startedAt   time.Time
	completedAt time.Time
	cancelledAt time.Time
}

func parseJobState(status string, startedAt, completedAt, cancelledAt *time.Time) (JobState, error) {
	parsed, err := ParseJobStatus(status)
	if err != nil {
		return JobState{}, err
	}
	state := JobState{status: parsed}
	if startedAt != nil {
		state.startedAt = *startedAt
	}
	if completedAt != nil {
		state.completedAt = *completedAt
	}
	if cancelledAt != nil {
		state.cancelledAt = *cancelledAt
	}
	switch parsed {
	case JobStatusQueued:
		if startedAt != nil || completedAt != nil || cancelledAt != nil {
			return JobState{}, errors.New("queued job cannot have lifecycle timestamps")
		}
	case JobStatusRunning:
		if startedAt == nil || startedAt.IsZero() {
			return JobState{}, errors.New("running job requires a start time")
		}
		if completedAt != nil {
			return JobState{}, errors.New("running job cannot have a completion time")
		}
	case JobStatusSucceeded, JobStatusFailed:
		if completedAt == nil || completedAt.IsZero() {
			return JobState{}, fmt.Errorf("%s job requires a completion time", parsed)
		}
		if cancelledAt != nil {
			return JobState{}, fmt.Errorf("%s job cannot have a cancellation time", parsed)
		}
	case JobStatusCancelled:
		if completedAt == nil || completedAt.IsZero() || cancelledAt == nil || cancelledAt.IsZero() {
			return JobState{}, errors.New("cancelled job requires completion and cancellation times")
		}
	}
	return state, nil
}

func (s JobState) Status() JobStatus { return s.status }

func (s JobState) StartedAt() (time.Time, bool) { return s.startedAt, !s.startedAt.IsZero() }

func (s JobState) CompletedAt() (time.Time, bool) { return s.completedAt, !s.completedAt.IsZero() }

func (s JobState) CancelledAt() (time.Time, bool) { return s.cancelledAt, !s.cancelledAt.IsZero() }

func (s JobState) CancellationRequested() bool { return !s.cancelledAt.IsZero() }

// JobEvent is a durable log line emitted during a job.
type JobEvent struct {
	ID         int64
	JobID      string
	Level      string
	Message    string
	RecordedAt time.Time
}

// DossierRecord is a persisted deterministic dossier snapshot.
type DossierRecord struct {
	ID              int64
	RepositoryID    int64
	RepoOwner       string
	RepoName        string
	CommitSHA       string
	AsOf            time.Time
	SectionMetadata string
	Snapshot        string
	GeneratedAt     time.Time
	CreatedAt       time.Time
}

// DossierMetadata is the small latest-snapshot projection needed by bounded
// repository reads.
type DossierMetadata struct {
	RepositoryID int64
	AsOf         time.Time
	GeneratedAt  time.Time
}

// DossierSource is one exact source recorded for a dossier.
type DossierSource struct {
	ID         int64
	DossierID  int64
	Source     string
	URL        string
	CommitSHA  string
	ObservedAt time.Time
	AsOf       time.Time
}

package corpus

import "time"

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
	Kind                string
	Number              int
	State               string
	Title               string
	Body                string
	Author              string
	Labels              []string
	ClosedAt            time.Time
	MergedAt            time.Time
	Merged              bool
	SourceCreatedAt     time.Time
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	// Rank is the query-specific FTS rank populated only by search results.
	Rank float64
}

// ThreadKind names the thread types stored by the corpus.
const (
	ThreadKindIssue       = "issue"
	ThreadKindPullRequest = "pull_request"
)

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
	ID          int64
	Kind        string
	Status      string
	StartedAt   time.Time
	CompletedAt *time.Time
	Stats       string
	Error       string
}

// RunEvent is a durable log line emitted during a run.
type RunEvent struct {
	ID         int64
	RunID      int64
	Level      string
	Message    string
	RecordedAt time.Time
}

// RunStatus values.
const (
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusPartial   = "partial"
	RunStatusFailed    = "failed"
)

// JobStatus values for the durable job lifecycle.
const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
)

// Job is a durable, cancellable unit of work.
type Job struct {
	ID          string
	Kind        string
	Status      string
	Request     string
	Result      string
	Error       string
	Progress    string
	Statistics  string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	UpdatedAt   time.Time
	CancelledAt *time.Time
}

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

package corpus

import "time"

// Repository is the current projection of a GitHub repository.
type Repository struct {
	ID                  int64
	Owner               string
	Name                string
	ExternalID          string
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
	SourceUpdatedAt     time.Time
	ObservationSequence int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
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
	RunStatusFailed    = "failed"
)

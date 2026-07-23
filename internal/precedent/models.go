// Package precedent owns dependency-neutral models used to load and rank
// historical threads without leaking database adapter types into application
// logic.
package precedent

import (
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// SourceRef identifies an input thread for precedent discovery.
type SourceRef struct {
	Repository domain.RepoRef
	Number     int
}

// Thread is the stored thread data needed by precedent scoring and output.
type Thread struct {
	ID          int64
	Kind        string
	Number      int
	State       string
	StateReason string
	Title       string
	Body        string
	Labels      []string
	ClosedAt    time.Time
	MergedAt    time.Time
	Merged      bool
}

// RepositorySnapshot contains all source threads and bounded closed history
// needed to score every input for one repository.
type RepositorySnapshot struct {
	Repository      domain.RepoRef
	Available       bool
	Sources         map[int]Thread
	Closed          []Thread
	ClosedTotal     int
	ClosedTruncated bool
}

// RepositoryKey provides a stable case-insensitive grouping key.
func RepositoryKey(ref domain.RepoRef) string {
	return strings.ToLower(ref.Owner) + "/" + strings.ToLower(ref.Repo)
}

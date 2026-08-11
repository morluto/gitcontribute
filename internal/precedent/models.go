// Package precedent owns dependency-neutral models used to load and rank
// historical threads without leaking database adapter types into application
// logic.
package precedent

import (
	"errors"
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
	Kind        domain.ThreadKind
	Number      int
	State       domain.ThreadState
	StateReason string
	Title       string
	Body        string
	Labels      []string
	ClosedAt    time.Time
	Merge       domain.MergeStatus
}

// RepositorySnapshot contains all source threads and bounded closed history
// needed to score every input for one repository.
type RepositorySnapshot struct {
	Repository      domain.RepoRef
	available       bool
	Sources         map[int]Thread
	Closed          []Thread
	ClosedTotal     int
	ClosedTruncated bool
}

// MissingRepositorySnapshot records that the requested repository is absent
// from the local corpus.
func MissingRepositorySnapshot(repository domain.RepoRef) RepositorySnapshot {
	return RepositorySnapshot{Repository: repository, Sources: map[int]Thread{}}
}

// AvailableRepositorySnapshot records one bounded, locally stored history.
func AvailableRepositorySnapshot(repository domain.RepoRef, sources map[int]Thread, closed []Thread, closedTotal int) (RepositorySnapshot, error) {
	if !repository.IsValid() || closedTotal < len(closed) {
		return RepositorySnapshot{}, errors.New("available precedent snapshot requires a repository and a complete population bound")
	}
	sourceCopy := make(map[int]Thread, len(sources))
	for number, thread := range sources {
		sourceCopy[number] = cloneThread(thread)
	}
	closedCopy := make([]Thread, len(closed))
	for index, thread := range closed {
		closedCopy[index] = cloneThread(thread)
	}
	return RepositorySnapshot{
		Repository: repository, available: true, Sources: sourceCopy, Closed: closedCopy,
		ClosedTotal: closedTotal, ClosedTruncated: len(closedCopy) < closedTotal,
	}, nil
}

func (s RepositorySnapshot) Available() bool { return s.available }

func cloneThread(thread Thread) Thread {
	thread.Labels = append([]string{}, thread.Labels...)
	return thread
}

// RepositoryKey provides a stable case-insensitive grouping key.
func RepositoryKey(ref domain.RepoRef) string {
	return strings.ToLower(ref.Owner()) + "/" + strings.ToLower(ref.Repo())
}

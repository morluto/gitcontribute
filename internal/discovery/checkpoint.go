package discovery

import (
	"context"
	"sync"
	"time"
)

// CheckpointStore is a small durable interface used by discovery sources.
// Implementations may be in-memory, file-backed, or persisted elsewhere, but
// must not perform network access or GitHub mutations.
type CheckpointStore interface {
	// GetTime returns the timestamp checkpoint for key. If no checkpoint exists,
	// the bool is false.
	GetTime(ctx context.Context, key string) (time.Time, bool, error)

	// SetTime stores a timestamp checkpoint for key.
	SetTime(ctx context.Context, key string, t time.Time) error

	// IsImported reports whether the given GH Archive hour has already been
	// imported.
	IsImported(ctx context.Context, hour string) (bool, error)

	// MarkImported records the given GH Archive hour as imported.
	MarkImported(ctx context.Context, hour string) error
}

// MemoryCheckpointStore is an in-memory CheckpointStore for tests and
// short-lived local use.
type MemoryCheckpointStore struct {
	mu    sync.Mutex
	times map[string]time.Time
	hours map[string]struct{}
}

// NewMemoryCheckpointStore returns a new in-memory checkpoint store.
func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{
		times: make(map[string]time.Time),
		hours: make(map[string]struct{}),
	}
}

// GetTime returns the timestamp checkpoint for key.
func (m *MemoryCheckpointStore) GetTime(ctx context.Context, key string) (time.Time, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.times[key]
	return t, ok, nil
}

// SetTime stores a timestamp checkpoint for key.
func (m *MemoryCheckpointStore) SetTime(ctx context.Context, key string, t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.times[key] = t
	return nil
}

// IsImported reports whether the given hour has already been imported.
func (m *MemoryCheckpointStore) IsImported(ctx context.Context, hour string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.hours[hour]
	return ok, nil
}

// MarkImported records the given hour as imported.
func (m *MemoryCheckpointStore) MarkImported(ctx context.Context, hour string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hours[hour] = struct{}{}
	return nil
}

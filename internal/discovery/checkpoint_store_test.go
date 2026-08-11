package discovery

import (
	"context"
	"sync"
	"time"
)

type MemoryCheckpointStore struct {
	mu    sync.Mutex
	times map[string]time.Time
	hours map[string]struct{}
}

func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{
		times: make(map[string]time.Time),
		hours: make(map[string]struct{}),
	}
}

func (m *MemoryCheckpointStore) GetTime(ctx context.Context, key string) (time.Time, bool, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.times[key]
	return t, ok, nil
}

func (m *MemoryCheckpointStore) SetTime(ctx context.Context, key string, value time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.times[key] = value
	return nil
}

func (m *MemoryCheckpointStore) IsImported(ctx context.Context, hour string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.hours[hour]
	return ok, nil
}

func (m *MemoryCheckpointStore) MarkImported(ctx context.Context, hour string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hours[hour] = struct{}{}
	return nil
}

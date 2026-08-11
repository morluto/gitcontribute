package discovery

import (
	"context"
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

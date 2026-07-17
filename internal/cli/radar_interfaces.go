package cli

import (
	"context"

	"github.com/morluto/gitcontribute/internal/radar"
)

// RadarService exposes explainable contribution ranking as a separate,
// optional offline-read capability.
type RadarService interface {
	ContributionRadar(ctx context.Context, opts RadarOptions) (*radar.Report, error)
}

// RadarOptions scopes one bounded, offline contribution ranking.
type RadarOptions struct {
	Repo  RepoRef
	Limit int
}

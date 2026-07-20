package cli

import (
	"context"
	"time"
)

// ArchiveService exposes explicit network-reading archive operations.
type ArchiveService interface {
	SyncPlanningService
	ArchiveSync(ctx context.Context, repo RepoRef, opts ArchiveSyncOptions) (*SyncResult, error)
	Hydrate(ctx context.Context, repo RepoRef, number int, opts HydrateOptions) (*HydrateResult, error)
}

// SyncPlanningService computes a bounded request plan without network or
// corpus access.
type SyncPlanningService interface {
	PlanArchiveSync(ctx context.Context, repo RepoRef, opts ArchiveSyncOptions) (*SyncPlanResult, error)
}

// ArchiveSyncOptions bounds and filters one explicit archive synchronization.
type ArchiveSyncOptions struct {
	State       string
	Since       time.Duration
	Numbers     []int
	MaxPages    int
	MaxRequests int
}

// HydrateOptions selects bounded child facets for one stored thread.
type HydrateOptions struct {
	Facets   []string
	MaxPages int
}

// HydrateResult reports the facets retrieved for one issue or pull request.
type HydrateResult struct {
	Repo     RepoRef         `json:"repo"`
	Number   int             `json:"number"`
	Kind     string          `json:"kind"`
	Facets   []HydratedFacet `json:"facets"`
	Pages    int             `json:"pages"`
	Requests int             `json:"requests"`
	Message  string          `json:"message"`
}

// HydratedFacet reports one retrieved facet's item and coverage counts.
type HydratedFacet struct {
	Facet    string `json:"facet"`
	Count    int    `json:"count"`
	Pages    int    `json:"pages"`
	Complete bool   `json:"complete"`
}

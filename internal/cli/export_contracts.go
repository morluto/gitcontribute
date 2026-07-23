package cli

import "context"

// ExportService renders redacted, deterministic local bundles.
type ExportService interface {
	ExportDossier(ctx context.Context, repo RepoRef, format string) (*ExportResult, error)
	ExportEvidence(ctx context.Context, investigationID, format string) (*ExportResult, error)
	ExportManifest(ctx context.Context, opportunityID string, opts ManifestExportOptions) (*ExportResult, error)
}

// ManifestExportOptions selects optional local identities for a manifest export.
type ManifestExportOptions struct {
	WorkspaceID string
	PullRequest *ManifestPullRequestRef
}

// ManifestPullRequestRef identifies one exact stored pull request.
type ManifestPullRequestRef struct {
	Owner  string
	Repo   string
	Number int
}

// ExportResult contains one rendered local export.
type ExportResult struct {
	Kind    string `json:"kind"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

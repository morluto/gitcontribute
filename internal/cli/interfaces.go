package cli

import "context"

// Service is the product-owned application interface used by the CLI and MCP
// adapters. Implementations live outside the CLI package and must not leak
// CLI or transport concerns.
type Service interface {
	Init(ctx context.Context) (*InitResult, error)
	Status(ctx context.Context) (*StatusResult, error)
	Sync(ctx context.Context, repo RepoRef) (*SyncResult, error)
	Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)
	Dossier(ctx context.Context, repo RepoRef) (*DossierResult, error)
	Index(ctx context.Context, repo RepoRef, path string) (*IndexResult, error)
}

// MCPRunner is the product-owned boundary for running an MCP server. The CLI
// adapter dispatches to it and does not own MCP protocol details.
type MCPRunner interface {
	Run(ctx context.Context, opts MCPOptions) error
}

// RepoRef identifies a GitHub repository.
type RepoRef struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

func (r RepoRef) String() string { return r.Owner + "/" + r.Repo }

// MCPOptions carries MCP server startup options.
type MCPOptions struct {
	Transport string
}

// SearchOptions carries parameters for a local corpus search.
type SearchOptions struct {
	Kind  string
	Repo  string
	Limit int
}

// InitResult is the result of initializing a local corpus.
type InitResult struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// StatusResult reports the health and identity of the local corpus.
type StatusResult struct {
	Healthy bool   `json:"healthy"`
	Corpus  string `json:"corpus"`
	Version string `json:"version"`
	Message string `json:"message"`
}

// SyncResult reports the outcome of syncing a repository.
type SyncResult struct {
	Repo    RepoRef `json:"repo"`
	Updated int     `json:"updated"`
	Message string  `json:"message"`
}

// SearchMatch is one local search result.
type SearchMatch struct {
	Kind   string  `json:"kind"`
	Repo   RepoRef `json:"repo"`
	Title  string  `json:"title"`
	Number int     `json:"number,omitempty"`
	URL    string  `json:"url,omitempty"`
	Score  float64 `json:"score"`
}

// SearchResult is the result of a local corpus search.
type SearchResult struct {
	Query   string        `json:"query"`
	Kind    string        `json:"kind"`
	Repo    string        `json:"repo,omitempty"`
	Limit   int           `json:"limit"`
	Total   int           `json:"total"`
	Matches []SearchMatch `json:"matches"`
}

// DossierResult is a summary view of a repository.
type DossierResult struct {
	Repo       RepoRef  `json:"repo"`
	Summary    string   `json:"summary"`
	Language   string   `json:"language"`
	Stars      int      `json:"stars"`
	OpenIssues int      `json:"open_issues"`
	Coverage   []string `json:"coverage"`
	Freshness  string   `json:"freshness"`
}

// IndexResult reports one immutable local code snapshot.
type IndexResult struct {
	Repo     RepoRef `json:"repo"`
	Path     string  `json:"path"`
	Commit   string  `json:"commit"`
	Files    int     `json:"files"`
	Bytes    int     `json:"bytes"`
	Inserted bool    `json:"inserted"`
	Message  string  `json:"message"`
}

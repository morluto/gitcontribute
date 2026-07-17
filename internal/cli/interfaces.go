package cli

import (
	"context"
	"time"
)

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

// DiscoveryService is the optional source and crawl capability used by the
// CLI without enlarging the core local archive contract.
type DiscoveryService interface {
	AddSearchSource(ctx context.Context, name, query string) (*SourceResult, error)
	ListSources(ctx context.Context) (*SourceListResult, error)
	Crawl(ctx context.Context, name string, opts CrawlOptions) (*CrawlResult, error)
}

type SourceResult struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Definition string `json:"definition"`
	Enabled    bool   `json:"enabled"`
}

type SourceListResult struct {
	Sources []SourceResult `json:"sources"`
}

type CrawlOptions struct {
	Since  time.Duration
	Budget int
}

type CrawlResult struct {
	Source       string `json:"source"`
	Windows      int    `json:"windows"`
	Repositories int    `json:"repositories"`
	Requests     int    `json:"requests"`
	Checkpoint   string `json:"checkpoint"`
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

// InvestigationService is the optional investigation and opportunity
// management capability used by the CLI.
type InvestigationService interface {
	StartInvestigation(ctx context.Context, repo RepoRef, commit, lens string) (*InvestigationResult, error)
	ShowInvestigation(ctx context.Context, id string) (*InvestigationResult, error)
	ListInvestigations(ctx context.Context) (*InvestigationListResult, error)
	AddHypothesis(ctx context.Context, investigationID, title, description, category string) (*HypothesisResult, error)
	ListHypotheses(ctx context.Context, investigationID string) (*HypothesisListResult, error)
	PromoteOpportunity(ctx context.Context, hypothesisID, problem, scope, impact, effort string, confidence float64) (*OpportunityResult, error)
	ShowOpportunity(ctx context.Context, id string) (*OpportunityResult, error)
	ListOpportunities(ctx context.Context, investigationID string) (*OpportunityListResult, error)
	SetOpportunityStatus(ctx context.Context, id, status, rationale string) (*OpportunityResult, error)
}

// InvestigationResult is a single investigation view.
type InvestigationResult struct {
	ID        string  `json:"id"`
	Repo      RepoRef `json:"repo"`
	CommitSHA string  `json:"commit_sha,omitempty"`
	Lens      string  `json:"lens,omitempty"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// InvestigationListResult is a collection of investigations.
type InvestigationListResult struct {
	Investigations []InvestigationResult `json:"investigations"`
}

// HypothesisResult is a single hypothesis view.
type HypothesisResult struct {
	ID              string `json:"id"`
	InvestigationID string `json:"investigation_id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// HypothesisListResult is a collection of hypotheses.
type HypothesisListResult struct {
	Hypotheses []HypothesisResult `json:"hypotheses"`
}

// OpportunityResult is a single opportunity view.
type OpportunityResult struct {
	ID               string  `json:"id"`
	InvestigationID  string  `json:"investigation_id"`
	HypothesisID     string  `json:"hypothesis_id"`
	Title            string  `json:"title"`
	ProblemStatement string  `json:"problem_statement"`
	Category         string  `json:"category"`
	Scope            string  `json:"scope"`
	Impact           string  `json:"impact"`
	ExpectedEffort   string  `json:"expected_effort"`
	Confidence       float64 `json:"confidence"`
	CollisionStatus  string  `json:"collision_status"`
	Status           string  `json:"status"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// OpportunityListResult is a collection of opportunities.
type OpportunityListResult struct {
	Opportunities []OpportunityResult `json:"opportunities"`
	Filter        string              `json:"filter,omitempty"`
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

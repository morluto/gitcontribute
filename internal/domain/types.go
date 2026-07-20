package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RepoRef identifies a repository by owner and name.
type RepoRef struct {
	Owner string
	Repo  string
}

func (r RepoRef) String() string {
	if r.Owner == "" && r.Repo == "" {
		return ""
	}
	return r.Owner + "/" + r.Repo
}

var (
	errOwnerEmpty = errors.New("owner is required")
	errRepoEmpty  = errors.New("repo is required")
	ownerRegex    = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`)
	repoRegex     = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
)

// Validate checks that the owner and repo are non-empty and syntactically valid.
func (r RepoRef) Validate() error {
	if strings.TrimSpace(r.Owner) == "" {
		return errOwnerEmpty
	}
	if strings.TrimSpace(r.Repo) == "" {
		return errRepoEmpty
	}
	if !ownerRegex.MatchString(r.Owner) {
		return fmt.Errorf("invalid owner %q", r.Owner)
	}
	if !repoRegex.MatchString(r.Repo) {
		return fmt.Errorf("invalid repo %q", r.Repo)
	}
	if r.Repo == "." || r.Repo == ".." || strings.Contains(r.Repo, "..") {
		return fmt.Errorf("invalid repo %q", r.Repo)
	}
	return nil
}

// ThreadKind distinguishes issues from pull requests.
type ThreadKind string

const (
	IssueKind       ThreadKind = "issue"
	PullRequestKind ThreadKind = "pull_request"
)

// ThreadState is the high-level lifecycle state of a thread.
type ThreadState string

const (
	OpenState   ThreadState = "open"
	ClosedState ThreadState = "closed"
)

// Thread is a product-owned model for an issue or pull request.
// It carries no vendor-specific API types.
type Thread struct {
	ID        int64
	Repo      RepoRef
	Kind      ThreadKind
	Number    int
	Title     string
	Body      string
	Author    string
	State     ThreadState
	Draft     bool
	Labels    []string
	Assignees []string
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  time.Time

	// PullRequest is present when Kind is PullRequestKind.
	PullRequest *PullRequestDetails
}

// Comment is a product-owned model for a thread comment.
type Comment struct {
	ID        int64
	Author    string
	Body      string
	CreatedAt time.Time
}

// PullRequestDetails contains PR-specific facets.
type PullRequestDetails struct {
	HeadRef string
	BaseRef string
	HeadSHA string
	BaseSHA string
	Merged  bool
	// MergedKnown distinguishes an observed false value from an unavailable
	// merge state, such as a pull request stored from header-only sync.
	MergedKnown    bool
	MergedAt       time.Time
	MergeCommitSHA string
	Additions      int
	Deletions      int
	ChangedFiles   int
	CIStatus       string
}

// Repository is a product-owned snapshot of repository metadata and counts.
type Repository struct {
	RepoRef
	ID                             int64
	Description                    string
	Topics                         []string
	Languages                      []string
	License                        string
	DefaultBranch                  string
	CommitSHA                      string
	Archived                       bool
	Fork                           bool
	Stars                          int
	Watchers                       int
	Forks                          int
	OpenIssueCount                 int
	ClosedIssueCount               int
	OpenPullRequestCount           int
	MergedPullRequestCount         int
	ClosedUnmergedPullRequestCount int
	ClosedPullRequestUnknownCount  int
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

// FreshnessStatus describes how current a facet is.
type FreshnessStatus string

const (
	Fresh   FreshnessStatus = "fresh"
	Stale   FreshnessStatus = "stale"
	Missing FreshnessStatus = "missing"
)

// Freshness records the observed time and status of a facet.
type Freshness struct {
	Status FreshnessStatus
	AsOf   time.Time
}

// FacetCoverage describes the presence, completeness, and freshness of one facet.
type FacetCoverage struct {
	Facet     string
	Present   bool
	Complete  bool
	Freshness Freshness
	Count     int
}

// Coverage is a product-owned model for corpus facet coverage and freshness.
type Coverage struct {
	AsOf   time.Time
	Facets []FacetCoverage
}

// SourceRef records the exact source, reference, and observation time.
// It carries no vendor-specific metadata objects.
type SourceRef struct {
	Source     string
	URL        string
	CommitSHA  string
	ObservedAt time.Time
	AsOf       time.Time
}

// Page is product-owned cursor pagination.
type Page struct {
	Limit int
	After string
}

// PageInfo describes pagination state for a result set.
type PageInfo struct {
	HasNext bool
	Next    string
	Total   int
}

// SearchQuery is a product-owned search request.
type SearchQuery struct {
	Text   string
	Repo   RepoRef
	Kinds  []ThreadKind
	States []ThreadState
	Labels []string
	Sort   string
	Page   Page
}

// SearchResultItem is one product-owned search result.
type SearchResultItem struct {
	Repo   RepoRef
	Kind   ThreadKind
	Number int
	Title  string
	Score  float64
	Ref    SourceRef
}

// SearchResult is a product-owned search result page.
type SearchResult struct {
	Items []SearchResultItem
	Page  PageInfo
	Ref   SourceRef
}

// DossierThread is a lightweight, deterministic view for dossier listings.
type DossierThread struct {
	Number    int
	Title     string
	Author    string
	State     ThreadState
	Draft     bool
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  time.Time
	MergedAt  time.Time
	Labels    []string
}

// Dossier is a bounded, source-backed repository context package.
type Dossier struct {
	Repo                             RepoRef
	CommitSHA                        string
	AsOf                             time.Time
	SourceRefs                       []SourceRef
	Coverage                         Coverage
	Repository                       Repository
	ContributionGuidance             string
	OpenIssueCount                   int
	ClosedIssueCount                 int
	OpenPullRequestCount             int
	MergedPullRequestCount           int
	ClosedUnmergedPullRequestCount   int
	ClosedPullRequestUnknownCount    int
	RecentMergedPullRequests         []DossierThread
	RecentOpenPullRequests           []DossierThread
	RecentClosedUnmergedPullRequests []DossierThread
	RecentClosedUnknownPullRequests  []DossierThread
	RecentIssues                     []DossierThread
}

// DossierSectionMetadata records the bounded sections used to create a dossier.
type DossierSectionMetadata struct {
	RecentLimit           int      `json:"recent_limit"`
	MergedPRCount         int      `json:"merged_pr_count"`
	OpenPRCount           int      `json:"open_pr_count"`
	ClosedUnmergedPRCount int      `json:"closed_unmerged_pr_count"`
	ClosedUnknownPRCount  int      `json:"closed_unknown_pr_count"`
	IssueCount            int      `json:"issue_count"`
	SourceClasses         []string `json:"source_classes"`
}

// SeedSourceClass identifies the stored thread class a seed was extracted from.
type SeedSourceClass string

const (
	SeedSourceClassMergedPR         SeedSourceClass = "merged_pr"
	SeedSourceClassClosedUnmergedPR SeedSourceClass = "closed_unmerged_pr"
	SeedSourceClassIssue            SeedSourceClass = "issue"
)

// Seed is an evidence-backed record derived from stored merged PRs,
// closed unmerged PRs, or issues.
type Seed struct {
	SourceClass SeedSourceClass
	Number      int
	Title       string
	Author      string
	State       string
	Labels      []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    time.Time
	MergedAt    time.Time
	Evidence    SeedEvidence
}

// SeedEvidence carries the observed patterns and extracted signals for a seed.
type SeedEvidence struct {
	TitleConvention         string
	IssueLinkages           []string
	ValidationIndicators    []string
	ApproximateScope        string
	ScopeEvidence           string
	RejectionOrSupersession string
	ProblemAreas            []string
}

// ExtractSeedsOptions selects the source classes and bounds the result set.
type ExtractSeedsOptions struct {
	Classes []SeedSourceClass
	Limit   int
}

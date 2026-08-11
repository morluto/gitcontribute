package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// RepoRef identifies a repository by owner and name.
type RepoRef struct {
	value string
}

func (r RepoRef) String() string {
	return r.value
}

var (
	errOwnerEmpty = errors.New("owner is required")
	errRepoEmpty  = errors.New("repo is required")
	ownerRegex    = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`)
	repoRegex     = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
)

// NewRepoRef parses a repository owner and name into a canonical identity.
func NewRepoRef(owner, repo string) (RepoRef, error) {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" {
		return RepoRef{}, errOwnerEmpty
	}
	if repo == "" {
		return RepoRef{}, errRepoEmpty
	}
	if !ownerRegex.MatchString(owner) {
		return RepoRef{}, fmt.Errorf("invalid owner %q", owner)
	}
	if !repoRegex.MatchString(repo) {
		return RepoRef{}, fmt.Errorf("invalid repo %q", repo)
	}
	if repo == "." || repo == ".." || strings.Contains(repo, "..") {
		return RepoRef{}, fmt.Errorf("invalid repo %q", repo)
	}
	return RepoRef{value: owner + "/" + repo}, nil
}

// ParseRepoRef parses the canonical owner/repository form.
func ParseRepoRef(value string) (RepoRef, error) {
	value = strings.TrimSpace(value)
	owner, repo, ok := strings.Cut(value, "/")
	if !ok || strings.Contains(repo, "/") {
		return RepoRef{}, fmt.Errorf("invalid repository reference %q", value)
	}
	return NewRepoRef(owner, repo)
}

// MustRepoRef returns a parsed repository identity or panics. It is intended
// for fixed program constants and test fixtures, not boundary input.
func MustRepoRef(owner, repo string) RepoRef {
	ref, err := NewRepoRef(owner, repo)
	if err != nil {
		panic(err)
	}
	return ref
}

// IsValid reports whether r is a parsed repository identity. The zero value is
// invalid and can be used where repository identity is optional.
func (r RepoRef) IsValid() bool { return r.value != "" }

// Equal compares canonical repository identities without exposing their representation.
func (r RepoRef) Equal(other RepoRef) bool { return r == other }

// Owner returns the repository owner.
func (r RepoRef) Owner() string {
	owner, _, _ := strings.Cut(r.value, "/")
	return owner
}

// Repo returns the repository name.
func (r RepoRef) Repo() string {
	_, repo, _ := strings.Cut(r.value, "/")
	return repo
}

type repoRefJSON struct {
	Owner string
	Repo  string
}

// MarshalJSON preserves the object representation used by workflow records.
func (r RepoRef) MarshalJSON() ([]byte, error) {
	return json.Marshal(repoRefJSON{Owner: r.Owner(), Repo: r.Repo()})
}

// UnmarshalJSON reparses workflow data before it enters the domain model.
func (r *RepoRef) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*r = RepoRef{}
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var stored repoRefJSON
	if err := decoder.Decode(&stored); err != nil {
		return fmt.Errorf("decode repository reference: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode repository reference: expected one JSON value")
	}
	if stored.Owner == "" && stored.Repo == "" {
		*r = RepoRef{}
		return nil
	}
	parsed, err := NewRepoRef(stored.Owner, stored.Repo)
	if err != nil {
		return err
	}
	*r = parsed
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

// ParseThreadKind parses the only supported thread variants.
func ParseThreadKind(value string) (ThreadKind, error) {
	switch ThreadKind(strings.TrimSpace(value)) {
	case IssueKind:
		return IssueKind, nil
	case PullRequestKind:
		return PullRequestKind, nil
	default:
		return "", fmt.Errorf("unsupported thread kind %q", value)
	}
}

// ParseThreadState parses the closed thread lifecycle used by GitHub issues
// and pull requests.
func ParseThreadState(value string) (ThreadState, error) {
	switch ThreadState(strings.TrimSpace(value)) {
	case OpenState:
		return OpenState, nil
	case ClosedState:
		return ClosedState, nil
	default:
		return "", fmt.Errorf("unsupported thread state %q", value)
	}
}

// ContributionKind identifies one GitHub contribution category. The
// vocabulary is intentionally open because GitHub may add categories, while
// the representation is still parsed so empty or non-canonical values cannot
// enter stored contribution records.
type ContributionKind string

const (
	CommitContributionKind            ContributionKind = "commit"
	IssueContributionKind             ContributionKind = "issue"
	PullRequestContributionKind       ContributionKind = "pull_request"
	PullRequestReviewContributionKind ContributionKind = "pull_request_review"
	RepositoryContributionKind        ContributionKind = "repository"
)

func ParseContributionKind(value string) (ContributionKind, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", errors.New("contribution kind is required")
	}
	return ContributionKind(value), nil
}

func (k ContributionKind) String() string { return string(k) }

// Thread is a product-owned model for an issue or pull request.
// It carries no vendor-specific API types.
type Thread struct {
	ID        int64
	Repo      RepoRef
	Type      ThreadType
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
}

// ThreadType is a parsed issue-or-pull-request variant. Its fields are private
// so pull-request details cannot be attached to an issue.
type ThreadType struct {
	kind               ThreadKind
	pullRequestDetails PullRequestDetails
}

// IssueThread returns the issue variant.
func IssueThread() ThreadType { return ThreadType{kind: IssueKind} }

// PullRequestThread returns a pull-request variant with its observed details.
func PullRequestThread(details PullRequestDetails) ThreadType {
	return ThreadType{kind: PullRequestKind, pullRequestDetails: details}
}

// Kind returns the kind derived from the sealed thread variant.
func (t Thread) Kind() ThreadKind {
	return t.Type.kind
}

// PullRequest returns PR details only for the pull-request variant.
func (t Thread) PullRequest() (PullRequestDetails, bool) {
	return t.Type.pullRequestDetails, t.Type.kind == PullRequestKind
}

// Equal compares parsed thread variants without exposing their representation.
func (t ThreadType) Equal(other ThreadType) bool {
	return t.kind == other.kind && t.pullRequestDetails == other.pullRequestDetails
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
	Merge MergeStatus
}

// Repository is a product-owned snapshot of repository metadata and counts.
type Repository struct {
	Ref                            RepoRef
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

// repositoryJSON preserves the original flattened repository identity used by
// persisted dossier snapshots. RepoRef is private in memory, but Owner and Repo
// remain top-level durable fields.
type repositoryJSON struct {
	Owner                          string
	Repo                           string
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

func (r Repository) MarshalJSON() ([]byte, error) {
	return json.Marshal(repositoryJSON{
		Owner: r.Ref.Owner(), Repo: r.Ref.Repo(), ID: r.ID, Description: r.Description,
		Topics: append([]string(nil), r.Topics...), Languages: append([]string(nil), r.Languages...),
		License: r.License, DefaultBranch: r.DefaultBranch, CommitSHA: r.CommitSHA,
		Archived: r.Archived, Fork: r.Fork, Stars: r.Stars, Watchers: r.Watchers, Forks: r.Forks,
		OpenIssueCount: r.OpenIssueCount, ClosedIssueCount: r.ClosedIssueCount,
		OpenPullRequestCount: r.OpenPullRequestCount, MergedPullRequestCount: r.MergedPullRequestCount,
		ClosedUnmergedPullRequestCount: r.ClosedUnmergedPullRequestCount,
		ClosedPullRequestUnknownCount:  r.ClosedPullRequestUnknownCount,
		CreatedAt:                      r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
}

func (r *Repository) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var stored repositoryJSON
	if err := decoder.Decode(&stored); err != nil {
		return fmt.Errorf("decode repository: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode repository: expected one JSON value")
	}
	var ref RepoRef
	if stored.Owner != "" || stored.Repo != "" {
		parsed, err := NewRepoRef(stored.Owner, stored.Repo)
		if err != nil {
			return err
		}
		ref = parsed
	}
	*r = Repository{
		Ref: ref, ID: stored.ID, Description: stored.Description,
		Topics: append([]string(nil), stored.Topics...), Languages: append([]string(nil), stored.Languages...),
		License: stored.License, DefaultBranch: stored.DefaultBranch, CommitSHA: stored.CommitSHA,
		Archived: stored.Archived, Fork: stored.Fork, Stars: stored.Stars, Watchers: stored.Watchers, Forks: stored.Forks,
		OpenIssueCount: stored.OpenIssueCount, ClosedIssueCount: stored.ClosedIssueCount,
		OpenPullRequestCount: stored.OpenPullRequestCount, MergedPullRequestCount: stored.MergedPullRequestCount,
		ClosedUnmergedPullRequestCount: stored.ClosedUnmergedPullRequestCount,
		ClosedPullRequestUnknownCount:  stored.ClosedPullRequestUnknownCount,
		CreatedAt:                      stored.CreatedAt, UpdatedAt: stored.UpdatedAt,
	}
	return nil
}

// FacetCoverage describes one present repository facet. Missing facets are
// absent from Coverage.Facets, so presence cannot contradict the observation.
type FacetCoverage struct {
	facet    string
	complete bool
	asOf     time.Time
	count    int
}

// NewFacetCoverage constructs a present facet observation.
func NewFacetCoverage(facet string, complete bool, asOf time.Time, count int) (FacetCoverage, error) {
	facet = strings.TrimSpace(facet)
	if facet == "" {
		return FacetCoverage{}, errors.New("coverage facet is required")
	}
	if asOf.IsZero() {
		return FacetCoverage{}, errors.New("coverage as-of time is required")
	}
	if count < 0 {
		return FacetCoverage{}, errors.New("coverage count cannot be negative")
	}
	return FacetCoverage{facet: facet, complete: complete, asOf: asOf, count: count}, nil
}

func (c FacetCoverage) Facet() string   { return c.facet }
func (c FacetCoverage) Complete() bool  { return c.complete }
func (c FacetCoverage) AsOf() time.Time { return c.asOf }
func (c FacetCoverage) Count() int      { return c.count }

// Equal compares parsed facet observations without exposing their representation.
func (c FacetCoverage) Equal(other FacetCoverage) bool { return c == other }

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

// SeedPolarity distinguishes accepted implementation examples from
// constraining evidence and issue-only context.
type SeedPolarity string

// Seed polarity values describe how stored evidence may be used.
const (
	SeedPolarityPositive SeedPolarity = "positive"
	SeedPolarityNegative SeedPolarity = "negative"
	SeedPolarityContext  SeedPolarity = "context"
)

// Seed is an evidence-backed record derived from stored merged PRs,
// closed unmerged PRs, or issues.
type Seed struct {
	SourceClass    SeedSourceClass
	Polarity       SeedPolarity
	PolarityReason string
	Number         int
	Title          string
	Author         string
	State          string
	Labels         []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ClosedAt       time.Time
	MergedAt       time.Time
	Evidence       SeedEvidence
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

// ExtractSeedsOptions selects source classes, evidence polarities, and the
// result bound. Empty Polarities defaults to positive and negative outcomes.
type ExtractSeedsOptions struct {
	Classes    []SeedSourceClass
	Polarities []SeedPolarity
	Limit      int
}

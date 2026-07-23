package cli

import "context"

// ConcernService manages repo-local concern intake without GitHub access.
type ConcernService interface {
	CreateConcern(context.Context, ConcernCreateOptions) (*ConcernResult, error)
	ListConcerns(context.Context, ConcernListOptions) (*ConcernListResult, error)
	ShowConcern(context.Context, string) (*ConcernResult, error)
	UpdateConcern(context.Context, string, ConcernUpdateOptions) (*ConcernResult, error)
	SetConcernStatus(context.Context, string, string, string) (*ConcernResult, error)
	LinkConcern(context.Context, string, ConcernLinkOptions) (*ConcernResult, error)
	PromoteConcern(context.Context, string, ConcernPromoteOptions) (*ConcernResult, error)
}

// ConcernCreateOptions carries local concern intake fields.
type ConcernCreateOptions struct {
	Repo             RepoRef
	CommitSHA        string
	WorkspaceID      string
	Title            string
	ProblemStatement string
	SuspectedOwner   string
	Confidence       float64
	Unknowns         []string
	SuccessCriterion string
	Notes            string
	EvidenceIDs      []string
}

// ConcernUpdateOptions carries optional replacement fields.
type ConcernUpdateOptions struct {
	Title            *string
	ProblemStatement *string
	SuspectedOwner   *string
	Confidence       *float64
	Unknowns         []string
	SuccessCriterion *string
	Notes            *string
	EvidenceIDs      []string
}

// ConcernListOptions bounds an offline concern list or search.
type ConcernListOptions struct {
	Repo   RepoRef
	Status string
	Query  string
	Limit  int
}

// ConcernLinkOptions identifies one explicit relationship.
type ConcernLinkOptions struct {
	Kind       string
	TargetType string
	TargetID   string
	Note       string
}

// ConcernPromoteOptions configures atomic workflow promotion.
type ConcernPromoteOptions struct {
	Kind           string
	Category       string
	Scope          string
	Impact         string
	ExpectedEffort string
}

// ConcernLinkResult is a transport-safe relationship view.
type ConcernLinkResult struct {
	Kind       string `json:"kind"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Note       string `json:"note,omitempty"`
}

// ConcernPromotionResult preserves downstream workflow IDs.
type ConcernPromotionResult struct {
	Kind            string `json:"kind"`
	InvestigationID string `json:"investigation_id"`
	HypothesisID    string `json:"hypothesis_id"`
	OpportunityID   string `json:"opportunity_id,omitempty"`
}

// ConcernResult omits source URLs and host paths. Source/evidence details stay
// available through their dedicated local records.
type ConcernResult struct {
	ID               string                  `json:"id"`
	Repo             RepoRef                 `json:"repo"`
	CommitSHA        string                  `json:"commit_sha,omitempty"`
	WorkspaceID      string                  `json:"workspace_id,omitempty"`
	Title            string                  `json:"title"`
	ProblemStatement string                  `json:"problem_statement"`
	SuspectedOwner   string                  `json:"suspected_owner,omitempty"`
	Confidence       float64                 `json:"confidence"`
	Unknowns         []string                `json:"unknowns,omitempty"`
	SuccessCriterion string                  `json:"success_criterion,omitempty"`
	Notes            string                  `json:"notes,omitempty"`
	EvidenceIDs      []string                `json:"evidence_ids,omitempty"`
	SourceRefCount   int                     `json:"source_ref_count"`
	Freshness        string                  `json:"freshness"`
	FreshnessReason  string                  `json:"freshness_reason"`
	Links            []ConcernLinkResult     `json:"links,omitempty"`
	Status           string                  `json:"status"`
	Promotion        *ConcernPromotionResult `json:"promotion,omitempty"`
	CreatedAt        string                  `json:"created_at"`
	UpdatedAt        string                  `json:"updated_at"`
}

// ConcernListResult contains one bounded result set.
type ConcernListResult struct {
	Concerns []ConcernResult `json:"concerns"`
	Total    int             `json:"total"`
}

package contracts

import (
	"context"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

// WorkflowService exposes typed investigation workflow operations.
type WorkflowService interface {
	UpdateHypothesisFields(ctx context.Context, id string, opts HypothesisUpdateOptions) (*investigation.Hypothesis, error)
	TransitionHypothesis(ctx context.Context, id, status, rationale string) (*investigation.Hypothesis, error)
	CheckHypothesisDuplicates(ctx context.Context, id string, limit int) (*DuplicateCheckResult, error)
	CheckOpportunityDuplicates(ctx context.Context, id string, limit int) (*DuplicateCheckResult, error)
	CheckHypothesisCollisions(ctx context.Context, id string, limit int) (*CollisionCheckResult, error)
	CheckOpportunityCollisions(ctx context.Context, id string, limit int) (*CollisionCheckResult, error)
	UpdateOpportunityCollisionStatus(ctx context.Context, id, status, rationale string) (*investigation.Opportunity, error)
	RecordEvidence(ctx context.Context, input RecordEvidenceInput) (*evidence.Evidence, error)
	WorkspaceDiff(ctx context.Context, id string) (*WorkspaceDiffResult, error)
	PrepareReviewReport(ctx context.Context, input PrepareReviewReportInput) (*ReviewReport, error)
}

type HypothesisUpdateOptions struct {
	Title              *string
	Description        *string
	Category           *string
	ExpectedBehavior   *string
	ObservedBehavior   *string
	PotentialImpact    *string
	OpenQuestions      []string
	AffectedComponents []string
	Rationale          string
}

type RecordEvidenceInput struct {
	InvestigationID  string
	HypothesisID     string
	OpportunityID    string
	Type             string
	Relation         string
	Description      string
	SourceRefs       []domain.SourceRef
	SourceProvenance []evidence.SourceRevision
}

type DuplicateCheckResult struct {
	HypothesisID   string              `json:"hypothesis_id,omitempty"`
	OpportunityID  string              `json:"opportunity_id,omitempty"`
	Repo           domain.RepoRef      `json:"repo"`
	Query          string              `json:"query"`
	Findings       []evidence.Evidence `json:"findings"`
	SourceRevision string              `json:"source_revision"`
	Limit          int                 `json:"limit"`
	Total          int                 `json:"total"`
}

type CollisionCheckResult struct {
	HypothesisID   string              `json:"hypothesis_id,omitempty"`
	OpportunityID  string              `json:"opportunity_id,omitempty"`
	Repo           domain.RepoRef      `json:"repo"`
	Query          string              `json:"query"`
	Findings       []evidence.Evidence `json:"findings"`
	SourceRevision string              `json:"source_revision"`
	Limit          int                 `json:"limit"`
	Total          int                 `json:"total"`
}

type PrepareReviewReportInput struct {
	OpportunityID string
	WorkspaceID   string
}

type EvidenceSummary struct {
	Supporting    int `json:"supporting"`
	Contradicting int `json:"contradicting"`
	Inconclusive  int `json:"inconclusive"`
	Stale         int `json:"stale"`
	Invalid       int `json:"invalid"`
	Total         int `json:"total"`
}

type ReviewReport struct {
	OpportunityID        string               `json:"opportunity_id,omitempty"`
	WorkspaceID          string               `json:"workspace_id,omitempty"`
	Repo                 RepoRef              `json:"repo"`
	OpportunityStatus    string               `json:"opportunity_status,omitempty"`
	CollisionStatus      string               `json:"collision_status,omitempty"`
	CollisionFindings    []evidence.Evidence  `json:"collision_findings"`
	DiffMetadata         *WorkspaceDiffResult `json:"diff_metadata,omitempty"`
	EvidenceSummary      EvidenceSummary      `json:"evidence_summary"`
	SuggestedReviewOrder []ReviewStep         `json:"suggested_review_order"`
	RenderedAt           time.Time            `json:"rendered_at"`
}

type ReviewStep struct {
	Path      string `json:"path"`
	Priority  int    `json:"priority"`
	Rationale string `json:"rationale"`
}

type WorkspaceDiffResult struct {
	ID               string       `json:"id"`
	Repo             RepoRef      `json:"repo"`
	BaseSHA          string       `json:"base_sha"`
	CandidateSHA     string       `json:"candidate_sha"`
	MergeBase        string       `json:"merge_base"`
	Dirty            bool         `json:"dirty"`
	HasUntracked     bool         `json:"has_untracked"`
	Diff             string       `json:"diff"`
	ChangedFiles     []string     `json:"changed_files"`
	ChangedFileCount int          `json:"changed_file_count"`
	DiffBytes        int          `json:"diff_bytes"`
	ReviewOrder      []ReviewStep `json:"review_order"`
}

// DossierService exposes typed repository dossier operations.
type DossierService interface {
	BuildRepositoryDossier(ctx context.Context, repo RepoRef) (*domain.Dossier, error)
	GetRepositoryDossier(ctx context.Context, repo RepoRef) (*domain.Dossier, error)
	ExtractSeeds(ctx context.Context, repo RepoRef, opts domain.ExtractSeedsOptions) ([]domain.Seed, error)
}

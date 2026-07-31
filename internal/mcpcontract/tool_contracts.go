package mcpcontract

import (
	"encoding/json"

	"github.com/morluto/gitcontribute/internal/manifest"
)

// ToolError is the stable, actionable shape for agent-correctable requests.
type ToolError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Field     string         `json:"path,omitempty"`
	Retryable bool           `json:"retryable"`
	Example   map[string]any `json:"example,omitempty"`
	Recovery  *RecoveryPlan  `json:"recovery,omitempty"`
}

func (e *ToolError) Error() string {
	payload, err := json.Marshal(e)
	if err != nil {
		return `{"code":"tool_error_encoding_failed","message":"The tool error could not be encoded.","retryable":false}`
	}
	return string(payload)
}

// InvalidArgument reports one agent-correctable request error.
func InvalidArgument(field, message string, example map[string]any) error {
	return &ToolError{Code: "invalid_argument", Field: field, Message: message, Example: example}
}

// Unavailable reports an agent-readable terminal state with explicit recovery
// actions. It is used when retrying the same read cannot make the object
// available without a distinct acquisition or build step.
func Unavailable(code, message string, actions ...ToolCall) error {
	return &ToolError{
		Code: code, Message: message, Retryable: false,
		Recovery: &RecoveryPlan{Version: RecoveryPlanVersion, Reason: code, Message: message, Then: append([]ToolCall(nil), actions...)},
	}
}

// Canonical MCP tool names group operations by capability and side-effect boundary.
const (
	ToolSearchRepositories           = "corpus.search_repositories"
	ToolSearchThreads                = "corpus.search_threads"
	ToolSearchCode                   = "corpus.search_code"
	ToolGetRepositories              = "corpus.get_repositories"
	ToolGetThreads                   = "corpus.get_threads"
	ToolGetThreadFacets              = "corpus.get_thread_facets"
	ToolRankThreads                  = "corpus.rank_contribution_candidates"
	ToolFindPrecedents               = "corpus.find_precedents"
	ToolPrepareIssueSet              = "workflow.prepare_issue_set"
	ToolExplainMatch                 = "corpus.explain_match"
	ToolFindClusters                 = "corpus.find_clusters"
	ToolFindNeighbors                = "corpus.find_neighbors"
	ToolGetCoverage                  = "corpus.get_coverage"
	ToolBuildRepositoryDossier       = "workflow.build_repository_dossier"
	ToolMineRepositoryFixPatterns    = "workflow.mine_repository_fix_patterns"
	ToolPreviewRepositoryFixPatterns = "corpus.preview_fix_patterns"
	ToolGetJob                       = "jobs.get"
	ToolCancelJob                    = "jobs.cancel"
	ToolSearchGitHubRepositories     = "github.search_repositories"
	ToolSyncRepositoryContext        = "github.sync_repository_context"
	ToolSyncThreads                  = "github.sync_threads"
	ToolHydrateThreads               = "github.sync_thread_facets"
	ToolSyncPortfolio                = "github.sync_pull_request_portfolio"
	ToolSyncPullRequestFeedback      = "github.sync_pull_request_feedback"
	ToolSyncCIFailures               = "github.sync_ci_failures"
	ToolListPullRequestPortfolio     = "corpus.list_pull_requests"
	ToolFindPortfolioOverlaps        = "corpus.find_pull_request_overlaps"
	ToolIndexRepositories            = "code.index_repositories"
	ToolPreparePullRequests          = "workspace.prepare_pull_requests"
	ToolCheckMergeConflicts          = "workspace.check_merge_conflicts"
	ToolInspectCommitChanges         = "workspace.inspect_commit_changes"
	ToolPlanSemanticCommits          = "workspace.plan_semantic_commits"
	ToolQueryDeepWiki                = "research.query_deepwiki"
	ToolCreateWorkspace              = "workspace.create"
	ToolAdoptWorkspace               = "workspace.adopt"
	ToolDefineValidation             = "validation.define"
	ToolRunValidation                = "validation.run"
	ToolAttachValidationReceipt      = "validation.attach_receipt"
	ToolStartInvestigation           = "workflow.start_investigation"
	ToolRecordHypothesis             = "workflow.record_hypothesis"
	ToolFindRelatedWork              = "workflow.find_related_work"
	ToolPromoteOpportunity           = "workflow.promote_opportunity"
	ToolPrepareContribution          = "workflow.prepare_contribution"
	ToolVerifyPublishedDraft         = "workflow.verify_published_draft"
	ToolExportManifest               = "workflow.export_manifest"
	ToolLinkPullRequest              = "workflow.link_pull_request"
)

// InspectCommitChangesInput selects one managed workspace.
type InspectCommitChangesInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"Managed workspace ID"`
}

// CommitUnitOutput is one indivisible file or hunk assignment unit.
type CommitUnitOutput struct {
	ID             string `json:"id" jsonschema:"Stable content-derived unit ID"`
	Kind           string `json:"kind" jsonschema:"Unit kind: hunk, file, or untracked"`
	Path           string `json:"path" jsonschema:"Repository-relative path"`
	OldPath        string `json:"old_path,omitempty" jsonschema:"Previous path for rename or copy changes"`
	Operation      string `json:"operation" jsonschema:"Git change operation"`
	OldStart       int32  `json:"old_start,omitempty" jsonschema:"Original hunk start line"`
	OldLines       int32  `json:"old_lines,omitempty" jsonschema:"Original hunk line count"`
	NewStart       int32  `json:"new_start,omitempty" jsonschema:"New hunk start line"`
	NewLines       int32  `json:"new_lines,omitempty" jsonschema:"New hunk line count"`
	Patch          string `json:"patch,omitempty" jsonschema:"Canonical hunk preview for semantic classification"`
	ContentSHA256  string `json:"content_sha256" jsonschema:"Exact unit identity digest"`
	Generated      bool   `json:"generated" jsonschema:"Path appears generated or snapshot-owned"`
	WhitespaceOnly bool   `json:"whitespace_only" jsonschema:"Hunk changes only whitespace"`
}

// CommitPlanWarningOutput flags changes needing explicit judgment.
type CommitPlanWarningOutput struct {
	Code    string `json:"code" jsonschema:"Stable warning code"`
	Message string `json:"message" jsonschema:"Actionable warning explanation"`
	Path    string `json:"path,omitempty" jsonschema:"Affected repository-relative path"`
	UnitID  string `json:"unit_id,omitempty" jsonschema:"Affected assignable unit ID"`
}

// CommitInventoryOutput freezes assignable units and exact source digests.
type CommitInventoryOutput struct {
	Units             []CommitUnitOutput        `json:"units" jsonschema:"Ordered assignable changes"`
	Warnings          []CommitPlanWarningOutput `json:"warnings,omitempty" jsonschema:"Conditions requiring explicit judgment"`
	SourcePatchSHA256 string                    `json:"source_patch_sha256" jsonschema:"SHA-256 of the exact Git patch bytes"`
	InventorySHA256   string                    `json:"inventory_sha256" jsonschema:"SHA-256 binding ordered tracked and untracked unit identities"`
}

// SemanticCommitGroupInput supplies judgment that cannot be inferred safely.
type SemanticCommitGroupInput struct {
	Name               string   `json:"name" jsonschema:"Unique group name used by dependency references"`
	Intent             string   `json:"intent" jsonschema:"Concrete outcome of this commit"`
	Type               string   `json:"type" jsonschema:"Conventional commit type"`
	Scope              string   `json:"scope,omitempty" jsonschema:"Optional conventional commit scope"`
	UnitIDs            []string `json:"unit_ids" jsonschema:"One or more IDs from workspace.inspect_commit_changes"`
	DependsOn          []string `json:"depends_on,omitempty" jsonschema:"Group names that must precede this group"`
	ValidationCommands []string `json:"validation_commands,omitempty" jsonschema:"Focused validation commands for this group"`
	TestOwners         []string `json:"test_owners,omitempty" jsonschema:"Tests or owners responsible for this group"`
}

// UnresolvedCommitUnitInput preserves ambiguity instead of inventing ownership.
type UnresolvedCommitUnitInput struct {
	UnitID string `json:"unit_id" jsonschema:"Unassigned file or hunk ID"`
	Reason string `json:"reason" jsonschema:"Why ownership remains ambiguous"`
}

// PlanSemanticCommitsInput binds agent-authored groups to a frozen inventory.
type PlanSemanticCommitsInput struct {
	WorkspaceID             string                      `json:"workspace_id" jsonschema:"Managed workspace ID"`
	ExpectedInventorySHA256 string                      `json:"expected_inventory_sha256" jsonschema:"Inventory digest returned by the preceding inspection"`
	Groups                  []SemanticCommitGroupInput  `json:"groups" jsonschema:"One to 100 proposed semantic commit groups"`
	Unresolved              []UnresolvedCommitUnitInput `json:"unresolved,omitempty" jsonschema:"Ambiguous units with explicit reasons"`
}

// SemanticCommitGroupOutput is one validated proposed commit.
type SemanticCommitGroupOutput struct {
	Name               string   `json:"name" jsonschema:"Unique group name"`
	Intent             string   `json:"intent" jsonschema:"Concrete commit outcome"`
	SuggestedSubject   string   `json:"suggested_subject" jsonschema:"Conventional commit subject derived from type, scope, and intent"`
	UnitIDs            []string `json:"unit_ids" jsonschema:"Assigned file and hunk IDs"`
	Files              []string `json:"files" jsonschema:"Sorted repository-relative paths in the group"`
	DependsOn          []string `json:"depends_on,omitempty" jsonschema:"Groups that must precede this group"`
	ValidationCommands []string `json:"validation_commands,omitempty" jsonschema:"Focused validation commands"`
	TestOwners         []string `json:"test_owners,omitempty" jsonschema:"Tests or owners responsible for validation"`
}

// UnresolvedCommitUnitOutput reports an unassigned unit and reason.
type UnresolvedCommitUnitOutput struct {
	UnitID string `json:"unit_id" jsonschema:"Unassigned file or hunk ID"`
	Reason string `json:"reason" jsonschema:"Why ownership remains ambiguous"`
}

// CommitReconstructionOutput proves exact one-to-one unit coverage.
type CommitReconstructionOutput struct {
	SourcePatchSHA256 string `json:"source_patch_sha256" jsonschema:"SHA-256 of the exact Git patch bytes"`
	InventorySHA256   string `json:"inventory_sha256" jsonschema:"Digest of every ordered source unit"`
	AssignedSHA256    string `json:"assigned_sha256" jsonschema:"Digest of every uniquely assigned unit in source order"`
	UnitCount         int    `json:"unit_count" jsonschema:"Total source units"`
	AssignedCount     int    `json:"assigned_count" jsonschema:"Uniquely assigned units"`
	Verified          bool   `json:"verified" jsonschema:"True only when every source unit is assigned exactly once"`
}

// SemanticCommitPlanOutput is a read-only plan; it contains no patch apply.
type SemanticCommitPlanOutput struct {
	Groups         []SemanticCommitGroupOutput  `json:"groups" jsonschema:"Validated semantic commit groups"`
	Unresolved     []UnresolvedCommitUnitOutput `json:"unresolved,omitempty" jsonschema:"Units still requiring ownership judgment"`
	Warnings       []CommitPlanWarningOutput    `json:"warnings,omitempty" jsonschema:"Mixed, generated, binary, formatting, and subject warnings"`
	Reconstruction CommitReconstructionOutput   `json:"reconstruction" jsonschema:"Exact one-to-one source coverage proof"`
}

// CreateConcernInput records one repository concern and its provenance.
type CreateConcernInput struct {
	Owner            string                   `json:"owner" jsonschema:"GitHub repository owner"`
	Repo             string                   `json:"repo" jsonschema:"GitHub repository name"`
	CommitSHA        string                   `json:"commit_sha,omitempty" jsonschema:"Source commit SHA; required unless workspace_id is set"`
	WorkspaceID      string                   `json:"workspace_id,omitempty" jsonschema:"Opaque workspace ID; required unless commit_sha is set"`
	Title            string                   `json:"title" jsonschema:"Concise concern title"`
	ProblemStatement string                   `json:"problem_statement" jsonschema:"Observed or suspected problem"`
	SuspectedOwner   string                   `json:"suspected_owner,omitempty" jsonschema:"Suspected code ownership boundary"`
	Confidence       Probability              `json:"confidence" jsonschema:"Confidence from 0 to 1"`
	Unknowns         []string                 `json:"unknowns,omitempty" jsonschema:"Explicit unknowns"`
	SuccessCriterion string                   `json:"success_criterion,omitempty" jsonschema:"Proof or success criterion"`
	Notes            string                   `json:"notes,omitempty" jsonschema:"Local notes"`
	EvidenceIDs      []string                 `json:"evidence_ids,omitempty" jsonschema:"Existing local evidence IDs"`
	SourceProvenance []EvidenceSourceRevision `json:"source_provenance,omitempty" jsonschema:"Exact stored source revisions used by this concern"`
}

// ListConcernsInput filters and bounds offline concern reads.
type ListConcernsInput struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Optional repository owner; provide with repo"`
	Repo   string `json:"repo,omitempty" jsonschema:"Optional repository name; provide with owner"`
	Status string `json:"status,omitempty" jsonschema:"Optional concern status"`
	Query  string `json:"query,omitempty" jsonschema:"Literal full-text search query"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

// UpdateConcernInput replaces explicitly supplied editable fields.
type UpdateConcernInput struct {
	ID               string       `json:"id" jsonschema:"Concern ID"`
	Title            *string      `json:"title,omitempty" jsonschema:"Replacement title"`
	ProblemStatement *string      `json:"problem_statement,omitempty" jsonschema:"Replacement problem statement"`
	SuspectedOwner   *string      `json:"suspected_owner,omitempty" jsonschema:"Replacement owner boundary"`
	Confidence       *Probability `json:"confidence,omitempty" jsonschema:"Replacement confidence from 0 to 1"`
	Unknowns         []string     `json:"unknowns,omitempty" jsonschema:"Replacement explicit unknowns"`
	SuccessCriterion *string      `json:"success_criterion,omitempty" jsonschema:"Replacement success criterion"`
	Notes            *string      `json:"notes,omitempty" jsonschema:"Replacement local notes"`
	EvidenceIDs      []string     `json:"evidence_ids,omitempty" jsonschema:"Replacement evidence IDs"`
}

// SetConcernStatusInput requests one lifecycle transition.
type SetConcernStatusInput struct {
	ID        string `json:"id" jsonschema:"Concern ID"`
	Status    string `json:"status" jsonschema:"Target lifecycle status"`
	Rationale string `json:"rationale" jsonschema:"Reason for the transition"`
}

// LinkConcernInput records one typed relationship.
type LinkConcernInput struct {
	ID         string `json:"id" jsonschema:"Concern ID"`
	Kind       string `json:"kind" jsonschema:"Relationship kind"`
	TargetType string `json:"target_type" jsonschema:"Target record type"`
	TargetID   string `json:"target_id" jsonschema:"Target record ID"`
	Note       string `json:"note,omitempty" jsonschema:"Relationship note"`
}

// PromoteConcernInput configures atomic downstream workflow creation.
type PromoteConcernInput struct {
	ID             string `json:"id" jsonschema:"Concern ID"`
	Kind           string `json:"kind" jsonschema:"Promotion target: investigation or opportunity"`
	Category       string `json:"category" jsonschema:"Contribution category"`
	Scope          string `json:"scope,omitempty" jsonschema:"Required opportunity scope"`
	Impact         string `json:"impact,omitempty" jsonschema:"Required opportunity impact"`
	ExpectedEffort string `json:"expected_effort,omitempty" jsonschema:"Required expected effort"`
}

// ConcernLinkOutput is a transport-safe relationship.
type ConcernLinkOutput struct {
	Kind       string `json:"kind" jsonschema:"Relationship kind"`
	TargetType string `json:"target_type" jsonschema:"Target record type"`
	TargetID   string `json:"target_id" jsonschema:"Target record ID"`
	Note       string `json:"note,omitempty" jsonschema:"Relationship note"`
}

// ConcernPromotionOutput preserves created downstream identities.
type ConcernPromotionOutput struct {
	Kind            string `json:"kind" jsonschema:"Promotion target kind"`
	InvestigationID string `json:"investigation_id" jsonschema:"Created investigation ID"`
	HypothesisID    string `json:"hypothesis_id" jsonschema:"Created hypothesis ID"`
	OpportunityID   string `json:"opportunity_id,omitempty" jsonschema:"Created opportunity ID"`
}

// ConcernOutput omits absolute paths and source-reference URLs.
type ConcernOutput struct {
	ID               string                  `json:"id" jsonschema:"Concern ID"`
	Owner            string                  `json:"owner" jsonschema:"Repository owner"`
	Repo             string                  `json:"repo" jsonschema:"Repository name"`
	CommitSHA        string                  `json:"commit_sha,omitempty" jsonschema:"Source commit SHA"`
	WorkspaceID      string                  `json:"workspace_id,omitempty" jsonschema:"Opaque workspace ID"`
	Title            string                  `json:"title" jsonschema:"Concern title"`
	ProblemStatement string                  `json:"problem_statement" jsonschema:"Concern problem statement"`
	SuspectedOwner   string                  `json:"suspected_owner,omitempty" jsonschema:"Suspected ownership boundary"`
	Confidence       Probability             `json:"confidence" jsonschema:"Confidence from 0 to 1"`
	Unknowns         []string                `json:"unknowns,omitempty" jsonschema:"Explicit unknowns"`
	SuccessCriterion string                  `json:"success_criterion,omitempty" jsonschema:"Proof or success criterion"`
	Notes            string                  `json:"notes,omitempty" jsonschema:"Local notes"`
	EvidenceIDs      []string                `json:"evidence_ids,omitempty" jsonschema:"Linked evidence IDs"`
	SourceRefCount   int                     `json:"source_ref_count" jsonschema:"Number of private source references retained locally"`
	Freshness        string                  `json:"freshness" jsonschema:"Derived source freshness"`
	FreshnessReason  string                  `json:"freshness_reason" jsonschema:"Freshness explanation"`
	Links            []ConcernLinkOutput     `json:"links,omitempty" jsonschema:"Explicit concern relationships"`
	Status           string                  `json:"status" jsonschema:"Concern lifecycle status"`
	Promotion        *ConcernPromotionOutput `json:"promotion,omitempty" jsonschema:"Downstream workflow identity"`
	CreatedAt        string                  `json:"created_at" jsonschema:"Creation time"`
	UpdatedAt        string                  `json:"updated_at" jsonschema:"Latest update time"`
}

// ConcernSummaryOutput contains the triage fields needed before reading a concern resource.
type ConcernSummaryOutput struct {
	ID         string      `json:"id" jsonschema:"Stable concern ID"`
	Owner      string      `json:"owner" jsonschema:"Repository owner"`
	Repo       string      `json:"repo" jsonschema:"Repository name"`
	Title      string      `json:"title" jsonschema:"Concern title"`
	Confidence Probability `json:"confidence" jsonschema:"Confidence from zero to one"`
	Freshness  string      `json:"freshness" jsonschema:"Derived freshness state"`
	Status     string      `json:"status" jsonschema:"Concern lifecycle status"`
	UpdatedAt  string      `json:"updated_at" jsonschema:"Latest update time"`
	URI        string      `json:"uri" jsonschema:"Exact opaque concern resource URI"`
}

// ConcernListOutput contains one bounded offline result set.
type ConcernListOutput struct {
	Concerns  []ConcernSummaryOutput `json:"concerns" jsonschema:"Bounded concern summaries with resource URIs"`
	Limit     int                    `json:"limit" jsonschema:"Effective result limit"`
	Total     int                    `json:"total" jsonschema:"Total matching concerns"`
	Truncated bool                   `json:"truncated" jsonschema:"Whether more matching concerns exist"`
}

// PrepareContributionInput renders a local issue or pull-request draft.
type PrepareContributionInput struct {
	OpportunityID string `json:"opportunity_id" jsonschema:"Opportunity ID"`
	Kind          string `json:"kind" jsonschema:"Contribution kind: issue or pull_request"`
	WorkspaceID   string `json:"workspace_id,omitempty" jsonschema:"Workspace ID for pull_request drafts"`
	Approach      string `json:"approach,omitempty" jsonschema:"Approach summary for pull requests"`
	Changes       string `json:"changes,omitempty" jsonschema:"Changes summary for pull requests"`
	Compatibility string `json:"compatibility,omitempty" jsonschema:"Compatibility notes for pull requests"`
	Limitations   string `json:"limitations,omitempty" jsonschema:"Limitations for pull requests"`
	LinkedIssue   string `json:"linked_issue,omitempty" jsonschema:"Linked issue for pull requests"`
	Guidance      string `json:"guidance,omitempty" jsonschema:"Optional guidance to include"`
	Success       string `json:"success,omitempty" jsonschema:"Success criteria for issue drafts"`
	ManifestID    string `json:"manifest_id,omitempty" jsonschema:"Stored evidence manifest ID to reference without copying its claims"`
}

// DraftOutput contains a rendered contribution draft.
type DraftOutput struct {
	ID            string                  `json:"id"`
	Revision      int                     `json:"revision"`
	OpportunityID string                  `json:"opportunity_id"`
	Kind          string                  `json:"kind"`
	Repository    string                  `json:"repository"`
	Title         string                  `json:"title"`
	Body          string                  `json:"body"`
	TitleBytes    int                     `json:"title_bytes"`
	BodyBytes     int                     `json:"body_bytes"`
	TitleSHA256   string                  `json:"title_sha256"`
	BodySHA256    string                  `json:"body_sha256"`
	EvidenceIDs   []string                `json:"evidence_ids,omitempty"`
	Warnings      []DraftDiagnosticOutput `json:"warnings,omitempty"`
	RenderedAt    string                  `json:"rendered_at"`
	ManifestID    string                  `json:"manifest_id,omitempty" jsonschema:"Referenced stored evidence manifest ID"`
}

type DraftDiagnosticOutput struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	ByteOffset int    `json:"byte_offset,omitempty"`
}

type VerifyPublishedDraftInput struct {
	DraftID  string `json:"draft_id" jsonschema:"Stored draft ID"`
	Revision int    `json:"revision" jsonschema:"Positive immutable draft revision"`
	Owner    string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo     string `json:"repo" jsonschema:"GitHub repository name"`
	Kind     string `json:"kind" jsonschema:"Published kind: issue or pull_request"`
	Number   int    `json:"number" jsonschema:"Positive issue or pull-request number"`
}

type PublishedDraftDifferenceOutput struct {
	FirstDifferingLine int `json:"first_differing_line,omitempty"`
	DraftBytes         int `json:"draft_bytes"`
	PublishedBytes     int `json:"published_bytes"`
}

type PublishedDraftVerificationOutput struct {
	Status               string                          `json:"status"`
	DraftID              string                          `json:"draft_id"`
	Revision             int                             `json:"revision"`
	PublishedRef         string                          `json:"published_ref"`
	TitleComparison      string                          `json:"title_comparison,omitempty"`
	BodyComparison       string                          `json:"body_comparison,omitempty"`
	DraftTitleSHA256     string                          `json:"draft_title_sha256"`
	DraftBodySHA256      string                          `json:"draft_body_sha256"`
	PublishedTitleSHA256 string                          `json:"published_title_sha256,omitempty"`
	PublishedBodySHA256  string                          `json:"published_body_sha256,omitempty"`
	ObservedAt           string                          `json:"observed_at,omitempty"`
	SourceUpdatedAt      string                          `json:"source_updated_at,omitempty"`
	CoverageStatus       string                          `json:"coverage_status"`
	Difference           *PublishedDraftDifferenceOutput `json:"difference,omitempty"`
	Reason               string                          `json:"reason,omitempty"`
}

// ExportManifestInput selects bounded local evidence for one contribution manifest.
type ExportManifestInput struct {
	OpportunityID  string                    `json:"opportunity_id" jsonschema:"Opportunity ID"`
	WorkspaceID    string                    `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID to bind"`
	PullRequest    *ManifestPullRequestInput `json:"pull_request,omitempty" jsonschema:"Exact stored pull request to include"`
	CorpusRevision *int64                    `json:"corpus_revision,omitempty" jsonschema:"Optional corpus revision pin for the source evidence"`
}

// ManifestPullRequestInput identifies one exact stored pull request.
type ManifestPullRequestInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Number int    `json:"number" jsonschema:"Positive pull request number"`
}

// ManifestOutput returns the stable identity and full in-toto-shaped statement.
type ManifestOutput struct {
	ManifestID     string             `json:"manifest_id" jsonschema:"Stable sha256-prefixed manifest ID"`
	ContentSHA256  string             `json:"content_sha256" jsonschema:"Hex SHA-256 of stable manifest content"`
	SchemaVersion  string             `json:"schema_version" jsonschema:"Contribution manifest predicate schema version"`
	Status         string             `json:"status" jsonschema:"Overall completeness status"`
	CorpusRevision int64              `json:"corpus_revision" jsonschema:"Corpus revision observed when this manifest was published or read"`
	Statement      manifest.Statement `json:"statement" jsonschema:"Typed in-toto-shaped evidence statement owned by GitContribute"`
}

// EvidenceInput filters and bounds stored evidence.
type EvidenceInput struct {
	InvestigationID string `json:"investigation_id,omitempty" jsonschema:"Filter by investigation ID"`
	OpportunityID   string `json:"opportunity_id,omitempty" jsonschema:"Filter by opportunity ID"`
	Relation        string `json:"relation,omitempty" jsonschema:"Optional relation filter: supporting, contradicting, inconclusive, stale, invalid"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

// EvidenceSourceSubject identifies one independently refreshed corpus subject.
type EvidenceSourceSubject struct {
	Kind       string `json:"kind"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	ThreadKind string `json:"thread_kind,omitempty"`
	Number     int    `json:"number,omitempty"`
	Facet      string `json:"facet,omitempty"`
}

// EvidenceSourceRevision records the source order used by evidence.
type EvidenceSourceRevision struct {
	Subject             EvidenceSourceSubject `json:"subject"`
	SourceUpdatedAt     string                `json:"source_updated_at,omitempty"`
	ObservationSequence int64                 `json:"observation_sequence"`
	ObservedAt          string                `json:"observed_at"`
}

// EvidenceItem is the stable MCP representation of one evidence record.
type EvidenceItem struct {
	ID               string                   `json:"id"`
	Type             string                   `json:"type"`
	Relation         string                   `json:"relation"`
	Description      string                   `json:"description"`
	SourceRefs       []SourceRef              `json:"source_refs,omitempty"`
	SourceProvenance []EvidenceSourceRevision `json:"source_provenance,omitempty"`
	Freshness        string                   `json:"freshness,omitempty"`
	FreshnessReason  string                   `json:"freshness_reason,omitempty"`
	CreatedAt        string                   `json:"created_at"`
}

// EvidenceOutput contains bounded evidence matching a filter.
type EvidenceOutput struct {
	InvestigationID string         `json:"investigation_id,omitempty"`
	OpportunityID   string         `json:"opportunity_id,omitempty"`
	Total           int            `json:"total"`
	Evidence        []EvidenceItem `json:"evidence"`
}

// RankOpportunitiesInput bounds ranking across stored repositories.
type RankOpportunitiesInput struct {
	Repositories            []RepositoryRef `json:"repositories" jsonschema:"Required 1-50 stored repositories"`
	Limit                   int             `json:"limit,omitempty" jsonschema:"Result bound from 1-100"`
	MaxResultsPerRepository int             `json:"max_results_per_repository,omitempty" jsonschema:"Per-repository bound from 1-100"`
	CorpusRevision          *int64          `json:"corpus_revision,omitempty" jsonschema:"Optional corpus revision pin from a previous offline read"`
}

// OpportunityCandidateOutput describes one ranked contribution candidate.
type OpportunityCandidateOutput struct {
	Rank               int                            `json:"rank"`
	Ref                string                         `json:"ref"`
	Repo               string                         `json:"repo"`
	Number             int                            `json:"number"`
	Title              string                         `json:"title"`
	URL                string                         `json:"url"`
	Score              RadarScore                     `json:"score" jsonschema:"Deterministic Contribution Radar score from 0 to 100"`
	Eligibility        string                         `json:"eligibility"`
	Confidence         string                         `json:"confidence" jsonschema:"Categorical evidence confidence such as low, medium, or high"`
	PositiveSignals    []string                       `json:"positive_signals,omitempty"`
	Risks              []string                       `json:"risks,omitempty"`
	Blockers           []string                       `json:"blockers,omitempty"`
	Unknowns           []string                       `json:"unknowns,omitempty"`
	LinkedPullRequests []int                          `json:"linked_pull_requests,omitempty"`
	RelatedWork        []OpportunityRelatedWorkOutput `json:"related_work,omitempty"`
	SourceUpdatedAt    string                         `json:"source_updated_at,omitempty"`
}

// RepositoryOpportunitySummaryOutput reports ranking coverage for one repository.
type RepositoryOpportunitySummaryOutput struct {
	Repo             string `json:"repo"`
	TotalOpenIssues  int    `json:"total_open_issues"`
	Considered       int    `json:"considered"`
	Returned         int    `json:"returned"`
	Truncated        bool   `json:"truncated"`
	PopulationCapped bool   `json:"population_capped"`
}

// RankOpportunitiesOutput combines deterministic cross-repository ranking with
// per-repository coverage or availability results.
type RankOpportunitiesOutput struct {
	Status         string                                          `json:"status"`
	Candidates     []OpportunityCandidateOutput                    `json:"candidates"`
	Repositories   []BatchItem[RepositoryOpportunitySummaryOutput] `json:"repositories"`
	GeneratedAt    string                                          `json:"generated_at"`
	Total          int                                             `json:"total"`
	Truncated      bool                                            `json:"truncated"`
	CorpusRevision int64                                           `json:"corpus_revision"`
}

// ReadinessInput selects a contribution opportunity readiness report.
type ReadinessInput struct {
	OpportunityID string `json:"opportunity_id" jsonschema:"Opportunity ID"`
}

// ReadinessCheck is one explainable readiness rule result.
type ReadinessCheck struct {
	CheckID      string   `json:"check_id"`
	RuleID       string   `json:"rule_id"`
	RuleVersion  string   `json:"rule_version"`
	Status       string   `json:"status"`
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	Remediation  string   `json:"remediation,omitempty"`
	EvaluatedAt  string   `json:"evaluated_at"`
}

// ReadinessOutput is the stable MCP representation of one readiness report.
type ReadinessOutput struct {
	OpportunityID  string           `json:"opportunity_id"`
	RuleSetVersion string           `json:"rule_set_version"`
	Status         string           `json:"status"`
	EvaluatedAt    string           `json:"evaluated_at"`
	Checks         []ReadinessCheck `json:"checks"`
}

// OpportunityRelatedWorkOutput is the compact MCP view of one Radar
// relationship. Exact source evidence remains available in the CLI JSON view.
type OpportunityRelatedWorkOutput struct {
	Ref       string `json:"ref"`
	Relation  string `json:"relation"`
	Direction string `json:"direction,omitempty"`
	State     string `json:"state,omitempty"`
}

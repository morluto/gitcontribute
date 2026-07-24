package mcpcontract

import (
	"context"
	"encoding/json"

	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/lens"
	"github.com/morluto/gitcontribute/internal/similarity"
)

// ToolError is the stable, actionable shape for agent-correctable requests.
type ToolError struct {
	Code             string            `json:"code"`
	Message          string            `json:"message"`
	Field            string            `json:"field,omitempty"`
	Retryable        bool              `json:"retryable"`
	Example          map[string]any    `json:"example,omitempty"`
	SuggestedActions []SuggestedAction `json:"suggested_actions,omitempty"`
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

// Canonical MCP tool names group operations by capability and side-effect boundary.
const (
	ToolSearchRepositories       = "corpus.search_repositories"
	ToolSearchThreads            = "corpus.search_threads"
	ToolSearchCode               = "corpus.search_code"
	ToolGetRepositories          = "corpus.get_repositories"
	ToolGetThreads               = "corpus.get_threads"
	ToolRankThreads              = "corpus.rank_threads"
	ToolFindPrecedents           = "corpus.find_precedents"
	ToolGetRepositoryDossier     = "corpus.get_repository_dossier"
	ToolExplainMatch             = "corpus.explain_match"
	ToolGetInvestigation         = "corpus.get_investigation"
	ToolListOpportunities        = "corpus.list_opportunities"
	ToolGetOpportunity           = "corpus.get_opportunity"
	ToolGetEvidence              = "corpus.get_evidence"
	ToolGetReadiness             = "corpus.get_readiness"
	ToolFindClusters             = "corpus.find_clusters"
	ToolFindNeighbors            = "corpus.find_neighbors"
	ToolGetCoverage              = "corpus.get_coverage"
	ToolBuildRepositoryDossier   = "corpus.build_repository_dossier"
	ToolGetJob                   = "jobs.get"
	ToolCancelJob                = "jobs.cancel"
	ToolSearchGitHubRepositories = "github.search_repositories"
	ToolSyncRepositoryMetadata   = "github.sync_repository_metadata"
	ToolSyncThreads              = "github.sync_threads"
	ToolHydrateThreads           = "github.hydrate_threads"
	ToolGetAuthenticatedIdentity = "github.get_authenticated_identity"
	ToolSyncAuthoredPullRequests = "github.sync_authored_pull_requests"
	ToolSyncPullRequestStatus    = "github.sync_pull_request_status"
	ToolListPullRequestPortfolio = "corpus.list_pull_request_portfolio"
	ToolFindPortfolioOverlaps    = "corpus.find_portfolio_overlaps"
	ToolIndexRepositories        = "code.index_repositories"
	ToolCheckMergeConflicts      = "workspace.check_merge_conflicts"
	ToolInspectCommitChanges     = "workspace.inspect_commit_changes"
	ToolPlanSemanticCommits      = "workspace.plan_semantic_commits"
	ToolQueryDeepWiki            = "research.query_deepwiki"
	ToolCreateWorkspace          = "workspace.create"
	ToolAdoptWorkspace           = "workspace.adopt"
	ToolDefineValidation         = "validation.define"
	ToolRunValidation            = "validation.run"
	ToolRunRepeatedValidation    = "validation.run_repeated"
	ToolStartInvestigation       = "workflow.start_investigation"
	ToolRecordHypothesis         = "workflow.record_hypothesis"
	ToolCheckDuplicates          = "workflow.check_duplicates"
	ToolFindCompetingWork        = "workflow.find_competing_work"
	ToolPromoteOpportunity       = "workflow.promote_opportunity"
	ToolPrepareContribution      = "workflow.prepare_contribution"
	ToolExportManifest           = "workflow.export_manifest"
	ToolLinkPullRequest          = "workflow.link_pull_request"
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
	Confidence       float64                  `json:"confidence" jsonschema:"Confidence from 0 to 1"`
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
	ID               string   `json:"id" jsonschema:"Concern ID"`
	Title            *string  `json:"title,omitempty" jsonschema:"Replacement title"`
	ProblemStatement *string  `json:"problem_statement,omitempty" jsonschema:"Replacement problem statement"`
	SuspectedOwner   *string  `json:"suspected_owner,omitempty" jsonschema:"Replacement owner boundary"`
	Confidence       *float64 `json:"confidence,omitempty" jsonschema:"Replacement confidence from 0 to 1"`
	Unknowns         []string `json:"unknowns,omitempty" jsonschema:"Replacement explicit unknowns"`
	SuccessCriterion *string  `json:"success_criterion,omitempty" jsonschema:"Replacement success criterion"`
	Notes            *string  `json:"notes,omitempty" jsonschema:"Replacement local notes"`
	EvidenceIDs      []string `json:"evidence_ids,omitempty" jsonschema:"Replacement evidence IDs"`
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
	Confidence       float64                 `json:"confidence" jsonschema:"Confidence from 0 to 1"`
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

// ConcernListOutput contains one bounded offline result set.
type ConcernListOutput struct {
	Concerns  []ConcernOutput `json:"concerns" jsonschema:"Bounded concern results"`
	Limit     int             `json:"limit" jsonschema:"Effective result limit"`
	Total     int             `json:"total" jsonschema:"Total matching concerns"`
	Truncated bool            `json:"truncated" jsonschema:"Whether more matching concerns exist"`
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
	OpportunityID string `json:"opportunity_id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	RenderedAt    string `json:"rendered_at"`
	ManifestID    string `json:"manifest_id,omitempty" jsonschema:"Referenced stored evidence manifest ID"`
}

// ExportManifestInput selects bounded local evidence for one contribution manifest.
type ExportManifestInput struct {
	OpportunityID string                    `json:"opportunity_id" jsonschema:"Opportunity ID"`
	WorkspaceID   string                    `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID to bind"`
	PullRequest   *ManifestPullRequestInput `json:"pull_request,omitempty" jsonschema:"Exact stored pull request to include"`
}

// ManifestPullRequestInput identifies one exact stored pull request.
type ManifestPullRequestInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Number int    `json:"number" jsonschema:"Positive pull request number"`
}

// ManifestOutput returns the stable identity and full in-toto-shaped statement.
type ManifestOutput struct {
	ManifestID    string         `json:"manifest_id" jsonschema:"Stable sha256-prefixed manifest ID"`
	ContentSHA256 string         `json:"content_sha256" jsonschema:"Hex SHA-256 of stable manifest content"`
	SchemaVersion string         `json:"schema_version" jsonschema:"Contribution manifest predicate schema version"`
	Status        string         `json:"status" jsonschema:"Overall completeness status"`
	Statement     map[string]any `json:"statement" jsonschema:"Full in-toto-shaped evidence statement"`
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
}

// OpportunityCandidateOutput describes one ranked contribution candidate.
type OpportunityCandidateOutput struct {
	Rank               int                            `json:"rank"`
	Ref                string                         `json:"ref"`
	Repo               string                         `json:"repo"`
	Number             int                            `json:"number"`
	Title              string                         `json:"title"`
	URL                string                         `json:"url"`
	Score              int                            `json:"score"`
	Eligibility        string                         `json:"eligibility"`
	Confidence         string                         `json:"confidence"`
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
	Status       string                                          `json:"status"`
	Candidates   []OpportunityCandidateOutput                    `json:"candidates"`
	Repositories []BatchItem[RepositoryOpportunitySummaryOutput] `json:"repositories"`
	GeneratedAt  string                                          `json:"generated_at"`
	Total        int                                             `json:"total"`
	Truncated    bool                                            `json:"truncated"`
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

// SearchGitHubRepositoriesInput defines one bounded live GitHub search.
type SearchGitHubRepositoriesInput struct {
	RawQuery       string   `json:"raw_query,omitempty" jsonschema:"Advanced raw GitHub query; exclusive with filters"`
	Text           string   `json:"text,omitempty" jsonschema:"Text to match"`
	MatchFields    []string `json:"match_fields,omitempty" jsonschema:"Text fields: name, description, or readme"`
	Topics         []string `json:"topics,omitempty" jsonschema:"Topics that must all match"`
	Language       string   `json:"language,omitempty" jsonschema:"Primary language"`
	StarsMin       int      `json:"stars_min,omitempty" jsonschema:"Minimum stargazer count"`
	StarsMax       int      `json:"stars_max,omitempty" jsonschema:"Maximum stargazer count"`
	CreatedAfter   string   `json:"created_after,omitempty" jsonschema:"Created on or after YYYY-MM-DD"`
	CreatedBefore  string   `json:"created_before,omitempty" jsonschema:"Created on or before YYYY-MM-DD"`
	PushedAfter    string   `json:"pushed_after,omitempty" jsonschema:"Pushed on or after YYYY-MM-DD"`
	PushedBefore   string   `json:"pushed_before,omitempty" jsonschema:"Pushed on or before YYYY-MM-DD"`
	Archived       *bool    `json:"archived,omitempty" jsonschema:"Archived state"`
	Fork           *bool    `json:"fork,omitempty" jsonschema:"Fork state"`
	Sort           string   `json:"sort,omitempty" jsonschema:"Optional GitHub sort: stars, forks, help-wanted-issues, or updated"`
	Order          string   `json:"order,omitempty" jsonschema:"Sort order: asc or desc"`
	Limit          int      `json:"limit,omitempty" jsonschema:"Results per page from 1 to 100"`
	Page           int      `json:"page,omitempty" jsonschema:"Result page within GitHub's 1,000-result cap"`
	ResponseFormat string   `json:"response_format,omitempty" jsonschema:"concise or detailed"`
}

// SuggestedAction describes a non-mandatory follow-up with reusable arguments.
type SuggestedAction struct {
	Tool      string         `json:"tool"`
	Reason    string         `json:"reason"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// SearchWarning explains a request-specific limitation and how to improve it.
type SearchWarning struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// RepositorySearchMatch is a token-bounded live repository search result.
type RepositorySearchMatch struct {
	Ref           string                   `json:"ref"`
	Owner         string                   `json:"owner"`
	Repo          string                   `json:"repo"`
	Description   *string                  `json:"description,omitempty"`
	Language      *string                  `json:"language,omitempty"`
	Stars         *int                     `json:"stars,omitempty"`
	PushedAt      string                   `json:"pushed_at,omitempty"`
	Metadata      RepositoryMetadataOutput `json:"metadata"`
	DefaultBranch *string                  `json:"default_branch,omitempty"`
	License       *string                  `json:"license,omitempty"`
	Topics        []string                 `json:"topics,omitempty"`
	Watchers      *int                     `json:"watchers,omitempty"`
	Forks         *int                     `json:"forks,omitempty"`
	OpenIssues    *int                     `json:"open_issues,omitempty"`
	Archived      *bool                    `json:"archived,omitempty"`
	Fork          *bool                    `json:"fork,omitempty"`
}

// SearchGitHubRepositoriesOutput contains search results and completeness metadata.
type SearchGitHubRepositoriesOutput struct {
	Status           string                             `json:"status"`
	Query            string                             `json:"query"`
	Interpretation   string                             `json:"interpretation"`
	ResponseFormat   string                             `json:"response_format"`
	Page             int                                `json:"page"`
	NextPage         int                                `json:"next_page,omitempty"`
	Total            int                                `json:"total"`
	Incomplete       bool                               `json:"incomplete"`
	Items            []BatchItem[RepositorySearchMatch] `json:"items"`
	Warnings         []SearchWarning                    `json:"warnings,omitempty"`
	SuggestedActions []SuggestedAction                  `json:"suggested_actions,omitempty"`
}

// RepositoryRef identifies one GitHub repository without implying that it has
// been fetched or indexed locally.
type RepositoryRef struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
}

// ThreadRef identifies an exact issue or pull request. Kind may be omitted only
// for tools that intentionally resolve a number without a preselected kind.
type ThreadRef struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Optional thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"Positive issue or pull request number"`
}

// BatchItem reports the outcome for one input-derived key while preserving
// input order. Value is present for complete items; recovery fields explain
// retryable, unavailable, or failed items without failing unrelated work.
type BatchItem[T any] struct {
	Key          string `json:"key"`
	Status       string `json:"status"`
	Value        *T     `json:"value,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
	RetryAfterMS int    `json:"retry_after_ms,omitempty"`
	NextAction   string `json:"next_action,omitempty"`
}

// GetRepositoriesInput selects repositories for a bounded corpus read.
type GetRepositoriesInput struct {
	Repositories []RepositoryRef `json:"repositories" jsonschema:"One to 100 repository identities"`
}

// RepositoryMetadataOutput describes the coverage of repository metadata.
type RepositoryMetadataOutput struct {
	Status          string `json:"status"`
	ObservedAt      string `json:"observed_at,omitempty"`
	SourceUpdatedAt string `json:"source_updated_at,omitempty"`
	NextAction      string `json:"next_action,omitempty"`
}

// TypedRepositoryOutput contains repository facts with explicit metadata coverage.
type TypedRepositoryOutput struct {
	Ref           string                   `json:"ref"`
	Owner         string                   `json:"owner"`
	Repo          string                   `json:"repo"`
	Metadata      RepositoryMetadataOutput `json:"metadata"`
	Description   *string                  `json:"description"`
	DefaultBranch *string                  `json:"default_branch"`
	Language      *string                  `json:"language"`
	License       *string                  `json:"license"`
	Topics        []string                 `json:"topics,omitempty"`
	Stars         *int                     `json:"stars"`
	Watchers      *int                     `json:"watchers"`
	Forks         *int                     `json:"forks"`
	OpenIssues    *int                     `json:"open_issues"`
	Archived      *bool                    `json:"archived"`
	Fork          *bool                    `json:"fork"`
}

// GetRepositoriesOutput preserves repository input order and represents
// unobserved metadata with nullable facts instead of false zero values.
type GetRepositoriesOutput struct {
	Status string                             `json:"status"`
	Items  []BatchItem[TypedRepositoryOutput] `json:"items"`
}

// GetThreadsInput selects exact threads and the desired response view.
type GetThreadsInput struct {
	Threads []ThreadRef `json:"threads" jsonschema:"One to 100 exact thread identities"`
	View    string      `json:"view,omitempty" jsonschema:"compact or full; compact omits bodies"`
}

// GetThreadsOutput preserves exact-thread input order and item-level failures.
type GetThreadsOutput struct {
	Status string                    `json:"status"`
	Items  []BatchItem[ThreadOutput] `json:"items"`
}

// FindPrecedentsInput selects source threads for offline analogue discovery.
type FindPrecedentsInput struct {
	Threads []ThreadRef `json:"threads" jsonschema:"One to 20 source threads"`
	Limit   int         `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

// PrecedentOutput describes one stored thread analogous to a source thread.
type PrecedentOutput struct {
	Source      string                 `json:"source"`
	Ref         string                 `json:"ref"`
	Kind        string                 `json:"kind"`
	State       string                 `json:"state"`
	StateReason string                 `json:"state_reason,omitempty"`
	Title       string                 `json:"title"`
	Score       float64                `json:"score"`
	RuleVersion similarity.RuleVersion `json:"rule_version"`
	Reasons     []string               `json:"reasons"`
	ClosedAt    string                 `json:"closed_at,omitempty"`
	MergedAt    string                 `json:"merged_at,omitempty"`
}

// FindPrecedentsOutput returns stored closed or merged analogues for each
// source thread; it does not perform a network read.
type FindPrecedentsOutput struct {
	Status string                    `json:"status"`
	Items  []BatchItem[PrecedentSet] `json:"items"`
	Total  int                       `json:"total"`
}

// PrecedentSet reports both scored results and bounded candidate coverage.
type PrecedentSet struct {
	Matches    []PrecedentOutput `json:"matches" jsonschema:"Ranked precedent matches"`
	Population int               `json:"population" jsonschema:"All stored closed candidates"`
	Considered int               `json:"considered" jsonschema:"Candidates scored under the bound"`
	Truncated  bool              `json:"truncated" jsonschema:"Whether candidates or matches were omitted"`
}

// GetJobsInput selects durable jobs for a bounded status read.
type GetJobsInput struct {
	IDs            []string `json:"ids" jsonschema:"One to 100 durable job IDs"`
	ResponseFormat string   `json:"response_format,omitempty" jsonschema:"concise omits request and result payloads; detailed includes them"`
}

// GetJobsOutput reports multiple durable jobs in requested order so callers can
// poll concurrent work with one MCP round trip.
type GetJobsOutput struct {
	Status string                    `json:"status"`
	Items  []BatchItem[GetJobOutput] `json:"items"`
}

// SyncRepositoryMetadataInput selects repositories for asynchronous metadata refresh.
type SyncRepositoryMetadataInput struct {
	Repositories []RepositoryRef `json:"repositories" jsonschema:"One to 100 explicit repositories"`
}

// SyncThreadsInput selects either bounded repository-wide header discovery or
// exact thread refresh. It never requests child comments, reviews, or code.
type SyncThreadsInput struct {
	Selection          string          `json:"selection" jsonschema:"repositories or threads"`
	Repositories       []RepositoryRef `json:"repositories,omitempty" jsonschema:"One to 50 repositories in repository mode"`
	Threads            []ThreadRef     `json:"threads,omitempty" jsonschema:"One to 100 exact threads in thread mode"`
	Kind               string          `json:"kind,omitempty" jsonschema:"issue, pull_request, or both in repository mode"`
	State              string          `json:"state,omitempty" jsonschema:"open, closed, or all in repository mode"`
	UpdatedAfter       string          `json:"updated_after,omitempty" jsonschema:"Optional RFC 3339 lower bound in repository mode"`
	LimitPerRepository int             `json:"limit_per_repository,omitempty" jsonschema:"Maximum headers per repository from 1 to 1000"`
	MaxRequests        int             `json:"max_requests,omitempty" jsonschema:"Maximum total GitHub requests from 9 to 1000"`
}

// HydrateThreadsInput requests explicit child facets for already selected
// threads. Facets must be non-empty to prevent accidental broad hydration.
type HydrateThreadsInput struct {
	Threads  []ThreadRef `json:"threads" jsonschema:"One to 100 exact threads"`
	Facets   []string    `json:"facets" jsonschema:"One or more explicit child facets"`
	MaxPages int         `json:"max_pages,omitempty" jsonschema:"Maximum pages per facet from 1 to 100"`
}

// AuthenticatedIdentityOutput identifies the account associated with active credentials.
type AuthenticatedIdentityOutput struct {
	Login      string `json:"login"`
	ID         int64  `json:"id"`
	NodeID     string `json:"node_id,omitempty"`
	ObservedAt string `json:"observed_at"`
}

// SyncAuthoredPullRequestsInput bounds authored pull-request discovery and refresh.
type SyncAuthoredPullRequestsInput struct {
	State        string `json:"state,omitempty" jsonschema:"open, closed, or all"`
	UpdatedAfter string `json:"updated_after,omitempty" jsonschema:"Optional RFC 3339 lower bound"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum authored pull requests from 1 to 500"`
	MaxRequests  int    `json:"max_requests,omitempty" jsonschema:"Maximum total GitHub requests from 11 to 1000"`
}

// SyncPullRequestStatusInput selects pull requests and bounds review hydration.
type SyncPullRequestStatusInput struct {
	PullRequests []ThreadRef `json:"pull_requests" jsonschema:"One to 50 exact pull requests"`
	MaxPages     int         `json:"max_pages,omitempty" jsonschema:"Maximum review pages from 1 to 20"`
}

// ListPullRequestPortfolioInput filters and bounds the stored pull-request portfolio.
type ListPullRequestPortfolioInput struct {
	Author string `json:"author,omitempty" jsonschema:"Optional authored GitHub login"`
	State  string `json:"state,omitempty" jsonschema:"open, closed, or all"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum pull requests from 1 to 100"`
}

// PullRequestPortfolioItem contains source-backed PR facts and a deterministic
// portfolio.v1 attention classification. Missing status facets remain explicit
// in StatusCoverage and Reasons.
type PullRequestPortfolioItem struct {
	Ref                     string                `json:"ref"`
	Owner                   string                `json:"owner"`
	Repo                    string                `json:"repo"`
	Number                  int                   `json:"number"`
	Title                   string                `json:"title"`
	State                   string                `json:"state"`
	Author                  string                `json:"author"`
	Draft                   bool                  `json:"draft"`
	Attention               string                `json:"attention"`
	Reasons                 []string              `json:"reasons"`
	Mergeable               *bool                 `json:"mergeable,omitempty"`
	MergeStateStatus        string                `json:"merge_state_status,omitempty"`
	HeadRef                 string                `json:"head_ref,omitempty"`
	HeadSHA                 string                `json:"head_sha,omitempty"`
	BaseRef                 string                `json:"base_ref,omitempty"`
	BaseSHA                 string                `json:"base_sha,omitempty"`
	ReviewDecision          string                `json:"review_decision,omitempty"`
	ChecksStatus            string                `json:"checks_status,omitempty"`
	ChecksTotal             int                   `json:"checks_total,omitempty"`
	UnresolvedReviewThreads *int                  `json:"unresolved_review_threads,omitempty"`
	MergeQueueState         string                `json:"merge_queue_state,omitempty"`
	MergeQueuePosition      int                   `json:"merge_queue_position,omitempty"`
	ClosingIssues           []string              `json:"closing_issues,omitempty"`
	ChangedFiles            []string              `json:"changed_files,omitempty"`
	StatusCoverage          string                `json:"status_coverage"`
	Facets                  []FacetCoverageOutput `json:"facets"`
	SourceUpdatedAt         string                `json:"source_updated_at"`
	StatusObservedAt        string                `json:"status_observed_at,omitempty"`
}

// ListPullRequestPortfolioOutput contains a deterministic portfolio projection.
type ListPullRequestPortfolioOutput struct {
	Status       string                     `json:"status"`
	RuleVersion  string                     `json:"rule_version"`
	GeneratedAt  string                     `json:"generated_at"`
	PullRequests []PullRequestPortfolioItem `json:"pull_requests"`
	Total        int                        `json:"total"`
	Truncated    bool                       `json:"truncated"`
}

// PortfolioSubjectInput identifies local candidate state for offline overlap analysis.
type PortfolioSubjectInput struct {
	Kind string `json:"kind" jsonschema:"Candidate kind: opportunity, workspace, or pull_request"`
	Ref  string `json:"ref" jsonschema:"Local candidate ID or corpus pull-request thread ID"`
}

// FindPortfolioOverlapsInput compares candidates with exact stored authored PRs.
type FindPortfolioOverlapsInput struct {
	Candidates   []PortfolioSubjectInput `json:"candidates" jsonschema:"One to 50 local candidate subjects"`
	PullRequests []ThreadRef             `json:"pull_requests" jsonschema:"One to 100 exact authored pull requests"`
}

// PortfolioOverlapEvidenceOutput is one exact observed overlap reason.
type PortfolioOverlapEvidenceOutput struct {
	Kind       string   `json:"kind"`
	Value      string   `json:"value"`
	Score      float64  `json:"score,omitempty"`
	SourceRefs []string `json:"source_refs"`
}

// PortfolioOverlapMatchOutput associates overlap evidence with one authored PR.
type PortfolioOverlapMatchOutput struct {
	PullRequestThreadID int64                            `json:"pull_request_thread_id"`
	Evidence            []PortfolioOverlapEvidenceOutput `json:"evidence"`
}

// PortfolioOverlapOutput preserves explicit coverage and never infers no overlap.
type PortfolioOverlapOutput struct {
	Candidate PortfolioSubjectInput         `json:"candidate"`
	Status    string                        `json:"status"`
	Coverage  map[string]string             `json:"coverage"`
	Matches   []PortfolioOverlapMatchOutput `json:"matches"`
}

// FindPortfolioOverlapsOutput preserves candidate input order.
type FindPortfolioOverlapsOutput struct {
	Status string                              `json:"status"`
	Items  []BatchItem[PortfolioOverlapOutput] `json:"items"`
}

// LinkPullRequestInput explicitly associates a stored PR with local workflow state.
type LinkPullRequestInput struct {
	PullRequest   ThreadRef `json:"pull_request" jsonschema:"Exact stored pull request to link"`
	OpportunityID string    `json:"opportunity_id,omitempty" jsonschema:"Optional local opportunity ID"`
	WorkspaceID   string    `json:"workspace_id,omitempty" jsonschema:"Optional managed workspace ID"`
}

// LinkPullRequestOutput reports the idempotently stored local relationship.
type LinkPullRequestOutput struct {
	ID                  int64  `json:"id"`
	PullRequestThreadID int64  `json:"pull_request_thread_id"`
	OpportunityID       string `json:"opportunity_id,omitempty"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	CreatedAt           string `json:"created_at"`
}

// IndexRepositoryInput identifies one repository to acquire and index.
type IndexRepositoryInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Remote string `json:"remote,omitempty" jsonschema:"Optional explicit credential-free Git remote"`
}

// IndexRepositoriesInput selects repositories for bounded asynchronous indexing.
type IndexRepositoriesInput struct {
	Repositories []IndexRepositoryInput `json:"repositories" jsonschema:"One to 10 repositories to acquire and index"`
}

// MergeConflictInput names two already-fetched revisions in a managed workspace.
type MergeConflictInput struct {
	WorkspaceID string `json:"workspace_id"`
	BaseOID     string `json:"base_oid"`
	HeadOID     string `json:"head_oid"`
}

// CheckMergeConflictsInput selects local revision comparisons.
type CheckMergeConflictsInput struct {
	Comparisons []MergeConflictInput `json:"comparisons" jsonschema:"One to 50 already-fetched revision comparisons"`
}

// MergeConflictOutput reports the result of one local revision comparison.
type MergeConflictOutput struct {
	WorkspaceID string `json:"workspace_id"`
	BaseOID     string `json:"base_oid"`
	HeadOID     string `json:"head_oid"`
	MergeBase   string `json:"merge_base,omitempty"`
	Conflicted  bool   `json:"conflicted"`
	Summary     string `json:"summary"`
}

// CheckMergeConflictsOutput preserves comparison order and isolates local Git
// failures to the affected comparison.
type CheckMergeConflictsOutput struct {
	Status string                           `json:"status"`
	Items  []BatchItem[MergeConflictOutput] `json:"items"`
}

// DeepWikiInput selects one bounded external derived-knowledge read. DeepWiki
// results are context, not authority for current GitHub state.
type DeepWikiInput struct {
	Action         string   `json:"action" jsonschema:"structure, contents, or question"`
	Repository     string   `json:"repository,omitempty" jsonschema:"OWNER/REPO for structure or contents"`
	Repositories   []string `json:"repositories,omitempty" jsonschema:"One to 10 OWNER/REPO values for question"`
	Question       string   `json:"question,omitempty" jsonschema:"Focused cross-repository question"`
	MaxOutputBytes int      `json:"max_output_bytes,omitempty" jsonschema:"Maximum returned bytes from 1024 to 1048576"`
}

// DeepWikiOutput labels provider prose as derived external content and reports
// provider-level unavailability without persisting the response.
type DeepWikiOutput struct {
	Status       string   `json:"status"`
	Provider     string   `json:"provider"`
	Action       string   `json:"action"`
	Repositories []string `json:"repositories"`
	Question     string   `json:"question,omitempty"`
	Result       string   `json:"result,omitempty"`
	SourceURL    string   `json:"source_url,omitempty"`
	RetrievedAt  string   `json:"retrieved_at"`
	Provenance   string   `json:"provenance"`
	Truncated    bool     `json:"truncated"`
	Reason       string   `json:"reason,omitempty"`
	NextAction   string   `json:"next_action,omitempty"`
}

// ErrNotFound lets readers distinguish absent corpus objects from failures.
var ErrNotFound = failure.NotFound(nil)

// Reader is the local, read-only application boundary exposed through MCP.
// Implementations must not perform network access.
type Reader interface {
	Search(context.Context, SearchInput) (SearchOutput, error)
	SearchRepositories(context.Context, SearchRepositoriesInput) (SearchRepositoriesOutput, error)
	Repository(context.Context, RepoInput) (RepositoryOutput, error)
	Thread(context.Context, ThreadInput) (ThreadOutput, error)
	ThreadByNumber(context.Context, ThreadByNumberInput) (ThreadOutput, error)
	Dossier(context.Context, RepoInput) (DossierOutput, error)
	SearchCode(context.Context, SearchCodeInput) (SearchCodeOutput, error)
	ExplainMatch(context.Context, ExplainMatchInput) (ExplainMatchOutput, error)
	GetJob(context.Context, GetJobInput) (GetJobOutput, error)
	Investigation(context.Context, InvestigationInput) (InvestigationOutput, error)
	ListOpportunities(context.Context, ListOpportunitiesInput) (ListOpportunitiesOutput, error)
	Opportunity(context.Context, OpportunityInput) (OpportunityOutput, error)
	Evidence(context.Context, EvidenceInput) (EvidenceOutput, error)
	Readiness(context.Context, ReadinessInput) (ReadinessOutput, error)
	FindClusters(context.Context, FindClustersInput) (FindClustersOutput, error)
	GetCoverage(context.Context, GetCoverageInput) (GetCoverageOutput, error)
	Lens(context.Context, LensInput) (LensOutput, error)
}

// RepoInput identifies a repository for an MCP operation.
type RepoInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
}

// ThreadInput identifies an issue or pull request for an MCP operation.
type ThreadInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"GitHub issue or pull request number"`
}

// SearchInput describes an offline thread search page.
type SearchInput struct {
	Query        string   `json:"query" jsonschema:"Full-text query"`
	Owner        string   `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo         string   `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Kind         string   `json:"kind,omitempty" jsonschema:"Optional thread kind"`
	State        string   `json:"state,omitempty"`
	StateReason  string   `json:"state_reason,omitempty"`
	Merged       *bool    `json:"merged,omitempty"`
	Author       string   `json:"author,omitempty"`
	Association  string   `json:"author_association,omitempty"`
	Assignee     string   `json:"assignee,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	UpdatedAfter string   `json:"updated_after,omitempty"`
	Limit        int      `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor       string   `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	Sort         string   `json:"sort,omitempty" jsonschema:"Order: relevance or updated"`
	MatchMode    string   `json:"match_mode,omitempty" jsonschema:"Term matching: all requires every term; any requires at least one term"`
	View         string   `json:"view,omitempty" jsonschema:"compact omits full bodies and keeps bounded excerpts; full includes stored bodies"`
}

// RepositoryOutput is the stable MCP representation of a repository.
type RepositoryOutput struct {
	Owner     string         `json:"owner"`
	Repo      string         `json:"repo"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// ThreadOutput is the stable MCP representation of an issue or pull request.
type ThreadOutput struct {
	Owner             string   `json:"owner"`
	Repo              string   `json:"repo"`
	Kind              string   `json:"kind"`
	Number            int      `json:"number"`
	State             string   `json:"state"`
	StateReason       string   `json:"state_reason,omitempty"`
	Title             string   `json:"title"`
	Body              string   `json:"body,omitempty"`
	Author            string   `json:"author,omitempty"`
	AuthorAssociation string   `json:"author_association,omitempty"`
	Labels            []string `json:"labels,omitempty"`
	Assignees         []string `json:"assignees,omitempty"`
	Draft             bool     `json:"draft,omitempty"`
	ClosedAt          string   `json:"closed_at,omitempty"`
	MergedAt          string   `json:"merged_at,omitempty"`
	Merged            *bool    `json:"merged,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
	MatchSource       string   `json:"match_source,omitempty"`
	MatchExcerpt      string   `json:"match_excerpt,omitempty"`
	MatchTruncated    bool     `json:"match_truncated,omitempty" jsonschema:"Whether the per-thread hydrated search document was bounded"`
	MatchUpdatedAt    string   `json:"match_updated_at,omitempty"`
}

// SearchOutput contains one page of offline thread matches.
type SearchOutput struct {
	Query               string         `json:"query"`
	QueryInterpretation string         `json:"query_interpretation"`
	MatchMode           string         `json:"match_mode"`
	View                string         `json:"view"`
	Matches             []ThreadOutput `json:"matches"`
	Total               int            `json:"total"`
	NextCursor          string         `json:"next_cursor,omitempty"`
	Suggestion          string         `json:"suggestion,omitempty"`
}

// DossierOutput contains a persisted repository dossier snapshot.
type DossierOutput struct {
	Owner    string         `json:"owner"`
	Repo     string         `json:"repo"`
	AsOf     string         `json:"as_of,omitempty"`
	Sections map[string]any `json:"sections"`
}

// SourceRef records provenance for an MCP result or workflow artifact.
type SourceRef struct {
	Source     string `json:"source" jsonschema:"Source identifier"`
	URL        string `json:"url,omitempty" jsonschema:"Source URL"`
	CommitSHA  string `json:"commit_sha,omitempty" jsonschema:"Source commit SHA"`
	ObservedAt string `json:"observed_at,omitempty" jsonschema:"Observation timestamp"`
	AsOf       string `json:"as_of,omitempty" jsonschema:"As-of timestamp"`
}

// SearchCodeInput describes an offline code search page.
type SearchCodeInput struct {
	Query  string `json:"query" jsonschema:"Code search query"`
	Owner  string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo   string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
}

// CodeMatchOutput identifies one stored code match.
type CodeMatchOutput struct {
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	Commit   string `json:"commit"`
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Snippet  string `json:"snippet"`
	Bytes    int    `json:"bytes"`
}

// CodeIndexCoverageOutput reports one selected snapshot's indexing coverage.
type CodeIndexCoverageOutput struct {
	Repo           string `json:"repo"`
	Status         string `json:"status" jsonschema:"Index coverage state"`
	Commit         string `json:"commit"`
	Truncated      bool   `json:"truncated" jsonschema:"Whether index limits omitted files"`
	IndexedFiles   int    `json:"indexed_files" jsonschema:"Files indexed in this snapshot"`
	TrackedEntries int    `json:"tracked_entries" jsonschema:"Tracked tree entries considered"`
	SkippedFiles   int    `json:"skipped_files" jsonschema:"Entries omitted by policy or limits"`
	SkippedPolicy  int    `json:"skipped_policy" jsonschema:"Invalid, excluded, or non-regular entries"`
	SkippedLimits  int    `json:"skipped_limits" jsonschema:"Entries omitted by file-size, total-size, or file-count bounds"`
	SkippedNonText int    `json:"skipped_non_text" jsonschema:"Entries omitted because content was binary or invalid UTF-8"`
}

// SearchCodeOutput contains one page of offline code matches.
type SearchCodeOutput struct {
	Query      string                    `json:"query"`
	Total      int                       `json:"total"`
	Matches    []CodeMatchOutput         `json:"matches"`
	Coverage   []CodeIndexCoverageOutput `json:"coverage,omitempty"`
	NextCursor string                    `json:"next_cursor,omitempty"`
}

// InvestigationInput selects an investigation and bounds nested hypotheses.
type InvestigationInput struct {
	ID              string `json:"id" jsonschema:"Investigation ID"`
	HypothesisLimit int    `json:"hypothesis_limit,omitempty" jsonschema:"Maximum hypotheses from 1 to 100"`
}

// HypothesisSummary is the compact hypothesis representation nested in an investigation.
type HypothesisSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

// InvestigationOutput is the stable MCP representation of an investigation.
type InvestigationOutput struct {
	ID              string              `json:"id"`
	Owner           string              `json:"owner"`
	Repo            string              `json:"repo"`
	CommitSHA       string              `json:"commit_sha,omitempty"`
	Lens            string              `json:"lens,omitempty"`
	Status          string              `json:"status"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at"`
	HypothesisTotal int                 `json:"hypothesis_total"`
	Hypotheses      []HypothesisSummary `json:"hypotheses,omitempty"`
}

// ListOpportunitiesInput selects and bounds opportunities for an investigation.
type ListOpportunitiesInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

// OpportunitySummary is the compact opportunity representation used in lists.
type OpportunitySummary struct {
	ID              string  `json:"id"`
	InvestigationID string  `json:"investigation_id"`
	Title           string  `json:"title"`
	Category        string  `json:"category"`
	Status          string  `json:"status"`
	Confidence      float64 `json:"confidence"`
	CollisionStatus string  `json:"collision_status"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// ListOpportunitiesOutput contains bounded opportunities for an investigation.
type ListOpportunitiesOutput struct {
	Opportunities []OpportunitySummary `json:"opportunities"`
	Total         int                  `json:"total"`
}

// OpportunityInput selects an opportunity and bounds nested evidence.
type OpportunityInput struct {
	ID            string `json:"id" jsonschema:"Opportunity ID"`
	EvidenceLimit int    `json:"evidence_limit,omitempty" jsonschema:"Maximum evidence IDs from 1 to 100"`
}

// OpportunityOutput is the stable MCP representation of a contribution opportunity.
type OpportunityOutput struct {
	ID                  string      `json:"id"`
	InvestigationID     string      `json:"investigation_id"`
	HypothesisID        string      `json:"hypothesis_id,omitempty"`
	Title               string      `json:"title"`
	ProblemStatement    string      `json:"problem_statement"`
	Category            string      `json:"category"`
	Scope               string      `json:"scope"`
	Impact              string      `json:"impact"`
	Confidence          float64     `json:"confidence"`
	ExpectedEffort      string      `json:"expected_effort,omitempty"`
	Dependencies        []string    `json:"dependencies,omitempty"`
	CollisionStatus     string      `json:"collision_status"`
	MaintainerAlignment string      `json:"maintainer_alignment,omitempty"`
	SourceRefs          []SourceRef `json:"source_refs,omitempty"`
	EvidenceTotal       int         `json:"evidence_total"`
	EvidenceIDs         []string    `json:"evidence_ids,omitempty"`
	Status              string      `json:"status"`
	CreatedAt           string      `json:"created_at"`
	UpdatedAt           string      `json:"updated_at"`
}

// FindClustersInput selects a repository and bounds duplicate clusters.
type FindClustersInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Optional member kind: issue or pull_request"`
	Number int    `json:"number,omitempty" jsonschema:"Optional positive member number"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum clusters from 1 to 100"`
}

// FindNeighborsInput selects a thread and bounds similar-thread results.
type FindNeighborsInput struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number int    `json:"number" jsonschema:"GitHub issue or pull request number"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum neighbors from 1 to 100"`
}

// NeighborOutput describes one similar stored thread and its score.
type NeighborOutput struct {
	Kind   string  `json:"kind"`
	Owner  string  `json:"owner"`
	Repo   string  `json:"repo"`
	Number int     `json:"number"`
	Title  string  `json:"title"`
	State  string  `json:"state"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// FindNeighborsOutput contains deterministic neighbors for a stored thread.
type FindNeighborsOutput struct {
	Owner          string           `json:"owner"`
	Repo           string           `json:"repo"`
	Kind           string           `json:"kind"`
	Number         int              `json:"number"`
	SourceRevision string           `json:"source_revision"`
	Neighbors      []NeighborOutput `json:"neighbors"`
}

// ClusterMemberOutput describes one member of a duplicate cluster.
type ClusterMemberOutput struct {
	Kind     string  `json:"kind"`
	Owner    string  `json:"owner"`
	Repo     string  `json:"repo"`
	Number   int     `json:"number"`
	Title    string  `json:"title,omitempty"`
	State    string  `json:"state,omitempty"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
	Included bool    `json:"included"`
}

// ClusterOutput contains a stable duplicate cluster and its canonical member.
type ClusterOutput struct {
	StableID    string                `json:"stable_id"`
	State       string                `json:"state"`
	Canonical   ClusterMemberOutput   `json:"canonical"`
	MemberCount int                   `json:"member_count"`
	Members     []ClusterMemberOutput `json:"members,omitempty"`
}

// FindClustersOutput contains duplicate clusters for a repository.
type FindClustersOutput struct {
	Owner       string                 `json:"owner"`
	Repo        string                 `json:"repo"`
	RuleVersion similarity.RuleVersion `json:"rule_version,omitempty"`
	Total       int                    `json:"total"`
	Clusters    []ClusterOutput        `json:"clusters"`
	Truncated   bool                   `json:"truncated" jsonschema:"Whether more clusters matched"`
}

// CoverageTarget selects repository-level coverage or, when kind and number
// are both present, coverage for one exact stored thread.
type CoverageTarget struct {
	Owner  string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo   string `json:"repo" jsonschema:"GitHub repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Optional thread kind: issue or pull_request"`
	Number int    `json:"number,omitempty" jsonschema:"Optional positive issue or pull request number"`
}

// GetCoverageInput selects bounded repository or thread facet coverage reads.
type GetCoverageInput struct {
	Targets []CoverageTarget `json:"targets" jsonschema:"One to 100 repository or exact-thread targets"`
}

// FacetCoverageOutput reports completeness and freshness for one facet.
type FacetCoverageOutput struct {
	Facet     string `json:"facet"`
	Complete  bool   `json:"complete"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

// CoverageOutput reports all known coverage for one repository or thread.
type CoverageOutput struct {
	Owner  string                `json:"owner"`
	Repo   string                `json:"repo"`
	Kind   string                `json:"kind,omitempty"`
	Number int                   `json:"number,omitempty"`
	AsOf   string                `json:"as_of"`
	Facets []FacetCoverageOutput `json:"facets"`
}

// GetCoverageOutput preserves target order and isolates missing or invalid
// targets without failing unrelated coverage reads.
type GetCoverageOutput struct {
	Status string                      `json:"status"`
	Items  []BatchItem[CoverageOutput] `json:"items"`
}

// LensInput selects a saved lens by name.
type LensInput struct {
	Name string `json:"name" jsonschema:"Lens name"`
}

// LensOutput contains a saved lens definition and timestamps.
type LensOutput struct {
	Name       string          `json:"name"`
	Definition lens.Definition `json:"definition"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

// Options selects MCP capability profiles. An empty Toolsets list is rejected.
type Options struct {
	Toolsets []string
	ReadOnly bool
}

// SearchRepositoriesInput describes an offline repository search page.
type SearchRepositoriesInput struct {
	Query  string `json:"query,omitempty" jsonschema:"Repository full-text query"`
	Owner  string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo   string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	Sort   string `json:"sort,omitempty" jsonschema:"Order: relevance or updated"`
}

// SearchRepositoriesOutput contains one page of repository matches.
type SearchRepositoriesOutput struct {
	Query      string             `json:"query"`
	Total      int                `json:"total"`
	Matches    []RepositoryOutput `json:"matches"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

// ExplainMatchInput identifies an exact stored result and its original query.
type ExplainMatchInput struct {
	Query  string `json:"query,omitempty" jsonschema:"Original search query"`
	Owner  string `json:"owner" jsonschema:"Repository owner"`
	Repo   string `json:"repo" jsonschema:"Repository name"`
	Kind   string `json:"kind,omitempty" jsonschema:"Match kind: repo, issue, pull_request, or code"`
	Number int    `json:"number,omitempty" jsonschema:"Thread number for issue or pull_request matches"`
	Path   string `json:"path,omitempty" jsonschema:"File path for code matches"`
	Commit string `json:"commit,omitempty" jsonschema:"Commit SHA for code matches"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum explanation facets from 1 to 100"`
}

// ExplainMatchOutput reports the stored facts that contributed to a match score.
type ExplainMatchOutput struct {
	Query           string                `json:"query"`
	Kind            string                `json:"kind"`
	Owner           string                `json:"owner"`
	Repo            string                `json:"repo"`
	Number          int                   `json:"number,omitempty"`
	Path            string                `json:"path,omitempty"`
	Commit          string                `json:"commit,omitempty"`
	State           string                `json:"state,omitempty"`
	Title           string                `json:"title"`
	Snippet         string                `json:"snippet,omitempty"`
	MatchSource     string                `json:"match_source,omitempty" jsonschema:"Stored search document or hydrated facet that matched"`
	RetrievalRank   *float64              `json:"retrieval_rank,omitempty" jsonschema:"Lower-is-better retrieval rank"`
	RankingMethod   string                `json:"ranking_method,omitempty" jsonschema:"Retrieval ranking method"`
	SearchTruncated bool                  `json:"search_truncated,omitempty" jsonschema:"Whether indexed hydrated text was bounded"`
	Reason          string                `json:"reason"`
	SourceRevision  string                `json:"source_revision,omitempty"`
	Facets          []FacetCoverageOutput `json:"facets,omitempty"`
	AsOf            string                `json:"as_of,omitempty"`
}

// GetJobInput selects a durable job by opaque ID.
type GetJobInput struct {
	ID string `json:"id" jsonschema:"Durable job ID"`
}

// GetJobOutput reports durable state and structured progress for a job.
type GetJobOutput struct {
	ID                    string `json:"id"`
	Kind                  string `json:"kind"`
	Status                string `json:"status"`
	Request               any    `json:"request,omitempty"`
	Result                any    `json:"result,omitempty"`
	Error                 string `json:"error,omitempty"`
	Phase                 string `json:"phase,omitempty"`
	CompletedItems        int    `json:"completed_items"`
	TotalItems            int    `json:"total_items"`
	ProgressPercent       int    `json:"progress_percent"`
	RetryAfterMS          int    `json:"retry_after_ms,omitempty"`
	CreatedAt             string `json:"created_at"`
	StartedAt             string `json:"started_at,omitempty"`
	CompletedAt           string `json:"completed_at,omitempty"`
	CancelledAt           string `json:"cancelled_at,omitempty"`
	CancellationRequested bool   `json:"cancellation_requested"`
}

// ThreadByNumberInput identifies a stored issue or pull request by number.
type ThreadByNumberInput struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

// JobReference is returned by long-running tools that submit durable jobs.
type JobReference struct {
	ID               string            `json:"id"`
	Ref              string            `json:"ref"`
	Kind             string            `json:"kind"`
	Status           string            `json:"status"`
	Message          string            `json:"message"`
	PollAfterMS      int               `json:"poll_after_ms,omitempty"`
	SuggestedActions []SuggestedAction `json:"suggested_actions,omitempty"`
}

// BuildRepositoryDossierInput selects a repository for durable dossier generation.
type BuildRepositoryDossierInput RepoInput

// StartInvestigationInput creates a local investigation for a repository revision.
type StartInvestigationInput struct {
	Owner     string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo      string `json:"repo" jsonschema:"GitHub repository name"`
	CommitSHA string `json:"commit_sha,omitempty" jsonschema:"Required commit SHA unless number selects a stored thread"`
	Lens      string `json:"lens,omitempty" jsonschema:"Optional lens name"`
	Kind      string `json:"kind,omitempty" jsonschema:"Optional stored thread kind"`
	Number    int    `json:"number,omitempty" jsonschema:"Stored thread number for atomic baseline creation"`
}

// RecordHypothesisInput records a structured hypothesis and its provenance.
type RecordHypothesisInput struct {
	InvestigationID    string      `json:"investigation_id" jsonschema:"Investigation ID"`
	Title              string      `json:"title" jsonschema:"Hypothesis title"`
	Description        string      `json:"description" jsonschema:"Hypothesis description"`
	Category           string      `json:"category" jsonschema:"Category such as bug, performance, or documentation"`
	ExpectedBehavior   string      `json:"expected_behavior,omitempty" jsonschema:"Expected behavior"`
	ObservedBehavior   string      `json:"observed_behavior,omitempty" jsonschema:"Observed behavior"`
	PotentialImpact    string      `json:"potential_impact,omitempty" jsonschema:"Potential impact"`
	OpenQuestions      []string    `json:"open_questions,omitempty" jsonschema:"Open questions"`
	AffectedComponents []string    `json:"affected_components,omitempty" jsonschema:"Affected components"`
	SourceRefs         []SourceRef `json:"source_refs,omitempty" jsonschema:"Source references"`
}

// HypothesisOutput is the stable MCP representation of a hypothesis.
type HypothesisOutput struct {
	ID                 string      `json:"id"`
	InvestigationID    string      `json:"investigation_id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Category           string      `json:"category"`
	ExpectedBehavior   string      `json:"expected_behavior,omitempty"`
	ObservedBehavior   string      `json:"observed_behavior,omitempty"`
	PotentialImpact    string      `json:"potential_impact,omitempty"`
	OpenQuestions      []string    `json:"open_questions,omitempty"`
	AffectedComponents []string    `json:"affected_components,omitempty"`
	SourceRefs         []SourceRef `json:"source_refs,omitempty"`
	Status             string      `json:"status"`
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`
}

// CheckDuplicatesInput selects a hypothesis or opportunity for duplicate analysis.
type CheckDuplicatesInput struct {
	Target string `json:"target" jsonschema:"Target scope: hypothesis or opportunity"`
	ID     string `json:"id" jsonschema:"Hypothesis or opportunity ID"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum findings from 1 to 100"`
}

// CheckCollisionsInput selects a hypothesis or opportunity for collision analysis.
type CheckCollisionsInput CheckDuplicatesInput

// CheckOutput contains evidence-backed duplicate or collision findings.
type CheckOutput struct {
	Target         string         `json:"target"`
	ID             string         `json:"id"`
	Repo           string         `json:"repo,omitempty"`
	Query          string         `json:"query,omitempty"`
	Total          int            `json:"total"`
	Findings       []EvidenceItem `json:"findings,omitempty"`
	SourceRevision string         `json:"source_revision,omitempty"`
	Limit          int            `json:"limit"`
}

// PromoteOpportunityInput converts a hypothesis into a scoped opportunity.
type PromoteOpportunityInput struct {
	HypothesisID        string      `json:"hypothesis_id" jsonschema:"Hypothesis ID to promote"`
	ProblemStatement    string      `json:"problem_statement" jsonschema:"Problem statement"`
	Scope               string      `json:"scope" jsonschema:"Scope of the opportunity"`
	Impact              string      `json:"impact" jsonschema:"Impact of the opportunity"`
	ExpectedEffort      string      `json:"expected_effort" jsonschema:"Expected effort"`
	Confidence          float64     `json:"confidence" jsonschema:"Confidence from 0.0 to 1.0"`
	Dependencies        []string    `json:"dependencies,omitempty" jsonschema:"Dependencies"`
	MaintainerAlignment string      `json:"maintainer_alignment,omitempty" jsonschema:"Maintainer alignment note"`
	SourceRefs          []SourceRef `json:"source_refs,omitempty" jsonschema:"Source references"`
}

// CancelJobInput selects durable jobs for bounded, persisted cancellation.
type CancelJobInput struct {
	IDs []string `json:"ids" jsonschema:"One to 100 durable job IDs"`
}

// RunValidationInput selects a validation definition and explicitly authorizes execution.
type RunValidationInput struct {
	ID      string `json:"id" jsonschema:"Validation definition ID"`
	Kind    string `json:"kind" jsonschema:"Run kind: base or candidate"`
	Execute bool   `json:"execute" jsonschema:"Must be true to authorize host execution"`
}

// RunRepeatedValidationInput configures one bounded repeat/stress job.
type RunRepeatedValidationInput struct {
	ID             string `json:"id" jsonschema:"Validation definition ID"`
	Target         string `json:"target" jsonschema:"Run target: base, candidate, or both"`
	RunCount       int    `json:"run_count,omitempty" jsonschema:"Attempts per target from 1 to 100"`
	Concurrency    int    `json:"concurrency,omitempty" jsonschema:"Concurrent attempts from 1 to 16"`
	PerRunTimeout  string `json:"per_run_timeout,omitempty" jsonschema:"Optional Go duration per attempt"`
	OverallTimeout string `json:"overall_timeout,omitempty" jsonschema:"Optional Go duration for the whole group"`
	SampleInterval string `json:"sample_interval,omitempty" jsonschema:"Process telemetry interval from 10ms to 10s"`
	Execute        bool   `json:"execute" jsonschema:"Must be true to authorize host execution"`
}

// DefineValidationInput records a bounded validation command without executing it.
type DefineValidationInput struct {
	InvestigationID      string                         `json:"investigation_id" jsonschema:"Investigation ID"`
	Kind                 string                         `json:"kind" jsonschema:"Validation kind"`
	Command              string                         `json:"command" jsonschema:"Shell-free command to execute"`
	WorkspaceID          string                         `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID used for both run kinds"`
	BaseWorkspaceID      string                         `json:"base_workspace_id,omitempty" jsonschema:"Managed base workspace ID; requires candidate_workspace_id"`
	CandidateWorkspaceID string                         `json:"candidate_workspace_id,omitempty" jsonschema:"Managed candidate workspace ID; requires base_workspace_id"`
	Env                  []string                       `json:"env,omitempty" jsonschema:"Allowed environment variable names"`
	Timeout              string                         `json:"timeout,omitempty" jsonschema:"Positive Go duration; defaults to 30m"`
	MaxOutputBytes       int64                          `json:"max_output_bytes,omitempty" jsonschema:"Maximum captured bytes per output stream; defaults to 65536"`
	Observation          *ValidationObservationContract `json:"observation,omitempty" jsonschema:"Expected bounded observations over captured base and candidate output"`
	Protocol             string                         `json:"protocol,omitempty" jsonschema:"Structured protocol adapter: mcp_stdio"`
	ReadinessTimeout     string                         `json:"readiness_timeout,omitempty" jsonschema:"Protocol initialization deadline; defaults to 30s"`
}

// ValidationExpectedObservation is one output assertion evaluated without a shell.
type ValidationExpectedObservation struct {
	Run        string `json:"run" jsonschema:"Run kind: base or candidate"`
	Name       string `json:"name" jsonschema:"Short observation name"`
	Source     string `json:"source" jsonschema:"Captured source: stdout, stderr, or artifact"`
	Matcher    string `json:"matcher" jsonschema:"Matcher: exact or regexp"`
	Pattern    string `json:"pattern" jsonschema:"Bounded exact string or Go regular expression"`
	Occurrence string `json:"occurrence,omitempty" jsonschema:"Expected occurrence: present or absent; defaults to present"`
	Path       string `json:"path,omitempty" jsonschema:"Relative artifact path; valid only when source is artifact"`
}

// ValidationObservationContract ties output assertions to the claimed behavior.
type ValidationObservationContract struct {
	Intent       string                          `json:"intent" jsonschema:"Short proof intent or invariant"`
	Observations []ValidationExpectedObservation `json:"observations" jsonschema:"One to eight expected observations for each of base and candidate"`
}

// ValidationOutput is the stable MCP representation of a validation definition.
type ValidationOutput struct {
	ID                   string                         `json:"id"`
	InvestigationID      string                         `json:"investigation_id"`
	Kind                 string                         `json:"kind"`
	Command              []string                       `json:"command"`
	WorkingDir           string                         `json:"working_dir"`
	BaseWorkingDir       string                         `json:"base_working_dir,omitempty"`
	CandidateDir         string                         `json:"candidate_dir,omitempty"`
	WorkspaceID          string                         `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID used for both run kinds"`
	BaseWorkspaceID      string                         `json:"base_workspace_id,omitempty" jsonschema:"Managed base workspace ID"`
	CandidateWorkspaceID string                         `json:"candidate_workspace_id,omitempty" jsonschema:"Managed candidate workspace ID"`
	Env                  []string                       `json:"environment_allowlist,omitempty"`
	Timeout              string                         `json:"timeout,omitempty"`
	MaxOutputBytes       int64                          `json:"max_output_bytes,omitempty"`
	Observation          *ValidationObservationContract `json:"observation,omitempty"`
	Protocol             string                         `json:"protocol,omitempty" jsonschema:"Declared structured protocol adapter"`
	ReadinessTimeout     string                         `json:"readiness_timeout,omitempty" jsonschema:"Protocol initialization deadline"`
	CreatedAt            string                         `json:"created_at"`
}

// CreateWorkspaceInput configures a durable managed-workspace creation job.
type CreateWorkspaceInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Remote          string `json:"remote,omitempty" jsonschema:"Git remote URL to clone; defaults to the investigation repository"`
	BaseRef         string `json:"base_ref,omitempty" jsonschema:"Base ref to resolve; defaults to the remote HEAD"`
	CandidateRef    string `json:"candidate_ref,omitempty" jsonschema:"Candidate ref to resolve; defaults to the investigation commit"`
	Name            string `json:"name,omitempty" jsonschema:"Workspace name; defaults to a generated ID"`
}

// AdoptWorkspaceInput identifies an existing local worktree and an already
// available base revision. Adoption never fetches or changes the worktree.
type AdoptWorkspaceInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Path            string `json:"path" jsonschema:"Existing local worktree root"`
	BaseRef         string `json:"base_ref" jsonschema:"Base ref already available in the repository"`
	Name            string `json:"name,omitempty" jsonschema:"Workspace name; defaults to a generated ID"`
}

// AdoptWorkspaceOutput deliberately omits host paths and remote URLs.
type AdoptWorkspaceOutput struct {
	ID              string `json:"id" jsonschema:"Workspace ID"`
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Owner           string `json:"owner" jsonschema:"Repository owner"`
	Repo            string `json:"repo" jsonschema:"Repository name"`
	BaseSHA         string `json:"base_sha" jsonschema:"Resolved base commit"`
	CandidateSHA    string `json:"candidate_sha" jsonschema:"Worktree HEAD observed during adoption"`
	MergeBase       string `json:"merge_base" jsonschema:"Merge base of base and candidate commits"`
	Dirty           bool   `json:"dirty" jsonschema:"Whether tracked or untracked changes were observed"`
	HasUntracked    bool   `json:"has_untracked" jsonschema:"Whether untracked non-ignored files were observed"`
	Ownership       string `json:"ownership" jsonschema:"Workspace ownership classification"`
}

package mcpcontract

const SourceAuditWorkflowVersion = "gitcontribute.source-audit.v1"

type GetSourceAuditWorkflowInput struct{}

type WorkflowAuthority struct {
	Network          bool `json:"network"`
	LocalWrite       bool `json:"local_write"`
	ProcessExecution bool `json:"process_execution"`
}

type WorkflowTransition struct {
	ID                  string            `json:"id"`
	Operation           string            `json:"operation"`
	RequiredInputToken  string            `json:"required_input_token,omitempty"`
	ExpectedResultType  string            `json:"expected_result_type"`
	AllowedNextActions  []string          `json:"allowed_next_actions"`
	Retryable           bool              `json:"retryable"`
	Authority           WorkflowAuthority `json:"authority"`
	IncompleteSemantics string            `json:"incomplete_semantics"`
}

type SourceAuditWorkflow struct {
	Version     string               `json:"version"`
	Transitions []WorkflowTransition `json:"transitions"`
}

func CanonicalSourceAuditWorkflow() SourceAuditWorkflow {
	return SourceAuditWorkflow{Version: SourceAuditWorkflowVersion, Transitions: []WorkflowTransition{
		{ID: "coverage", Operation: ToolGetCoverage, ExpectedResultType: "coverage_result", AllowedNextActions: []string{ToolEnsureCoverage, ToolSyncThreads, ToolHydrateThreads}, Authority: WorkflowAuthority{}, IncompleteSemantics: "Missing facets are unknown, never negative evidence; follow the item-level typed recovery action."},
		{ID: "ensure_coverage", Operation: ToolEnsureCoverage, ExpectedResultType: "job_reference", AllowedNextActions: []string{ToolGetJob}, Retryable: true, Authority: WorkflowAuthority{Network: true, LocalWrite: true}, IncompleteSemantics: "Unknown and incomplete facets remain explicit per stage."},
		{ID: "jobs_get", Operation: ToolGetJob, RequiredInputToken: "job_id", ExpectedResultType: "job_status", AllowedNextActions: []string{ToolGetCoverage, ToolGetThreads, ToolGetThreadFacets}, Retryable: true, Authority: WorkflowAuthority{}, IncompleteSemantics: "Only succeeded jobs may hand off a snapshot token; reread through an advertised offline corpus tool."},
		{ID: "offline_reread", Operation: ToolGetThreads, RequiredInputToken: "snapshot_token", ExpectedResultType: "snapshot_bound_result", AllowedNextActions: []string{ToolFindClusters, ToolFindNeighbors, ToolFindPrecedents}, Authority: WorkflowAuthority{}, IncompleteSemantics: "Unavailable tokens fail; they never fall back to current projections."},
		{ID: "duplicate_checks", Operation: ToolFindClusters, RequiredInputToken: "snapshot_token", ExpectedResultType: "duplicate_result", AllowedNextActions: []string{ToolFindNeighbors, ToolFindPrecedents, ToolSyncThreads}, Authority: WorkflowAuthority{}, IncompleteSemantics: "Missing coverage cannot prove absence."},
		{ID: "live_verification", Operation: ToolSyncThreads, ExpectedResultType: "job_reference", AllowedNextActions: []string{ToolGetJob}, Retryable: true, Authority: WorkflowAuthority{Network: true, LocalWrite: true}, IncompleteSemantics: "Live facts remain provenance-separated from offline evidence and must be polled before use."},
		{ID: "verification_jobs_get", Operation: ToolGetJob, RequiredInputToken: "job_id", ExpectedResultType: "job_status", AllowedNextActions: []string{ToolAttachValidationReceipt}, Retryable: true, Authority: WorkflowAuthority{}, IncompleteSemantics: "Only a terminal verification job may be represented by an external receipt."},
		{ID: "receipt_attachment", Operation: ToolAttachValidationReceipt, RequiredInputToken: "receipt_json", ExpectedResultType: "evidence_receipt", AllowedNextActions: []string{ToolPrepareContribution}, Authority: WorkflowAuthority{LocalWrite: true}, IncompleteSemantics: "Receipt limitations and incomplete status are preserved."},
		{ID: "handoff", Operation: ToolPrepareContribution, RequiredInputToken: "opportunity_id", ExpectedResultType: "draft_result", AllowedNextActions: []string{}, Authority: WorkflowAuthority{LocalWrite: true}, IncompleteSemantics: "Final handoff reports all known limitations."},
	}}
}

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
		{ID: "coverage", Operation: ToolGetCoverage, ExpectedResultType: "coverage_result", AllowedNextActions: []string{ToolEnsureCoverage}, Authority: WorkflowAuthority{}, IncompleteSemantics: "Missing facets are unknown, never negative evidence."},
		{ID: "ensure_coverage", Operation: ToolEnsureCoverage, ExpectedResultType: "coverage_job", AllowedNextActions: []string{ToolGetJob}, Retryable: true, Authority: WorkflowAuthority{Network: true, LocalWrite: true}, IncompleteSemantics: "Unknown and incomplete facets remain explicit per stage."},
		{ID: "jobs_get", Operation: ToolGetJob, RequiredInputToken: "job_id", ExpectedResultType: "job_status", AllowedNextActions: []string{"read_snapshot"}, Retryable: true, Authority: WorkflowAuthority{}, IncompleteSemantics: "Only succeeded jobs may hand off a snapshot token."},
		{ID: "offline_reread", Operation: "read_snapshot", RequiredInputToken: "snapshot_token", ExpectedResultType: "snapshot_bound_result", AllowedNextActions: []string{"duplicate_checks"}, Authority: WorkflowAuthority{}, IncompleteSemantics: "Unavailable tokens fail; they never fall back to current projections."},
		{ID: "duplicate_checks", Operation: "duplicate_checks", RequiredInputToken: "snapshot_token", ExpectedResultType: "duplicate_result", AllowedNextActions: []string{"live_verification"}, Authority: WorkflowAuthority{}, IncompleteSemantics: "Missing coverage cannot prove absence."},
		{ID: "live_verification", Operation: "live_verification", ExpectedResultType: "live_verification_result", AllowedNextActions: []string{ToolAttachValidationReceipt}, Retryable: true, Authority: WorkflowAuthority{Network: true}, IncompleteSemantics: "Live facts remain provenance-separated from offline evidence."},
		{ID: "receipt_attachment", Operation: ToolAttachValidationReceipt, RequiredInputToken: "receipt_digest", ExpectedResultType: "evidence_receipt", AllowedNextActions: []string{"evidence_or_draft_handoff"}, Authority: WorkflowAuthority{LocalWrite: true}, IncompleteSemantics: "Receipt limitations and incomplete status are preserved."},
		{ID: "handoff", Operation: "evidence_or_draft_handoff", RequiredInputToken: "snapshot_token", ExpectedResultType: "source_backed_handoff", AllowedNextActions: []string{}, Authority: WorkflowAuthority{LocalWrite: true}, IncompleteSemantics: "Final handoff reports all known limitations."},
	}}
}

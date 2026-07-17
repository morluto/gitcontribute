package cli

// EvidenceResult is the evidence packet for an investigation.
type EvidenceResult struct {
	InvestigationID string         `json:"investigation_id"`
	Evidence        []EvidenceItem `json:"evidence"`
}

// EvidenceItem is a single piece of evidence with derived corpus freshness.
type EvidenceItem struct {
	ID               string                         `json:"id"`
	Type             string                         `json:"type"`
	Relation         string                         `json:"relation"`
	Description      string                         `json:"description"`
	ValidationRunID  string                         `json:"validation_run_id,omitempty"`
	OpportunityID    string                         `json:"opportunity_id,omitempty"`
	SourceRefs       []WorkflowSourceRefResult      `json:"source_refs,omitempty"`
	SourceProvenance []EvidenceSourceRevisionResult `json:"source_provenance,omitempty"`
	Freshness        string                         `json:"freshness"`
	FreshnessReason  string                         `json:"freshness_reason,omitempty"`
	CreatedAt        string                         `json:"created_at"`
}

// EvidenceSourceSubjectResult identifies the independently refreshed corpus
// projection used by an evidence item.
type EvidenceSourceSubjectResult struct {
	Kind       string `json:"kind"`
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	ThreadKind string `json:"thread_kind,omitempty"`
	Number     int    `json:"number,omitempty"`
	Facet      string `json:"facet,omitempty"`
}

// EvidenceSourceRevisionResult is the portable recorded source order.
type EvidenceSourceRevisionResult struct {
	Subject             EvidenceSourceSubjectResult `json:"subject"`
	SourceUpdatedAt     string                      `json:"source_updated_at,omitempty"`
	ObservationSequence int64                       `json:"observation_sequence"`
	ObservedAt          string                      `json:"observed_at"`
}

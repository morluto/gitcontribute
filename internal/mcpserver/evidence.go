package mcpserver

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

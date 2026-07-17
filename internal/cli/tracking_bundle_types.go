package cli

import "encoding/json"

type MetadataExportOptions struct {
	Limit int
}

type MetadataExportResult struct {
	SchemaVersion        int             `json:"schema_version"`
	Data                 json.RawMessage `json:"data"`
	TriageEvents         int             `json:"triage_events"`
	Contributions        int             `json:"contributions"`
	ContributionOutcomes int             `json:"contribution_outcomes"`
	Evidence             int             `json:"evidence"`
}

type MetadataImportOptions struct {
	Data []byte
}

type MetadataImportResult struct {
	SchemaVersion        int `json:"schema_version"`
	TriageEvents         int `json:"triage_events"`
	Contributions        int `json:"contributions"`
	ContributionOutcomes int `json:"contribution_outcomes"`
	Evidence             int `json:"evidence"`
}

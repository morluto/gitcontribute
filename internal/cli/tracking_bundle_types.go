package cli

import "encoding/json"

// MetadataExportOptions bounds a local tracking metadata export.
type MetadataExportOptions struct {
	Limit int
}

// MetadataExportResult contains the exported tracking bundle and record counts.
type MetadataExportResult struct {
	SchemaVersion        int             `json:"schema_version"`
	Data                 json.RawMessage `json:"data"`
	TriageEvents         int             `json:"triage_events"`
	Contributions        int             `json:"contributions"`
	ContributionOutcomes int             `json:"contribution_outcomes"`
	Evidence             int             `json:"evidence"`
}

// MetadataImportOptions carries a serialized local tracking bundle.
type MetadataImportOptions struct {
	Data []byte
}

// MetadataImportResult reports the imported bundle version and record counts.
type MetadataImportResult struct {
	SchemaVersion        int `json:"schema_version"`
	TriageEvents         int `json:"triage_events"`
	Contributions        int `json:"contributions"`
	ContributionOutcomes int `json:"contribution_outcomes"`
	Evidence             int `json:"evidence"`
}

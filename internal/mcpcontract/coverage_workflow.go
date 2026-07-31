package mcpcontract

type EnsureCoverageInput struct {
	Target             CoverageTarget `json:"target"`
	Facets             []string       `json:"facets,omitempty" jsonschema:"Selected facet names required by the caller"`
	MaxRequests        int            `json:"max_requests,omitempty" jsonschema:"Maximum GitHub requests from 1 to 1000"`
	MaxPages           int            `json:"max_pages,omitempty" jsonschema:"Maximum pages per selected facet from 1 to 100"`
	LimitPerRepository int            `json:"limit_per_repository,omitempty" jsonschema:"Maximum thread headers in repository mode from 1 to 1000"`
}

type CoverageStageOutcome struct {
	Stage      string         `json:"stage"`
	Status     string         `json:"status"`
	Message    string         `json:"message,omitempty"`
	Retryable  bool           `json:"retryable"`
	RetryAfter NonNegativeInt `json:"retry_after_ms,omitempty"`
}

type EnsureCoverageJobResult struct {
	Status          string                 `json:"status"`
	CoverageBefore  *CoverageOutput        `json:"coverage_before,omitempty"`
	PlannedStages   []string               `json:"planned_stages"`
	CompletedStages []string               `json:"completed_stages"`
	StageOutcomes   []CoverageStageOutcome `json:"stage_outcomes"`
	NextAction      FollowUpAction         `json:"next_action"`
	RetryAfterMS    NonNegativeInt         `json:"retry_after_ms,omitempty"`
	SnapshotToken   string                 `json:"snapshot_token,omitempty"`
	ArtifactDigest  string                 `json:"artifact_digest,omitempty"`
	Unknown         bool                   `json:"unknown"`
	Incomplete      bool                   `json:"incomplete"`
}

type CorpusSnapshotArtifact struct {
	SnapshotToken        string `json:"snapshot_token"`
	ContractVersion      string `json:"contract_version"`
	ObservationWatermark int64  `json:"observation_watermark"`
	Scope                any    `json:"scope"`
	SourceManifestSHA256 string `json:"source_manifest_sha256"`
	DerivedVersions      any    `json:"derived_versions"`
	Completeness         any    `json:"completeness"`
	Provenance           any    `json:"provenance"`
	ArtifactKind         string `json:"artifact_kind"`
	ArtifactDigest       string `json:"artifact_digest"`
	Payload              any    `json:"payload"`
	CreatedAt            string `json:"created_at"`
}

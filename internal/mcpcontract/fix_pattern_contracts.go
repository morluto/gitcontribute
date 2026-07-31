package mcpcontract

const (
	DefaultFixPatternCandidateLimit      = 100
	DefaultFixPatternHydrationLimit      = 25
	DefaultFixPatternRepresentativeLimit = 5
)

// FixPatternOutcome is a pull-request disposition. Merge state comes from
// GitHub; superseded requires an explicit replacement relationship.
type FixPatternOutcome string

// FixPatternRelationship describes evidence connecting an issue and pull
// request. Similarity is intentionally distinct from an explicit link.
type FixPatternRelationship string

// FixPatternReportStatus describes whether all bounded workflow evidence is
// complete or whether coverage limits or failures remain.
type FixPatternReportStatus string

// FixPatternProofStyle is a bounded evidence style detected in stored PR text.
type FixPatternProofStyle string

// FixPatternRelatedKind identifies the stored thread kind of a related target.
type FixPatternRelatedKind string

// FixPatternTimeWindow bounds stored thread observations considered by a
// repository pattern-mining workflow.
type FixPatternTimeWindow struct {
	UpdatedAfter  string `json:"updated_after" jsonschema:"Required RFC 3339 inclusive lower bound"`
	UpdatedBefore string `json:"updated_before,omitempty" jsonschema:"Optional RFC 3339 inclusive upper bound"`
}

// FixPatternSymptom defines one caller-owned symptom category using literal
// search terms. Terms use any-term matching within the category.
type FixPatternSymptom struct {
	Name  string   `json:"name" jsonschema:"Stable human-readable category name"`
	Terms []string `json:"terms" jsonschema:"One to 12 literal stored-thread search terms"`
}

// MineRepositoryFixPatternsInput requests a bounded network-assisted analysis
// of accepted and rejected pull-request patterns in one stored repository.
type MineRepositoryFixPatternsInput struct {
	Repository          RepositoryRef        `json:"repository" jsonschema:"Stored GitHub repository whose pull-request history should be analyzed"`
	TimeWindow          FixPatternTimeWindow `json:"time_window" jsonschema:"Inclusive stored-observation window used to select pull requests"`
	SymptomTaxonomy     []FixPatternSymptom  `json:"symptom_taxonomy" jsonschema:"One to 12 caller-defined symptom categories"`
	MergeOutcomes       []FixPatternOutcome  `json:"merge_outcomes,omitempty" jsonschema:"Outcomes whose representative examples should be returned"`
	CandidateLimit      int                  `json:"candidate_limit,omitempty" jsonschema:"Maximum stored candidates examined per symptom from 1 to 100"`
	HydrationLimit      *int                 `json:"hydration_limit,omitempty" jsonschema:"Maximum unknown-state pull requests refreshed from GitHub from 0 to 100; defaults to 25"`
	RepresentativeLimit int                  `json:"representative_limit,omitempty" jsonschema:"Maximum examples returned per symptom from 1 to 20"`
	CorpusRevision      *int64               `json:"corpus_revision,omitempty" jsonschema:"Optional corpus revision pin from a previous offline read"`
}

// PreviewRepositoryFixPatternsInput is the read-only counterpart to the
// durable mining operation. It never creates a job or persists a report.
type PreviewRepositoryFixPatternsInput MineRepositoryFixPatternsInput

// FixPatternOutcomeCounts preserves unknown state rather than treating it as
// unmerged.
type FixPatternOutcomeCounts struct {
	Merged         NonNegativeInt `json:"merged"`
	ClosedUnmerged NonNegativeInt `json:"closed_unmerged"`
	Superseded     NonNegativeInt `json:"superseded"`
	Open           NonNegativeInt `json:"open"`
	Unknown        NonNegativeInt `json:"unknown"`
}

// FixPatternCoverage reports the bounded universe examined and the effect of
// finalist hydration.
type FixPatternCoverage struct {
	CandidateMatches     NonNegativeInt `json:"candidate_matches"`
	UniqueCandidates     NonNegativeInt `json:"unique_candidates"`
	UnknownBefore        NonNegativeInt `json:"unknown_before"`
	SelectedForHydration NonNegativeInt `json:"selected_for_hydration"`
	Hydrated             NonNegativeInt `json:"hydrated"`
	HydrationFailed      NonNegativeInt `json:"hydration_failed"`
	UnknownAfter         NonNegativeInt `json:"unknown_after"`
	CandidateTruncated   bool           `json:"candidate_truncated"`
}

// FixPatternExample is one bounded, source-backed representative.
type FixPatternExample struct {
	PullRequest          ThreadRef              `json:"pull_request"`
	RelatedThread        *ThreadRef             `json:"related_thread,omitempty"`
	RelatedKind          FixPatternRelatedKind  `json:"related_kind,omitempty"`
	Title                string                 `json:"title"`
	Outcome              FixPatternOutcome      `json:"outcome"`
	Relationship         FixPatternRelationship `json:"relationship"`
	RelationshipEvidence string                 `json:"relationship_evidence,omitempty"`
	AcceptedFix          bool                   `json:"accepted_fix"`
	ProofStyles          []FixPatternProofStyle `json:"proof_styles,omitempty"`
	UpdatedAt            string                 `json:"updated_at"`
}

// FixPatternCluster summarizes one caller-defined symptom category.
type FixPatternCluster struct {
	Name              string                  `json:"name"`
	Terms             []string                `json:"terms"`
	CandidateCount    NonNegativeInt          `json:"candidate_count"`
	UnknownBefore     NonNegativeInt          `json:"unknown_before"`
	UnknownAfter      NonNegativeInt          `json:"unknown_after"`
	Outcomes          FixPatternOutcomeCounts `json:"outcomes"`
	Examples          []FixPatternExample     `json:"examples"`
	ExamplesTruncated bool                    `json:"examples_truncated"`
}

// FixPatternHydrationFailure records one finalist that could not be refreshed.
type FixPatternHydrationFailure struct {
	PullRequest ThreadRef `json:"pull_request"`
	Reason      string    `json:"reason"`
	Message     string    `json:"message"`
	Retryable   bool      `json:"retryable"`
}

// FixPatternReport is persisted as the typed result of a durable pattern
// mining job and is readable through an MCP resource.
type FixPatternReport struct {
	Status                    FixPatternReportStatus       `json:"status"`
	Repository                RepositoryRef                `json:"repository"`
	TimeWindow                FixPatternTimeWindow         `json:"time_window"`
	GeneratedAt               string                       `json:"generated_at"`
	Coverage                  FixPatternCoverage           `json:"coverage"`
	Clusters                  []FixPatternCluster          `json:"clusters"`
	Failures                  []FixPatternHydrationFailure `json:"failures,omitempty"`
	Limitations               []string                     `json:"limitations,omitempty"`
	Persisted                 bool                         `json:"persisted"`
	CorpusRevision            int64                        `json:"corpus_revision"`
	SnapshotToken             string                       `json:"snapshot_token"`
	ArtifactDigest            string                       `json:"artifact_digest,omitempty"`
	ObservationWatermark      int64                        `json:"observation_watermark"`
	QueryDigestSHA256         string                       `json:"query_digest_sha256"`
	Complete                  bool                         `json:"complete"`
	Truncated                 bool                         `json:"truncated"`
	UnknownCoverage           bool                         `json:"unknown_coverage"`
	ExternalContextProvenance []string                     `json:"external_context_provenance,omitempty"`
}

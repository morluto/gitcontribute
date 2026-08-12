package mcpcontract

const (
	DefaultFixPatternCandidateLimit      = 100
	DefaultFixPatternRepresentativeLimit = 5
)

type FixPatternOutcome string

const (
	FixPatternMerged         FixPatternOutcome = "merged"
	FixPatternClosedUnmerged FixPatternOutcome = "closed_unmerged"
	FixPatternSuperseded     FixPatternOutcome = "superseded"
	FixPatternOpen           FixPatternOutcome = "open"
	FixPatternUnknown        FixPatternOutcome = "unknown"
)

type FixPatternRelationship string

const (
	FixPatternCloses              FixPatternRelationship = "closes"
	FixPatternReferences          FixPatternRelationship = "references"
	FixPatternExplicitReplacement FixPatternRelationship = "explicit_replacement"
	FixPatternSimilarityOnly      FixPatternRelationship = "similarity_only"
)

type FixPatternProofStyle string

const (
	FixPatternRegressionTest FixPatternProofStyle = "regression_test"
	FixPatternReproduction   FixPatternProofStyle = "reproduction"
	FixPatternBenchmark      FixPatternProofStyle = "benchmark"
	FixPatternBeforeAfter    FixPatternProofStyle = "before_after"
	FixPatternScreenshot     FixPatternProofStyle = "screenshot"
)

type FixPatternTimeWindow struct {
	UpdatedAfter  string `json:"updated_after" jsonschema:"Required RFC 3339 inclusive lower bound"`
	UpdatedBefore string `json:"updated_before,omitempty" jsonschema:"Optional RFC 3339 inclusive upper bound"`
}

type FixPatternSymptom struct {
	Name  string   `json:"name" jsonschema:"Stable human-readable category name"`
	Terms []string `json:"terms" jsonschema:"One to 12 literal stored-thread search terms"`
}

// AnalyzeFixPatternsInput selects a bounded offline repository-history
// analysis. Missing source coverage is never hydrated implicitly.
type AnalyzeFixPatternsInput struct {
	Repository          RepositoryRef        `json:"repository" jsonschema:"Stored repository whose pull-request history should be analyzed"`
	TimeWindow          FixPatternTimeWindow `json:"time_window" jsonschema:"Inclusive stored-observation window"`
	SymptomTaxonomy     []FixPatternSymptom  `json:"symptom_taxonomy" jsonschema:"One to 12 caller-defined symptom categories"`
	MergeOutcomes       []FixPatternOutcome  `json:"merge_outcomes,omitempty" jsonschema:"Outcomes whose representative examples should be returned"`
	CandidateLimit      int                  `json:"candidate_limit,omitempty" jsonschema:"Maximum stored candidates examined per symptom from 1 to 100"`
	RepresentativeLimit int                  `json:"representative_limit,omitempty" jsonschema:"Maximum examples returned per symptom from 1 to 20"`
	SnapshotToken       string               `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token"`
}

type FixPatternOutcomeCounts struct {
	Merged         NonNegativeInt `json:"merged"`
	ClosedUnmerged NonNegativeInt `json:"closed_unmerged"`
	Superseded     NonNegativeInt `json:"superseded"`
	Open           NonNegativeInt `json:"open"`
	Unknown        NonNegativeInt `json:"unknown"`
}

type FixPatternCoverage struct {
	CandidateMatches     NonNegativeInt `json:"candidate_matches"`
	UniqueCandidates     NonNegativeInt `json:"unique_candidates"`
	UnknownOutcomes      NonNegativeInt `json:"unknown_outcomes"`
	UnknownPullRequests  []ThreadRef    `json:"unknown_pull_requests,omitempty" jsonschema:"Bounded exact pull requests whose merge outcome requires explicit detail acquisition"`
	UnknownRefsTruncated bool           `json:"unknown_refs_truncated" jsonschema:"Whether more unknown-outcome pull requests exist than the exact recovery batch can carry"`
	CandidateTruncated   bool           `json:"candidate_truncated"`
	HistoryStatus        string         `json:"history_status" jsonschema:"complete, partial, or unknown"`
}

type FixPatternExample struct {
	PullRequest          ThreadRef              `json:"pull_request"`
	RelatedThread        *ThreadRef             `json:"related_thread,omitempty"`
	Title                string                 `json:"title"`
	Outcome              FixPatternOutcome      `json:"outcome"`
	Relationship         FixPatternRelationship `json:"relationship"`
	RelationshipEvidence string                 `json:"relationship_evidence,omitempty"`
	AcceptedFix          bool                   `json:"accepted_fix"`
	ProofStyles          []FixPatternProofStyle `json:"proof_styles,omitempty"`
	UpdatedAt            string                 `json:"updated_at"`
}

type FixPatternCluster struct {
	Name              string                  `json:"name"`
	Terms             []string                `json:"terms"`
	CandidateCount    NonNegativeInt          `json:"candidate_count"`
	Outcomes          FixPatternOutcomeCounts `json:"outcomes"`
	Examples          []FixPatternExample     `json:"examples"`
	ExamplesTruncated bool                    `json:"examples_truncated"`
}

type AnalyzeFixPatternsOutput struct {
	Status          string               `json:"status" jsonschema:"complete or partial"`
	Repository      RepositoryRef        `json:"repository"`
	TimeWindow      FixPatternTimeWindow `json:"time_window"`
	Coverage        FixPatternCoverage   `json:"coverage"`
	Clusters        []FixPatternCluster  `json:"clusters"`
	Limitations     []string             `json:"limitations"`
	SnapshotToken   string               `json:"snapshot_token"`
	Truncated       bool                 `json:"truncated"`
	UnknownCoverage bool                 `json:"unknown_coverage"`
	Recovery        *RecoveryPlan        `json:"recovery,omitempty"`
	Provenance      CorpusReadProvenance `json:"provenance"`
}

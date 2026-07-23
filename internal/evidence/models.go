package evidence

import (
	"context"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

// EvidenceType names the kind of proof being recorded.
type EvidenceType string

const (
	EvidenceTypeBaseFailingRegression      EvidenceType = "base_failing_regression"
	EvidenceTypeCandidatePassingRegression EvidenceType = "candidate_passing_regression"
	EvidenceTypeMinimalReproduction        EvidenceType = "minimal_reproduction"
	EvidenceTypeBenchmark                  EvidenceType = "benchmark"
	EvidenceTypeProfiler                   EvidenceType = "profiler"
	EvidenceTypeInvariantViolation         EvidenceType = "invariant_violation"
	EvidenceTypeCompatibilityMatrix        EvidenceType = "compatibility_matrix"
	EvidenceTypeStaticAnalysis             EvidenceType = "static_analysis"
	EvidenceTypeManualObservation          EvidenceType = "manual_observation"
	EvidenceTypeGitHubSource               EvidenceType = "github_source"
)

// Relation describes how the evidence affects a hypothesis or opportunity.
type Relation string

const (
	RelationSupporting    Relation = "supporting"
	RelationContradicting Relation = "contradicting"
	RelationInconclusive  Relation = "inconclusive"
	RelationStale         Relation = "stale"
	RelationInvalid       Relation = "invalid"
)

// RunKind distinguishes a validation run against the base or candidate branch.
type RunKind string

const (
	RunKindBase      RunKind = "base"
	RunKindCandidate RunKind = "candidate"
)

// RunClassification is the high-level outcome of a single validation run.
type RunClassification string

const (
	RunClassificationPassing   RunClassification = "passing"
	RunClassificationFailing   RunClassification = "failing"
	RunClassificationError     RunClassification = "error"
	RunClassificationCancelled RunClassification = "cancelled"
)

// RunRequest is a shell-free command execution request.
type RunRequest struct {
	Args             []string
	Dir              string
	Env              []string
	MaxOutputBytes   int64
	SampleInterval   time.Duration
	ReadinessTimeout time.Duration
}

// RunResult is the captured output of one command execution.
type RunResult struct {
	ExitCode       int
	Stdout         string
	Stderr         string
	Truncated      bool
	StartedAt      time.Time
	CompletedAt    time.Time
	Error          string
	Classification RunClassification
	Process        ProcessIdentity
	Phases         RunPhases
	TimeoutPhase   string
	FailurePhase   string
	Resources      ResourceTelemetry
	Cleanup        CleanupResult
}

// ProcessIdentity prevents PID reuse from merging unrelated process samples.
type ProcessIdentity struct {
	PID                 int32
	CreateTimeUnixMilli int64
}

// RunPhases records generic process boundaries. Protocol readiness milestones
// require a declared adapter and are never inferred from command output.
type RunPhases struct {
	SpawnStartedAt    time.Time
	ProcessStartedAt  time.Time
	InitializedAt     time.Time
	ToolsListedAt     time.Time
	FirstResponseAt   time.Time
	ExecutionEndedAt  time.Time
	ShutdownStartedAt time.Time
	ShutdownCheckedAt time.Time
}

// Int64Metric represents a sampled value. Nil means unavailable, never zero.
type Int64Metric struct {
	Value             *int64
	UnavailableReason string
}

// Uint64Metric represents a sampled unsigned value.
type Uint64Metric struct {
	Value             *uint64
	UnavailableReason string
}

// ResourceTelemetry contains bounded process-tree high-water marks.
type ResourceTelemetry struct {
	Provider                   string
	Platform                   string
	SampleInterval             time.Duration
	SampleCount                int
	CPUTimeMillis              Int64Metric
	PeakRSSBytes               Uint64Metric
	PeakChildCount             Int64Metric
	SamplerOverheadNanoseconds int64
}

// CleanupResult records whether sampled descendants survived the shutdown
// boundary. Survivors are matched by PID and creation time.
type CleanupResult struct {
	Status    string
	Reason    string
	Survivors []ProcessIdentity
	CheckedAt time.Time
}

// Runner executes an explicit argv inside a workspace directory without a shell.
type Runner interface {
	Run(ctx context.Context, req RunRequest) (*RunResult, error)
}

// ObservationSource selects captured command output to inspect.
type ObservationSource string

const (
	// ObservationStdout inspects captured standard output.
	ObservationStdout ObservationSource = "stdout"
	// ObservationStderr inspects captured standard error.
	ObservationStderr ObservationSource = "stderr"
	// ObservationArtifact inspects a declared workspace artifact.
	ObservationArtifact ObservationSource = "artifact"
)

// ObservationMatcher selects how captured output is inspected.
type ObservationMatcher string

const (
	// ObservationExact performs literal substring matching.
	ObservationExact ObservationMatcher = "exact"
	// ObservationRegexp performs regular-expression matching.
	ObservationRegexp ObservationMatcher = "regexp"
)

// ObservationOccurrence declares whether the matcher must be present or absent.
type ObservationOccurrence string

const (
	// ObservationPresent requires a match.
	ObservationPresent ObservationOccurrence = "present"
	// ObservationAbsent requires no match.
	ObservationAbsent ObservationOccurrence = "absent"
)

// ExpectedObservation is one bounded assertion over captured output.
type ExpectedObservation struct {
	Name       string
	Source     ObservationSource
	Matcher    ObservationMatcher
	Pattern    string
	Occurrence ObservationOccurrence
	Path       string
}

// ObservationContract ties validation output to the intended proof.
type ObservationContract struct {
	Intent    string
	Base      []ExpectedObservation
	Candidate []ExpectedObservation
}

// ObservationStatus is the aggregate outcome of a run's output assertions.
type ObservationStatus string

const (
	// ObservationNotEvaluated means no observation contract applied.
	ObservationNotEvaluated ObservationStatus = "not_evaluated"
	// ObservationMatched means every expected observation matched.
	ObservationMatched ObservationStatus = "matched"
	// ObservationMismatched means at least one observation did not match.
	ObservationMismatched ObservationStatus = "mismatched"
)

// ObservationResult records one assertion and a bounded matching excerpt.
type ObservationResult struct {
	ExpectedObservation
	Status  ObservationStatus
	Excerpt string
	Error   string
}

// ValidationDefinition captures an explicit validation command and its workspace.
type ValidationDefinition struct {
	ID                   string
	InvestigationID      string
	HypothesisID         string
	OpportunityID        string
	Name                 string
	Kind                 string
	Command              []string
	WorkingDir           string
	BaseWorkingDir       string
	CandidateDir         string
	WorkspaceID          string
	BaseWorkspaceID      string
	CandidateWorkspaceID string
	Env                  []string // variable names allowed through from the host environment
	Timeout              time.Duration
	MaxOutputBytes       int64
	Observation          *ObservationContract
	Protocol             ValidationProtocol
	ReadinessTimeout     time.Duration
	CreatedAt            time.Time
}

// ValidationProtocol selects an explicit structured adapter. Empty means the
// generic command runner; protocol milestones are never inferred from stdout.
type ValidationProtocol string

const (
	// ValidationProtocolMCPStdio measures initialize and tools/list through the official MCP SDK.
	ValidationProtocolMCPStdio ValidationProtocol = "mcp_stdio"
)

// ValidationRun records the outcome of one execution of a validation definition.
type ValidationRun struct {
	ID                      string
	DefinitionID            string
	InvestigationID         string
	HypothesisID            string
	OpportunityID           string
	Kind                    RunKind
	StartedAt               time.Time
	CompletedAt             time.Time
	ExitCode                int
	Stdout                  string
	Stderr                  string
	Truncated               bool
	Error                   string
	Classification          RunClassification
	ObservationStatus       ObservationStatus
	Observations            []ObservationResult
	WorkspaceSnapshotBefore string
	WorkspaceSnapshotAfter  string
	WorkspaceBindingStatus  string
	WorkspaceBindingReason  string
	Process                 ProcessIdentity
	Phases                  RunPhases
	TimeoutPhase            string
	FailurePhase            string
	Resources               ResourceTelemetry
	Cleanup                 CleanupResult
}

// RunGroupClassification summarizes repeated, semantically comparable runs.
type RunGroupClassification string

const (
	// RunGroupStablePass means every comparable attempt passed.
	RunGroupStablePass RunGroupClassification = "stable_pass"
	// RunGroupStableFail means every comparable attempt failed.
	RunGroupStableFail RunGroupClassification = "stable_fail"
	// RunGroupFlaky means comparable attempts disagreed.
	RunGroupFlaky RunGroupClassification = "flaky"
	// RunGroupInconclusive means attempts could not support a semantic conclusion.
	RunGroupInconclusive RunGroupClassification = "inconclusive"
	// RunGroupCancelled means the requested sample was not completed.
	RunGroupCancelled RunGroupClassification = "cancelled"
)

// RepeatValidationOptions bounds one repeat/stress request.
type RepeatValidationOptions struct {
	Kinds          []RunKind
	RunCount       int
	Concurrency    int
	PerRunTimeout  time.Duration
	OverallTimeout time.Duration
	SampleInterval time.Duration
}

// ValidationAttempt is a bounded summary of one independently timed run. Full
// bounded output remains in the referenced ValidationRun record.
type ValidationAttempt struct {
	Index             int
	Kind              RunKind
	RunID             string
	StartedAt         time.Time
	CompletedAt       time.Time
	ExitCode          int
	Classification    RunClassification
	ObservationStatus ObservationStatus
	TimeoutPhase      string
	FailurePhase      string
	Error             string
	Process           ProcessIdentity
	Phases            RunPhases
	Resources         ResourceTelemetry
	Cleanup           CleanupResult
}

// ValidationAggregate preserves semantic and resource conclusions separately.
type ValidationAggregate struct {
	Kind                   RunKind
	Requested              int
	Completed              int
	Passing                int
	Failing                int
	Inconclusive           int
	Cancelled              int
	Classification         RunGroupClassification
	ResourceClassification string
}

// ValidationGroupComparison compares stable base and candidate aggregates.
type ValidationGroupComparison struct {
	Classification ComparisonClassification
	Explanation    string
}

// ValidationRunGroup is one persisted bounded repeat/stress execution.
type ValidationRunGroup struct {
	ID                  string
	DefinitionID        string
	InvestigationID     string
	HypothesisID        string
	OpportunityID       string
	ConfigurationSHA256 string
	RequestedRuns       int
	CompletedRuns       int
	Concurrency         int
	PerRunTimeout       time.Duration
	OverallTimeout      time.Duration
	SampleInterval      time.Duration
	Attempts            []ValidationAttempt
	Aggregates          []ValidationAggregate
	Classification      RunGroupClassification
	Comparison          *ValidationGroupComparison
	StartedAt           time.Time
	CompletedAt         time.Time
}

// Evidence is a piece of supporting, contradicting, or inconclusive proof.
type Evidence struct {
	ID               string
	InvestigationID  string
	HypothesisID     string
	OpportunityID    string
	ValidationRunID  string
	Type             EvidenceType
	Relation         Relation
	Description      string
	SourceRefs       []domain.SourceRef
	SourceProvenance []SourceRevision
	CreatedAt        time.Time
}

// ComparisonClassification is the result of comparing a base run to a candidate run.
type ComparisonClassification string

const (
	ComparisonFixed        ComparisonClassification = "fixed"
	ComparisonNotFixed     ComparisonClassification = "not_fixed"
	ComparisonRegression   ComparisonClassification = "regression"
	ComparisonNoDifference ComparisonClassification = "no_difference"
	ComparisonInconclusive ComparisonClassification = "inconclusive"
)

// ComparisonResult pairs a base and candidate run with a deterministic classification.
type ComparisonResult struct {
	Base           *ValidationRun
	Candidate      *ValidationRun
	Classification ComparisonClassification
	Explanation    string
}

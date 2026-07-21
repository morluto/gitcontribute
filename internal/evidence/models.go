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
	Args           []string
	Dir            string
	Env            []string
	MaxOutputBytes int64
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
}

// Runner executes an explicit argv inside a workspace directory without a shell.
type Runner interface {
	Run(ctx context.Context, req RunRequest) (*RunResult, error)
}

// ObservationSource selects captured command output to inspect.
type ObservationSource string

const (
	ObservationStdout   ObservationSource = "stdout"
	ObservationStderr   ObservationSource = "stderr"
	ObservationArtifact ObservationSource = "artifact"
)

// ObservationMatcher selects how captured output is inspected.
type ObservationMatcher string

const (
	ObservationExact  ObservationMatcher = "exact"
	ObservationRegexp ObservationMatcher = "regexp"
)

// ObservationOccurrence declares whether the matcher must be present or absent.
type ObservationOccurrence string

const (
	ObservationPresent ObservationOccurrence = "present"
	ObservationAbsent  ObservationOccurrence = "absent"
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
	ObservationNotEvaluated ObservationStatus = "not_evaluated"
	ObservationMatched      ObservationStatus = "matched"
	ObservationMismatched   ObservationStatus = "mismatched"
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
	ID              string
	InvestigationID string
	HypothesisID    string
	OpportunityID   string
	Name            string
	Kind            string
	Command         []string
	WorkingDir      string
	BaseWorkingDir  string
	CandidateDir    string
	Env             []string // variable names allowed through from the host environment
	Timeout         time.Duration
	MaxOutputBytes  int64
	Observation     *ObservationContract
	CreatedAt       time.Time
}

// ValidationRun records the outcome of one execution of a validation definition.
type ValidationRun struct {
	ID                string
	DefinitionID      string
	InvestigationID   string
	HypothesisID      string
	OpportunityID     string
	Kind              RunKind
	StartedAt         time.Time
	CompletedAt       time.Time
	ExitCode          int
	Stdout            string
	Stderr            string
	Truncated         bool
	Error             string
	Classification    RunClassification
	ObservationStatus ObservationStatus
	Observations      []ObservationResult
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

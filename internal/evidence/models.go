package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

// ParseEvidenceType converts an exact boundary value into a supported evidence type.
func ParseEvidenceType(value string) (EvidenceType, error) {
	typeValue := EvidenceType(value)
	if !isValidEvidenceType(typeValue) {
		return "", fmt.Errorf("%w: %q", ErrInvalidEvidenceType, value)
	}
	return typeValue, nil
}

// Relation describes how the evidence affects a hypothesis or opportunity.
type Relation string

const (
	RelationSupporting    Relation = "supporting"
	RelationContradicting Relation = "contradicting"
	RelationInconclusive  Relation = "inconclusive"
	RelationStale         Relation = "stale"
	RelationInvalid       Relation = "invalid"
)

// ParseRelation converts an exact boundary value into a supported evidence relation.
func ParseRelation(value string) (Relation, error) {
	relation := Relation(value)
	if !isValidRelation(relation) {
		return "", fmt.Errorf("%w: %q", ErrInvalidRelation, value)
	}
	return relation, nil
}

// RunKind distinguishes a validation run against the base or candidate branch.
type RunKind string

const (
	RunKindBase      RunKind = "base"
	RunKindCandidate RunKind = "candidate"
)

// ParseRunKind converts an exact boundary value into a supported validation target.
func ParseRunKind(value string) (RunKind, error) {
	kind := RunKind(value)
	if !validRunKind(kind) {
		return "", ErrMissingRunKind
	}
	return kind, nil
}

// RunClassification is the high-level outcome of a single validation run.
type RunClassification string

const (
	RunClassificationPassing   RunClassification = "passing"
	RunClassificationFailing   RunClassification = "failing"
	RunClassificationError     RunClassification = "error"
	RunClassificationCancelled RunClassification = "cancelled"
)

// ParseRunClassification converts an exact boundary value into a supported run outcome.
func ParseRunClassification(value string) (RunClassification, error) {
	classification := RunClassification(value)
	if !validRunClassification(classification) {
		return "", fmt.Errorf("unsupported external run classification %q", value)
	}
	return classification, nil
}

// RunRequest is a shell-free command execution request.
type RunRequest struct {
	Args             []string
	Dir              string
	Env              []string
	MaxOutputBytes   int64
	SampleInterval   time.Duration
	ReadinessTimeout time.Duration
}

// ValidationPhase identifies a process or protocol boundary associated with a
// failure or timeout. Empty means no such phase was recorded.
type ValidationPhase string

const (
	ValidationPhaseNone      ValidationPhase = ""
	ValidationPhaseStartup   ValidationPhase = "startup"
	ValidationPhaseReadiness ValidationPhase = "readiness"
	ValidationPhaseExecution ValidationPhase = "execution"
	ValidationPhaseShutdown  ValidationPhase = "shutdown"
)

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
	TimeoutPhase   ValidationPhase
	FailurePhase   ValidationPhase
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

// Int64Metric is either a sampled value or an unavailable reason. Its private
// representation prevents both claims from being populated simultaneously.
type Int64Metric struct {
	value             *int64
	unavailableReason string
}

// Uint64Metric is either a sampled unsigned value or an unavailable reason.
type Uint64Metric struct {
	value             *uint64
	unavailableReason string
}

func AvailableInt64Metric(value int64) Int64Metric { return Int64Metric{value: &value} }
func UnavailableInt64Metric(reason string) Int64Metric {
	return Int64Metric{unavailableReason: reason}
}
func (m Int64Metric) Value() (int64, bool) {
	if m.value == nil {
		return 0, false
	}
	return *m.value, true
}
func (m Int64Metric) ValuePointer() *int64 {
	if m.value == nil {
		return nil
	}
	value := *m.value
	return &value
}
func (m Int64Metric) UnavailableReason() string { return m.unavailableReason }

func AvailableUint64Metric(value uint64) Uint64Metric { return Uint64Metric{value: &value} }
func UnavailableUint64Metric(reason string) Uint64Metric {
	return Uint64Metric{unavailableReason: reason}
}
func (m Uint64Metric) Value() (uint64, bool) {
	if m.value == nil {
		return 0, false
	}
	return *m.value, true
}
func (m Uint64Metric) ValuePointer() *uint64 {
	if m.value == nil {
		return nil
	}
	value := *m.value
	return &value
}
func (m Uint64Metric) UnavailableReason() string { return m.unavailableReason }

type int64MetricJSON struct {
	Value             *int64
	UnavailableReason string
}

func (m Int64Metric) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64MetricJSON{Value: m.ValuePointer(), UnavailableReason: m.unavailableReason})
}

func (m *Int64Metric) UnmarshalJSON(data []byte) error {
	var stored int64MetricJSON
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	if stored.Value != nil && stored.UnavailableReason != "" {
		return errors.New("int64 metric cannot be both available and unavailable")
	}
	if stored.Value != nil {
		*m = AvailableInt64Metric(*stored.Value)
	} else {
		*m = UnavailableInt64Metric(stored.UnavailableReason)
	}
	return nil
}

type uint64MetricJSON struct {
	Value             *uint64
	UnavailableReason string
}

func (m Uint64Metric) MarshalJSON() ([]byte, error) {
	return json.Marshal(uint64MetricJSON{Value: m.ValuePointer(), UnavailableReason: m.unavailableReason})
}

func (m *Uint64Metric) UnmarshalJSON(data []byte) error {
	var stored uint64MetricJSON
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	if stored.Value != nil && stored.UnavailableReason != "" {
		return errors.New("uint64 metric cannot be both available and unavailable")
	}
	if stored.Value != nil {
		*m = AvailableUint64Metric(*stored.Value)
	} else {
		*m = UnavailableUint64Metric(stored.UnavailableReason)
	}
	return nil
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
type CleanupStatus string

const (
	CleanupUnknown     CleanupStatus = ""
	CleanupClean       CleanupStatus = "clean"
	CleanupFailed      CleanupStatus = "failed"
	CleanupUnavailable CleanupStatus = "unavailable"
)

type CleanupResult struct {
	Status    CleanupStatus
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

// ExpectedObservationSpec is the untrusted representation parsed at command,
// protocol, and storage boundaries.
type ExpectedObservationSpec struct {
	Name       string
	Source     ObservationSource
	Matcher    ObservationMatcher
	Pattern    string
	Occurrence ObservationOccurrence
	Path       string
}

// ExpectedObservation is one parsed, bounded assertion over captured output.
// Its representation is private so execution never receives an unknown source,
// an invalid matcher, or an artifact observation without a safe relative path.
type ExpectedObservation struct {
	name       string
	source     ObservationSource
	matcher    ObservationMatcher
	pattern    string
	occurrence ObservationOccurrence
	path       string
	compiled   *regexp.Regexp
}

// ObservationContractSpec is the untrusted representation of a proof contract.
type ObservationContractSpec struct {
	Intent    string
	Base      []ExpectedObservationSpec
	Candidate []ExpectedObservationSpec
}

// ObservationContract ties validation output to the intended proof. Contracts
// can only be populated by ParseObservationContract or JSON decoding, both of
// which establish the same invariants.
type ObservationContract struct {
	intent    string
	base      []ExpectedObservation
	candidate []ExpectedObservation
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
	WorkspaceBindingStatus  WorkspaceBindingStatus
	WorkspaceBindingReason  string
	Process                 ProcessIdentity
	Phases                  RunPhases
	TimeoutPhase            ValidationPhase
	FailurePhase            ValidationPhase
	Resources               ResourceTelemetry
	Cleanup                 CleanupResult
	ExecutionOrigin         ExecutionOrigin
	External                *ExternalReceiptProvenance
	JUnitReport             *JUnitReport
}

// WorkspaceBindingStatus describes whether a locally executed run remained
// bound to the same managed workspace identity.
type WorkspaceBindingStatus string

const (
	WorkspaceBindingUnknown      WorkspaceBindingStatus = ""
	WorkspaceBindingUnavailable  WorkspaceBindingStatus = "unavailable"
	WorkspaceBindingIncomplete   WorkspaceBindingStatus = "incomplete"
	WorkspaceBindingChanged      WorkspaceBindingStatus = "changed"
	WorkspaceBindingBound        WorkspaceBindingStatus = "bound"
	WorkspaceBindingStale        WorkspaceBindingStatus = "stale"
	WorkspaceBindingIncompatible WorkspaceBindingStatus = "incompatible"
)

// ExecutionOrigin distinguishes local execution from imported observations.
// Empty is the durable representation of a locally executed run.
type ExecutionOrigin string

const (
	ExecutionOriginLocal    ExecutionOrigin = ""
	ExecutionOriginExternal ExecutionOrigin = "external"
)

// ExternalReceiptProvenance preserves the trust and source boundary of a
// validation observation produced outside GitContribute.
type ExternalReceiptProvenance struct {
	SchemaVersion  string
	Producer       string
	ValidationID   string
	ReceiptSHA256  string
	Repository     string
	Revision       string
	ArtifactSHA256 string
	Provider       string
	ExternalRunID  string
	Command        []string
	WorkingDir     string
	Environment    map[string]string
	Artifacts      map[string]string
	Limitations    []string
	Incomplete     bool
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
	TimeoutPhase      ValidationPhase
	FailurePhase      ValidationPhase
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
	ResourceClassification ResourceClassification
}

// ResourceClassification summarizes whether repeat-run resource evidence is usable.
type ResourceClassification string

const (
	ResourceUnknown       ResourceClassification = ""
	ResourceAvailable     ResourceClassification = "available"
	ResourceCleanupFailed ResourceClassification = "cleanup_failed"
	ResourceInconclusive  ResourceClassification = "inconclusive"
)

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
	ID                   string
	InvestigationID      string
	HypothesisID         string
	OpportunityID        string
	ValidationRunID      string
	Type                 EvidenceType
	Relation             Relation
	Description          string
	SourceRefs           []domain.SourceRef
	SourceProvenance     []SourceRevision
	CreatedAt            time.Time
	ValidationRun        *ValidationRun
	ValidationDefinition *ValidationDefinition
	External             *ExternalEvidenceProvenance
	Measurements         map[string]any
}

// ExternalEvidenceProvenance keeps producer claims separate from local
// execution and records the source identity needed for later freshness review.
type ExternalEvidenceProvenance struct {
	SchemaVersion  string
	Producer       string
	Repository     string
	Revision       string
	ArtifactSHA256 string
	ObservedAt     time.Time
	Environment    map[string]string
	Completeness   ExternalEvidenceCompleteness
	Integrity      ExternalEvidenceIntegrity
	Limitations    []string
	RawSHA256      string
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

// ParseStored parses validation-definition discriminators after
// durable JSON decoding.
func (d *ValidationDefinition) ParseStored() error {
	if d == nil || d.ID == "" {
		return errors.New("validation definition ID is required")
	}
	if d.Protocol != "" && d.Protocol != ValidationProtocolMCPStdio {
		return fmt.Errorf("unsupported validation protocol %q", d.Protocol)
	}
	if d.Observation != nil && d.Observation.intent == "" {
		return errors.New("stored observation contract was not parsed")
	}
	return nil
}

// ParseStored parses validation-run outcomes after durable JSON
// decoding.
func (r *ValidationRun) ParseStored() error {
	if r == nil || r.ID == "" || r.DefinitionID == "" {
		return errors.New("validation run identity is required")
	}
	if !validRunKind(r.Kind) {
		return fmt.Errorf("unsupported validation run kind %q", r.Kind)
	}
	if !validRunClassification(r.Classification) {
		return fmt.Errorf("unsupported validation run classification %q", r.Classification)
	}
	// Empty is the legacy representation of a run with no observation contract.
	if r.ObservationStatus == "" {
		r.ObservationStatus = ObservationNotEvaluated
	}
	if !validObservationStatus(r.ObservationStatus) {
		return fmt.Errorf("unsupported observation status %q", r.ObservationStatus)
	}
	if !validWorkspaceBindingStatus(r.WorkspaceBindingStatus) {
		return fmt.Errorf("unsupported workspace binding status %q", r.WorkspaceBindingStatus)
	}
	if !validExecutionOrigin(r.ExecutionOrigin) {
		return fmt.Errorf("unsupported execution origin %q", r.ExecutionOrigin)
	}
	if !validCleanupStatus(r.Cleanup.Status) {
		return fmt.Errorf("unsupported cleanup status %q", r.Cleanup.Status)
	}
	if !validValidationPhase(r.TimeoutPhase) || !validValidationPhase(r.FailurePhase) {
		return errors.New("validation run has an unsupported failure or timeout phase")
	}
	if r.JUnitReport != nil {
		if err := r.JUnitReport.ParseStored(); err != nil {
			return fmt.Errorf("stored JUnit report: %w", err)
		}
	}
	return nil
}

// ParseStored parses repeat-run classifications after durable JSON
// decoding.
func (g *ValidationRunGroup) ParseStored() error {
	if g == nil || g.ID == "" || g.DefinitionID == "" {
		return errors.New("validation run group identity is required")
	}
	if !validRunGroupClassification(g.Classification) {
		return fmt.Errorf("unsupported validation group classification %q", g.Classification)
	}
	for i := range g.Attempts {
		attempt := &g.Attempts[i]
		if attempt.ObservationStatus == "" {
			attempt.ObservationStatus = ObservationNotEvaluated
		}
		if !validRunKind(attempt.Kind) || !validRunClassification(attempt.Classification) || !validObservationStatus(attempt.ObservationStatus) || !validCleanupStatus(attempt.Cleanup.Status) || !validValidationPhase(attempt.TimeoutPhase) || !validValidationPhase(attempt.FailurePhase) {
			return fmt.Errorf("validation attempt %d has an unsupported discriminator", i)
		}
	}
	for i, aggregate := range g.Aggregates {
		if !validRunKind(aggregate.Kind) || !validRunGroupClassification(aggregate.Classification) || !validResourceClassification(aggregate.ResourceClassification) {
			return fmt.Errorf("validation aggregate %d has an unsupported discriminator", i)
		}
	}
	if g.Comparison != nil && !validComparisonClassification(g.Comparison.Classification) {
		return fmt.Errorf("unsupported validation comparison %q", g.Comparison.Classification)
	}
	return nil
}

// ParseStored parses evidence type and relation claims after
// durable JSON decoding.
func (e *Evidence) ParseStored() error {
	if e == nil || e.ID == "" {
		return errors.New("evidence ID is required")
	}
	if !isValidEvidenceType(e.Type) {
		return fmt.Errorf("unsupported evidence type %q", e.Type)
	}
	if !isValidRelation(e.Relation) {
		return fmt.Errorf("unsupported evidence relation %q", e.Relation)
	}
	if e.ValidationDefinition != nil {
		if err := e.ValidationDefinition.ParseStored(); err != nil {
			return fmt.Errorf("embedded validation definition: %w", err)
		}
	}
	if e.ValidationRun != nil {
		if err := e.ValidationRun.ParseStored(); err != nil {
			return fmt.Errorf("embedded validation run: %w", err)
		}
	}
	if e.External != nil {
		if _, err := ParseExternalEvidenceCompleteness(string(e.External.Completeness)); err != nil {
			return fmt.Errorf("unsupported external evidence completeness %q", e.External.Completeness)
		}
		if e.External.Integrity != ExternalEvidenceVerified && e.External.Integrity != ExternalEvidenceUnverified {
			return fmt.Errorf("unsupported external evidence integrity %q", e.External.Integrity)
		}
	}
	return nil
}

func validRunKind(kind RunKind) bool { return kind == RunKindBase || kind == RunKindCandidate }

func validRunClassification(classification RunClassification) bool {
	switch classification {
	case RunClassificationPassing, RunClassificationFailing, RunClassificationError, RunClassificationCancelled:
		return true
	default:
		return false
	}
}

func validObservationStatus(status ObservationStatus) bool {
	switch status {
	case ObservationNotEvaluated, ObservationMatched, ObservationMismatched:
		return true
	default:
		return false
	}
}

func validCleanupStatus(status CleanupStatus) bool {
	switch status {
	case CleanupUnknown, CleanupClean, CleanupFailed, CleanupUnavailable:
		return true
	default:
		return false
	}
}

func validValidationPhase(phase ValidationPhase) bool {
	switch phase {
	case ValidationPhaseNone, ValidationPhaseStartup, ValidationPhaseReadiness, ValidationPhaseExecution, ValidationPhaseShutdown:
		return true
	default:
		return false
	}
}

func validWorkspaceBindingStatus(status WorkspaceBindingStatus) bool {
	switch status {
	case WorkspaceBindingUnknown, WorkspaceBindingUnavailable, WorkspaceBindingIncomplete,
		WorkspaceBindingChanged, WorkspaceBindingBound, WorkspaceBindingStale, WorkspaceBindingIncompatible:
		return true
	default:
		return false
	}
}

func validExecutionOrigin(origin ExecutionOrigin) bool {
	return origin == ExecutionOriginLocal || origin == ExecutionOriginExternal
}

func validResourceClassification(classification ResourceClassification) bool {
	switch classification {
	case ResourceUnknown, ResourceAvailable, ResourceCleanupFailed, ResourceInconclusive:
		return true
	default:
		return false
	}
}

func validRunGroupClassification(classification RunGroupClassification) bool {
	switch classification {
	case RunGroupStablePass, RunGroupStableFail, RunGroupFlaky, RunGroupInconclusive, RunGroupCancelled:
		return true
	default:
		return false
	}
}

func validComparisonClassification(classification ComparisonClassification) bool {
	switch classification {
	case ComparisonFixed, ComparisonNotFixed, ComparisonRegression, ComparisonNoDifference, ComparisonInconclusive:
		return true
	default:
		return false
	}
}

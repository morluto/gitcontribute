package cli

import (
	"context"
	"time"
)

// ValidationService is the optional validation management capability used by the CLI.
type ValidationService interface {
	DefineValidation(ctx context.Context, investigationID string, opts DefineValidationOptions) (*ValidationResult, error)
	ShowValidation(ctx context.Context, id string) (*ValidationResult, error)
	RunValidation(ctx context.Context, id string, opts RunValidationOptions) (*ValidationRunResult, error)
	RunValidationGroup(ctx context.Context, id string, opts RepeatValidationOptions) (*ValidationRunGroupResult, error)
	CompareValidation(ctx context.Context, baseRunID, candidateRunID string) (*ValidationComparisonResult, error)
}

// RunValidationOptions carries the run target and explicit host-execution authorization.
type RunValidationOptions struct {
	Kind    string
	Execute bool
}

// RepeatValidationOptions bounds an explicitly authorized run group.
type RepeatValidationOptions struct {
	Kinds          []string
	RunCount       int
	Concurrency    int
	PerRunTimeout  time.Duration
	OverallTimeout time.Duration
	SampleInterval time.Duration
	Execute        bool
}

// DefineValidationOptions carries an explicit validation definition.
type DefineValidationOptions struct {
	Kind                 string
	Command              string
	WorkingDir           string
	BaseWorkingDir       string
	CandidateDir         string
	WorkspaceID          string
	BaseWorkspaceID      string
	CandidateWorkspaceID string
	Env                  []string
	Timeout              time.Duration
	MaxOutputBytes       int64
	Observation          *ValidationObservationContract
	Protocol             string
	ReadinessTimeout     time.Duration
}

// ValidationResult is a stored validation definition view.
type ValidationResult struct {
	ID                   string                         `json:"id"`
	InvestigationID      string                         `json:"investigation_id"`
	Kind                 string                         `json:"kind"`
	Command              []string                       `json:"command"`
	WorkingDir           string                         `json:"working_dir"`
	BaseWorkingDir       string                         `json:"base_working_dir,omitempty"`
	CandidateDir         string                         `json:"candidate_dir,omitempty"`
	WorkspaceID          string                         `json:"workspace_id,omitempty"`
	BaseWorkspaceID      string                         `json:"base_workspace_id,omitempty"`
	CandidateWorkspaceID string                         `json:"candidate_workspace_id,omitempty"`
	Env                  []string                       `json:"environment_allowlist,omitempty"`
	Timeout              string                         `json:"timeout,omitempty"`
	MaxOutputBytes       int64                          `json:"max_output_bytes,omitempty"`
	Observation          *ValidationObservationContract `json:"observation,omitempty"`
	Protocol             string                         `json:"protocol,omitempty"`
	ReadinessTimeout     string                         `json:"readiness_timeout,omitempty"`
	CreatedAt            string                         `json:"created_at"`
}

// ValidationRunResult is the captured outcome of one validation run.
type ValidationRunResult struct {
	ID                      string                        `json:"id"`
	DefinitionID            string                        `json:"definition_id"`
	InvestigationID         string                        `json:"investigation_id"`
	Kind                    string                        `json:"kind"`
	ExitCode                int                           `json:"exit_code"`
	Stdout                  string                        `json:"stdout"`
	Stderr                  string                        `json:"stderr"`
	Truncated               bool                          `json:"truncated"`
	Error                   string                        `json:"error,omitempty"`
	Classification          string                        `json:"classification"`
	ObservationStatus       string                        `json:"observation_status"`
	Observations            []ValidationObservationResult `json:"observations,omitempty"`
	StartedAt               string                        `json:"started_at"`
	CompletedAt             string                        `json:"completed_at"`
	WorkspaceSnapshotBefore string                        `json:"workspace_snapshot_before,omitempty"`
	WorkspaceSnapshotAfter  string                        `json:"workspace_snapshot_after,omitempty"`
	WorkspaceBindingStatus  string                        `json:"workspace_binding_status,omitempty"`
	WorkspaceBindingReason  string                        `json:"workspace_binding_reason,omitempty"`
	Process                 ValidationProcessIdentity     `json:"process"`
	Phases                  ValidationRunPhases           `json:"phases"`
	TimeoutPhase            string                        `json:"timeout_phase,omitempty"`
	FailurePhase            string                        `json:"failure_phase,omitempty"`
	Resources               ValidationResourceTelemetry   `json:"resources"`
	Cleanup                 ValidationCleanupResult       `json:"cleanup"`
}

// ValidationProcessIdentity identifies a sampled process without conflating PID reuse.
type ValidationProcessIdentity struct {
	PID                 int32 `json:"pid,omitempty"`
	CreateTimeUnixMilli int64 `json:"create_time_unix_milli,omitempty"`
}

// ValidationRunPhases exposes process and declared protocol milestones.
type ValidationRunPhases struct {
	SpawnStartedAt    string `json:"spawn_started_at,omitempty"`
	ProcessStartedAt  string `json:"process_started_at,omitempty"`
	InitializedAt     string `json:"initialized_at,omitempty"`
	ToolsListedAt     string `json:"tools_listed_at,omitempty"`
	FirstResponseAt   string `json:"first_response_at,omitempty"`
	ExecutionEndedAt  string `json:"execution_ended_at,omitempty"`
	ShutdownStartedAt string `json:"shutdown_started_at,omitempty"`
	ShutdownCheckedAt string `json:"shutdown_checked_at,omitempty"`
}

// ValidationInt64Metric distinguishes an observed zero from unavailable data.
type ValidationInt64Metric struct {
	Value             *int64 `json:"value,omitempty"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// ValidationUint64Metric distinguishes an observed zero from unavailable data.
type ValidationUint64Metric struct {
	Value             *uint64 `json:"value,omitempty"`
	UnavailableReason string  `json:"unavailable_reason,omitempty"`
}

// ValidationResourceTelemetry reports bounded process-tree high-water marks.
type ValidationResourceTelemetry struct {
	Provider                   string                 `json:"provider"`
	Platform                   string                 `json:"platform"`
	SampleInterval             string                 `json:"sample_interval"`
	SampleCount                int                    `json:"sample_count"`
	CPUTimeMillis              ValidationInt64Metric  `json:"cpu_time_millis"`
	PeakRSSBytes               ValidationUint64Metric `json:"peak_rss_bytes"`
	PeakChildCount             ValidationInt64Metric  `json:"peak_child_count"`
	SamplerOverheadNanoseconds int64                  `json:"sampler_overhead_nanoseconds"`
}

// ValidationCleanupResult reports sampled descendants after shutdown.
type ValidationCleanupResult struct {
	Status    string                      `json:"status"`
	Reason    string                      `json:"reason,omitempty"`
	Survivors []ValidationProcessIdentity `json:"survivors,omitempty"`
	CheckedAt string                      `json:"checked_at,omitempty"`
}

// ValidationAttemptResult summarizes one independently timed attempt.
type ValidationAttemptResult struct {
	Index             int                         `json:"index"`
	Kind              string                      `json:"kind"`
	RunID             string                      `json:"run_id,omitempty"`
	StartedAt         string                      `json:"started_at"`
	CompletedAt       string                      `json:"completed_at"`
	ExitCode          int                         `json:"exit_code"`
	Classification    string                      `json:"classification"`
	ObservationStatus string                      `json:"observation_status"`
	TimeoutPhase      string                      `json:"timeout_phase,omitempty"`
	FailurePhase      string                      `json:"failure_phase,omitempty"`
	Error             string                      `json:"error,omitempty"`
	Process           ValidationProcessIdentity   `json:"process"`
	Phases            ValidationRunPhases         `json:"phases"`
	Resources         ValidationResourceTelemetry `json:"resources"`
	Cleanup           ValidationCleanupResult     `json:"cleanup"`
}

// ValidationAggregateResult classifies comparable attempts for one run kind.
type ValidationAggregateResult struct {
	Kind                   string `json:"kind"`
	Requested              int    `json:"requested"`
	Completed              int    `json:"completed"`
	Passing                int    `json:"passing"`
	Failing                int    `json:"failing"`
	Inconclusive           int    `json:"inconclusive"`
	Cancelled              int    `json:"cancelled"`
	Classification         string `json:"classification"`
	ResourceClassification string `json:"resource_classification"`
}

// ValidationGroupComparisonResult compares stable base and candidate aggregates.
type ValidationGroupComparisonResult struct {
	Classification string `json:"classification"`
	Explanation    string `json:"explanation"`
}

// ValidationRunGroupResult is the bounded repeat/stress result returned to clients.
type ValidationRunGroupResult struct {
	ID                  string                           `json:"id"`
	DefinitionID        string                           `json:"definition_id"`
	InvestigationID     string                           `json:"investigation_id"`
	ConfigurationSHA256 string                           `json:"configuration_sha256"`
	RequestedRuns       int                              `json:"requested_runs"`
	CompletedRuns       int                              `json:"completed_runs"`
	Concurrency         int                              `json:"concurrency"`
	PerRunTimeout       string                           `json:"per_run_timeout"`
	OverallTimeout      string                           `json:"overall_timeout"`
	SampleInterval      string                           `json:"sample_interval"`
	Attempts            []ValidationAttemptResult        `json:"attempts"`
	Aggregates          []ValidationAggregateResult      `json:"aggregates"`
	Classification      string                           `json:"classification"`
	Comparison          *ValidationGroupComparisonResult `json:"comparison,omitempty"`
	StartedAt           string                           `json:"started_at"`
	CompletedAt         string                           `json:"completed_at"`
}

// ValidationComparisonResult classifies a base run against a candidate run.
type ValidationComparisonResult struct {
	Base           *ValidationRunResult `json:"base"`
	Candidate      *ValidationRunResult `json:"candidate"`
	Classification string               `json:"classification"`
	Explanation    string               `json:"explanation"`
}

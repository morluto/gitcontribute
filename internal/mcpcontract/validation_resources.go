package mcpcontract

// ValidationGroupResource exposes persisted execution outcomes and exact run
// identities without leaking commands, environment values, or host paths.
type ValidationGroupResource struct {
	ID                  string                      `json:"id"`
	DefinitionID        string                      `json:"definition_id"`
	InvestigationID     string                      `json:"investigation_id"`
	ConfigurationSHA256 string                      `json:"configuration_sha256"`
	RequestedRuns       int                         `json:"requested_runs"`
	CompletedRuns       int                         `json:"completed_runs"`
	Classification      string                      `json:"classification"`
	StartedAt           string                      `json:"started_at"`
	CompletedAt         string                      `json:"completed_at"`
	Attempts            []ValidationAttemptResource `json:"attempts"`
}

type ValidationAttemptResource struct {
	Index             int    `json:"index"`
	Kind              string `json:"kind"`
	RunID             string `json:"run_id"`
	Classification    string `json:"classification"`
	ObservationStatus string `json:"observation_status,omitempty"`
	ExitCode          int    `json:"exit_code"`
	TimeoutPhase      string `json:"timeout_phase,omitempty"`
	FailurePhase      string `json:"failure_phase,omitempty"`
	StartedAt         string `json:"started_at"`
	CompletedAt       string `json:"completed_at"`
}

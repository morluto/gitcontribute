package cli

import "time"

type validationCmd struct {
	Define  defineValidationCmd  `cmd:"" help:"Define a validation"`
	Run     runValidationCmd     `cmd:"" help:"Run a validation definition"`
	Repeat  repeatValidationCmd  `cmd:"" help:"Run a bounded repeat/stress validation"`
	Compare compareValidationCmd `cmd:"" help:"Compare two validation runs"`
}

type defineValidationCmd struct {
	InvestigationID      string        `arg:"" help:"Investigation ID"`
	Kind                 string        `name:"kind" required:"" help:"Validation kind"`
	Command              string        `name:"command" required:"" help:"Command argv as a single string"`
	WorkingDir           string        `name:"working-dir" help:"Working directory for both runs"`
	BaseWorkingDir       string        `name:"base-working-dir" help:"Base workspace directory"`
	CandidateDir         string        `name:"candidate-dir" help:"Candidate workspace directory"`
	WorkspaceID          string        `name:"workspace-id" help:"Managed workspace ID for both runs"`
	BaseWorkspaceID      string        `name:"base-workspace-id" help:"Managed base workspace ID"`
	CandidateWorkspaceID string        `name:"candidate-workspace-id" help:"Managed candidate workspace ID"`
	Env                  []string      `name:"env" help:"Host environment variable names to pass through"`
	Timeout              time.Duration `name:"timeout" help:"Maximum execution time"`
	MaxOutput            int64         `name:"max-output" help:"Maximum captured output bytes per stream"`
	Observation          string        `name:"observation-contract" help:"JSON observation contract for base and candidate output"`
	Protocol             string        `name:"protocol" help:"Structured protocol adapter (mcp_stdio)"`
	ReadinessTimeout     time.Duration `name:"readiness-timeout" help:"Protocol initialization deadline"`
	JSON                 bool          `name:"json" help:"Print the result as JSON"`
}

type runValidationCmd struct {
	ID      string `arg:"" help:"Validation definition ID"`
	Kind    string `name:"kind" required:"" enum:"base,candidate" help:"Run kind"`
	Execute bool   `name:"execute" help:"Authorize execution of the displayed command on the host"`
	JSON    bool   `name:"json" help:"Print the result as JSON"`
}

type repeatValidationCmd struct {
	ID             string        `arg:"" help:"Validation definition ID"`
	Kind           string        `name:"kind" default:"candidate" enum:"base,candidate,both" help:"Run target"`
	Runs           int           `name:"runs" default:"3" help:"Attempts per target (1-100)"`
	Concurrency    int           `name:"concurrency" default:"1" help:"Concurrent attempts (1-16)"`
	PerRunTimeout  time.Duration `name:"per-run-timeout" help:"Per-attempt timeout; defaults to the validation definition"`
	OverallTimeout time.Duration `name:"overall-timeout" help:"Overall group timeout"`
	SampleInterval time.Duration `name:"sample-interval" default:"100ms" help:"Process telemetry interval (10ms-10s)"`
	Execute        bool          `name:"execute" help:"Authorize execution of the displayed command on the host"`
	JSON           bool          `name:"json" help:"Print the result as JSON"`
}

type compareValidationCmd struct {
	BaseRunID      string `arg:"" help:"Base run ID"`
	CandidateRunID string `arg:"" help:"Candidate run ID"`
	JSON           bool   `name:"json" help:"Print the result as JSON"`
}

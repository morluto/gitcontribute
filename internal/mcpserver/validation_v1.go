package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunValidationInput selects a validation definition and explicitly authorizes execution.
type RunValidationInput struct {
	ID      string `json:"id" jsonschema:"Validation definition ID"`
	Kind    string `json:"kind" jsonschema:"Run kind: base or candidate"`
	Execute bool   `json:"execute" jsonschema:"Must be true to authorize host execution"`
}

// RunRepeatedValidationInput configures one bounded repeat/stress job.
type RunRepeatedValidationInput struct {
	ID             string `json:"id" jsonschema:"Validation definition ID"`
	Target         string `json:"target" jsonschema:"Run target: base, candidate, or both"`
	RunCount       int    `json:"run_count,omitempty" jsonschema:"Attempts per target from 1 to 100"`
	Concurrency    int    `json:"concurrency,omitempty" jsonschema:"Concurrent attempts from 1 to 16"`
	PerRunTimeout  string `json:"per_run_timeout,omitempty" jsonschema:"Optional Go duration per attempt"`
	OverallTimeout string `json:"overall_timeout,omitempty" jsonschema:"Optional Go duration for the whole group"`
	SampleInterval string `json:"sample_interval,omitempty" jsonschema:"Process telemetry interval from 10ms to 10s"`
	Execute        bool   `json:"execute" jsonschema:"Must be true to authorize host execution"`
}

// DefineValidationInput records a bounded validation command without executing it.
type DefineValidationInput struct {
	InvestigationID      string                         `json:"investigation_id" jsonschema:"Investigation ID"`
	Kind                 string                         `json:"kind" jsonschema:"Validation kind"`
	Command              string                         `json:"command" jsonschema:"Shell-free command to execute"`
	WorkspaceID          string                         `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID used for both run kinds"`
	BaseWorkspaceID      string                         `json:"base_workspace_id,omitempty" jsonschema:"Managed base workspace ID; requires candidate_workspace_id"`
	CandidateWorkspaceID string                         `json:"candidate_workspace_id,omitempty" jsonschema:"Managed candidate workspace ID; requires base_workspace_id"`
	Env                  []string                       `json:"env,omitempty" jsonschema:"Allowed environment variable names"`
	Timeout              string                         `json:"timeout,omitempty" jsonschema:"Positive Go duration; defaults to 30m"`
	MaxOutputBytes       int64                          `json:"max_output_bytes,omitempty" jsonschema:"Maximum captured bytes per output stream; defaults to 65536"`
	Observation          *ValidationObservationContract `json:"observation,omitempty" jsonschema:"Expected bounded observations over captured base and candidate output"`
	Protocol             string                         `json:"protocol,omitempty" jsonschema:"Structured protocol adapter: mcp_stdio"`
	ReadinessTimeout     string                         `json:"readiness_timeout,omitempty" jsonschema:"Protocol initialization deadline; defaults to 30s"`
}

// ValidationExpectedObservation is one output assertion evaluated without a shell.
type ValidationExpectedObservation struct {
	Run        string `json:"run" jsonschema:"Run kind: base or candidate"`
	Name       string `json:"name" jsonschema:"Short observation name"`
	Source     string `json:"source" jsonschema:"Captured source: stdout, stderr, or artifact"`
	Matcher    string `json:"matcher" jsonschema:"Matcher: exact or regexp"`
	Pattern    string `json:"pattern" jsonschema:"Bounded exact string or Go regular expression"`
	Occurrence string `json:"occurrence,omitempty" jsonschema:"Expected occurrence: present or absent; defaults to present"`
	Path       string `json:"path,omitempty" jsonschema:"Relative artifact path; valid only when source is artifact"`
}

// ValidationObservationContract ties output assertions to the claimed behavior.
type ValidationObservationContract struct {
	Intent       string                          `json:"intent" jsonschema:"Short proof intent or invariant"`
	Observations []ValidationExpectedObservation `json:"observations" jsonschema:"One to eight expected observations for each of base and candidate"`
}

// ValidationOutput is the stable MCP representation of a validation definition.
type ValidationOutput struct {
	ID                   string                         `json:"id"`
	InvestigationID      string                         `json:"investigation_id"`
	Kind                 string                         `json:"kind"`
	Command              []string                       `json:"command"`
	WorkingDir           string                         `json:"working_dir"`
	BaseWorkingDir       string                         `json:"base_working_dir,omitempty"`
	CandidateDir         string                         `json:"candidate_dir,omitempty"`
	WorkspaceID          string                         `json:"workspace_id,omitempty" jsonschema:"Managed workspace ID used for both run kinds"`
	BaseWorkspaceID      string                         `json:"base_workspace_id,omitempty" jsonschema:"Managed base workspace ID"`
	CandidateWorkspaceID string                         `json:"candidate_workspace_id,omitempty" jsonschema:"Managed candidate workspace ID"`
	Env                  []string                       `json:"environment_allowlist,omitempty"`
	Timeout              string                         `json:"timeout,omitempty"`
	MaxOutputBytes       int64                          `json:"max_output_bytes,omitempty"`
	Observation          *ValidationObservationContract `json:"observation,omitempty"`
	Protocol             string                         `json:"protocol,omitempty" jsonschema:"Declared structured protocol adapter"`
	ReadinessTimeout     string                         `json:"readiness_timeout,omitempty" jsonschema:"Protocol initialization deadline"`
	CreatedAt            string                         `json:"created_at"`
}

func (s *Server) runValidation(ctx context.Context, _ *mcp.CallToolRequest, in RunValidationInput) (*mcp.CallToolResult, JobReference, error) {
	id, err := normalizeID("id", in.ID)
	if err != nil {
		return nil, JobReference{}, err
	}
	in.ID = id
	if in.Kind != "base" && in.Kind != "candidate" {
		return nil, JobReference{}, errors.New("kind must be base or candidate")
	}
	if !in.Execute {
		return nil, JobReference{}, errors.New("execute must be true to authorize host command execution")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, JobReference{}, errors.New("validation is not available")
	}
	out, err := operator.RunValidation(ctx, in)
	return nil, out, err
}

func (s *Server) runRepeatedValidation(ctx context.Context, _ *mcp.CallToolRequest, in RunRepeatedValidationInput) (*mcp.CallToolResult, JobReference, error) {
	id, err := normalizeID("id", in.ID)
	if err != nil {
		return nil, JobReference{}, err
	}
	in.ID = id
	if in.RunCount == 0 {
		in.RunCount = 3
	}
	if in.Concurrency == 0 {
		in.Concurrency = 1
	}
	if in.SampleInterval == "" {
		in.SampleInterval = "100ms"
	}
	if in.Target != "base" && in.Target != "candidate" && in.Target != "both" {
		return nil, JobReference{}, errors.New("target must be base, candidate, or both")
	}
	if !in.Execute {
		return nil, JobReference{}, errors.New("execute must be true to authorize host command execution")
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, JobReference{}, errors.New("validation is not available")
	}
	out, err := operator.RunRepeatedValidation(ctx, in)
	return nil, out, err
}

func (s *Server) defineValidation(ctx context.Context, _ *mcp.CallToolRequest, in DefineValidationInput) (*mcp.CallToolResult, ValidationOutput, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, ValidationOutput{}, err
	}
	in.Kind = strings.TrimSpace(in.Kind)
	in.Command = strings.TrimSpace(in.Command)
	in.WorkspaceID = strings.TrimSpace(in.WorkspaceID)
	in.BaseWorkspaceID = strings.TrimSpace(in.BaseWorkspaceID)
	in.CandidateWorkspaceID = strings.TrimSpace(in.CandidateWorkspaceID)
	if in.Kind == "" || in.Command == "" {
		return nil, ValidationOutput{}, InvalidArgument("command", "investigation_id, kind, and command are required", map[string]any{"investigation_id": in.InvestigationID, "kind": "regression", "command": "go test ./..."})
	}
	if in.WorkspaceID != "" && (in.BaseWorkspaceID != "" || in.CandidateWorkspaceID != "") {
		return nil, ValidationOutput{}, InvalidArgument("workspace_id", "cannot be combined with base_workspace_id or candidate_workspace_id", map[string]any{"workspace_id": in.WorkspaceID})
	}
	if in.WorkspaceID == "" && (in.BaseWorkspaceID == "" || in.CandidateWorkspaceID == "") {
		return nil, ValidationOutput{}, InvalidArgument("base_workspace_id", "base_workspace_id and candidate_workspace_id must be provided together", map[string]any{"base_workspace_id": "<base-id>", "candidate_workspace_id": "<candidate-id>"})
	}
	if in.Timeout != "" {
		if _, err := time.ParseDuration(in.Timeout); err != nil {
			return nil, ValidationOutput{}, InvalidArgument("timeout", "must be a positive Go duration", map[string]any{"timeout": "30m"})
		}
	}
	if in.ReadinessTimeout != "" {
		if duration, err := time.ParseDuration(in.ReadinessTimeout); err != nil || duration <= 0 {
			return nil, ValidationOutput{}, InvalidArgument("readiness_timeout", "must be a positive Go duration", map[string]any{"readiness_timeout": "30s"})
		}
		if in.Protocol == "" {
			return nil, ValidationOutput{}, InvalidArgument("protocol", "readiness_timeout requires a declared protocol adapter", map[string]any{"protocol": "mcp_stdio", "readiness_timeout": "30s"})
		}
	}
	if in.MaxOutputBytes < 0 {
		return nil, ValidationOutput{}, InvalidArgument("max_output_bytes", "cannot be negative", map[string]any{"max_output_bytes": 65536})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, ValidationOutput{}, errors.New("validation definition is not available")
	}
	out, err := operator.DefineValidation(ctx, in)
	return nil, out, err
}

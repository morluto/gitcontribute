package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// RunValidationInput selects a validation definition and explicitly authorizes execution.

// DefineValidationInput records a bounded validation command without executing it.

// ValidationExpectedObservation is one output assertion evaluated without a shell.

// ValidationObservationContract ties output assertions to the claimed behavior.

// ValidationOutput is the stable MCP representation of a validation definition.

func (s *Server) runValidation(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.RunValidationInput) (*mcp.CallToolResult, mcpcontract.JobReference, error) {
	id, err := normalizeID("id", in.ID)
	if err != nil {
		return nil, mcpcontract.JobReference{}, err
	}
	in.ID = id
	if in.RunCount == 0 {
		in.RunCount = 1
	}
	if in.Concurrency == 0 {
		in.Concurrency = 1
	}
	if in.SampleInterval == "" {
		in.SampleInterval = "100ms"
	}
	if in.Target != "base" && in.Target != "candidate" && in.Target != "both" {
		return nil, mcpcontract.JobReference{}, mcpcontract.InvalidArgument("target", "must be base, candidate, or both", map[string]any{"target": "candidate"})
	}
	if !in.Execute {
		return nil, mcpcontract.JobReference{}, mcpcontract.InvalidArgument("execute", "must be true to authorize host command execution", map[string]any{"execute": true})
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.JobReference{}, errors.New("validation is not available")
	}
	out, err := operator.RunValidation(ctx, in)
	return nil, out, err
}

func (s *Server) defineValidation(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.DefineValidationInput) (*mcp.CallToolResult, mcpcontract.ValidationOutput, error) {
	if _, err := normalizeID("investigation_id", in.InvestigationID); err != nil {
		return nil, mcpcontract.ValidationOutput{}, err
	}
	in.Kind = strings.TrimSpace(in.Kind)
	in.Command = strings.TrimSpace(in.Command)
	in.WorkspaceID = strings.TrimSpace(in.WorkspaceID)
	in.BaseWorkspaceID = strings.TrimSpace(in.BaseWorkspaceID)
	in.CandidateWorkspaceID = strings.TrimSpace(in.CandidateWorkspaceID)
	if in.Kind == "" || in.Command == "" {
		return nil, mcpcontract.ValidationOutput{}, mcpcontract.InvalidArgument("command", "investigation_id, kind, and command are required", map[string]any{"investigation_id": in.InvestigationID, "kind": "regression", "command": "go test ./..."})
	}
	if in.Timeout != "" {
		if _, err := time.ParseDuration(in.Timeout); err != nil {
			return nil, mcpcontract.ValidationOutput{}, mcpcontract.InvalidArgument("timeout", "must be a positive Go duration", map[string]any{"timeout": "30m"})
		}
	}
	if in.ReadinessTimeout != "" {
		if duration, err := time.ParseDuration(in.ReadinessTimeout); err != nil || duration <= 0 {
			return nil, mcpcontract.ValidationOutput{}, mcpcontract.InvalidArgument("readiness_timeout", "must be a positive Go duration", map[string]any{"readiness_timeout": "30s"})
		}
		if in.Protocol == "" {
			return nil, mcpcontract.ValidationOutput{}, mcpcontract.InvalidArgument("protocol", "readiness_timeout requires a declared protocol adapter", map[string]any{"protocol": "mcp_stdio", "readiness_timeout": "30s"})
		}
	}
	operator, ok := s.reader.(Operator)
	if !ok {
		return nil, mcpcontract.ValidationOutput{}, errors.New("validation definition is not available")
	}
	out, err := operator.DefineValidation(ctx, in)
	return nil, out, err
}

func (s *Server) attachValidationReceipt(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.AttachValidationReceiptInput) (*mcp.CallToolResult, mcpcontract.ExternalValidationReceiptOutput, error) {
	if len(in.ReceiptJSON) == 0 || len(in.ReceiptJSON) > 2<<20 {
		return nil, mcpcontract.ExternalValidationReceiptOutput{}, mcpcontract.InvalidArgument("receipt_json", "must contain one receipt no larger than 2 MiB", nil)
	}
	operator, ok := s.reader.(ValidationReceiptOperator)
	if !ok {
		return nil, mcpcontract.ExternalValidationReceiptOutput{}, errors.New("external validation receipt import is unavailable")
	}
	out, err := operator.AttachValidationReceipt(ctx, in)
	return nil, out, err
}

func (s *Server) importExternalEvidenceManifest(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.ImportExternalEvidenceManifestInput) (*mcp.CallToolResult, mcpcontract.ImportExternalEvidenceManifestOutput, error) {
	if len(in.ManifestJSON) == 0 || len(in.ManifestJSON) > 2<<20 {
		return nil, mcpcontract.ImportExternalEvidenceManifestOutput{}, mcpcontract.InvalidArgument("manifest_json", "must contain one manifest no larger than 2 MiB", nil)
	}
	operator, ok := s.reader.(ExternalEvidenceImporter)
	if !ok {
		return nil, mcpcontract.ImportExternalEvidenceManifestOutput{}, errors.New("external evidence manifest import is unavailable")
	}
	out, err := operator.ImportExternalEvidenceManifest(ctx, in)
	return nil, out, err
}

func (s *Server) attachJUnitReport(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.AttachJUnitReportInput) (*mcp.CallToolResult, mcpcontract.AttachJUnitReportOutput, error) {
	if strings.TrimSpace(in.RunID) == "" || len(in.ReportXML) == 0 || len(in.ReportXML) > 2<<20 {
		return nil, mcpcontract.AttachJUnitReportOutput{}, mcpcontract.InvalidArgument("report_xml", "run_id and one JUnit report no larger than 2 MiB are required", nil)
	}
	operator, ok := s.reader.(JUnitReportImporter)
	if !ok {
		return nil, mcpcontract.AttachJUnitReportOutput{}, errors.New("JUnit report import is unavailable")
	}
	out, err := operator.AttachJUnitReport(ctx, in)
	return nil, out, err
}

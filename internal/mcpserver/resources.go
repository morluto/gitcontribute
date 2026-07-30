package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type concernResourceReader interface {
	Concern(context.Context, mcpcontract.ConcernInput) (mcpcontract.ConcernOutput, error)
}

type draftResourceReader interface {
	Draft(context.Context, mcpcontract.DraftInput) (mcpcontract.DraftOutput, error)
}

type manifestResourceReader interface {
	Manifest(context.Context, mcpcontract.ManifestInput) (mcpcontract.ManifestOutput, error)
}

type workspaceResourceReader interface {
	Workspace(context.Context, string) (mcpcontract.WorkspaceResource, error)
}

type pullRequestWorkflowResourceReader interface {
	PullRequestFeedbackResource(context.Context, string, string, int) (map[string]any, error)
	CIFailureResource(context.Context, string, string, int) (map[string]any, error)
	CIJobLogResource(context.Context, string, string, int, int64) (map[string]any, error)
}

func (s *Server) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	u, err := url.Parse(uri)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	value, err := s.readResourceValue(ctx, resourceRequest{
		uri: uri, scheme: u.Scheme, host: u.Host,
		parts: strings.Split(strings.Trim(u.Path, "/"), "/"),
	})
	if isNotFound(err) {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", uri, err)
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI: uri, MIMEType: "application/json", Text: string(payload),
	}}}, nil
}

type resourceRequest struct {
	uri    string
	scheme string
	host   string
	parts  []string
}

func (s *Server) readResourceValue(ctx context.Context, req resourceRequest) (any, error) {
	if req.scheme != "gitcontribute" {
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
	switch req.host {
	case "repository":
		return s.readRepositoryResource(ctx, req)
	case "dossier":
		return s.readDossierResource(ctx, req)
	case "thread":
		return s.readTypedThreadResource(ctx, req)
	case "investigation":
		return s.readInvestigationResource(ctx, req)
	case "opportunity":
		return s.readOpportunityResource(ctx, req)
	case "evidence":
		return s.readEvidenceResource(ctx, req)
	case "readiness":
		return s.readReadinessResource(ctx, req)
	case "lens":
		return s.readLensResource(ctx, req)
	case "fix-pattern-report":
		return s.readFixPatternReportResource(ctx, req)
	case "concern":
		return s.readConcernResource(ctx, req)
	case "draft":
		return s.readDraftResource(ctx, req)
	case "manifest":
		return s.readManifestResource(ctx, req)
	case "workspace":
		return s.readWorkspaceResource(ctx, req)
	case "pull-request-feedback":
		return s.readPullRequestFeedbackResource(ctx, req)
	case "ci-failure-report":
		return s.readCIFailureResource(ctx, req)
	case "ci-job-log":
		return s.readCIJobLogResource(ctx, req)
	default:
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
}

func (s *Server) readPullRequestFeedbackResource(ctx context.Context, req resourceRequest) (map[string]any, error) {
	reader, ok := s.reader.(pullRequestWorkflowResourceReader)
	number, valid := pullRequestResourceNumber(req.parts)
	if !ok || !valid {
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
	return reader.PullRequestFeedbackResource(ctx, req.parts[0], req.parts[1], number)
}

func (s *Server) readCIFailureResource(ctx context.Context, req resourceRequest) (map[string]any, error) {
	reader, ok := s.reader.(pullRequestWorkflowResourceReader)
	number, valid := pullRequestResourceNumber(req.parts)
	if !ok || !valid {
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
	return reader.CIFailureResource(ctx, req.parts[0], req.parts[1], number)
}

func (s *Server) readCIJobLogResource(ctx context.Context, req resourceRequest) (map[string]any, error) {
	reader, ok := s.reader.(pullRequestWorkflowResourceReader)
	if !ok || len(req.parts) != 4 {
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
	number, valid := positivePathNumber(req.parts[2])
	jobID, jobErr := strconv.ParseInt(req.parts[3], 10, 64)
	if !valid || jobErr != nil || jobID <= 0 {
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
	return reader.CIJobLogResource(ctx, req.parts[0], req.parts[1], number, jobID)
}

func pullRequestResourceNumber(parts []string) (int, bool) {
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return 0, false
	}
	return positivePathNumber(parts[2])
}

func (s *Server) readWorkspaceResource(ctx context.Context, req resourceRequest) (mcpcontract.WorkspaceResource, error) {
	if len(req.parts) != 1 || strings.TrimSpace(req.parts[0]) == "" {
		return mcpcontract.WorkspaceResource{}, mcp.ResourceNotFoundError(req.uri)
	}
	reader, ok := s.reader.(workspaceResourceReader)
	if !ok {
		return mcpcontract.WorkspaceResource{}, mcp.ResourceNotFoundError(req.uri)
	}
	return reader.Workspace(ctx, req.parts[0])
}

func (s *Server) readConcernResource(ctx context.Context, req resourceRequest) (mcpcontract.ConcernOutput, error) {
	if len(req.parts) != 1 || strings.TrimSpace(req.parts[0]) == "" {
		return mcpcontract.ConcernOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	reader, ok := s.reader.(concernResourceReader)
	if !ok {
		return mcpcontract.ConcernOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return reader.Concern(ctx, mcpcontract.ConcernInput{ID: req.parts[0]})
}

func (s *Server) readDraftResource(ctx context.Context, req resourceRequest) (mcpcontract.DraftOutput, error) {
	if len(req.parts) != 2 || strings.TrimSpace(req.parts[0]) == "" {
		return mcpcontract.DraftOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	revision, ok := positivePathNumber(req.parts[1])
	if !ok {
		return mcpcontract.DraftOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	reader, ok := s.reader.(draftResourceReader)
	if !ok {
		return mcpcontract.DraftOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return reader.Draft(ctx, mcpcontract.DraftInput{ID: req.parts[0], Revision: revision})
}

func (s *Server) readManifestResource(ctx context.Context, req resourceRequest) (mcpcontract.ManifestOutput, error) {
	if len(req.parts) != 1 || strings.TrimSpace(req.parts[0]) == "" {
		return mcpcontract.ManifestOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	reader, ok := s.reader.(manifestResourceReader)
	if !ok {
		return mcpcontract.ManifestOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return reader.Manifest(ctx, mcpcontract.ManifestInput{ID: req.parts[0]})
}

func (s *Server) readFixPatternReportResource(ctx context.Context, req resourceRequest) (mcpcontract.FixPatternReport, error) {
	if len(req.parts) != 1 {
		return mcpcontract.FixPatternReport{}, mcp.ResourceNotFoundError(req.uri)
	}
	reader, ok := s.reader.(FixPatternReader)
	if !ok {
		return mcpcontract.FixPatternReport{}, mcp.ResourceNotFoundError(req.uri)
	}
	return reader.GetFixPatternReport(ctx, req.parts[0])
}

func (s *Server) readRepositoryResource(ctx context.Context, req resourceRequest) (mcpcontract.RepositoryOutput, error) {
	if len(req.parts) != 2 {
		return mcpcontract.RepositoryOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Repository(ctx, mcpcontract.RepoInput{Owner: req.parts[0], Repo: req.parts[1]})
}

func (s *Server) readDossierResource(ctx context.Context, req resourceRequest) (mcpcontract.DossierOutput, error) {
	if len(req.parts) != 2 {
		return mcpcontract.DossierOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Dossier(ctx, mcpcontract.RepoInput{Owner: req.parts[0], Repo: req.parts[1]})
}

func (s *Server) readTypedThreadResource(ctx context.Context, req resourceRequest) (mcpcontract.ThreadOutput, error) {
	if len(req.parts) != 4 {
		return mcpcontract.ThreadOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	number, ok := positivePathNumber(req.parts[3])
	if !ok {
		return mcpcontract.ThreadOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Thread(ctx, mcpcontract.ThreadInput{
		Owner: req.parts[0], Repo: req.parts[1], Kind: req.parts[2], Number: number,
	})
}

func (s *Server) readNumberedThreadResource(ctx context.Context, req resourceRequest) (mcpcontract.ThreadOutput, error) {
	if len(req.parts) != 3 {
		return mcpcontract.ThreadOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	number, ok := positivePathNumber(req.parts[2])
	if !ok {
		return mcpcontract.ThreadOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.ThreadByNumber(ctx, mcpcontract.ThreadByNumberInput{
		Owner: req.parts[0], Repo: req.parts[1], Number: number,
	})
}

func (s *Server) readInvestigationResource(ctx context.Context, req resourceRequest) (mcpcontract.InvestigationOutput, error) {
	if len(req.parts) != 1 {
		return mcpcontract.InvestigationOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Investigation(ctx, mcpcontract.InvestigationInput{ID: req.parts[0], HypothesisLimit: 100})
}

func (s *Server) readOpportunitiesResource(ctx context.Context, req resourceRequest) (any, error) {
	if len(req.parts) != 1 {
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.ListOpportunities(ctx, mcpcontract.ListOpportunitiesInput{InvestigationID: req.parts[0], Limit: 100})
}

func (s *Server) readOpportunityResource(ctx context.Context, req resourceRequest) (mcpcontract.OpportunityOutput, error) {
	if len(req.parts) != 1 {
		return mcpcontract.OpportunityOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Opportunity(ctx, mcpcontract.OpportunityInput{ID: req.parts[0], EvidenceLimit: 100})
}

func (s *Server) readEvidenceResource(ctx context.Context, req resourceRequest) (mcpcontract.EvidenceOutput, error) {
	in, ok := evidenceResourceInput(req.scheme, req.parts)
	if !ok {
		return mcpcontract.EvidenceOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Evidence(ctx, in)
}

func (s *Server) readReadinessResource(ctx context.Context, req resourceRequest) (mcpcontract.ReadinessOutput, error) {
	if len(req.parts) != 1 {
		return mcpcontract.ReadinessOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Readiness(ctx, mcpcontract.ReadinessInput{OpportunityID: req.parts[0]})
}

func readWorkflowResource(req resourceRequest) (ContributionWorkflowResource, error) {
	if len(req.parts) != 2 || req.parts[0] != "contribution" {
		return ContributionWorkflowResource{}, mcp.ResourceNotFoundError(req.uri)
	}
	return contributionWorkflowResource(req.parts[1]), nil
}

func (s *Server) readLensResource(ctx context.Context, req resourceRequest) (mcpcontract.LensOutput, error) {
	if len(req.parts) != 1 {
		return mcpcontract.LensOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Lens(ctx, mcpcontract.LensInput{Name: req.parts[0]})
}

func positivePathNumber(value string) (int, bool) {
	number, err := strconv.Atoi(value)
	return number, err == nil && number > 0
}

func evidenceResourceInput(_ string, parts []string) (mcpcontract.EvidenceInput, bool) {
	var in mcpcontract.EvidenceInput
	if len(parts) != 2 {
		return mcpcontract.EvidenceInput{}, false
	}
	switch parts[0] {
	case "investigation":
		in.InvestigationID = parts[1]
	case "opportunity":
		in.OpportunityID = parts[1]
	default:
		return mcpcontract.EvidenceInput{}, false
	}
	in.Limit = 100
	return in, true
}

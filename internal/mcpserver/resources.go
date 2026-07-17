package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	if errors.Is(err, ErrNotFound) {
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
	switch req.host {
	case "repository", "repositories":
		return s.readRepositoryResource(ctx, req)
	case "dossier", "dossiers":
		return s.readDossierResource(ctx, req)
	case "thread":
		return s.readTypedThreadResource(ctx, req)
	case "threads":
		return s.readNumberedThreadResource(ctx, req)
	case "investigation", "investigations":
		return s.readInvestigationResource(ctx, req)
	case "opportunities":
		return s.readOpportunitiesResource(ctx, req)
	case "opportunity":
		return s.readOpportunityResource(ctx, req)
	case "evidence":
		return s.readEvidenceResource(ctx, req)
	case "readiness":
		return s.readReadinessResource(ctx, req)
	case "workflow", "workflows":
		return readWorkflowResource(req)
	case "lens", "lenses":
		return s.readLensResource(ctx, req)
	case "job", "jobs":
		return s.readJobResource(ctx, req)
	default:
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
}

func (s *Server) readRepositoryResource(ctx context.Context, req resourceRequest) (RepositoryOutput, error) {
	if len(req.parts) != 2 {
		return RepositoryOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Repository(ctx, RepoInput{Owner: req.parts[0], Repo: req.parts[1]})
}

func (s *Server) readDossierResource(ctx context.Context, req resourceRequest) (DossierOutput, error) {
	if len(req.parts) != 2 {
		return DossierOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Dossier(ctx, RepoInput{Owner: req.parts[0], Repo: req.parts[1]})
}

func (s *Server) readTypedThreadResource(ctx context.Context, req resourceRequest) (ThreadOutput, error) {
	if len(req.parts) != 4 {
		return ThreadOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	number, ok := positivePathNumber(req.parts[3])
	if !ok {
		return ThreadOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Thread(ctx, ThreadInput{
		Owner: req.parts[0], Repo: req.parts[1], Kind: req.parts[2], Number: number,
	})
}

func (s *Server) readNumberedThreadResource(ctx context.Context, req resourceRequest) (ThreadOutput, error) {
	if len(req.parts) != 3 {
		return ThreadOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	number, ok := positivePathNumber(req.parts[2])
	if !ok {
		return ThreadOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.ThreadByNumber(ctx, ThreadByNumberInput{
		Owner: req.parts[0], Repo: req.parts[1], Number: number,
	})
}

func (s *Server) readInvestigationResource(ctx context.Context, req resourceRequest) (InvestigationOutput, error) {
	if len(req.parts) != 1 {
		return InvestigationOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Investigation(ctx, InvestigationInput{ID: req.parts[0], HypothesisLimit: 100})
}

func (s *Server) readOpportunitiesResource(ctx context.Context, req resourceRequest) (any, error) {
	if len(req.parts) != 1 {
		return nil, mcp.ResourceNotFoundError(req.uri)
	}
	if req.scheme == "github-index" {
		return s.reader.Opportunity(ctx, OpportunityInput{ID: req.parts[0], EvidenceLimit: 100})
	}
	return s.reader.ListOpportunities(ctx, ListOpportunitiesInput{InvestigationID: req.parts[0], Limit: 100})
}

func (s *Server) readOpportunityResource(ctx context.Context, req resourceRequest) (OpportunityOutput, error) {
	if len(req.parts) != 1 {
		return OpportunityOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Opportunity(ctx, OpportunityInput{ID: req.parts[0], EvidenceLimit: 100})
}

func (s *Server) readEvidenceResource(ctx context.Context, req resourceRequest) (EvidenceOutput, error) {
	in, ok := evidenceResourceInput(req.scheme, req.parts)
	if !ok {
		return EvidenceOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Evidence(ctx, in)
}

func (s *Server) readReadinessResource(ctx context.Context, req resourceRequest) (ReadinessOutput, error) {
	if len(req.parts) != 1 {
		return ReadinessOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Readiness(ctx, ReadinessInput{OpportunityID: req.parts[0]})
}

func readWorkflowResource(req resourceRequest) (ContributionWorkflowResource, error) {
	if len(req.parts) != 2 || req.parts[0] != "contribution" {
		return ContributionWorkflowResource{}, mcp.ResourceNotFoundError(req.uri)
	}
	return contributionWorkflowResource(req.parts[1]), nil
}

func (s *Server) readLensResource(ctx context.Context, req resourceRequest) (LensOutput, error) {
	if len(req.parts) != 1 {
		return LensOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.Lens(ctx, LensInput{Name: req.parts[0]})
}

func (s *Server) readJobResource(ctx context.Context, req resourceRequest) (GetJobOutput, error) {
	if len(req.parts) != 1 {
		return GetJobOutput{}, mcp.ResourceNotFoundError(req.uri)
	}
	return s.reader.GetJob(ctx, GetJobInput{ID: req.parts[0]})
}

func positivePathNumber(value string) (int, bool) {
	number, err := strconv.Atoi(value)
	return number, err == nil && number > 0
}

func evidenceResourceInput(scheme string, parts []string) (EvidenceInput, bool) {
	var in EvidenceInput
	if scheme == "github-index" {
		if len(parts) != 1 {
			return EvidenceInput{}, false
		}
		in.InvestigationID = parts[0]
	} else {
		if len(parts) != 2 {
			return EvidenceInput{}, false
		}
		switch parts[0] {
		case "investigation":
			in.InvestigationID = parts[1]
		case "opportunity":
			in.OpportunityID = parts[1]
		default:
			return EvidenceInput{}, false
		}
	}
	in.Limit = 100
	return in, true
}

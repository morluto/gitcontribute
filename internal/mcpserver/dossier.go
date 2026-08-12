package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) materializeRepositoryDossier(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.MaterializeRepositoryDossierInput) (*mcp.CallToolResult, mcpcontract.DurableArtifactReference, error) {
	owner, repo, err := normalizeRepository(in.Owner, in.Repo)
	if err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	materializer, ok := s.reader.(DossierMaterializer)
	if !ok {
		return nil, mcpcontract.DurableArtifactReference{}, errors.New("repository dossier materialization is not available")
	}
	if _, err := materializer.MaterializeRepositoryDossier(ctx, mcpcontract.MaterializeRepositoryDossierInput{Owner: owner, Repo: repo}); err != nil {
		return nil, mcpcontract.DurableArtifactReference{}, err
	}
	uri := fmt.Sprintf("gitcontribute://dossier/%s/%s", owner, repo)
	ref := mcpcontract.DurableArtifactReference{Kind: "repository_dossier", ID: owner + "/" + repo, URI: uri}
	return linkedResource(uri, "repository-dossier", "Repository dossier", "Persisted deterministic dossier derived from local corpus facts."), ref, nil
}

package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) verifyPublishedDraft(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.VerifyPublishedDraftInput) (*mcp.CallToolResult, mcpcontract.PublishedDraftVerificationOutput, error) {
	if _, err := normalizeID("draft_id", in.DraftID); err != nil {
		return nil, mcpcontract.PublishedDraftVerificationOutput{}, err
	}
	if in.Revision < 1 || in.Number < 1 {
		return nil, mcpcontract.PublishedDraftVerificationOutput{}, mcpcontract.InvalidArgument("revision", "revision and number must be positive", nil)
	}
	if in.Kind != "issue" && in.Kind != "pull_request" {
		return nil, mcpcontract.PublishedDraftVerificationOutput{}, mcpcontract.InvalidArgument("kind", "must be issue or pull_request", nil)
	}
	verifier, ok := s.reader.(PublishedDraftVerifier)
	if !ok {
		return nil, mcpcontract.PublishedDraftVerificationOutput{}, errors.New("published draft verification is unavailable")
	}
	out, err := verifier.VerifyPublishedDraft(ctx, in)
	return nil, out, err
}

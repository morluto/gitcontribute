package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func (s *Server) analyzeFixPatterns(ctx context.Context, _ *mcp.CallToolRequest, in mcpcontract.AnalyzeFixPatternsInput) (*mcp.CallToolResult, mcpcontract.AnalyzeFixPatternsOutput, error) {
	analyzer, ok := s.reader.(FixPatternAnalyzer)
	if !ok {
		return nil, mcpcontract.AnalyzeFixPatternsOutput{}, errors.New("fix-pattern analysis is not available")
	}
	out, err := analyzer.AnalyzeFixPatterns(ctx, in)
	return nil, out, err
}

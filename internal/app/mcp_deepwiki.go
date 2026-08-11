package app

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/deepwiki"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type deepWikiQuery struct {
	request        deepwiki.Request
	maxOutputBytes int
}

func parseDeepWikiQuery(in mcpcontract.DeepWikiInput) (deepWikiQuery, error) {
	request, err := deepwiki.ParseRequest(in.Action, in.Repository, in.Repositories, in.Question)
	if err != nil {
		return deepWikiQuery{}, err
	}
	maxBytes := in.MaxOutputBytes
	if maxBytes == 0 {
		maxBytes = mcpcontract.DeepWikiDefaultOutputBytes
	}
	if maxBytes < mcpcontract.DeepWikiMinOutputBytes || maxBytes > mcpcontract.DeepWikiMaxOutputBytes {
		return deepWikiQuery{}, errors.New("max_output_bytes must be between 1024 and 1048576")
	}
	return deepWikiQuery{request: request, maxOutputBytes: maxBytes}, nil
}

// DeepWiki performs one external derived-knowledge read and does not persist
// its response.
func (r *MCPReader) DeepWiki(ctx context.Context, in mcpcontract.DeepWikiInput) (mcpcontract.DeepWikiOutput, error) {
	query, err := parseDeepWikiQuery(in)
	if err != nil {
		return mcpcontract.DeepWikiOutput{}, err
	}
	res, err := r.deepWiki().Read(ctx, query.request)
	if err != nil {
		return mcpcontract.DeepWikiOutput{}, err
	}
	repositories := query.request.Repositories()
	out := mcpcontract.DeepWikiOutput{
		Status:       "complete",
		Provider:     "deepwiki",
		Action:       query.request.Action().String(),
		Repositories: repositories,
		Question:     query.request.Question(),
		Result:       res.Text(),
		SourceURL:    res.SourceURL(),
		RetrievedAt:  formatTime(r.now()),
		Provenance:   "derived_external",
	}
	if !res.Available() {
		out.Status, out.Reason = "unavailable", "blocked"
		out.Recovery = recoveryPlan("blocked", "Use GitHub metadata, stored corpus data, or explicit code acquisition instead.")
		return out, nil
	}
	if len(out.Result) > query.maxOutputBytes {
		out.Result = validUTF8Prefix(out.Result, query.maxOutputBytes)
		out.Truncated = true
		out.Reason = "output_limit"
		if query.request.Action() == deepwiki.Contents {
			out.Recovery = recoveryPlan(
				"blocked",
				"Call structure, then ask a focused question about the relevant section. Increase max_output_bytes only when the focused read is still incomplete.",
				mcpcontract.RecoveryAction(mcpcontract.DeepWikiInput{Action: "structure", Repository: repositories[0]}),
			)
		} else {
			out.Recovery = recoveryPlan("blocked", "Narrow the question or repository set. Increase max_output_bytes only when the focused read is still incomplete.")
		}
	}
	return out, nil
}

func validUTF8Prefix(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.ValidString(value[:maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// SearchCode searches indexed code snapshots in the local corpus.
func (r *MCPReader) SearchCode(ctx context.Context, in mcpserver.SearchCodeInput) (mcpserver.SearchCodeOutput, error) {
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return mcpserver.SearchCodeOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpserver.SearchCodeOutput{}, errors.New("limit must be between 1 and 100")
	}
	var ref domain.RepoRef
	if in.Owner != "" || in.Repo != "" {
		if (in.Owner == "") != (in.Repo == "") {
			return mcpserver.SearchCodeOutput{}, errors.New("owner and repo must be provided together")
		}
		ref = domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
		if err := ref.Validate(); err != nil {
			return mcpserver.SearchCodeOutput{}, err
		}
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpserver.SearchCodeOutput{}, err
	}
	page, err := c.SearchCodeWithOptions(ctx, in.Query, corpus.CodeSearchOptions{Ref: ref, Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return mcpserver.SearchCodeOutput{}, fmt.Errorf("search code: %w", err)
	}
	out := make([]mcpserver.CodeMatchOutput, len(page.Matches))
	coverage := make([]mcpserver.CodeIndexCoverageOutput, 0, len(page.Snapshots)+1)
	for _, snapshot := range page.Snapshots {
		manifest := snapshot.Manifest
		entry := mcpserver.CodeIndexCoverageOutput{Repo: snapshot.Repo.String(), Status: "indexed_coverage_unknown", Commit: snapshot.CommitSHA, Truncated: manifest.Truncated}
		if manifest.CoverageKnown {
			entry.Status = "indexed"
		}
		entry.IndexedFiles, entry.TrackedEntries = manifest.IndexedFiles, manifest.TrackedEntries
		entry.SkippedPolicy = manifest.SkippedInvalidPath + manifest.SkippedExcluded + manifest.SkippedNonRegular
		entry.SkippedLimits = manifest.SkippedOversize + manifest.SkippedTotalBudget + manifest.SkippedFileLimit
		entry.SkippedNonText = manifest.SkippedNonText
		entry.SkippedFiles = entry.SkippedPolicy + entry.SkippedLimits + entry.SkippedNonText
		coverage = append(coverage, entry)
	}
	if ref != (domain.RepoRef{}) && len(page.Snapshots) == 0 {
		coverage = append(coverage, mcpserver.CodeIndexCoverageOutput{Repo: ref.String(), Status: "missing"})
	}
	for i, match := range page.Matches {
		repo := match.Repo.String()
		out[i] = mcpserver.CodeMatchOutput{
			ID: fmt.Sprintf("%s@%s:%s", repo, match.Commit, match.Path), Repo: repo,
			Commit: match.Commit, Path: match.Path, Language: match.Language,
			Snippet: boundedText(match.Content, 2000), Bytes: match.Bytes,
		}
	}
	return mcpserver.SearchCodeOutput{Query: in.Query, Total: page.Total, Matches: out, Coverage: coverage, NextCursor: page.NextCursor}, nil
}

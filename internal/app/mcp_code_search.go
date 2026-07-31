package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// SearchCode searches indexed code snapshots in the local corpus.
func (r *MCPReader) SearchCode(ctx context.Context, in mcpcontract.SearchCodeInput) (mcpcontract.SearchCodeOutput, error) {
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return mcpcontract.SearchCodeOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.SearchCodeOutput{}, errors.New("limit must be between 1 and 100")
	}
	var ref domain.RepoRef
	if in.Owner != "" || in.Repo != "" {
		if (in.Owner == "") != (in.Repo == "") {
			return mcpcontract.SearchCodeOutput{}, errors.New("owner and repo must be provided together")
		}
		ref = domain.RepoRef{Owner: in.Owner, Repo: in.Repo}
		if err := ref.Validate(); err != nil {
			return mcpcontract.SearchCodeOutput{}, err
		}
	}
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, in.CorpusRevision)
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	page, err := c.SearchCodeWithOptions(ctx, in.Query, corpus.CodeSearchOptions{Ref: ref, Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, fmt.Errorf("search code: %w", err)
	}
	out := make([]mcpcontract.CodeMatchOutput, len(page.Matches))
	coverage := make([]mcpcontract.CodeIndexCoverageOutput, 0, len(page.Snapshots)+1)
	for _, snapshot := range page.Snapshots {
		manifest := snapshot.Manifest
		entry := mcpcontract.CodeIndexCoverageOutput{Repo: snapshot.Repo.String(), Status: "indexed_coverage_unknown", Commit: snapshot.CommitSHA, Truncated: manifest.Truncated}
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
		coverage = append(coverage, mcpcontract.CodeIndexCoverageOutput{Repo: ref.String(), Status: "missing"})
	}
	for i, match := range page.Matches {
		repo := match.Repo.String()
		out[i] = mcpcontract.CodeMatchOutput{
			ID: fmt.Sprintf("%s@%s:%s", repo, match.Commit, match.Path), Repo: repo,
			Commit: match.Commit, Path: match.Path, Language: match.Language,
			Snippet: boundedText(match.Content, 2000), Bytes: match.Bytes,
		}
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	truncated := page.NextCursor != ""
	unknownCoverage := false
	for _, entry := range coverage {
		truncated = truncated || entry.Truncated
		unknownCoverage = unknownCoverage || entry.Status != "indexed"
	}
	provenance, err := offlineReadProvenance("code_search", revision, in, !truncated && !unknownCoverage, truncated, unknownCoverage)
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	return mcpcontract.SearchCodeOutput{Query: in.Query, Total: page.Total, Matches: out, Coverage: coverage, NextCursor: page.NextCursor, CorpusRevision: revision, Provenance: provenance}, nil
}

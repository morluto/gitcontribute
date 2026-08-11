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

type mcpCodeSearchRequest struct {
	query         string
	repository    domain.RepoRef
	page          corpus.SearchPage
	snapshotToken string
}

func parseMCPCodeSearchInput(in mcpcontract.SearchCodeInput) (mcpCodeSearchRequest, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return mcpCodeSearchRequest{}, errors.New("query is required")
	}
	page, err := corpus.ParseSearchPage(in.Limit, in.Cursor)
	if err != nil {
		return mcpCodeSearchRequest{}, err
	}
	repository, err := optionalRepoRef(in.Owner, in.Repo)
	if err != nil {
		return mcpCodeSearchRequest{}, err
	}
	return mcpCodeSearchRequest{query: query, repository: repository, page: page, snapshotToken: in.SnapshotToken}, nil
}

func (r mcpCodeSearchRequest) canonical() mcpcontract.SearchCodeInput {
	in := mcpcontract.SearchCodeInput{Query: r.query, Limit: r.page.Limit(), Cursor: r.page.Cursor(), SnapshotToken: r.snapshotToken}
	if r.repository.IsValid() {
		in.Owner, in.Repo = r.repository.Owner(), r.repository.Repo()
	}
	return in
}

type mcpCodeSearchBatchRequest struct {
	repository    domain.RepoRef
	queries       []string
	page          corpus.SearchPage
	snapshotToken string
}

func parseMCPCodeSearchBatchInput(in mcpcontract.SearchCodeBatchInput) (mcpCodeSearchBatchRequest, error) {
	if len(in.Queries) < 1 || len(in.Queries) > 20 {
		return mcpCodeSearchBatchRequest{}, errors.New("queries must contain 1 to 20 items")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpCodeSearchBatchRequest{}, errors.New("limit must be between 1 and 100")
	}
	page, err := corpus.ParseSearchPage(in.Limit, "")
	if err != nil {
		return mcpCodeSearchBatchRequest{}, err
	}
	if in.Owner == "" || in.Repo == "" {
		return mcpCodeSearchBatchRequest{}, errors.New("owner and repo are required")
	}
	repository, err := domain.NewRepoRef(in.Owner, in.Repo)
	if err != nil {
		return mcpCodeSearchBatchRequest{}, err
	}
	queries := make([]string, len(in.Queries))
	for i, query := range in.Queries {
		queries[i] = strings.TrimSpace(query)
		if queries[i] == "" {
			return mcpCodeSearchBatchRequest{}, fmt.Errorf("queries[%d] is required", i)
		}
	}
	return mcpCodeSearchBatchRequest{repository: repository, queries: queries, page: page, snapshotToken: in.SnapshotToken}, nil
}

func (r mcpCodeSearchBatchRequest) canonical() mcpcontract.SearchCodeBatchInput {
	return mcpcontract.SearchCodeBatchInput{
		Owner: r.repository.Owner(), Repo: r.repository.Repo(), Queries: append([]string(nil), r.queries...),
		Limit: r.page.Limit(), SnapshotToken: r.snapshotToken,
	}
}

// SearchCode searches indexed code snapshots in the local corpus.
func (r *MCPReader) SearchCode(ctx context.Context, in mcpcontract.SearchCodeInput) (mcpcontract.SearchCodeOutput, error) {
	request, err := parseMCPCodeSearchInput(in)
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	canonical := request.canonical()
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, request.snapshotToken)
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	out, coverage, page, truncated, unknownCoverage, err := r.searchCodeAtRevision(ctx, c, request)
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	provenance, err := offlineReadProvenance("code_search", revision, canonical, truncated, unknownCoverage)
	if err != nil {
		return mcpcontract.SearchCodeOutput{}, err
	}
	var recovery *mcpcontract.RecoveryPlan
	if unknownCoverage || codeCoverageTruncated(coverage) {
		recovery = codeSearchRecovery(canonical, coverage, truncated, unknownCoverage)
	} else if page.NextCursor != "" {
		recovery = codeSearchPageRecovery(canonical, page.NextCursor, request.snapshotToken)
	}
	provenance.Recovery = recovery
	return mcpcontract.SearchCodeOutput{Query: request.query, Total: page.Total, Matches: out, Coverage: coverage, NextCursor: page.NextCursor, SnapshotToken: snapshotIdentity(request.snapshotToken, revision), Recovery: recovery, Provenance: provenance}, nil
}

func (r *MCPReader) searchCodeAtRevision(ctx context.Context, c *corpus.Corpus, request mcpCodeSearchRequest) (
	[]mcpcontract.CodeMatchOutput, []mcpcontract.CodeIndexCoverageOutput, corpus.CodeSearchPage, bool, bool, error,
) {
	canonical := request.canonical()
	page, err := c.SearchCodeWithOptions(ctx, request.query, corpus.CodeSearchOptions{Ref: request.repository, Page: request.page})
	if err != nil {
		return nil, nil, corpus.CodeSearchPage{}, false, false, fmt.Errorf("search code: %w", err)
	}
	matches := make([]mcpcontract.CodeMatchOutput, len(page.Matches))
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
		if entry.Status != "indexed" || entry.Truncated {
			entry.Recovery = codeIndexRecovery(canonical, []mcpcontract.CodeIndexCoverageOutput{entry}, entry.Truncated, entry.Status != "indexed")
		}
		coverage = append(coverage, entry)
	}
	if request.repository.IsValid() && len(page.Snapshots) == 0 {
		entry := mcpcontract.CodeIndexCoverageOutput{Repo: request.repository.String(), Status: "missing"}
		entry.Recovery = codeIndexRecovery(canonical, []mcpcontract.CodeIndexCoverageOutput{entry}, false, true)
		coverage = append(coverage, entry)
	}
	for i, match := range page.Matches {
		repo := match.Repo.String()
		matches[i] = mcpcontract.CodeMatchOutput{
			ID: fmt.Sprintf("%s@%s:%s", repo, match.Commit, match.Path), Repo: repo,
			Commit: match.Commit, Path: match.Path, Language: match.Language,
			Snippet: boundedText(match.Content, 2000), Bytes: match.Bytes,
		}
	}
	truncated := page.NextCursor != ""
	unknownCoverage := !request.repository.IsValid() || len(coverage) == 0
	for _, entry := range coverage {
		truncated = truncated || entry.Truncated
		unknownCoverage = unknownCoverage || entry.Status != "indexed"
	}
	return matches, coverage, page, truncated, unknownCoverage, nil
}

func codeSearchRecovery(in mcpcontract.SearchCodeInput, coverage []mcpcontract.CodeIndexCoverageOutput, truncated, unknown bool) *mcpcontract.RecoveryPlan {
	if (in.Owner != "" && in.Repo != "") || len(coverage) > 0 {
		return codeIndexRecovery(in, coverage, truncated, unknown)
	}
	return recoveryPlan("code_index_coverage_unknown", "Code search is limited to locally indexed snapshots. Select a repository, index it, then repeat before inferring absence.", mcpcontract.RecoveryAction(mcpcontract.SearchGitHubRepositoriesInput{Text: in.Query, Limit: searchRecoveryLimit(in.Limit)}))
}

func codeIndexRecovery(in mcpcontract.SearchCodeInput, coverage []mcpcontract.CodeIndexCoverageOutput, truncated, unknown bool) *mcpcontract.RecoveryPlan {
	refs := make([]mcpcontract.IndexRepositoryInput, 0, len(coverage))
	seen := make(map[string]struct{}, len(coverage))
	if in.Owner != "" && in.Repo != "" {
		refs = append(refs, mcpcontract.IndexRepositoryInput{Owner: in.Owner, Repo: in.Repo})
	} else {
		for _, entry := range coverage {
			if entry.Status == "indexed" && !entry.Truncated {
				continue
			}
			owner, repo, ok := strings.Cut(entry.Repo, "/")
			if !ok || owner == "" || repo == "" {
				continue
			}
			if _, exists := seen[entry.Repo]; exists {
				continue
			}
			seen[entry.Repo] = struct{}{}
			refs = append(refs, mcpcontract.IndexRepositoryInput{Owner: owner, Repo: repo})
			if len(refs) == 10 {
				break
			}
		}
	}
	if len(refs) == 0 {
		return codeSearchRecovery(in, nil, truncated, unknown)
	}
	reason, message := "code_index_coverage_unknown", "Re-index the exact repositories, poll the job, then repeat code search before inferring absence."
	if truncated && !unknown {
		reason = "code_index_truncated"
		message = "The code index omitted files under its configured bounds. Re-index the exact repositories, then repeat code search."
	}
	return recoveryPlan(reason, message, mcpcontract.RecoveryAction(mcpcontract.IndexRepositoriesInput{Repositories: refs}))
}

// SearchCodeBatch performs ordered offline code searches over one shared
// corpus revision. Each query keeps the single-query coverage and truncation
// semantics; the tool only removes the model-side fan-out loop.
func (r *MCPReader) SearchCodeBatch(ctx context.Context, in mcpcontract.SearchCodeBatchInput) (mcpcontract.SearchCodeBatchOutput, error) {
	request, err := parseMCPCodeSearchBatchInput(in)
	if err != nil {
		return mcpcontract.SearchCodeBatchOutput{}, err
	}
	canonical := request.canonical()
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchCodeBatchOutput{}, err
	}
	revision, err := beginCorpusRead(ctx, c, request.snapshotToken)
	if err != nil {
		return mcpcontract.SearchCodeBatchOutput{}, err
	}
	out := mcpcontract.SearchCodeBatchOutput{Status: "complete", Repository: mcpcontract.RepositoryRef{Owner: request.repository.Owner(), Repo: request.repository.Repo()}, SnapshotToken: snapshotIdentity(request.snapshotToken, revision), Items: make([]mcpcontract.BatchItem[mcpcontract.SearchCodeOutput], len(request.queries))}
	allTruncated, allUnknown, allIndexGap := false, false, false
	for i, query := range request.queries {
		searchRequest := mcpCodeSearchRequest{query: query, repository: request.repository, page: request.page, snapshotToken: request.snapshotToken}
		searchIn := searchRequest.canonical()
		matches, coverage, page, truncated, unknown, searchErr := r.searchCodeAtRevision(ctx, c, searchRequest)
		item := mcpcontract.BatchItem[mcpcontract.SearchCodeOutput]{Key: query, Status: "complete"}
		if searchErr != nil {
			item.Status, item.Reason, item.Message = "failed", "code_search_failed", searchErr.Error()
			item.Recovery = codeSearchRecovery(searchIn, nil, false, true)
			out.Status = "partial"
			allUnknown, allIndexGap = true, true
			out.Items[i] = item
			continue
		}
		provenance, provenanceErr := offlineReadProvenance("code_search_batch", revision, searchIn, truncated, unknown)
		if provenanceErr != nil {
			return mcpcontract.SearchCodeBatchOutput{}, provenanceErr
		}
		var recovery *mcpcontract.RecoveryPlan
		if unknown || codeCoverageTruncated(coverage) {
			recovery = codeSearchRecovery(searchIn, coverage, truncated, unknown)
		} else if page.NextCursor != "" {
			recovery = codeSearchPageRecovery(searchIn, page.NextCursor, searchIn.SnapshotToken)
		}
		provenance.Recovery = recovery
		value := mcpcontract.SearchCodeOutput{Query: query, Total: page.Total, Matches: matches, Coverage: coverage, NextCursor: page.NextCursor, SnapshotToken: out.SnapshotToken, Recovery: recovery, Provenance: provenance}
		item.Value = &value
		out.Items[i] = item
		allTruncated, allUnknown = allTruncated || truncated, allUnknown || unknown
		allIndexGap = allIndexGap || codeCoverageTruncated(coverage)
		if truncated || unknown {
			out.Status = "partial"
		}
	}
	if err := finishCorpusRead(ctx, c, revision); err != nil {
		return mcpcontract.SearchCodeBatchOutput{}, err
	}
	provenance, err := offlineReadProvenance("code_search_batch", revision, canonical, allTruncated, allUnknown)
	if err != nil {
		return mcpcontract.SearchCodeBatchOutput{}, err
	}
	if allUnknown || allIndexGap {
		recovery := codeSearchRecovery(mcpCodeSearchRequest{query: request.queries[0], repository: request.repository, page: request.page}.canonical(), nil, allTruncated, allUnknown)
		out.Recovery = recovery
		provenance.Recovery = recovery
	} else if allTruncated {
		for _, item := range out.Items {
			if item.Value != nil && item.Value.NextCursor != "" {
				recovery := codeSearchPageRecovery(mcpCodeSearchRequest{query: item.Value.Query, repository: request.repository, page: request.page}.canonical(), item.Value.NextCursor, request.snapshotToken)
				out.Recovery = recovery
				provenance.Recovery = recovery
				break
			}
		}
	}
	out.Provenance = provenance
	return out, nil
}

func codeCoverageTruncated(coverage []mcpcontract.CodeIndexCoverageOutput) bool {
	for _, entry := range coverage {
		if entry.Status != "indexed" || entry.Truncated {
			return true
		}
	}
	return false
}

func codeSearchPageRecovery(in mcpcontract.SearchCodeInput, cursor, snapshotToken string) *mcpcontract.RecoveryPlan {
	next := in
	next.Cursor = cursor
	next.SnapshotToken = snapshotToken
	return recoveryPlan("code_search_page_truncated", "More indexed code matches exist. Read the returned next cursor before treating this page as exhaustive.", mcpcontract.RecoveryAction(next))
}

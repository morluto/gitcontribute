package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

const (
	githubThreadSearchArtifactKind = "github-thread-search.v1"
	sourceBundleArtifactKind       = "source-bundle.v1"
	maxSourceFileRequests          = 20
	defaultSourcePerFileBytes      = 256 * 1024
	defaultSourceTotalBytes        = 2 * 1024 * 1024
	maxSourcePerFileBytes          = 1024 * 1024
	maxSourceTotalBytes            = 4 * 1024 * 1024
)

// SearchGitHubThreads performs one bounded live issue-search request, records
// returned thread observations, and creates an immutable query-result
// artifact. It never claims repository-wide thread coverage.
func (r *MCPReader) SearchGitHubThreads(ctx context.Context, in mcpcontract.SearchGitHubThreadsInput) (mcpcontract.SearchGitHubThreadsOutput, error) {
	if err := validateGitHubThreadSearchInput(&in); err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	reader, err := r.githubReader() //nolint:contextcheck // construction does not perform a request
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	searcher, ok := reader.(github.ThreadSearcher)
	if !ok {
		return mcpcontract.SearchGitHubThreadsOutput{}, errors.New("configured GitHub reader does not support thread search")
	}
	result, err := searcher.SearchThreads(ctx, github.ThreadSearchOptions{
		Owner: in.Owner, Repo: in.Repo, Query: in.Query, Kind: github.ThreadKind(in.Kind), State: in.State,
		Sort: in.Sort, Order: in.Order, PageOptions: github.PageOptions{Page: in.Page, PerPage: in.Limit},
	})
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	return r.persistGitHubThreadSearch(ctx, in, result)
}

func validateGitHubThreadSearchInput(in *mcpcontract.SearchGitHubThreadsInput) error {
	if err := (domain.RepoRef{Owner: in.Owner, Repo: in.Repo}).Validate(); err != nil {
		return err
	}
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		return errors.New("query is required")
	}
	if in.Kind != "" && in.Kind != "issue" && in.Kind != "pull_request" {
		return fmt.Errorf("kind must be issue or pull_request")
	}
	if in.State == "" {
		in.State = "all"
	}
	if in.State != "open" && in.State != "closed" && in.State != "all" {
		return fmt.Errorf("state must be open, closed, or all")
	}
	if in.Sort != "" && in.Sort != "comments" && in.Sort != "created" && in.Sort != "updated" && in.Sort != "reactions" {
		return fmt.Errorf("sort must be comments, created, updated, or reactions")
	}
	if in.Order == "" {
		in.Order = "desc"
	}
	if in.Order != "asc" && in.Order != "desc" {
		return fmt.Errorf("order must be asc or desc")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return fmt.Errorf("limit must be between 1 and 100")
	}
	if in.Page == 0 {
		in.Page = 1
	}
	if in.Page < 1 || in.Page > 1000 || (in.Page-1)*in.Limit >= 1000 {
		return fmt.Errorf("page must keep the requested result offset below GitHub's 1,000-result cap")
	}
	return nil
}

func (r *MCPReader) persistGitHubThreadSearch(ctx context.Context, in mcpcontract.SearchGitHubThreadsInput, result github.ThreadSearchResult) (mcpcontract.SearchGitHubThreadsOutput, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	now := r.now().UTC()
	out := mcpcontract.SearchGitHubThreadsOutput{
		Status: "complete", Repository: mcpcontract.RepositoryRef{Owner: in.Owner, Repo: in.Repo}, Query: in.Query,
		ProviderQuery: result.Query, Kind: in.Kind, State: in.State, Sort: in.Sort, Order: in.Order,
		Page: in.Page, Limit: in.Limit, Total: result.Total, Incomplete: result.Incomplete,
		Rate: githubRateOutput(result.Rate), Coverage: "repository_thread_coverage_incomplete", ObservedAt: formatTime(now),
		Items: make([]mcpcontract.BatchItem[mcpcontract.ThreadOutput], len(result.Items)),
	}
	artifact := mcpcontract.GitHubThreadSearchArtifact{
		SchemaVersion: githubThreadSearchArtifactKind, ArtifactKind: githubThreadSearchArtifactKind,
		Repository: out.Repository, Query: in.Query, ProviderQuery: result.Query, Kind: in.Kind, State: in.State,
		Sort: in.Sort, Order: in.Order, Page: in.Page, Limit: in.Limit, Total: result.Total,
		Incomplete: result.Incomplete, HasNextPage: result.Page.HasNext, Rate: githubRateOutput(result.Rate),
		Provenance: mcpcontract.GitHubAcquisitionProvenance{Provider: "github", Endpoint: "search/issues", ObservedAt: formatTime(now)},
		CreatedAt:  formatTime(now), Items: make([]mcpcontract.GitHubThreadSearchArtifactItem, len(result.Items)),
	}
	if result.Page.HasNext {
		out.NextPage = result.Page.NextPage
	} else if in.Page*in.Limit < result.Total && in.Page*in.Limit < 1000 {
		out.NextPage = in.Page + 1
	}
	artifact.NextPage = out.NextPage
	hasNextPage := out.NextPage != 0
	artifact.HasNextPage = hasNextPage
	artifact.Completeness = mcpcontract.GitHubThreadSearchCompleteness{
		Status: "page_complete", IncompleteResults: result.Incomplete, HasNextPage: hasNextPage,
		RepositoryThreadCoverageKnown: false, RepositoryThreadCoverageFull: false,
	}
	if result.Incomplete || hasNextPage {
		out.Status = "partial"
		artifact.Completeness.Status = "partial"
	}
	if hasNextPage {
		next := in
		next.Page = out.NextPage
		out.RecoveryPlans = append(out.RecoveryPlans, *recoveryPlan("github_search_next_page", "More provider results exist. Request the returned next page before treating this page as exhaustive.", mcpcontract.RecoveryAction(next)))
	}
	if result.Incomplete {
		retry := in
		retry.Limit = min(100, max(in.Limit*2, in.Limit+1))
		out.RecoveryPlans = append(out.RecoveryPlans, *recoveryPlan("github_search_incomplete", "GitHub marked this search page incomplete. Replay the exact search with a larger page bound, then narrow the query if it remains incomplete.", mcpcontract.RecoveryAction(retry)))
	}
	artifact.RecoveryPlans = append([]mcpcontract.RecoveryPlan(nil), out.RecoveryPlans...)

	repo, err := ensureSearchRepository(ctx, c, in.Owner, in.Repo)
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	for index, issue := range result.Items {
		if issue.RepositoryOwner == "" {
			issue.RepositoryOwner = in.Owner
		}
		if issue.RepositoryName == "" {
			issue.RepositoryName = in.Repo
		}
		item := mcpcontract.BatchItem[mcpcontract.ThreadOutput]{Key: threadSearchItemKey(issue, index), Status: "complete"}
		if !strings.EqualFold(issue.RepositoryOwner, in.Owner) || !strings.EqualFold(issue.RepositoryName, in.Repo) {
			item.Status = "failed"
			item.Reason = "repository_scope_mismatch"
			item.Message = fmt.Sprintf("provider returned %s/%s for requested %s/%s", issue.RepositoryOwner, issue.RepositoryName, in.Owner, in.Repo)
			out.Status = "partial"
			out.Items[index] = item
			artifact.Items[index] = githubThreadSearchArtifactItem(issue, index, in.Owner, in.Repo)
			continue
		}
		thread, payload, payloadErr := threadFromIssue(issue)
		value := liveThreadOutput(issue)
		item.Value = &value
		if payloadErr == nil {
			thread.RepositoryID = repo.ID
			if _, upsertErr := c.UpsertThread(ctx, thread, payload); upsertErr != nil {
				payloadErr = upsertErr
			}
		}
		if payloadErr != nil {
			item.Status = "failed"
			item.Value = nil
			item.Reason = "observation_not_persisted"
			item.Message = payloadErr.Error()
			out.Status = "partial"
		}
		out.Items[index] = item
		artifact.Items[index] = githubThreadSearchArtifactItem(issue, index, in.Owner, in.Repo)
	}

	snapshot, err := c.MaterializeReadSnapshot(ctx, corpus.SnapshotMaterialization{
		Kind:            githubThreadSearchArtifactKind,
		Scope:           map[string]any{"repository": in.Owner + "/" + in.Repo, "query": in.Query, "page": in.Page},
		SourceManifest:  map[string]any{"provider_query": result.Query, "item_ids": artifactItemIDs(artifact.Items)},
		DerivedVersions: map[string]string{"github_thread_search": "v1"},
		Completeness:    artifact.Completeness,
		Provenance:      artifact.Provenance,
		Payload:         artifact,
	})
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, fmt.Errorf("store github thread search artifact: %w", err)
	}
	out.ArtifactDigest = snapshot.ArtifactDigest
	out.ResourceURI = "gitcontribute://artifact/github-thread-search/" + snapshot.ArtifactDigest
	return out, nil
}

// ReadGitHubThreadSearchArtifact is a local-only typed resource reader.
func (r *MCPReader) ReadGitHubThreadSearchArtifact(ctx context.Context, digest string) (mcpcontract.GitHubThreadSearchArtifact, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.GitHubThreadSearchArtifact{}, err
	}
	artifact, err := c.ResolveReadArtifact(ctx, githubThreadSearchArtifactKind, digest)
	if err != nil {
		if errors.Is(err, corpus.ErrSnapshotUnavailable) {
			return mcpcontract.GitHubThreadSearchArtifact{}, mcpcontract.ErrNotFound
		}
		return mcpcontract.GitHubThreadSearchArtifact{}, err
	}
	var out mcpcontract.GitHubThreadSearchArtifact
	if err := json.Unmarshal(artifact.Payload, &out); err != nil {
		return mcpcontract.GitHubThreadSearchArtifact{}, fmt.Errorf("decode github thread search artifact: %w", err)
	}
	if out.SchemaVersion != githubThreadSearchArtifactKind || out.ArtifactKind != githubThreadSearchArtifactKind {
		return mcpcontract.GitHubThreadSearchArtifact{}, errors.New("github thread search artifact schema mismatch")
	}
	return out, nil
}

// ReadSourceFiles performs bounded live source acquisition and stores only the
// resulting immutable source bundle. It does not touch thread facets or code
// index projections.
func (r *MCPReader) ReadSourceFiles(ctx context.Context, in mcpcontract.ReadSourceFilesInput) (mcpcontract.ReadSourceFilesOutput, error) {
	if err := validateReadSourceFilesInput(&in); err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, err
	}
	reader, err := r.githubReader() //nolint:contextcheck // construction does not perform a request
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, err
	}
	fileReader, ok := reader.(github.SourceFileReader)
	if !ok {
		return mcpcontract.ReadSourceFilesOutput{}, errors.New("configured GitHub reader does not support bounded source reads")
	}
	requests := make([]github.SourceFileRequest, len(in.Files))
	for i, file := range in.Files {
		requests[i] = github.SourceFileRequest{Path: file.Path, StartLine: file.StartLine, EndLine: file.EndLine}
	}
	result, err := fileReader.ReadSourceFiles(ctx, in.Owner, in.Repo, in.Ref, requests, github.SourceFileReadOptions{PerFileBytes: in.PerFileBytes, TotalBytes: in.TotalBytes})
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, err
	}
	return r.persistSourceBundle(ctx, in, result)
}

func validateReadSourceFilesInput(in *mcpcontract.ReadSourceFilesInput) error {
	if err := (domain.RepoRef{Owner: in.Owner, Repo: in.Repo}).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(in.Ref) == "" {
		return errors.New("ref is required")
	}
	if len(in.Files) < 1 || len(in.Files) > maxSourceFileRequests {
		return fmt.Errorf("files must contain 1 to %d items", maxSourceFileRequests)
	}
	if in.PerFileBytes == 0 {
		in.PerFileBytes = defaultSourcePerFileBytes
	}
	if in.TotalBytes == 0 {
		in.TotalBytes = defaultSourceTotalBytes
	}
	if in.PerFileBytes < 1 || in.PerFileBytes > maxSourcePerFileBytes {
		return fmt.Errorf("per_file_bytes must be between 1 and %d", maxSourcePerFileBytes)
	}
	if in.TotalBytes < 1 || in.TotalBytes > maxSourceTotalBytes {
		return fmt.Errorf("total_bytes must be between 1 and %d", maxSourceTotalBytes)
	}
	seen := make(map[string]struct{}, len(in.Files))
	for i, file := range in.Files {
		clean := strings.TrimSpace(file.Path)
		if clean == "" || strings.HasPrefix(clean, "/") || strings.Contains(clean, "\\") || clean != path.Clean(clean) || clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
			return fmt.Errorf("files[%d].path must be a repository-relative path without traversal", i)
		}
		if file.StartLine < 0 || file.EndLine < 0 || (file.StartLine > 0 && file.EndLine > 0 && file.EndLine < file.StartLine) {
			return fmt.Errorf("files[%d] line range must be inclusive and ordered", i)
		}
		if _, ok := seen[clean]; ok {
			return fmt.Errorf("files[%d].path is duplicated", i)
		}
		seen[clean] = struct{}{}
		in.Files[i].Path = clean
	}
	return nil
}

func (r *MCPReader) persistSourceBundle(ctx context.Context, in mcpcontract.ReadSourceFilesInput, result github.SourceFileReadResult) (mcpcontract.ReadSourceFilesOutput, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, err
	}
	now := r.now().UTC()
	out := mcpcontract.ReadSourceFilesOutput{
		Status: "complete", Repository: mcpcontract.RepositoryRef{Owner: in.Owner, Repo: in.Repo},
		RequestedRef: result.Resolution.RequestedRef, ResolvedRef: result.Resolution.ResolvedRef, CommitSHA: result.Resolution.CommitSHA,
		PerFileBytes: in.PerFileBytes, TotalByteLimit: in.TotalBytes, TotalBytes: result.TotalBytes,
		Items: make([]mcpcontract.SourceFileBatchItem, len(result.Items)), ObservedAt: formatTime(now), Rate: githubRateOutput(result.Rate),
	}
	artifact := mcpcontract.SourceBundleArtifact{
		SchemaVersion: sourceBundleArtifactKind, ArtifactKind: sourceBundleArtifactKind, Repository: out.Repository,
		RequestedRef: out.RequestedRef, ResolvedRef: out.ResolvedRef, CommitSHA: out.CommitSHA,
		PerFileBytes: in.PerFileBytes, TotalByteLimit: in.TotalBytes, TotalBytes: result.TotalBytes,
		Rate:       githubRateOutput(result.Rate),
		Items:      make([]mcpcontract.SourceFileBatchItem, len(result.Items)),
		Provenance: mcpcontract.GitHubAcquisitionProvenance{Provider: "github", Endpoint: "repos/contents", ObservedAt: formatTime(now)}, CreatedAt: formatTime(now),
	}
	for i, item := range result.Items {
		value := sourceFileOutput(item, result.Resolution, now)
		artifactItem := mcpcontract.SourceFileBatchItem{Key: item.Request.Path, Status: mcpcontract.SourceFileStatus(item.Status), Value: &value, Message: item.Message}
		if item.RetryAfter > 0 {
			artifactItem.RetryAfterMS = mcpcontract.NonNegativeInt(item.RetryAfter.Milliseconds())
		}
		if item.Status == "too_large" || item.Status == "retryable" {
			artifactItem.Recovery = sourceFileRecovery(in, item.Request, item.Status)
		}
		artifact.Items[i] = artifactItem
		compact := artifactItem
		if compact.Value != nil {
			copyValue := *compact.Value
			copyValue.Content = ""
			compact.Value = &copyValue
		}
		out.Items[i] = compact
		if item.Status != "complete" {
			out.Status = "partial"
		}
		if item.Status == "complete" {
			artifact.Completeness.CompleteItems++
		} else {
			artifact.Completeness.FailedItems++
		}
	}
	artifact.Completeness.RequestedItems = len(result.Items)
	artifact.Completeness.Status = out.Status
	artifact.Completeness.ContentsBounded = true
	snapshot, err := c.MaterializeReadSnapshot(ctx, corpus.SnapshotMaterialization{
		Kind:            sourceBundleArtifactKind,
		Scope:           map[string]any{"repository": in.Owner + "/" + in.Repo, "requested_ref": in.Ref, "paths": sourceBundlePaths(in.Files)},
		SourceManifest:  map[string]any{"commit_sha": result.Resolution.CommitSHA, "item_statuses": sourceBundleStatuses(result.Items)},
		DerivedVersions: map[string]string{"source_bundle": "v1"}, Completeness: artifact.Completeness,
		Provenance: artifact.Provenance, Payload: artifact,
	})
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, fmt.Errorf("store source bundle artifact: %w", err)
	}
	out.ArtifactDigest = snapshot.ArtifactDigest
	out.ResourceURI = "gitcontribute://artifact/source-bundle/" + snapshot.ArtifactDigest
	return out, nil
}

func sourceFileRecovery(in mcpcontract.ReadSourceFilesInput, request github.SourceFileRequest, status string) *mcpcontract.RecoveryPlan {
	next := in
	next.Files = []mcpcontract.SourceFileRequest{{Path: request.Path, StartLine: request.StartLine, EndLine: request.EndLine}}
	if status == "too_large" {
		next.PerFileBytes = min(1024*1024, max(in.PerFileBytes*2, in.PerFileBytes+1))
		next.TotalBytes = min(4*1024*1024, max(in.TotalBytes*2, in.TotalBytes+1))
		return recoveryPlan("source_file_too_large", "The selected file exceeded the current byte bound. Retry this exact file with the returned larger bounds or narrow its line range.", mcpcontract.RecoveryAction(next))
	}
	return recoveryPlan("source_file_retryable", "The provider returned a retryable source-file outcome. Replay this exact file request after the returned retry delay.", mcpcontract.RecoveryAction(next))
}

// ReadSourceBundleArtifact is a local-only typed resource reader.
func (r *MCPReader) ReadSourceBundleArtifact(ctx context.Context, digest string) (mcpcontract.SourceBundleArtifact, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return mcpcontract.SourceBundleArtifact{}, err
	}
	artifact, err := c.ResolveReadArtifact(ctx, sourceBundleArtifactKind, digest)
	if err != nil {
		if errors.Is(err, corpus.ErrSnapshotUnavailable) {
			return mcpcontract.SourceBundleArtifact{}, mcpcontract.ErrNotFound
		}
		return mcpcontract.SourceBundleArtifact{}, err
	}
	var out mcpcontract.SourceBundleArtifact
	if err := json.Unmarshal(artifact.Payload, &out); err != nil {
		return mcpcontract.SourceBundleArtifact{}, fmt.Errorf("decode source bundle artifact: %w", err)
	}
	if out.SchemaVersion != sourceBundleArtifactKind || out.ArtifactKind != sourceBundleArtifactKind {
		return mcpcontract.SourceBundleArtifact{}, errors.New("source bundle artifact schema mismatch")
	}
	return out, nil
}

func ensureSearchRepository(ctx context.Context, c *corpus.Corpus, owner, name string) (*corpus.Repository, error) {
	repo, err := c.GetRepository(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	if repo != nil {
		return repo, nil
	}
	return c.UpsertRepository(ctx, corpus.Repository{Owner: owner, Name: name}, `{"source":"github-thread-search"}`)
}

func liveThreadOutput(issue github.Issue) mcpcontract.ThreadOutput {
	return mcpcontract.ThreadOutput{
		Owner: issue.RepositoryOwner, Repo: issue.RepositoryName, Kind: string(issue.Kind), Number: issue.Number,
		State: issue.State, StateReason: issue.StateReason, Title: issue.Title, Author: issue.Author,
		AuthorAssociation: issue.AuthorAssociation, Labels: append([]string(nil), issue.Labels...), Assignees: append([]string(nil), issue.Assignees...),
		Draft: issue.Draft, ClosedAt: formatTimePtr(issue.ClosedAt), UpdatedAt: formatTime(issue.UpdatedAt),
	}
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func threadSearchItemKey(issue github.Issue, index int) string {
	if issue.ID != 0 {
		return fmt.Sprintf("%s#%d:%d", issue.Kind, issue.Number, issue.ID)
	}
	return fmt.Sprintf("%s#%d:%d", issue.Kind, issue.Number, index)
}

func githubThreadSearchArtifactItem(issue github.Issue, index int, owner, repo string) mcpcontract.GitHubThreadSearchArtifactItem {
	if issue.RepositoryOwner == "" {
		issue.RepositoryOwner = owner
	}
	if issue.RepositoryName == "" {
		issue.RepositoryName = repo
	}
	return mcpcontract.GitHubThreadSearchArtifactItem{
		Position: index, ID: issue.ID, NodeID: issue.NodeID, Owner: issue.RepositoryOwner, Repo: issue.RepositoryName,
		Kind: string(issue.Kind), Number: issue.Number, Title: issue.Title, State: issue.State, SourceURL: issue.HTMLURL,
		CreatedAt: formatTime(issue.CreatedAt), UpdatedAt: formatTime(issue.UpdatedAt), ClosedAt: formatTimePtr(issue.ClosedAt),
	}
}

func artifactItemIDs(items []mcpcontract.GitHubThreadSearchArtifactItem) []int64 {
	ids := make([]int64, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func githubRateOutput(rate github.RateInfo) mcpcontract.GitHubRateOutput {
	return mcpcontract.GitHubRateOutput{Limit: rate.Limit, Remaining: rate.Remaining, Used: rate.Used, Reset: formatTime(rate.Reset), Resource: rate.Resource}
}

func sourceFileOutput(item github.SourceFileReadItem, resolution github.RefResolution, observedAt time.Time) mcpcontract.SourceFileOutput {
	startLine, endLine := item.StartLine, item.EndLine
	if startLine == 0 && item.Request.StartLine != 0 {
		startLine = item.Request.StartLine
	}
	if endLine == 0 && item.Request.EndLine != 0 {
		endLine = item.Request.EndLine
	}
	return mcpcontract.SourceFileOutput{
		Path: item.Request.Path, RequestedRef: resolution.RequestedRef, ResolvedRef: resolution.ResolvedRef, CommitSHA: resolution.CommitSHA,
		BlobSHA: item.File.BlobSHA, SourceURL: item.File.HTMLURL, ContentSHA256: item.ContentSHA, Bytes: item.Bytes,
		StartLine: startLine, EndLine: endLine, Content: item.File.Content, ObservedAt: formatTime(observedAt),
	}
}

func sourceBundlePaths(files []mcpcontract.SourceFileRequest) []string {
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	return paths
}

func sourceBundleStatuses(items []github.SourceFileReadItem) []string {
	statuses := make([]string, len(items))
	for i, item := range items {
		statuses[i] = item.Status
	}
	return statuses
}

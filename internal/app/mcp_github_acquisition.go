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

type githubThreadSearchSort uint8

const (
	githubThreadSearchBestMatch githubThreadSearchSort = iota
	githubThreadSearchComments
	githubThreadSearchCreated
	githubThreadSearchUpdated
	githubThreadSearchReactions
)

func parseGitHubThreadSearchSort(value string) (githubThreadSearchSort, error) {
	switch strings.TrimSpace(value) {
	case "":
		return githubThreadSearchBestMatch, nil
	case "comments":
		return githubThreadSearchComments, nil
	case "created":
		return githubThreadSearchCreated, nil
	case "updated":
		return githubThreadSearchUpdated, nil
	case "reactions":
		return githubThreadSearchReactions, nil
	default:
		return 0, errors.New("sort must be comments, created, updated, or reactions")
	}
}

func (s githubThreadSearchSort) String() string {
	return [...]string{"", "comments", "created", "updated", "reactions"}[s]
}

type githubThreadSearchRequest struct {
	repository domain.RepoRef
	query      string
	kind       corpus.ThreadKindFilter
	state      corpus.ThreadStateFilter
	sort       githubThreadSearchSort
	order      githubSearchOrder
	page       githubSearchPage
}

type repositoryRelativePath string

func parseRepositoryRelativePath(value string) (repositoryRelativePath, error) {
	clean := strings.TrimSpace(value)
	if clean == "" || strings.HasPrefix(clean, "/") || strings.Contains(clean, "\\") || clean != path.Clean(clean) || clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", errors.New("must be a repository-relative path without traversal")
	}
	return repositoryRelativePath(clean), nil
}

type sourceFileSelection struct {
	path               repositoryRelativePath
	startLine, endLine int
}

func (s sourceFileSelection) githubRequest() github.SourceFileRequest {
	return github.SourceFileRequest{Path: string(s.path), StartLine: s.startLine, EndLine: s.endLine}
}

func (s sourceFileSelection) canonical() mcpcontract.SourceFileRequest {
	return mcpcontract.SourceFileRequest{Path: string(s.path), StartLine: s.startLine, EndLine: s.endLine}
}

type sourceFilesRequest struct {
	repository   domain.RepoRef
	ref          string
	files        []sourceFileSelection
	perFileBytes int
	totalBytes   int
}

type githubThreadSearchSnapshotScope struct {
	Repository string `json:"repository"`
	Query      string `json:"query"`
	Page       int    `json:"page"`
}

type githubThreadSearchSnapshotSource struct {
	ProviderQuery string  `json:"provider_query"`
	ItemIDs       []int64 `json:"item_ids"`
}

type githubThreadSearchSnapshotVersions struct {
	GitHubThreadSearch string `json:"github_thread_search"`
}

type sourceBundleSnapshotScope struct {
	Repository   string   `json:"repository"`
	RequestedRef string   `json:"requested_ref"`
	Paths        []string `json:"paths"`
}

type sourceBundleSnapshotSource struct {
	CommitSHA    string   `json:"commit_sha"`
	ItemStatuses []string `json:"item_statuses"`
}

type sourceBundleSnapshotVersions struct {
	SourceBundle string `json:"source_bundle"`
}

func (r sourceFilesRequest) canonical() mcpcontract.ReadSourceFilesInput {
	files := make([]mcpcontract.SourceFileRequest, len(r.files))
	for i, file := range r.files {
		files[i] = file.canonical()
	}
	return mcpcontract.ReadSourceFilesInput{
		Repository: mcpcontract.RepositoryRef{Owner: r.repository.Owner(), Repo: r.repository.Repo()},
		Ref:        r.ref, Files: files, PerFileBytes: r.perFileBytes, TotalBytes: r.totalBytes,
	}
}

func (r githubThreadSearchRequest) canonical() mcpcontract.SearchGitHubThreadsInput {
	state := r.state.String()
	if r.state.IsAny() {
		state = "all"
	}
	return mcpcontract.SearchGitHubThreadsInput{
		Repository: mcpcontract.RepositoryRef{Owner: r.repository.Owner(), Repo: r.repository.Repo()},
		Query:      r.query, Kind: r.kind.String(), State: state, Sort: r.sort.String(), Order: r.order.String(),
		Page: r.page.number, Limit: r.page.limit,
	}
}

// SearchGitHubThreads performs one bounded live issue-search request, records
// returned thread observations, and creates an immutable query-result
// artifact. It never claims repository-wide thread coverage.
func (r *MCPReader) SearchGitHubThreads(ctx context.Context, in mcpcontract.SearchGitHubThreadsInput) (mcpcontract.SearchGitHubThreadsOutput, error) {
	request, err := parseGitHubThreadSearchInput(in)
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	canonical := request.canonical()
	reader, err := r.githubReader() //nolint:contextcheck // construction does not perform a request
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	searcher, ok := reader.(github.ThreadSearcher)
	if !ok {
		return mcpcontract.SearchGitHubThreadsOutput{}, errors.New("configured GitHub reader does not support thread search")
	}
	result, err := searcher.SearchThreads(ctx, github.ThreadSearchOptions{
		Owner: request.repository.Owner(), Repo: request.repository.Repo(), Query: request.query, Kind: domain.ThreadKind(request.kind.String()), State: canonical.State,
		Sort: request.sort.String(), Order: request.order.String(), PageOptions: github.PageOptions{Page: request.page.number, PerPage: request.page.limit},
	})
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	return r.persistGitHubThreadSearch(ctx, canonical, result)
}

func parseGitHubThreadSearchInput(in mcpcontract.SearchGitHubThreadsInput) (githubThreadSearchRequest, error) {
	repository, err := domain.NewRepoRef(in.Repository.Owner, in.Repository.Repo)
	if err != nil {
		return githubThreadSearchRequest{}, err
	}
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return githubThreadSearchRequest{}, errors.New("query is required")
	}
	kind, err := corpus.ParseThreadKindFilter(in.Kind)
	if err != nil {
		return githubThreadSearchRequest{}, errors.New("kind must be issue or pull_request")
	}
	state, err := corpus.ParseThreadStateFilter(in.State)
	if err != nil {
		return githubThreadSearchRequest{}, errors.New("state must be open, closed, or all")
	}
	sortMode, err := parseGitHubThreadSearchSort(in.Sort)
	if err != nil {
		return githubThreadSearchRequest{}, err
	}
	order, err := parseGitHubSearchOrder(in.Order, githubSearchDescending)
	if err != nil {
		return githubThreadSearchRequest{}, err
	}
	page, pageProblem := parseGitHubSearchPage(in.Limit, in.Page)
	if pageProblem == githubSearchLimitInvalid {
		return githubThreadSearchRequest{}, errors.New("limit must be between 1 and 100")
	}
	if pageProblem == githubSearchPageInvalid {
		return githubThreadSearchRequest{}, errors.New("page must keep the requested result offset below GitHub's 1,000-result cap")
	}
	return githubThreadSearchRequest{
		repository: repository, query: query, kind: kind, state: state,
		sort: sortMode, order: order, page: page,
	}, nil
}

func (r *MCPReader) persistGitHubThreadSearch(ctx context.Context, in mcpcontract.SearchGitHubThreadsInput, result github.ThreadSearchResult) (mcpcontract.SearchGitHubThreadsOutput, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	now := r.now().UTC()
	out := mcpcontract.SearchGitHubThreadsOutput{
		Status: "complete", Repository: in.Repository, Query: in.Query,
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

	repo, err := ensureSearchRepository(ctx, c, in.Repository.Owner, in.Repository.Repo)
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, err
	}
	for index, issue := range result.Items {
		if issue.RepositoryOwner == "" {
			issue.RepositoryOwner = in.Repository.Owner
		}
		if issue.RepositoryName == "" {
			issue.RepositoryName = in.Repository.Repo
		}
		item := mcpcontract.BatchItem[mcpcontract.ThreadOutput]{Key: threadSearchItemKey(issue, index), Status: "complete"}
		if !strings.EqualFold(issue.RepositoryOwner, in.Repository.Owner) || !strings.EqualFold(issue.RepositoryName, in.Repository.Repo) {
			item.Status = "failed"
			item.Reason = "repository_scope_mismatch"
			item.Message = fmt.Sprintf("provider returned %s/%s for requested %s/%s", issue.RepositoryOwner, issue.RepositoryName, in.Repository.Owner, in.Repository.Repo)
			out.Status = "partial"
			out.Items[index] = item
			artifact.Items[index] = githubThreadSearchArtifactItem(issue, index, in.Repository.Owner, in.Repository.Repo)
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
		artifact.Items[index] = githubThreadSearchArtifactItem(issue, index, in.Repository.Owner, in.Repository.Repo)
	}

	materialization, err := corpus.NewSnapshotMaterialization(
		githubThreadSearchArtifactKind,
		githubThreadSearchSnapshotScope{Repository: in.Repository.Owner + "/" + in.Repository.Repo, Query: in.Query, Page: in.Page},
		githubThreadSearchSnapshotSource{ProviderQuery: result.Query, ItemIDs: artifactItemIDs(artifact.Items)},
		githubThreadSearchSnapshotVersions{GitHubThreadSearch: "v1"}, artifact.Completeness, artifact.Provenance, artifact,
	)
	if err != nil {
		return mcpcontract.SearchGitHubThreadsOutput{}, fmt.Errorf("prepare github thread search artifact: %w", err)
	}
	snapshot, err := c.MaterializeReadSnapshot(ctx, materialization)
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
	request, err := parseReadSourceFilesInput(in)
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, err
	}
	canonical := request.canonical()
	reader, err := r.githubReader() //nolint:contextcheck // construction does not perform a request
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, err
	}
	fileReader, ok := reader.(github.SourceFileReader)
	if !ok {
		return mcpcontract.ReadSourceFilesOutput{}, errors.New("configured GitHub reader does not support bounded source reads")
	}
	requests := make([]github.SourceFileRequest, len(request.files))
	for i, file := range request.files {
		requests[i] = file.githubRequest()
	}
	result, err := fileReader.ReadSourceFiles(ctx, request.repository.Owner(), request.repository.Repo(), request.ref, requests, github.SourceFileReadOptions{PerFileBytes: request.perFileBytes, TotalBytes: request.totalBytes})
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, err
	}
	return r.persistSourceBundle(ctx, canonical, result)
}

func parseReadSourceFilesInput(in mcpcontract.ReadSourceFilesInput) (sourceFilesRequest, error) {
	repository, err := domain.NewRepoRef(in.Repository.Owner, in.Repository.Repo)
	if err != nil {
		return sourceFilesRequest{}, err
	}
	ref := strings.TrimSpace(in.Ref)
	if ref == "" {
		return sourceFilesRequest{}, errors.New("ref is required")
	}
	if len(in.Files) < 1 || len(in.Files) > maxSourceFileRequests {
		return sourceFilesRequest{}, fmt.Errorf("files must contain 1 to %d items", maxSourceFileRequests)
	}
	if in.PerFileBytes == 0 {
		in.PerFileBytes = defaultSourcePerFileBytes
	}
	if in.TotalBytes == 0 {
		in.TotalBytes = defaultSourceTotalBytes
	}
	if in.PerFileBytes < 1 || in.PerFileBytes > maxSourcePerFileBytes {
		return sourceFilesRequest{}, fmt.Errorf("per_file_bytes must be between 1 and %d", maxSourcePerFileBytes)
	}
	if in.TotalBytes < 1 || in.TotalBytes > maxSourceTotalBytes {
		return sourceFilesRequest{}, fmt.Errorf("total_bytes must be between 1 and %d", maxSourceTotalBytes)
	}
	seen := make(map[string]struct{}, len(in.Files))
	files := make([]sourceFileSelection, len(in.Files))
	for i, file := range in.Files {
		parsedPath, err := parseRepositoryRelativePath(file.Path)
		if err != nil {
			return sourceFilesRequest{}, fmt.Errorf("files[%d].path %w", i, err)
		}
		if file.StartLine < 0 || file.EndLine < 0 || (file.StartLine > 0 && file.EndLine > 0 && file.EndLine < file.StartLine) {
			return sourceFilesRequest{}, fmt.Errorf("files[%d] line range must be inclusive and ordered", i)
		}
		if _, ok := seen[string(parsedPath)]; ok {
			return sourceFilesRequest{}, fmt.Errorf("files[%d].path is duplicated", i)
		}
		seen[string(parsedPath)] = struct{}{}
		files[i] = sourceFileSelection{path: parsedPath, startLine: file.StartLine, endLine: file.EndLine}
	}
	return sourceFilesRequest{
		repository: repository, ref: ref, files: files,
		perFileBytes: in.PerFileBytes, totalBytes: in.TotalBytes,
	}, nil
}

func (r *MCPReader) persistSourceBundle(ctx context.Context, in mcpcontract.ReadSourceFilesInput, result github.SourceFileReadResult) (mcpcontract.ReadSourceFilesOutput, error) {
	c, err := r.openCorpus(ctx)
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, err
	}
	now := r.now().UTC()
	out := mcpcontract.ReadSourceFilesOutput{
		Status: "complete", Repository: in.Repository,
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
		status, err := sourceFileStatus(item.Status)
		if err != nil {
			return mcpcontract.ReadSourceFilesOutput{}, err
		}
		value := sourceFileOutput(item, result.Resolution, now)
		artifactItem := mcpcontract.SourceFileBatchItem{Key: item.Request.Path, Status: status, Value: &value, Message: item.Message}
		if item.RetryAfter > 0 {
			artifactItem.RetryAfterMS = mcpcontract.NonNegativeInt(item.RetryAfter.Milliseconds())
		}
		if item.Status == github.SourceFileReadTooLarge || item.Status == github.SourceFileReadRetryable {
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
		if item.Status != github.SourceFileReadComplete {
			out.Status = "partial"
		}
		if item.Status == github.SourceFileReadComplete {
			artifact.Completeness.CompleteItems++
		} else {
			artifact.Completeness.FailedItems++
		}
	}
	artifact.Completeness.RequestedItems = len(result.Items)
	artifact.Completeness.Status = out.Status
	artifact.Completeness.ContentsBounded = true
	materialization, err := corpus.NewSnapshotMaterialization(
		sourceBundleArtifactKind,
		sourceBundleSnapshotScope{Repository: in.Repository.Owner + "/" + in.Repository.Repo, RequestedRef: in.Ref, Paths: sourceBundlePaths(in.Files)},
		sourceBundleSnapshotSource{CommitSHA: result.Resolution.CommitSHA, ItemStatuses: sourceBundleStatuses(result.Items)},
		sourceBundleSnapshotVersions{SourceBundle: "v1"}, artifact.Completeness, artifact.Provenance, artifact,
	)
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, fmt.Errorf("prepare source bundle artifact: %w", err)
	}
	snapshot, err := c.MaterializeReadSnapshot(ctx, materialization)
	if err != nil {
		return mcpcontract.ReadSourceFilesOutput{}, fmt.Errorf("store source bundle artifact: %w", err)
	}
	out.ArtifactDigest = snapshot.ArtifactDigest
	out.ResourceURI = "gitcontribute://artifact/source-bundle/" + snapshot.ArtifactDigest
	return out, nil
}

func sourceFileRecovery(in mcpcontract.ReadSourceFilesInput, request github.SourceFileRequest, status github.SourceFileReadStatus) *mcpcontract.RecoveryPlan {
	next := in
	next.Files = []mcpcontract.SourceFileRequest{{Path: request.Path, StartLine: request.StartLine, EndLine: request.EndLine}}
	if status == github.SourceFileReadTooLarge {
		next.PerFileBytes = min(1024*1024, max(in.PerFileBytes*2, in.PerFileBytes+1))
		next.TotalBytes = min(4*1024*1024, max(in.TotalBytes*2, in.TotalBytes+1))
		return recoveryPlan("source_file_too_large", "The selected file exceeded the current byte bound. Retry this exact file with the returned larger bounds or narrow its line range.", mcpcontract.RecoveryAction(next))
	}
	return recoveryPlan("source_file_retryable", "The provider returned a retryable source-file outcome. Replay this exact file request after the returned retry delay.", mcpcontract.RecoveryAction(next))
}

func sourceFileStatus(status github.SourceFileReadStatus) (mcpcontract.SourceFileStatus, error) {
	switch status {
	case github.SourceFileReadComplete:
		return mcpcontract.SourceFileComplete, nil
	case github.SourceFileReadNotFound:
		return mcpcontract.SourceFileNotFound, nil
	case github.SourceFileReadTooLarge:
		return mcpcontract.SourceFileTooLarge, nil
	case github.SourceFileReadRetryable:
		return mcpcontract.SourceFileRetryable, nil
	case github.SourceFileReadUnavailable:
		return mcpcontract.SourceFileUnavailable, nil
	case github.SourceFileReadFailed:
		return mcpcontract.SourceFileFailed, nil
	default:
		return "", fmt.Errorf("unsupported source-file read status %q", status)
	}
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
		statuses[i] = string(item.Status)
	}
	return statuses
}

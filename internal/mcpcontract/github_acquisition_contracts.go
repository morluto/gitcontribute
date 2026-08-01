package mcpcontract

// GitHubRateOutput is the provider rate-limit state observed with one live
// request. A zero value means the provider did not return rate metadata.
type GitHubRateOutput struct {
	Limit     int    `json:"limit,omitempty"`
	Remaining int    `json:"remaining,omitempty"`
	Used      int    `json:"used,omitempty"`
	Reset     string `json:"reset,omitempty"`
	Resource  string `json:"resource,omitempty"`
}

// SearchGitHubThreadsInput selects one bounded live GitHub issue-search page.
// The repository is required; live code search is intentionally not part of
// this operation.
type SearchGitHubThreadsInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
	Query string `json:"query" jsonschema:"User search text or GitHub issue-search qualifiers"`
	Kind  string `json:"kind,omitempty" jsonschema:"Optional issue or pull_request filter"`
	State string `json:"state,omitempty" jsonschema:"Optional open, closed, or all state filter"`
	Sort  string `json:"sort,omitempty" jsonschema:"Optional GitHub issue-search sort: comments, created, updated, or reactions"`
	Order string `json:"order,omitempty" jsonschema:"Optional asc or desc order"`
	Page  int    `json:"page,omitempty" jsonschema:"GitHub result page from 1 to 1000"`
	Limit int    `json:"limit,omitempty" jsonschema:"Results per page from 1 to 100"`
}

// SearchGitHubThreadsOutput is the compact live result. The complete ordered
// result, including provider provenance and item IDs, is available through
// ResourceURI.
type SearchGitHubThreadsOutput struct {
	Status         string                    `json:"status"`
	Repository     RepositoryRef             `json:"repository"`
	Query          string                    `json:"query"`
	ProviderQuery  string                    `json:"provider_query"`
	Kind           string                    `json:"kind,omitempty"`
	State          string                    `json:"state,omitempty"`
	Sort           string                    `json:"sort,omitempty"`
	Order          string                    `json:"order,omitempty"`
	Page           int                       `json:"page"`
	Limit          int                       `json:"limit"`
	NextPage       int                       `json:"next_page,omitempty"`
	Total          int                       `json:"total"`
	Incomplete     bool                      `json:"incomplete"`
	Rate           GitHubRateOutput          `json:"rate"`
	Items          []BatchItem[ThreadOutput] `json:"items"`
	Coverage       string                    `json:"coverage" jsonschema:"Repository-wide thread coverage remains incomplete; this search page is not proof of absence"`
	ObservedAt     string                    `json:"observed_at"`
	ArtifactDigest string                    `json:"artifact_digest"`
	ResourceURI    string                    `json:"resource_uri"`
}

// GitHubThreadSearchArtifact is the immutable github-thread-search.v1
// payload. Items preserve provider ordering and identity even when a compact
// tool result omits fields needed only for later inspection.
type GitHubThreadSearchArtifact struct {
	SchemaVersion string                           `json:"schema_version"`
	ArtifactKind  string                           `json:"artifact_kind"`
	Repository    RepositoryRef                    `json:"repository"`
	Query         string                           `json:"query"`
	ProviderQuery string                           `json:"provider_query"`
	Kind          string                           `json:"kind,omitempty"`
	State         string                           `json:"state,omitempty"`
	Sort          string                           `json:"sort,omitempty"`
	Order         string                           `json:"order,omitempty"`
	Page          int                              `json:"page"`
	Limit         int                              `json:"limit"`
	NextPage      int                              `json:"next_page,omitempty"`
	Total         int                              `json:"total"`
	Incomplete    bool                             `json:"incomplete"`
	HasNextPage   bool                             `json:"has_next_page"`
	Rate          GitHubRateOutput                 `json:"rate"`
	Items         []GitHubThreadSearchArtifactItem `json:"items"`
	Completeness  GitHubThreadSearchCompleteness   `json:"completeness"`
	Provenance    GitHubAcquisitionProvenance      `json:"provenance"`
	CreatedAt     string                           `json:"created_at"`
}

type GitHubThreadSearchArtifactItem struct {
	Position  int    `json:"position"`
	ID        int64  `json:"id"`
	NodeID    string `json:"node_id,omitempty"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Kind      string `json:"kind"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	SourceURL string `json:"source_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	ClosedAt  string `json:"closed_at,omitempty"`
}

type GitHubThreadSearchCompleteness struct {
	Status                        string `json:"status"`
	IncompleteResults             bool   `json:"incomplete_results"`
	HasNextPage                   bool   `json:"has_next_page"`
	RepositoryThreadCoverageKnown bool   `json:"repository_thread_coverage_known"`
	RepositoryThreadCoverageFull  bool   `json:"repository_thread_coverage_complete"`
}

// GitHubAcquisitionProvenance identifies the live provider request and its
// local observation time.
type GitHubAcquisitionProvenance struct {
	Provider   string `json:"provider"`
	Endpoint   string `json:"endpoint"`
	ObservedAt string `json:"observed_at"`
}

// ReadSourceFilesInput selects a bounded source bundle at a commit or named
// ref. Named refs are resolved before content is read and are not authoritative
// provenance.
type ReadSourceFilesInput struct {
	Owner        string              `json:"owner" jsonschema:"GitHub repository owner"`
	Repo         string              `json:"repo" jsonschema:"GitHub repository name"`
	Ref          string              `json:"ref" jsonschema:"Commit SHA, branch, or tag; resolved commit is authoritative"`
	Files        []SourceFileRequest `json:"files" jsonschema:"Ordered repository-relative files with optional inclusive line ranges"`
	PerFileBytes int                 `json:"per_file_bytes,omitempty" jsonschema:"Maximum decoded bytes per file from 1 to 1048576"`
	TotalBytes   int                 `json:"total_bytes,omitempty" jsonschema:"Maximum selected bytes for the complete bundle from 1 to 4194304"`
}

type SourceFileRequest struct {
	Path      string `json:"path" jsonschema:"Repository-relative file path"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"Inclusive 1-based start line; zero means beginning"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"Inclusive 1-based end line; zero means end"`
}

type SourceFileOutput struct {
	Path          string `json:"path"`
	RequestedRef  string `json:"requested_ref"`
	ResolvedRef   string `json:"resolved_ref"`
	CommitSHA     string `json:"commit_sha"`
	BlobSHA       string `json:"blob_sha"`
	SourceURL     string `json:"source_url,omitempty"`
	ContentSHA256 string `json:"content_sha256,omitempty"`
	Bytes         int    `json:"bytes"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	Content       string `json:"content,omitempty"`
	ObservedAt    string `json:"observed_at"`
}

// SourceFileStatus preserves the distinct bounded-read outcomes that are
// useful to callers: a missing path or an oversized file is not the same as a
// provider retry or an unexpected decoding failure.
type SourceFileStatus string

type SourceFileBatchItem struct {
	Key          string            `json:"key"`
	Status       SourceFileStatus  `json:"item_status"`
	Value        *SourceFileOutput `json:"value,omitempty"`
	Reason       string            `json:"reason,omitempty"`
	Message      string            `json:"message,omitempty"`
	RetryAfterMS NonNegativeInt    `json:"retry_after_ms,omitempty"`
}

type ReadSourceFilesOutput struct {
	Status         string                   `json:"status"`
	Repository     RepositoryRef            `json:"repository"`
	RequestedRef   string                   `json:"requested_ref"`
	ResolvedRef    string                   `json:"resolved_ref"`
	CommitSHA      string                   `json:"commit_sha"`
	PerFileBytes   int                      `json:"per_file_bytes"`
	TotalByteLimit int                      `json:"total_byte_limit"`
	TotalBytes     int                      `json:"total_bytes"`
	Items          []SourceFileBatchItem    `json:"items"`
	Completeness   SourceBundleCompleteness `json:"completeness"`
	ObservedAt     string                   `json:"observed_at"`
	Rate           GitHubRateOutput         `json:"rate"`
	ArtifactDigest string                   `json:"artifact_digest"`
	ResourceURI    string                   `json:"resource_uri"`
}

// SourceBundleArtifact is the canonical source-bundle.v1 resource payload.
// It contains bounded content only for complete items; failed items preserve
// their status and recovery message.
type SourceBundleArtifact struct {
	SchemaVersion  string                      `json:"schema_version"`
	ArtifactKind   string                      `json:"artifact_kind"`
	Repository     RepositoryRef               `json:"repository"`
	RequestedRef   string                      `json:"requested_ref"`
	ResolvedRef    string                      `json:"resolved_ref"`
	CommitSHA      string                      `json:"commit_sha"`
	PerFileBytes   int                         `json:"per_file_bytes"`
	TotalByteLimit int                         `json:"total_byte_limit"`
	TotalBytes     int                         `json:"total_bytes"`
	Rate           GitHubRateOutput            `json:"rate"`
	Items          []SourceFileBatchItem       `json:"items"`
	Completeness   SourceBundleCompleteness    `json:"completeness"`
	Provenance     GitHubAcquisitionProvenance `json:"provenance"`
	CreatedAt      string                      `json:"created_at"`
}

type SourceBundleCompleteness struct {
	Status          string `json:"status"`
	CompleteItems   int    `json:"complete_items"`
	RequestedItems  int    `json:"requested_items"`
	FailedItems     int    `json:"failed_items"`
	ContentsBounded bool   `json:"contents_bounded"`
}

// SearchCodeBatchInput composes up to 20 offline code searches over one
// repository scope and one optional snapshot precondition.
type SearchCodeBatchInput struct {
	Owner         string   `json:"owner" jsonschema:"Repository owner"`
	Repo          string   `json:"repo" jsonschema:"Repository name"`
	Queries       []string `json:"queries" jsonschema:"One to 20 code queries, returned in input order"`
	Limit         int      `json:"limit,omitempty" jsonschema:"Shared per-query result limit from 1 to 100"`
	SnapshotToken string   `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token"`
}

type SearchCodeBatchOutput struct {
	Status        string                        `json:"status"`
	Repository    RepositoryRef                 `json:"repository,omitempty"`
	Items         []BatchItem[SearchCodeOutput] `json:"items"`
	SnapshotToken string                        `json:"snapshot_token"`
	Provenance    CorpusReadProvenance          `json:"provenance"`
}

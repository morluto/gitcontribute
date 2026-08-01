package github

import "time"

// RefResolution records the provider ref supplied by a caller and the commit
// that GitHub resolved it to. CommitSHA is the authoritative source revision;
// ResolvedRef is the ref used for the content request (normally that commit).
type RefResolution struct {
	RequestedRef string
	ResolvedRef  string
	CommitSHA    string
}

// RepositoryFile is a bounded text file read from a repository at an explicit
// resolved revision. Content API types terminate in the GitHub adapter.
type RepositoryFile struct {
	Path         string
	BlobSHA      string
	CommitSHA    string
	RequestedRef string
	ResolvedRef  string
	HTMLURL      string
	Content      string
}

// SourceFileRequest identifies one repository-relative file and optional
// inclusive 1-based line range. A zero range requests the complete file.
type SourceFileRequest struct {
	Path      string
	StartLine int
	EndLine   int
}

// SourceFileReadOptions bounds one batch of repository content reads.
type SourceFileReadOptions struct {
	PerFileBytes int
	TotalBytes   int
}

// SourceFileReadItem is one ordered adapter-level content outcome. Content is
// populated only for complete reads; callers may persist the item unchanged.
type SourceFileReadItem struct {
	Request    SourceFileRequest
	Status     string
	File       RepositoryFile
	StartLine  int
	EndLine    int
	Bytes      int
	ContentSHA string
	Message    string
	RetryAfter time.Duration
}

// SourceFileReadResult preserves the single resolved revision and rate state
// shared by a bounded source-file batch.
type SourceFileReadResult struct {
	Resolution RefResolution
	Items      []SourceFileReadItem
	TotalBytes int
	Rate       RateInfo
}

// ThreadSearchOptions selects one bounded GitHub issue-search page for one
// repository. Query is the user text before provider qualifiers are added.
type ThreadSearchOptions struct {
	Owner string
	Repo  string
	Query string
	Kind  ThreadKind
	State string
	Sort  string
	Order string
	PageOptions
}

// ThreadSearchResult preserves the exact provider query and search metadata.
type ThreadSearchResult struct {
	Query      string
	Total      int
	Incomplete bool
	Items      []Issue
	Page       PageInfo
	Rate       RateInfo
}

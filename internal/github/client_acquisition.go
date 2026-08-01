package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	gh "github.com/google/go-github/v89/github"
)

// RepositoryFileReader is the optional exact-file capability used to ingest a
// small, fixed set of contribution-policy documents during explicit syncs.
type RepositoryFileReader interface {
	GetRepositoryFile(ctx context.Context, owner, name, path string) (RepositoryFile, RateInfo, error)
}

// RepositoryRefResolver resolves a named branch, tag, or commit to one
// authoritative commit. The result is a read-only provenance record.
type RepositoryRefResolver interface {
	ResolveRepositoryRef(ctx context.Context, owner, name, requestedRef string) (RefResolution, RateInfo, error)
}

// RepositoryFileAtRefReader reads one file at an explicit named ref. The
// adapter resolves the ref before requesting contents and preserves both the
// requested ref and resolved commit in RepositoryFile.
type RepositoryFileAtRefReader interface {
	GetRepositoryFileAtRef(ctx context.Context, owner, name, path, requestedRef string) (RepositoryFile, RateInfo, error)
}

// RepositoryFileAtResolvedRefReader reads one file using a resolution already
// shared by a caller, avoiding one ref-resolution request per file.
type RepositoryFileAtResolvedRefReader interface {
	GetRepositoryFileAtResolvedRef(ctx context.Context, owner, name, path string, resolution RefResolution) (RepositoryFile, RateInfo, error)
}

// ThreadSearcher is the optional live GitHub issue-search capability.
type ThreadSearcher interface {
	SearchThreads(ctx context.Context, opts ThreadSearchOptions) (ThreadSearchResult, error)
}

// SourceFileReader is the optional bounded batch source acquisition capability.
type SourceFileReader interface {
	ReadSourceFiles(ctx context.Context, owner, name, requestedRef string, requests []SourceFileRequest, opts SourceFileReadOptions) (SourceFileReadResult, error)
}

func sourceLineRange(content string, start, end int) (string, int, int, bool) {
	lines := strings.SplitAfter(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	lineCount := len(lines)
	if lineCount == 0 {
		lineCount = 1
		lines = []string{""}
	}
	if start == 0 && end == 0 {
		return content, 1, lineCount, true
	}
	if start == 0 {
		start = 1
	}
	if end == 0 {
		end = lineCount
	}
	if start < 1 || end < start || start > lineCount || end > lineCount {
		return "", 0, 0, false
	}
	return strings.Join(lines[start-1:end], ""), start, end, true
}

func sourceReadErrorStatus(err error) (string, time.Duration) {
	var primary *PrimaryRateLimitError
	var secondary *SecondaryRateLimitError
	var transient *TransientError
	var notFound *NotFoundError
	var denied *AccessDeniedError
	switch {
	case errors.As(err, &primary):
		return "retryable", primary.RetryAfter
	case errors.As(err, &secondary):
		return "retryable", secondary.RetryAfter
	case errors.As(err, &transient):
		return "retryable", time.Second
	case errors.As(err, &notFound):
		return "not_found", 0
	case errors.As(err, &denied):
		return "unavailable", 0
	default:
		return "failed", 0
	}
}

// ResolveRepositoryRef resolves one branch, tag, or commit to a commit SHA.
func (c *Client) ResolveRepositoryRef(ctx context.Context, owner, name, requestedRef string) (RefResolution, RateInfo, error) {
	requestedRef = strings.TrimSpace(requestedRef)
	if requestedRef == "" {
		requestedRef = "HEAD"
	}
	commit, resp, err := c.gh.Repositories.GetCommit(ctx, owner, name, requestedRef, nil)
	if err != nil {
		return RefResolution{}, responseRateInfo(resp), classifyError(err)
	}
	commitSHA := commit.GetSHA()
	if commitSHA == "" {
		return RefResolution{}, responseRateInfo(resp), fmt.Errorf("resolve repository ref %q: response did not include a commit SHA", requestedRef)
	}
	return RefResolution{RequestedRef: requestedRef, ResolvedRef: commitSHA, CommitSHA: commitSHA}, responseRateInfo(resp), nil
}

// GetRepositoryFile reads one text file at GitHub's HEAD after resolving that
// ref to a commit. Callers that need a named branch or tag should use
// GetRepositoryFileAtRef directly.
func (c *Client) GetRepositoryFile(ctx context.Context, owner, name, path string) (RepositoryFile, RateInfo, error) {
	return c.GetRepositoryFileAtRef(ctx, owner, name, path, "HEAD")
}

// GetRepositoryFileAtRef resolves requestedRef once and reads the file at the
// resulting commit. A named branch or tag is never used as authoritative
// provenance after this method returns.
func (c *Client) GetRepositoryFileAtRef(ctx context.Context, owner, name, path, requestedRef string) (RepositoryFile, RateInfo, error) {
	resolution, resolutionRate, err := c.ResolveRepositoryRef(ctx, owner, name, requestedRef)
	if err != nil {
		return RepositoryFile{}, resolutionRate, err
	}
	file, contentRate, err := c.GetRepositoryFileAtResolvedRef(ctx, owner, name, path, resolution)
	if err != nil {
		return RepositoryFile{}, contentRate, err
	}
	if contentRate == (RateInfo{}) {
		return file, resolutionRate, nil
	}
	return file, contentRate, nil
}

// GetRepositoryFileAtResolvedRef reads contents with the resolved commit in
// the ref query parameter. It performs no ref resolution or other network
// access beyond the content request itself.
func (c *Client) GetRepositoryFileAtResolvedRef(ctx context.Context, owner, name, path string, resolution RefResolution) (RepositoryFile, RateInfo, error) {
	if strings.TrimSpace(resolution.CommitSHA) == "" {
		return RepositoryFile{}, RateInfo{}, errors.New("resolved repository ref requires a commit SHA")
	}
	file, _, resp, err := c.gh.Repositories.GetContents(ctx, owner, name, path, &gh.RepositoryContentGetOptions{Ref: resolution.CommitSHA})
	if err != nil {
		return RepositoryFile{}, responseRateInfo(resp), classifyError(err)
	}
	if file == nil {
		return RepositoryFile{}, responseRateInfo(resp), &NotFoundError{Resource: path}
	}
	content, err := file.GetContent()
	if err != nil {
		return RepositoryFile{}, responseRateInfo(resp), fmt.Errorf("decode repository file %q: %w", path, err)
	}
	return RepositoryFile{
		Path: file.GetPath(), BlobSHA: file.GetSHA(), CommitSHA: resolution.CommitSHA,
		RequestedRef: resolution.RequestedRef, ResolvedRef: resolution.ResolvedRef,
		HTMLURL: file.GetHTMLURL(), Content: content,
	}, responseRateInfo(resp), nil
}

// ReadSourceFiles resolves one ref and reads a bounded, ordered batch. Limits
// apply to decoded file bytes before line-range slicing so a large remote file
// cannot bypass the per-file acquisition bound.
func (c *Client) ReadSourceFiles(ctx context.Context, owner, name, requestedRef string, requests []SourceFileRequest, opts SourceFileReadOptions) (SourceFileReadResult, error) {
	if len(requests) == 0 {
		return SourceFileReadResult{}, errors.New("source file request batch is empty")
	}
	if opts.PerFileBytes <= 0 || opts.TotalBytes <= 0 {
		return SourceFileReadResult{}, errors.New("source file byte limits must be positive")
	}
	resolution, rate, err := c.ResolveRepositoryRef(ctx, owner, name, requestedRef)
	if err != nil {
		return SourceFileReadResult{}, err
	}
	result := SourceFileReadResult{Resolution: resolution, Items: make([]SourceFileReadItem, len(requests)), Rate: rate}
	for index, request := range requests {
		item := SourceFileReadItem{Request: request, Status: "failed", StartLine: request.StartLine, EndLine: request.EndLine}
		if err := ctx.Err(); err != nil {
			return SourceFileReadResult{}, err
		}
		cleanPath := strings.TrimSpace(request.Path)
		if cleanPath == "" {
			item.Message = "path is required"
			result.Items[index] = item
			continue
		}
		if cleanPath != request.Path || cleanPath != path.Clean(cleanPath) || strings.HasPrefix(cleanPath, "/") || strings.Contains(cleanPath, "\\") || cleanPath == "." || strings.HasPrefix(cleanPath, "../") || strings.Contains(cleanPath, "/../") {
			item.Message = "path must be repository-relative without traversal"
			result.Items[index] = item
			continue
		}
		if request.StartLine < 0 || request.EndLine < 0 || (request.StartLine > 0 && request.EndLine > 0 && request.EndLine < request.StartLine) {
			item.Message = "line range must be inclusive, positive, and ordered"
			result.Items[index] = item
			continue
		}
		file, contentRate, readErr := c.GetRepositoryFileAtResolvedRef(ctx, owner, name, request.Path, resolution)
		if contentRate != (RateInfo{}) {
			result.Rate = contentRate
		}
		if readErr != nil {
			if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
				return SourceFileReadResult{}, readErr
			}
			status, retryAfter := sourceReadErrorStatus(readErr)
			item.Status, item.Message, item.RetryAfter = status, readErr.Error(), retryAfter
			result.Items[index] = item
			continue
		}
		metadata := file
		metadata.Content = ""
		item.File = metadata
		if len(file.Content) > opts.PerFileBytes {
			item.Status, item.Bytes, item.Message = "too_large", len(file.Content), fmt.Sprintf("file exceeds %d-byte per-file limit", opts.PerFileBytes)
			contentDigest := sha256.Sum256([]byte(file.Content))
			item.ContentSHA = hex.EncodeToString(contentDigest[:])
			result.Items[index] = item
			continue
		}
		content, startLine, endLine, ok := sourceLineRange(file.Content, request.StartLine, request.EndLine)
		if !ok {
			item.Status, item.Message = "failed", "requested line range is outside the file"
			result.Items[index] = item
			continue
		}
		if result.TotalBytes+len(content) > opts.TotalBytes {
			item.Status, item.Bytes, item.Message = "too_large", len(content), fmt.Sprintf("batch exceeds %d-byte total limit", opts.TotalBytes)
			contentDigest := sha256.Sum256([]byte(content))
			item.ContentSHA = hex.EncodeToString(contentDigest[:])
			result.Items[index] = item
			continue
		}
		contentDigest := sha256.Sum256([]byte(content))
		item.Status, item.File, item.StartLine, item.EndLine, item.Bytes, item.ContentSHA = "complete", file, startLine, endLine, len(content), hex.EncodeToString(contentDigest[:])
		item.File.Content = content
		result.TotalBytes += len(content)
		result.Items[index] = item
	}
	return result, nil
}

// SearchThreads searches one repository through GitHub's issue-search
// endpoint. The repository, kind, and state qualifiers are part of the
// preserved provider query; sort and order remain explicit request options.
func (c *Client) SearchThreads(ctx context.Context, opts ThreadSearchOptions) (ThreadSearchResult, error) {
	owner, repo := strings.TrimSpace(opts.Owner), strings.TrimSpace(opts.Repo)
	if owner == "" || repo == "" {
		return ThreadSearchResult{}, errors.New("thread search repository owner and name are required")
	}
	queryParts := []string{"repo:" + owner + "/" + repo}
	if text := strings.TrimSpace(opts.Query); text != "" {
		queryParts = append(queryParts, text)
	}
	switch opts.Kind {
	case ThreadKindIssue:
		queryParts = append(queryParts, "is:issue")
	case ThreadKindPullRequest:
		queryParts = append(queryParts, "is:pr")
	case "":
	default:
		return ThreadSearchResult{}, fmt.Errorf("unsupported thread kind %q", opts.Kind)
	}
	if opts.State != "" && opts.State != "all" {
		if opts.State != "open" && opts.State != "closed" {
			return ThreadSearchResult{}, fmt.Errorf("unsupported thread state %q", opts.State)
		}
		queryParts = append(queryParts, "is:"+opts.State)
	}
	query := strings.Join(queryParts, " ")
	result, resp, err := c.gh.Search.Issues(ctx, query, &gh.SearchOptions{
		Sort: opts.Sort, Order: opts.Order,
		ListOptions: gh.ListOptions{Page: opts.Page, PerPage: opts.PerPage},
	})
	if err != nil {
		return ThreadSearchResult{}, classifyError(err)
	}
	items := make([]Issue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		converted := convertIssue(issue)
		if converted.RepositoryOwner == "" {
			converted.RepositoryOwner = owner
		}
		if converted.RepositoryName == "" {
			converted.RepositoryName = repo
		}
		items = append(items, converted)
	}
	return ThreadSearchResult{
		Query: query, Total: result.GetTotal(), Incomplete: result.GetIncompleteResults(),
		Items: items, Page: pageInfo(resp), Rate: rateInfo(resp.Rate),
	}, nil
}

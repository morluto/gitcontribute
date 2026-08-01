package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type panicOfflineGitHubReader struct{}

func (panicOfflineGitHubReader) GetRepository(context.Context, string, string) (github.Repository, github.RateInfo, error) {
	panic("offline code batch contacted GitHub")
}
func (panicOfflineGitHubReader) ListIssues(context.Context, string, string, github.ListIssueOptions) (github.ListResult[github.Issue], error) {
	panic("offline code batch contacted GitHub")
}
func (panicOfflineGitHubReader) ListIssueComments(context.Context, string, string, int, github.PageOptions) (github.ListResult[github.IssueComment], error) {
	panic("offline code batch contacted GitHub")
}
func (panicOfflineGitHubReader) GetPullRequestDetails(context.Context, string, string, int) (github.PullRequestDetails, github.RateInfo, error) {
	panic("offline code batch contacted GitHub")
}
func (panicOfflineGitHubReader) ListPullRequestReviews(context.Context, string, string, int, github.PageOptions) (github.ListResult[github.Review], error) {
	panic("offline code batch contacted GitHub")
}
func (panicOfflineGitHubReader) ListPullRequestComments(context.Context, string, string, int, github.PageOptions) (github.ListResult[github.ReviewComment], error) {
	panic("offline code batch contacted GitHub")
}

func TestMCPReaderSearchGitHubThreadsPersistsArtifactWithoutFullCoverage(t *testing.T) {
	var searchCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/search/issues" {
			http.NotFound(w, r)
			return
		}
		searchCalls++
		if !strings.Contains(r.URL.Query().Get("q"), "repo:acme/rocket") || !strings.Contains(r.URL.Query().Get("q"), "is:issue") {
			t.Errorf("provider query = %q", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://api.github.com/search/issues?page=2>; rel="next"`)
		json.NewEncoder(w).Encode(map[string]any{
			"total_count": 4, "incomplete_results": false,
			"items": []any{map[string]any{
				"id": 101, "node_id": "I_101", "number": 9, "title": "persist this",
				"state": "open", "body": "body", "repository_url": "https://api.github.com/repos/acme/rocket",
				"html_url": "https://github.com/acme/rocket/issues/9", "user": map[string]any{"login": "alice"},
				"created_at": "2026-07-01T00:00:00Z", "updated_at": "2026-07-02T00:00:00Z",
			}},
		})
	}))
	defer srv.Close()

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	now := time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	reader := &MCPReader{svc}
	out, err := reader.SearchGitHubThreads(context.Background(), mcpcontract.SearchGitHubThreadsInput{Owner: "acme", Repo: "rocket", Query: "persist", Kind: "issue", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if searchCalls != 1 || out.Status != "partial" || out.NextPage != 2 || out.Total != 4 || out.Coverage != "repository_thread_coverage_incomplete" || out.ArtifactDigest == "" {
		t.Fatalf("search output = %+v", out)
	}
	if len(out.Items) != 1 || out.Items[0].Value == nil || out.Items[0].Value.Owner != "acme" || out.Items[0].Value.Number != 9 {
		t.Fatalf("search items = %+v", out.Items)
	}

	c, err := svc.openCorpus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.GetRepository(context.Background(), "acme", "rocket")
	if err != nil || repo == nil {
		t.Fatalf("stored repository = %+v, err=%v", repo, err)
	}
	thread, err := c.GetThread(context.Background(), repo.ID, "issue", 9)
	if err != nil || thread == nil {
		t.Fatalf("stored thread = %+v, err=%v", thread, err)
	}
	coverage, err := c.GetCoverage(context.Background(), repo.ID, nil, "metadata")
	if err != nil {
		t.Fatal(err)
	}
	if coverage != nil {
		t.Fatalf("thread search unexpectedly completed repository metadata coverage: %+v", coverage)
	}

	artifact, err := reader.ReadGitHubThreadSearchArtifact(context.Background(), out.ArtifactDigest)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SchemaVersion != githubThreadSearchArtifactKind || artifact.ProviderQuery == "" || !artifact.HasNextPage || artifact.Completeness.RepositoryThreadCoverageFull || len(artifact.Items) != 1 || artifact.Items[0].ID != 101 || artifact.Items[0].Position != 0 {
		t.Fatalf("thread search artifact = %+v", artifact)
	}
}

func TestMCPReaderReadSourceFilesStoresCommitAndBlobProvenanceAndReadsLocally(t *testing.T) {
	var contentCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/acme/rocket/commits/main":
			json.NewEncoder(w).Encode(map[string]any{"sha": "commit-9"})
		case "/api/v3/repos/acme/rocket/contents/README.md":
			contentCalls++
			if r.URL.Query().Get("ref") != "commit-9" {
				t.Errorf("content ref = %q", r.URL.Query().Get("ref"))
			}
			json.NewEncoder(w).Encode(map[string]any{
				"type": "file", "path": "README.md", "sha": "blob-9", "html_url": "https://github.com/acme/rocket/blob/commit-9/README.md",
				"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("first\nsecond\nthird\n")),
			})
		case "/api/v3/repos/acme/rocket/contents/missing.md":
			contentCalls++
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"message": "Not Found"})
		default:
			http.NotFound(w, r)
		}
	}))

	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	reader := &MCPReader{svc}
	out, err := reader.ReadSourceFiles(context.Background(), mcpcontract.ReadSourceFilesInput{
		Owner: "acme", Repo: "rocket", Ref: "main", Files: []mcpcontract.SourceFileRequest{{Path: "README.md", StartLine: 2, EndLine: 2}, {Path: "missing.md"}},
		PerFileBytes: 100, TotalBytes: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if contentCalls != 2 || out.CommitSHA != "commit-9" || out.ResolvedRef != "commit-9" || out.RequestedRef != "main" || out.ArtifactDigest == "" || out.Status != "partial" {
		t.Fatalf("source output = %+v calls=%d", out, contentCalls)
	}
	if len(out.Items) != 2 || out.Items[0].Value == nil || out.Items[0].Value.Content != "" || out.Items[0].Value.CommitSHA != "commit-9" || out.Items[0].Value.BlobSHA != "blob-9" || out.Items[0].Value.StartLine != 2 || out.Items[0].Value.EndLine != 2 {
		t.Fatalf("compact source items = %+v", out.Items)
	}
	if out.Items[1].Status != "not_found" {
		t.Fatalf("missing source status = %+v", out.Items[1])
	}

	srv.Close()
	artifact, err := reader.ReadSourceBundleArtifact(context.Background(), out.ArtifactDigest)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SchemaVersion != sourceBundleArtifactKind || artifact.CommitSHA != "commit-9" || artifact.Items[0].Value == nil || artifact.Items[0].Value.Content != "second\n" || artifact.Items[0].Value.ContentSHA256 == "" || artifact.Items[1].Status != "not_found" {
		t.Fatalf("source artifact = %+v", artifact)
	}
}

func TestMCPReaderSearchCodeBatchUsesOneOfflineRevisionAndPreservesQueryOrder(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, domain.RepoRef{Owner: "acme", Repo: "rocket"}, codeindex.Snapshot{
		RepoPath: "/rocket", Commit: "commit-1", CreatedAt: time.Now().UTC(), TotalBytes: 40,
		Documents: []codeindex.Document{
			{Path: "parser.go", Content: "func parser() {}", Bytes: 16, LanguageHint: "go"},
			{Path: "README.md", Content: "parser notes", Bytes: 12, LanguageHint: "markdown"},
		}, Manifest: codeindex.Manifest{CoverageKnown: true, IndexedFiles: 2, TrackedEntries: 2},
	}); err != nil {
		t.Fatal(err)
	}
	reader := &MCPReader{svc}
	svc.SetGitHubReader(panicOfflineGitHubReader{})
	out, err := reader.SearchCodeBatch(ctx, mcpcontract.SearchCodeBatchInput{Owner: "acme", Repo: "rocket", Queries: []string{"parser", "func"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.Items) != 2 || out.Items[0].Key != "parser" || out.Items[1].Key != "func" || out.SnapshotToken == "" {
		t.Fatalf("batch output = %+v", out)
	}
	for i, item := range out.Items {
		if item.Status != "complete" || item.Value == nil || item.Value.SnapshotToken != out.SnapshotToken || item.Value.Provenance.SnapshotToken == "" {
			t.Fatalf("batch item %d = %+v", i, item)
		}
	}
	if len(out.Items[0].Value.Matches) != 1 || len(out.Items[1].Value.Matches) != 1 || out.Items[0].Value.Matches[0].Path != "parser.go" || out.Items[1].Value.Matches[0].Path != "parser.go" {
		t.Fatalf("batch matches = %+v", out.Items)
	}
	if out.Provenance.SnapshotToken == "" || out.Provenance.QueryDigestSHA256 == "" {
		t.Fatalf("batch provenance = %+v", out.Provenance)
	}
}

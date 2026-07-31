package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestCodeIndexArtifactsRemainDistinctAcrossCommitsAndExposeResourceHandoff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	for _, snapshot := range []codeindex.Snapshot{
		{RepoPath: "/repo", Commit: "commit-a", CreatedAt: time.Unix(1, 0), TotalBytes: 5, Documents: []codeindex.Document{{Path: "a.go", Content: "alpha", Bytes: 5}}, Manifest: codeindex.Manifest{CoverageKnown: true, TrackedEntries: 1, IndexedFiles: 1}},
		{RepoPath: "/repo", Commit: "commit-b", CreatedAt: time.Unix(2, 0), TotalBytes: 4, Documents: []codeindex.Document{{Path: "b.go", Content: "beta", Bytes: 4}}, Manifest: codeindex.Manifest{CoverageKnown: true, TrackedEntries: 1, IndexedFiles: 1}},
	} {
		if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, ref, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	reader := &MCPReader{Service: svc}
	a, err := reader.CodeIndexArtifact(ctx, ref.Owner, ref.Repo, "commit-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := reader.CodeIndexArtifact(ctx, ref.Owner, ref.Repo, "commit-b")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || a.ManifestID == b.ManifestID || a.ResourceURI == b.ResourceURI || a.CommitSHA == b.CommitSHA {
		t.Fatalf("commit identities collapsed: a=%+v b=%+v", a, b)
	}
	if a.ResourceURI != "gitcontribute://code-index/owner/repo/commit-a" || a.FollowUp == nil || a.FollowUp.ResourceURI != a.ResourceURI || a.FollowUp.Arguments.Repository != "owner/repo" || a.FollowUp.Arguments.Commit != "commit-a" {
		t.Fatalf("resource handoff = %+v", a)
	}
	if a.Kind != "code_index" || a.FileCount != mcpcontract.NonNegativeInt(1) || a.TrackedEntries != mcpcontract.NonNegativeInt(1) || a.ManifestSHA256 == "" {
		t.Fatalf("artifact metadata = %+v", a)
	}
}

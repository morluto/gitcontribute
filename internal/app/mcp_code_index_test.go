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
	aRecord, err := svc.corpus.LatestCodeIndexArtifact(ctx, ref, "commit-a")
	if err != nil || aRecord == nil {
		t.Fatalf("first artifact record = %+v, %v", aRecord, err)
	}
	bRecord, err := svc.corpus.LatestCodeIndexArtifact(ctx, ref, "commit-b")
	if err != nil || bRecord == nil {
		t.Fatalf("second artifact record = %+v, %v", bRecord, err)
	}
	a, err := reader.CodeIndexArtifact(ctx, aRecord.Digest)
	if err != nil {
		t.Fatal(err)
	}
	b, err := reader.CodeIndexArtifact(ctx, bRecord.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || a.ManifestID == b.ManifestID || a.ResourceURI == b.ResourceURI || a.CommitSHA == b.CommitSHA {
		t.Fatalf("commit identities collapsed: a=%+v b=%+v", a, b)
	}
	if a.ResourceURI != "gitcontribute://artifact/code-index/"+aRecord.Digest || a.FollowUp == nil || a.FollowUp.Action.ReadResource == nil || a.FollowUp.Action.ReadResource.URI != a.ResourceURI {
		t.Fatalf("resource handoff = %+v", a)
	}
	if a.Kind != "code_index" || a.FileCount != mcpcontract.NonNegativeInt(1) || a.TrackedEntries != mcpcontract.NonNegativeInt(1) || a.ManifestSHA256 == "" {
		t.Fatalf("artifact metadata = %+v", a)
	}
}

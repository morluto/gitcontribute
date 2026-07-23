package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func TestMCPReaderExplainMatchReturnsMatchingExcerpt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSearchTestService(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: ref.Owner, Name: ref.Repo, Description: "unrelated description",
		Topics: []string{"synthwave"}, SourceUpdatedAt: time.Unix(1, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open",
		Title: "ordinary title", Body: strings.Repeat("padding ", 400) + "deepthreadneedle",
		SourceUpdatedAt: time.Unix(2, 0).UTC(),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("prefix ", 400) + "deepcodeneedle"
	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, ref, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc123", CreatedAt: time.Unix(3, 0).UTC(), TotalBytes: len(content),
		Documents: []codeindex.Document{{Path: "deep.go", Content: content, Bytes: len(content), LanguageHint: "go"}},
	}); err != nil {
		t.Fatal(err)
	}

	reader := svc.MCPReader()
	threadOut, err := reader.ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: ref.Owner, Repo: ref.Repo, Kind: "issue", Number: thread.Number, Query: "deepthreadneedle",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(threadOut.Snippet, "deepthreadneedle") {
		t.Fatalf("thread explanation omitted match: %q", threadOut.Snippet)
	}

	codeOut, err := reader.ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: ref.Owner, Repo: ref.Repo, Kind: "code", Path: "deep.go", Commit: "abc123", Query: "deepcodeneedle",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(codeOut.Snippet, "deepcodeneedle") {
		t.Fatalf("code explanation omitted match: %q", codeOut.Snippet)
	}

	repoOut, err := reader.ExplainMatch(ctx, mcpserver.ExplainMatchInput{
		Owner: ref.Owner, Repo: ref.Repo, Kind: "repo", Query: "synthwave",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(repoOut.Snippet, "synthwave") {
		t.Fatalf("repository explanation omitted topic match: %q", repoOut.Snippet)
	}
}

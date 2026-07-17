package app

import (
	"context"
	"testing"

	"github.com/morluto/gitcontribute/internal/corpus"
)

func TestTUILoadReadsBoundedLocalData(t *testing.T) {
	ctx := context.Background()
	svc := newTestServiceNoNetwork(t)
	defer svc.Close()
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo := seedRepoForNeighbors(t, c)
	seedIssueForNeighbors(t, c, repo.ID, 7, "local issue", "local body", "alice", []string{"bug"})
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "metadata", repo.SourceUpdatedAt, true, 0); err != nil {
		t.Fatal(err)
	}

	data, err := svc.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Repositories) != 1 || data.Repositories[0].Ref != "owner/repo" {
		t.Fatalf("repositories=%+v", data.Repositories)
	}
	if len(data.Threads) != 1 || data.Threads[0].Detail != "local body" || data.Threads[0].Kind != corpus.ThreadKindIssue {
		t.Fatalf("threads=%+v", data.Threads)
	}
	if len(data.Repositories[0].Coverage) != 1 || !data.Repositories[0].Coverage[0].Complete {
		t.Fatalf("coverage=%+v", data.Repositories[0].Coverage)
	}
}

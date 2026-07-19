package corpus

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/precedent"
)

func TestLoadPrecedentRepositoriesGroupsSourcesByRepository(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, thread := range []Thread{
		{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 1, State: "open", Title: "source one", SourceUpdatedAt: time.Unix(3, 0)},
		{RepositoryID: repo.ID, Kind: ThreadKindIssue, Number: 2, State: "open", Title: "source two", SourceUpdatedAt: time.Unix(2, 0)},
		{RepositoryID: repo.ID, Kind: ThreadKindPullRequest, Number: 3, State: "closed", Title: "history", SourceUpdatedAt: time.Unix(1, 0)},
	} {
		if _, err := c.UpsertThread(ctx, thread, `{}`); err != nil {
			t.Fatal(err)
		}
	}

	snapshots, err := c.LoadPrecedentRepositories(ctx, []precedent.SourceRef{
		{Repository: domain.RepoRef{Owner: "acme", Repo: "rocket"}, Number: 1},
		{Repository: domain.RepoRef{Owner: "acme", Repo: "rocket"}, Number: 2},
	}, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || !snapshots[0].Available {
		t.Fatalf("snapshots = %+v", snapshots)
	}
	if len(snapshots[0].Sources) != 2 || snapshots[0].Sources[1].Title != "source one" || snapshots[0].Sources[2].Title != "source two" {
		t.Fatalf("sources = %+v", snapshots[0].Sources)
	}
	if len(snapshots[0].Closed) != 1 || snapshots[0].Closed[0].Number != 3 {
		t.Fatalf("closed = %+v", snapshots[0].Closed)
	}
}

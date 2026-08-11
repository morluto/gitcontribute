package corpus

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestPortfolioReadRejectsInvalidStoredThreadProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	now := time.Unix(1, 0).UTC()
	repo, err := c.UpsertRepository(ctx, Repository{Owner: "acme", Name: "rocket", SourceUpdatedAt: now}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := c.UpsertThread(ctx, Thread{
		RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: 1, State: domain.OpenState,
		Title: "invalid projection fixture", SourceCreatedAt: now, SourceUpdatedAt: now,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ExecContext(ctx, `UPDATE threads SET state = 'invented' WHERE id = ?`, thread.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListPullRequestPortfolioPage(ctx, "", AnyThreadState(), nil, 10); err == nil {
		t.Fatal("portfolio read accepted an invalid stored thread projection")
	}
}

package corpus

import (
	"context"
	"testing"
	"time"
)

func TestChangeWatchDetectsCommitFromCorpusConnection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	watch, err := c.BeginChangeWatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watch.Close() }()
	if unchanged, err := watch.Unchanged(ctx); err != nil || !unchanged {
		t.Fatalf("new watch = (%v, %v)", unchanged, err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "id", time.Unix(1, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	if unchanged, err := watch.Unchanged(ctx); err != nil || unchanged {
		t.Fatalf("watch after commit = (%v, %v)", unchanged, err)
	}
}

package corpus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestCorpusRevisionIsMonotonicAndDetectsStaleReads(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	initial, err := c.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if initial != 0 {
		t.Fatalf("initial corpus revision = %d, want 0", initial)
	}
	if _, _, err := c.StoreCodeSnapshot(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "first", CreatedAt: time.Unix(1, 0),
		Documents: []codeindex.Document{{Path: "main.go", Content: "package main", Bytes: 12}},
	}); err != nil {
		t.Fatal(err)
	}
	current, err := c.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current <= initial {
		t.Fatalf("corpus revision = %d after mutation, want > %d", current, initial)
	}
	var stale *StaleCorpusRevisionError
	if err := c.RequireCorpusRevision(ctx, initial); !errors.As(err, &stale) || stale.Current != current || stale.Expected != initial {
		t.Fatalf("stale revision error = %v, want typed expected=%d current=%d", err, initial, current)
	}
	if err := c.RequireCorpusRevision(ctx, current); err != nil {
		t.Fatal(err)
	}
	job, err := c.CreateJob(ctx, "test", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	deferred, err := c.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deferred != current {
		t.Fatalf("job bookkeeping changed corpus revision from %d to %d", current, deferred)
	}
	if err := c.RequestJobCancellation(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	unchanged, err := c.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != current {
		t.Fatalf("job cancellation changed corpus revision from %d to %d", current, unchanged)
	}
}

func TestActorWritesAdvanceCorpusRevision(t *testing.T) {
	t.Parallel()
	c, _ := openTestCorpus(t)
	ctx := context.Background()
	before, err := c.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyActorIdentityObservation(ctx, "github", "mona", "U_1", nil, "user", "public", time.Unix(1, 0).UTC(), nil); err != nil {
		t.Fatal(err)
	}
	after, err := c.CorpusRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after <= before {
		t.Fatalf("actor write left corpus revision at %d, want greater than %d", after, before)
	}
}

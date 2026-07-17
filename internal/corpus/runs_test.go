package corpus

import (
	"context"
	"testing"
	"time"
)

func TestListRunsBounded(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	for i := 0; i < 3; i++ {
		run, err := c.StartRun(ctx, "sync")
		if err != nil {
			t.Fatalf("start run %d: %v", i, err)
		}
		if err := c.FinishRun(ctx, run.ID, `{"pages":1}`); err != nil {
			t.Fatalf("finish run %d: %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}

	runs, err := c.ListRuns(ctx, 2)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].ID <= runs[1].ID {
		t.Fatalf("expected runs ordered by descending id, got %d then %d", runs[0].ID, runs[1].ID)
	}
	if runs[0].Status != RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", runs[0].Status)
	}

	all, err := c.ListRuns(ctx, 0)
	if err != nil {
		t.Fatalf("list all runs: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(all))
	}
}

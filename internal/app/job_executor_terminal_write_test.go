package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
)

type gatedFinishJobStore struct {
	jobStore
	entered  chan struct{}
	release  chan struct{}
	timedOut chan struct{}
	once     sync.Once
}

func (s *gatedFinishJobStore) TransitionJob(ctx context.Context, id string, transition corpus.JobTransition, result, errStr string) error {
	if transition.From() != corpus.JobStatusRunning {
		return s.jobStore.TransitionJob(ctx, id, transition, result, errStr)
	}
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return s.jobStore.TransitionJob(ctx, id, transition, result, errStr)
	case <-ctx.Done():
		close(s.timedOut)
		return ctx.Err()
	}
}

func TestJobExecutorAllowsNormalTerminalWritePastCleanupTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newJobTestService(t)
	store := &gatedFinishJobStore{
		jobStore: svc.corpus,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
		timedOut: make(chan struct{}),
	}
	jobs, err := newJobExecutorWithConfig(ctx, store, jobExecutorConfig{
		pollInterval: time.Hour, cleanupTimeout: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	svc.jobs = jobs

	id, err := jobs.Submit(ctx, "gated-finish", nil, func(context.Context, func(string, string) error) (any, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("job did not begin its terminal write")
	}
	select {
	case <-store.timedOut:
		t.Fatal("normal terminal write was limited by the cleanup timeout")
	case <-time.After(3 * 25 * time.Millisecond):
	}
	close(store.release)
	waitForJobStatus(t, jobs, id, corpus.JobStatusSucceeded, time.Second)
}

package corpus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCreateAndGetJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	job, err := c.CreateJob(ctx, "sync", `{"repo":"owner/repo"}`)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.ID == "" {
		t.Fatal("job id is empty")
	}
	if job.Status != JobStatusQueued {
		t.Fatalf("status = %q, want %q", job.Status, JobStatusQueued)
	}
	if job.Request != `{"repo":"owner/repo"}` {
		t.Fatalf("request = %q", job.Request)
	}

	got, err := c.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got == nil || got.ID != job.ID {
		t.Fatalf("job not found: %+v", got)
	}
}

func TestListJobsFiltersByStatusAndLimits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	ids := make(map[string]struct{})
	for i := range 5 {
		job, err := c.CreateJob(ctx, fmt.Sprintf("kind-%d", i), `{}`)
		if err != nil {
			t.Fatal(err)
		}
		ids[job.ID] = struct{}{}
		if i%2 == 0 {
			if err := c.StartJob(ctx, job.ID); err != nil {
				t.Fatal(err)
			}
		}
	}

	running, err := c.ListJobs(ctx, JobStatusRunning, 10)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(running) != 3 {
		t.Fatalf("running jobs = %d, want 3", len(running))
	}

	queued, err := c.ListJobs(ctx, JobStatusQueued, 10)
	if err != nil {
		t.Fatalf("list queued: %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("queued jobs = %d, want 2", len(queued))
	}

	limited, err := c.ListJobs(ctx, "", 2)
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("limited jobs = %d, want 2", len(limited))
	}
}

func TestJobStatusTransitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	job, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.StartJob(ctx, job.ID); err != nil {
		t.Fatalf("start job: %v", err)
	}
	job, err = c.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusRunning || job.StartedAt == nil || job.StartedAt.IsZero() {
		t.Fatalf("job not running: %+v", job)
	}

	if err := c.UpdateJobProgress(ctx, job.ID, "50%", `{"items":42}`); err != nil {
		t.Fatalf("update progress: %v", err)
	}
	job, _ = c.GetJob(ctx, job.ID)
	if job.Progress != "50%" || job.Statistics != `{"items":42}` {
		t.Fatalf("progress/statistics mismatch: %+v", job)
	}

	if err := c.TransitionJob(ctx, job.ID, JobStatusRunning, JobStatusSucceeded, `{"done":true}`, ""); err != nil {
		t.Fatalf("complete job: %v", err)
	}
	job, _ = c.GetJob(ctx, job.ID)
	if job.Status != JobStatusSucceeded || job.Result != `{"done":true}` || job.CompletedAt == nil {
		t.Fatalf("job not succeeded: %+v", job)
	}

	// Terminal state protection: cannot restart or transition again.
	if err := c.StartJob(ctx, job.ID); err == nil {
		t.Fatal("expected error starting completed job")
	}
	if err := c.TransitionJob(ctx, job.ID, JobStatusRunning, JobStatusFailed, "", "nope"); err == nil {
		t.Fatal("expected error transitioning completed job")
	}
	if err := c.UpdateJobProgress(ctx, job.ID, "100%", "{}"); err == nil {
		t.Fatal("expected error updating progress on completed job")
	}
}

func TestJobCancellation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	queued, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RequestJobCancellation(ctx, queued.ID); err != nil {
		t.Fatalf("cancel queued: %v", err)
	}
	job, err := c.GetJob(ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusCancelled || job.CompletedAt == nil || job.CancelledAt == nil {
		t.Fatalf("queued job not cancelled: %+v", job)
	}

	// Cancel a running job.
	running, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartJob(ctx, running.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.RequestJobCancellation(ctx, running.ID); err != nil {
		t.Fatalf("request cancellation: %v", err)
	}
	job, err = c.GetJob(ctx, running.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusRunning {
		t.Fatalf("status = %q, want %q", job.Status, JobStatusRunning)
	}
	if job.CancelledAt == nil || job.CancelledAt.IsZero() {
		t.Fatal("cancelled_at not set")
	}

	// Completing a cancelled job as succeeded is blocked; cancelled is allowed.
	if err := c.TransitionJob(ctx, running.ID, JobStatusRunning, JobStatusSucceeded, "", ""); err == nil {
		t.Fatal("expected error completing cancelled job as succeeded")
	}
	if err := c.TransitionJob(ctx, running.ID, JobStatusRunning, JobStatusCancelled, "", "user cancelled"); err != nil {
		t.Fatalf("complete as cancelled: %v", err)
	}

	// Cancelling terminal job fails.
	if err := c.RequestJobCancellation(ctx, running.ID); err == nil {
		t.Fatal("expected error cancelling terminal job")
	}
}

func TestRecordAndListJobEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	job, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RecordJobEvent(ctx, job.ID, "info", "created"); err != nil {
		t.Fatal(err)
	}
	if err := c.RecordJobEvent(ctx, job.ID, "warn", "slow"); err != nil {
		t.Fatal(err)
	}

	events, err := c.ListJobEvents(ctx, job.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Message != "created" || events[1].Message != "slow" {
		t.Fatalf("event order wrong: %+v", events)
	}
}

func TestReconcileInterruptedJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	plain, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartJob(ctx, plain.ID); err != nil {
		t.Fatal(err)
	}

	cancelled, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartJob(ctx, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.RequestJobCancellation(ctx, cancelled.ID); err != nil {
		t.Fatal(err)
	}

	if err := c.ReconcileInterruptedJobs(ctx, time.Second); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	plain, _ = c.GetJob(ctx, plain.ID)
	if plain.Status != JobStatusFailed || plain.Error != "interrupted by restart" {
		t.Fatalf("plain job status = %q, error = %q", plain.Status, plain.Error)
	}

	cancelled, _ = c.GetJob(ctx, cancelled.ID)
	if cancelled.Status != JobStatusCancelled {
		t.Fatalf("cancelled job status = %q, want %q", cancelled.Status, JobStatusCancelled)
	}

	events, err := c.ListJobEvents(ctx, plain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Message != "interrupted by restart" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestConcurrentReadWhileJobRunning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	job, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := c.GetJob(ctx, job.ID)
			if err != nil {
				errs <- err
				return
			}
			if j == nil {
				errs <- fmt.Errorf("job not found")
				return
			}
			if _, err := c.ListJobs(ctx, JobStatusRunning, 100); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent read error: %v", err)
	}

	// Complete the job to leave a clean state.
	if err := c.TransitionJob(ctx, job.ID, JobStatusRunning, JobStatusSucceeded, "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestHeartbeatAndDeleteJobOwner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	ownerID := "owner-1"
	if err := c.RegisterJobOwner(ctx, ownerID, 1, time.Now().UTC()); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	if err := c.HeartbeatJobOwner(ctx, ownerID, time.Now().UTC()); err != nil {
		t.Fatalf("heartbeat owner: %v", err)
	}
	if err := c.DeleteJobOwner(ctx, ownerID); err != nil {
		t.Fatalf("delete owner: %v", err)
	}
	if err := c.HeartbeatJobOwner(ctx, ownerID, time.Now().UTC()); !errors.Is(err, ErrJobOwnerNotFound) {
		t.Fatalf("expected ErrJobOwnerNotFound, got %v", err)
	}
}

func TestJobListOrderingAndBounds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	for i := range 5 {
		_, err := c.CreateJob(ctx, "sync", fmt.Sprintf(`{"i":%d}`, i))
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}

	jobs, err := c.ListJobs(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 {
		t.Fatalf("jobs = %d, want 5", len(jobs))
	}
	for i := 1; i < len(jobs); i++ {
		if jobs[i-1].CreatedAt.Before(jobs[i].CreatedAt) {
			t.Fatalf("jobs not newest first at %d", i)
		}
	}
}

func TestStartJobAsClaimsOwner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	owner := "owner-1"
	if err := c.RegisterJobOwner(ctx, owner, 1, time.Now().UTC()); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	job, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartJobAs(ctx, job.ID, owner); err != nil {
		t.Fatalf("start job as owner: %v", err)
	}

	var got string
	if err := c.db.QueryRowContext(ctx, `SELECT owner_id FROM jobs WHERE id = ?`, job.ID).Scan(&got); err != nil {
		t.Fatalf("select owner: %v", err)
	}
	if got != owner {
		t.Fatalf("owner_id = %q, want %q", got, owner)
	}

	if err := c.StartJobAs(ctx, job.ID, "owner-2"); err == nil {
		t.Fatal("expected error claiming already-running job")
	}
}

func TestReconcileRespectsLiveOwners(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	liveOwner := "live-owner"
	if err := c.RegisterJobOwner(ctx, liveOwner, 1, time.Now().UTC()); err != nil {
		t.Fatalf("register live owner: %v", err)
	}
	liveJob, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartJobAs(ctx, liveJob.ID, liveOwner); err != nil {
		t.Fatalf("start live job: %v", err)
	}

	staleOwner := "stale-owner"
	if err := c.RegisterJobOwner(ctx, staleOwner, 2, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("register stale owner: %v", err)
	}
	staleJob, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartJobAs(ctx, staleJob.ID, staleOwner); err != nil {
		t.Fatalf("start stale job: %v", err)
	}
	if err := c.RequestJobCancellation(ctx, staleJob.ID); err != nil {
		t.Fatalf("request cancellation: %v", err)
	}

	if err := c.ReconcileInterruptedJobs(ctx, 30*time.Second); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	liveJob, _ = c.GetJob(ctx, liveJob.ID)
	if liveJob.Status != JobStatusRunning {
		t.Fatalf("live job reconciled: status=%q", liveJob.Status)
	}

	staleJob, _ = c.GetJob(ctx, staleJob.ID)
	if staleJob.Status != JobStatusCancelled {
		t.Fatalf("stale job status = %q, want cancelled", staleJob.Status)
	}
	if staleJob.Error != "interrupted by restart (cancellation requested)" {
		t.Fatalf("stale job error = %q", staleJob.Error)
	}
}

func TestRequestJobCancellationRowsAffected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	if err := c.RequestJobCancellation(ctx, "missing-id"); err == nil {
		t.Fatal("expected error cancelling missing job")
	}

	job, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.TransitionJob(ctx, job.ID, JobStatusRunning, JobStatusSucceeded, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := c.RequestJobCancellation(ctx, job.ID); err == nil {
		t.Fatal("expected error cancelling terminal job")
	}
}

func TestZeroRowTransitionPropagatesGetJobError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)

	corrupt := func(id, status string) {
		t.Helper()
		if _, err := c.db.ExecContext(ctx, `UPDATE jobs SET status = ?, created_at = 'corrupt' WHERE id = ?`, status, id); err != nil {
			t.Fatalf("corrupt job: %v", err)
		}
	}

	start, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	corrupt(start.ID, JobStatusSucceeded)
	if err := c.StartJob(ctx, start.ID); err == nil {
		t.Fatal("expected error starting corrupt completed job")
	} else if strings.Contains(err.Error(), "job not found") {
		t.Fatalf("expected GetJob error to be propagated, got: %v", err)
	}

	progress, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	corrupt(progress.ID, JobStatusSucceeded)
	if err := c.UpdateJobProgress(ctx, progress.ID, "50%", "{}"); err == nil {
		t.Fatal("expected error updating progress on corrupt completed job")
	} else if strings.Contains(err.Error(), "job not found") {
		t.Fatalf("expected GetJob error to be propagated, got: %v", err)
	}

	transition, err := c.CreateJob(ctx, "sync", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	corrupt(transition.ID, JobStatusSucceeded)
	if err := c.TransitionJob(ctx, transition.ID, JobStatusRunning, JobStatusFailed, "", ""); err == nil {
		t.Fatal("expected error transitioning corrupt completed job")
	} else if strings.Contains(err.Error(), "job not found") {
		t.Fatalf("expected GetJob error to be propagated, got: %v", err)
	}
}

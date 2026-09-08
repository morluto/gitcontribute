package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

func TestReconciliationOwnsQueuedJobsAndStopsAbandonedWork(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(fmt.Sprintf("queued=%t", queued), func(t *testing.T) {
			ctx := context.Background()
			svc := newJobTestService(t)
			jobs := newJobExecutorOnService(t, svc, jobExecutorConfig{maxConcurrentJobs: 1, heartbeatInterval: time.Hour, pollInterval: 10 * time.Millisecond})
			started, stopped := make(chan struct{}), make(chan struct{})
			running, err := jobs.Submit(ctx, "blocked", nil, func(ctx context.Context, _ func(string, string) error) (any, error) {
				close(started)
				<-ctx.Done()
				close(stopped)
				return nil, ctx.Err()
			})
			if err != nil {
				t.Fatal(err)
			}
			<-started
			id := running
			if queued {
				id, err = jobs.Submit(ctx, "waiting", nil, func(context.Context, func(string, string) error) (any, error) {
					t.Error("abandoned queued work executed")
					return struct{}{}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			other, err := corpus.Open(ctx, svc.databasePath())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = other.Close() }()
			if err := other.ReconcileInterruptedJobs(ctx, time.Minute); err != nil {
				t.Fatal(err)
			}
			live, err := other.GetJob(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if live.State.Status().Terminal() {
				t.Fatalf("live owner's job reconciled: %s", live.State.Status())
			}
			if err := other.RegisterJobOwner(ctx, jobs.ownerID, 1, time.Now().Add(-time.Hour)); err != nil {
				t.Fatal(err)
			}
			if err := other.ReconcileInterruptedJobs(ctx, time.Minute); err != nil {
				t.Fatal(err)
			}
			abandoned, err := other.GetJob(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if abandoned.State.Status() != corpus.JobStatusFailed {
				t.Fatalf("abandoned job remains %s", abandoned.State.Status())
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("reconciled worker kept running")
			}
		})
	}
}

func TestRelatedWorkRejectsUnrelatedFindings(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.UpsertRepository(ctx, corpus.Repository{Owner: "owner", Name: "repo", ExternalID: "ranking"}, "{}")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i, title := range []string{"quasar nebula", "orchid nectar", "quasar detector"} {
		_, err := c.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: i + 1, State: "open", Title: title, SourceCreatedAt: now, SourceUpdatedAt: now}, "{}")
		if err != nil {
			t.Fatal(err)
		}
	}
	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, "commit", "")
	if err != nil {
		t.Fatal(err)
	}
	h, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{Title: "quasar nebula", Description: "", Category: investigation.CategoryBug})
	if err != nil {
		t.Fatal(err)
	}
	dup, err := svc.CheckHypothesisDuplicates(ctx, h.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	col, err := svc.CheckHypothesisCollisions(ctx, h.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	for name, findings := range map[string][]evidence.Evidence{"duplicates": dup.Findings, "collisions": col.Findings} {
		if len(findings) != 2 {
			t.Errorf("%s returned %d findings, want exact and weak positive matches only", name, len(findings))
		}
		for _, f := range findings {
			if strings.Contains(f.Description, "orchid nectar") {
				t.Errorf("%s includes unrelated finding: %s", name, f.Description)
			}
			if f.Relation != evidence.RelationInconclusive {
				t.Errorf("%s lexical candidate claims %s evidence", name, f.Relation)
			}
		}
		if len(findings) > 0 && !strings.Contains(findings[0].Description, "quasar nebula") {
			t.Errorf("%s exact title was not first", name)
		}
	}
}

func TestRelatedWorkDoesNotHideCandidateBound(t *testing.T) {
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.UpsertRepository(ctx, corpus.Repository{Owner: "owner", Name: "repo", ExternalID: "bounded-ranking"}, "{}")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 1; i <= minCandidateLimit+1; i++ {
		title := "orchid nectar"
		if i == 1 {
			title = "quasar nebula"
		}
		if _, err := c.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: domain.PullRequestKind, Number: i, State: "open", Title: title, SourceCreatedAt: now, SourceUpdatedAt: now}, "{}"); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.AdvanceFacet(ctx, repo.ID, nil, "threads", now, true, 0); err != nil {
		t.Fatal(err)
	}
	inv, err := svc.StartInvestigation(ctx, contracts.RepoRef{Owner: "owner", Repo: "repo"}, "commit", "")
	if err != nil {
		t.Fatal(err)
	}
	h, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{Title: "quasar nebula", Category: investigation.CategoryBug})
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).CheckDuplicates(ctx, mcpcontract.CheckDuplicatesInput{Target: "hypothesis", ID: h.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || !out.Truncated || out.Recovery == nil {
		t.Fatalf("bounded empty result claims completeness: %+v", out)
	}
	larger, err := (&MCPReader{svc}).CheckDuplicates(ctx, mcpcontract.CheckDuplicatesInput{Target: "hypothesis", ID: h.ID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if larger.Total != 1 || larger.Truncated {
		t.Fatalf("larger bound should recover exact match: %+v", larger)
	}
}

func TestLostOwnerStopsWorkAndAdmission(t *testing.T) {
	ctx := context.Background()
	svc := newJobTestService(t)
	jobs := newJobExecutorOnService(t, svc, jobExecutorConfig{heartbeatInterval: 10 * time.Millisecond})
	started := make(chan struct{})
	id, err := jobs.Submit(ctx, "owner-loss", nil, func(ctx context.Context, _ func(string, string) error) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := svc.corpus.DeleteJobOwner(ctx, jobs.ownerID); err != nil {
		t.Fatal(err)
	}
	waitForJobStatus(t, jobs, id, corpus.JobStatusCancelled, time.Second)
	if _, err := jobs.Submit(ctx, "after-owner-loss", nil, func(context.Context, func(string, string) error) (any, error) {
		t.Error("unowned work executed")
		return struct{}{}, nil
	}); err == nil {
		t.Fatal("executor admitted work after losing ownership")
	}
}

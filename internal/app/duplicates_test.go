package app

import (
	"context"
	"testing"
	"time"

	cli "github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func TestDuplicateAndCollisionChecks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()

	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	repo, err := c.UpsertRepository(ctx, corpus.Repository{
		Owner:           "owner",
		Name:            "repo",
		ExternalID:      "R_1",
		Description:     "test repo",
		DefaultBranch:   "main",
		SourceCreatedAt: time.Now().UTC(),
		SourceUpdatedAt: time.Now().UTC(),
	}, "{}")
	if err != nil {
		t.Fatalf("upsert repository: %v", err)
	}
	now := time.Now().UTC()
	if _, err := c.UpsertThread(ctx, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindIssue,
		Number:          1,
		State:           "open",
		Title:           "race in parser",
		Body:            "data race under load",
		Author:          "alice",
		SourceCreatedAt: now,
		SourceUpdatedAt: now,
	}, "{}"); err != nil {
		t.Fatalf("upsert issue: %v", err)
	}
	if _, err := c.UpsertThread(ctx, corpus.Thread{
		RepositoryID:    repo.ID,
		Kind:            corpus.ThreadKindPullRequest,
		Number:          2,
		State:           "open",
		Title:           "fix race in parser",
		Body:            "addresses the panic",
		Author:          "bob",
		SourceCreatedAt: now,
		SourceUpdatedAt: now,
	}, "{}"); err != nil {
		t.Fatalf("upsert pr: %v", err)
	}

	inv, err := svc.StartInvestigation(ctx, cli.RepoRef{Owner: "owner", Repo: "repo"}, "abc", "")
	if err != nil {
		t.Fatalf("start investigation: %v", err)
	}
	h, err := svc.CreateHypothesis(ctx, inv.ID, investigation.CreateHypothesisInput{
		Title:       "race in parser",
		Description: "data race under load",
		Category:    investigation.CategoryBug,
	})
	if err != nil {
		t.Fatalf("create hypothesis: %v", err)
	}

	dup, err := svc.CheckHypothesisDuplicates(ctx, h.ID, 0)
	if err != nil {
		t.Fatalf("check duplicates: %v", err)
	}
	if dup.Total == 0 || dup.Limit != defaultNeighborsLimit {
		t.Fatalf("duplicate result = %+v", dup)
	}
	if _, err := svc.CheckHypothesisDuplicates(ctx, h.ID, maxResultLimit+1); err == nil {
		t.Fatal("oversized duplicate limit was accepted")
	}

	coll, err := svc.CheckHypothesisCollisions(ctx, h.ID, 0)
	if err != nil {
		t.Fatalf("check collisions: %v", err)
	}
	if coll.Total == 0 || coll.Limit != defaultNeighborsLimit {
		t.Fatalf("collision result = %+v", coll)
	}
	for _, f := range coll.Findings {
		if f.Relation != evidence.RelationContradicting {
			t.Fatalf("collision finding should be contradicting, got %q", f.Relation)
		}
	}
}

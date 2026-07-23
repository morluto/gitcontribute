package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/concern"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

func TestConcernWorkflowRemainsLocalAndPromotes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	created, err := svc.CreateConcern(ctx, cli.ConcernCreateOptions{
		Repo: cli.RepoRef{Owner: "owner", Repo: "repo"}, CommitSHA: "abc",
		Title: "flaky MCP test", ProblemStatement: "fails intermittently", Confidence: 0.4,
		EvidenceIDs: []string{"evidence-1"}, Unknowns: []string{"scheduler timing"},
		Notes: "private scratch note",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "untriaged" || created.Freshness != "unknown" {
		t.Fatalf("unexpected created concern: %+v", created)
	}
	empty := ""
	updated, err := svc.UpdateConcern(ctx, created.ID, cli.ConcernUpdateOptions{Notes: &empty})
	if err != nil || updated.Notes != "" {
		t.Fatalf("clear concern notes: result=%+v err=%v", updated, err)
	}
	if _, err := svc.LinkConcern(ctx, created.ID, cli.ConcernLinkOptions{Kind: "duplicate_candidate", TargetType: "thread", TargetID: "owner/repo:issue#7"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetConcernStatus(ctx, created.ID, "accepted", "worth investigation"); err != nil {
		t.Fatal(err)
	}
	promoted, err := svc.PromoteConcern(ctx, created.ID, cli.ConcernPromoteOptions{
		Kind: "opportunity", Category: "testing", Scope: "MCP transport tests", Impact: "flaky CI", ExpectedEffort: "small",
	})
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Status != "promoted" || promoted.Promotion == nil || promoted.Promotion.OpportunityID == "" || len(promoted.Links) != 3 {
		t.Fatalf("unexpected promotion: %+v", promoted)
	}
	c, err := svc.openReadOnlyCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	opportunity, err := c.GetOpportunity(ctx, promoted.Promotion.OpportunityID)
	if err != nil {
		t.Fatal(err)
	}
	if len(opportunity.EvidenceIDs) != 1 || opportunity.EvidenceIDs[0] != "evidence-1" {
		t.Fatalf("promotion lost evidence IDs: %+v", opportunity)
	}
	listed, err := svc.ListConcerns(ctx, cli.ConcernListOptions{Repo: created.Repo, Query: "scheduler", Limit: 10})
	if err != nil || listed.Total != 1 {
		t.Fatalf("search promoted concern: result=%+v err=%v", listed, err)
	}
}

func TestConcernFreshnessIsDerivedFromCurrentCorpus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newLocalService(t)
	defer func() { _ = svc.Close() }()
	c, err := svc.openCorpus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "R1", time.Unix(10, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	revision, err := c.CurrentSourceRevision(ctx, evidence.SourceSubject{Kind: evidence.SourceSubjectRepository, Owner: "owner", Repo: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.createConcern(ctx, &concern.Concern{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, CommitSHA: "abc", Title: "metadata", ProblemStatement: "may be stale",
		Confidence: 0.2, SourceProvenance: []evidence.SourceRevision{*revision},
	})
	if err != nil || created.Freshness != "fresh" {
		t.Fatalf("initial concern freshness: result=%+v err=%v", created, err)
	}
	if _, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "R2", time.Unix(11, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	shown, err := svc.ShowConcern(ctx, created.ID)
	if err != nil || shown.Freshness != "stale" {
		t.Fatalf("advanced concern freshness: result=%+v err=%v", shown, err)
	}
}

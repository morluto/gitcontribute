package corpus

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/concern"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func TestConcernPersistenceSearchAndLinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, err := Open(ctx, filepath.Join(t.TempDir(), "concerns.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	svc := concern.NewService(c)
	first, err := svc.Create(ctx, &concern.Concern{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, CommitSHA: "abc",
		Title: "flaky MCP test", ProblemStatement: "transport occasionally stalls", Confidence: 0.5,
		Unknowns: []string{"scheduler timing"}, SuccessCriterion: "100 repeated runs pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(ctx, &concern.Concern{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, CommitSHA: "def",
		Title: "live read boundary", ProblemStatement: "offline read may contact network", Confidence: 0.7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Link(ctx, first.ID, concern.Link{Kind: concern.LinkRelated, TargetType: "concern", TargetID: second.ID, Note: "same adapter"}); err != nil {
		t.Fatal(err)
	}
	items, err := svc.List(ctx, concern.Filter{Repo: domain.RepoRef{Owner: "OWNER", Repo: "REPO"}, Query: "scheduler", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("unexpected search results: %+v", items)
	}
	shown, err := svc.Get(ctx, first.ID)
	if err != nil || len(shown.Links) != 1 {
		t.Fatalf("linked concern: item=%+v err=%v", shown, err)
	}
	if _, err := svc.Get(ctx, "missing"); !errors.Is(err, concern.ErrNotFound) {
		t.Fatalf("missing error = %v", err)
	}
}

func TestPromoteConcernIsAtomic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, err := Open(ctx, filepath.Join(t.TempDir(), "concerns.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	svc := concern.NewService(c)
	item, err := svc.Create(ctx, &concern.Concern{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, CommitSHA: "abc",
		Title: "flaky test", ProblemStatement: "fails intermittently", Confidence: 0.6,
		EvidenceIDs: []string{"evidence-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	inv := &investigation.Investigation{ID: "inv-1", Repo: item.Repo, CommitSHA: item.CommitSHA, Status: investigation.InvestigationOpen, CreatedAt: now, UpdatedAt: now}
	hypothesis := &investigation.Hypothesis{ID: "hyp-1", InvestigationID: inv.ID, Title: item.Title, Description: item.ProblemStatement, Category: investigation.CategoryTesting, Status: investigation.HypothesisPromoted, CreatedAt: now, UpdatedAt: now}
	inv.SeedHypothesisID = hypothesis.ID
	opportunity := &investigation.Opportunity{ID: "opp-1", InvestigationID: inv.ID, HypothesisID: hypothesis.ID, Title: item.Title, ProblemStatement: item.ProblemStatement, Category: investigation.CategoryTesting, Confidence: item.Confidence, EvidenceIDs: item.EvidenceIDs, Status: investigation.OpportunityHypothesis, CreatedAt: now, UpdatedAt: now}
	if _, err := c.PromoteConcern(ctx, item.ID, inv, hypothesis, opportunity); !errors.Is(err, concern.ErrInvalidTransition) {
		t.Fatalf("untriaged promotion error = %v", err)
	}
	if _, err := c.GetInvestigation(ctx, inv.ID); !errors.Is(err, investigation.ErrNotFound) {
		t.Fatalf("failed promotion persisted investigation: %v", err)
	}
	if _, err := svc.SetStatus(ctx, item.ID, concern.StatusAccepted, "triaged"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Link(ctx, item.ID, concern.Link{Kind: concern.LinkRelated, TargetType: "thread", TargetID: "owner/repo:issue#7"}); err != nil {
		t.Fatal(err)
	}
	promoted, err := c.PromoteConcern(ctx, item.ID, inv, hypothesis, opportunity)
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Status != concern.StatusPromoted || promoted.Promotion == nil || promoted.Promotion.OpportunityID != opportunity.ID || len(promoted.Links) != 3 {
		t.Fatalf("unexpected promoted concern: %+v", promoted)
	}
	if got, err := c.GetOpportunity(ctx, opportunity.ID); err != nil || len(got.EvidenceIDs) != 1 {
		t.Fatalf("opportunity provenance lost: got=%+v err=%v", got, err)
	}
}

func TestConcernSearchTreatsFTSOperatorsLiterally(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, err := Open(ctx, filepath.Join(t.TempDir(), "concerns.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	svc := concern.NewService(c)
	if _, err := svc.Create(ctx, &concern.Concern{
		Repo: domain.RepoRef{Owner: "o", Repo: "r"}, CommitSHA: "abc",
		Title: "OR token", ProblemStatement: "literal operator", Confidence: 0.1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(ctx, concern.Filter{Query: `OR "`, Limit: 10}); err != nil {
		t.Fatalf("literal FTS query failed: %v", err)
	}
}

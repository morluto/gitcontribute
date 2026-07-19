package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

func TestFindPortfolioOverlapsIsolatesInvalidCandidatesAndMissingPullRequests(t *testing.T) {
	ctx := context.Background()
	svc := newSearchTestService(t)
	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "acme", Name: "rocket"}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 1, State: "open", SourceUpdatedAt: time.Unix(10, 0).UTC()}, `{}`); err != nil {
		t.Fatal(err)
	}
	out, err := (&MCPReader{svc}).FindPortfolioOverlaps(ctx, mcpserver.FindPortfolioOverlapsInput{
		Candidates:   []mcpserver.PortfolioSubjectInput{{Kind: "invalid", Ref: "x"}, {Kind: corpus.PortfolioSubjectOpportunity, Ref: "opp-1"}},
		PullRequests: []mcpserver.ThreadRef{{Owner: "acme", Repo: "rocket", Number: 1}, {Owner: "acme", Repo: "missing", Number: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "partial" || len(out.Items) != 2 || out.Items[0].Status != "failed" || out.Items[1].Status != "retryable" || out.Items[1].Reason != "comparison_set_incomplete" {
		t.Fatalf("overlap batch = %+v", out)
	}
}

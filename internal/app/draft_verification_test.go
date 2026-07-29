package app

import (
	"context"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/research"
)

func TestVerifyPublishedDraftExactMismatchAndStale(t *testing.T) {
	fixture := newResearchFixture(t)
	started, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	opportunity, err := fixture.svc.PromoteOpportunity(fixture.ctx, started.Hypothesis.ID, "verify bytes", "draft", "safe handoff", "small", 0.8)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := fixture.svc.PrepareIssue(fixture.ctx, opportunity.ID, contracts.PrepareIssueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	verify := func() *contracts.PublishedDraftVerification {
		t.Helper()
		result, err := fixture.svc.VerifyPublishedDraft(context.Background(), contracts.VerifyPublishedDraftInput{
			DraftID: draft.ID, Revision: draft.Revision, Owner: "owner", Repo: "repo", Kind: "issue", Number: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := verify(); result.Status != "unknown" {
		t.Fatalf("stale result = %+v", result)
	}
	observedAt := time.Now().UTC().Add(time.Minute)
	_, err = fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
		RepositoryID: fixture.repoID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open",
		Title: draft.Title, Body: draft.Body, SourceUpdatedAt: observedAt,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if result := verify(); result.Status != "exact_match" || result.ObservedAt == "" {
		t.Fatalf("exact result = %+v", result)
	}
	_, err = fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
		RepositoryID: fixture.repoID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open",
		Title: draft.Title, Body: draft.Body + "\nliteral change", SourceUpdatedAt: observedAt.Add(time.Minute),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if result := verify(); result.Status != "mismatch" || result.Difference == nil || result.Difference.FirstDifferingLine == 0 {
		t.Fatalf("mismatch result = %+v", result)
	}
}

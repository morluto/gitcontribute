package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
	"github.com/morluto/gitcontribute/internal/mcpserver"
	"github.com/morluto/gitcontribute/internal/research"
)

func TestStartInvestigationFromThreadPreservesExactBaselineAndReusesOpenPair(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	ref := research.ThreadRef{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 1}
	thread, err := fixture.svc.corpus.GetThreadByNumber(fixture.ctx, fixture.repoID, 1)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := fixture.svc.corpus.GetThreadObservationRevision(fixture.ctx, thread.ID, thread.SourceUpdatedAt, thread.ObservationSequence)
	if err != nil || observation == nil {
		t.Fatalf("fixture observation = (%+v, %v)", observation, err)
	}

	first, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	assertThreadStartResult(t, first, thread, observation)

	newer, err := fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
		RepositoryID: fixture.repoID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open",
		Title: "new title after baseline", Body: "new body after baseline", Author: "alice",
		SourceCreatedAt: thread.SourceCreatedAt, SourceUpdatedAt: fixture.now.Add(time.Hour),
	}, `{"revision":"new"}`)
	if err != nil {
		t.Fatal(err)
	}
	if newer.ObservationSequence == observation.ObservationSequence {
		t.Fatal("fixture did not create a newer projection revision")
	}

	second, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.Investigation.ID != first.Investigation.ID || second.Hypothesis.ID != first.Hypothesis.ID ||
		second.Hypothesis.Title != thread.Title || second.Investigation.ThreadBaseline.ObservationID != observation.ID {
		t.Fatalf("repeated start drifted baseline = %+v", second)
	}
	investigations, err := fixture.svc.corpus.ListInvestigations(fixture.ctx)
	if err != nil || len(investigations) != 1 {
		t.Fatalf("investigations = (%+v, %v)", investigations, err)
	}
	hypotheses, err := fixture.svc.corpus.ListHypotheses(fixture.ctx, first.Investigation.ID)
	if err != nil || len(hypotheses) != 1 {
		t.Fatalf("hypotheses = (%+v, %v)", hypotheses, err)
	}
}

func TestStartInvestigationFromThreadBoundsDescription(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	thread, err := fixture.svc.corpus.GetThreadByNumber(fixture.ctx, fixture.repoID, 1)
	if err != nil {
		t.Fatal(err)
	}
	thread.Body = strings.Repeat("界", maxThreadSeedDescription+10)
	thread.SourceUpdatedAt = fixture.now.Add(time.Hour)
	if _, err := fixture.svc.corpus.UpsertThread(fixture.ctx, *thread, `{"large":true}`); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Number: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Investigation.ThreadBaseline.DescriptionTruncated || utf8.RuneCountInString(result.Hypothesis.Description) != maxThreadSeedDescription+1 {
		t.Fatalf("bounded description = baseline:%+v runes:%d", result.Investigation.ThreadBaseline, utf8.RuneCountInString(result.Hypothesis.Description))
	}
}

func TestStartInvestigationFromPullRequestUsesResolvedKind(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	result, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Number: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Investigation.ThreadBaseline.Kind != string(domain.PullRequestKind) ||
		result.Investigation.ThreadBaseline.Ref != "pull_request:owner/repo#9" || len(result.Hypothesis.Links) != 1 ||
		result.Hypothesis.Links[0].Kind != string(domain.PullRequestKind) {
		t.Fatalf("pull-request baseline = %+v", result)
	}
}

func TestMCPStartInvestigationFromStoredThreadCreatesBaselineHypothesis(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	out, err := (&MCPReader{Service: fixture.svc}).StartInvestigation(fixture.ctx, mcpserver.StartInvestigationInput{
		Owner: "owner", Repo: "repo", Number: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ID == "" || out.HypothesisTotal != 1 || len(out.Hypotheses) != 1 || out.Hypotheses[0].Title == "" {
		t.Fatalf("atomic thread investigation = %+v", out)
	}
}

func TestStartInvestigationFromThreadErrorsAndCancellation(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	_, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 9})
	var cliErr *cli.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != cli.ExitNotFound || !errors.Is(err, research.ErrThreadKindMismatch) {
		t.Fatalf("kind mismatch error = %v", err)
	}
	_, err = fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{Repo: repo, Number: 404})
	if !errors.As(err, &cliErr) || cliErr.Code != cli.ExitNotFound || !errors.Is(err, research.ErrThreadNotFound) {
		t.Fatalf("missing thread error = %v", err)
	}
	_, err = fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "other", Repo: "repo"}, Number: 1,
	})
	if !errors.As(err, &cliErr) || cliErr.Code != cli.ExitNotFound {
		t.Fatalf("repository isolation error = %v", err)
	}

	ctx, cancel := context.WithCancel(fixture.ctx)
	cancel()
	_, err = fixture.svc.StartInvestigationFromThread(ctx, research.ThreadRef{Repo: repo, Number: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start error = %v", err)
	}
	investigations, listErr := fixture.svc.corpus.ListInvestigations(fixture.ctx)
	if listErr != nil || len(investigations) != 0 {
		t.Fatalf("failed starts wrote investigations = (%+v, %v)", investigations, listErr)
	}
}

func TestGetThreadObservationRevisionSelectsExactProjectionRevision(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	thread, err := fixture.svc.corpus.GetThreadByNumber(fixture.ctx, fixture.repoID, 1)
	if err != nil {
		t.Fatal(err)
	}
	oldSequence := thread.ObservationSequence
	oldSourceTime := thread.SourceUpdatedAt
	if _, err := fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
		RepositoryID: fixture.repoID, Kind: thread.Kind, Number: thread.Number, State: thread.State,
		Title: "newer", SourceCreatedAt: thread.SourceCreatedAt, SourceUpdatedAt: fixture.now.Add(2 * time.Hour),
	}, `{"newer":true}`); err != nil {
		t.Fatal(err)
	}
	oldObservation, err := fixture.svc.corpus.GetThreadObservationRevision(fixture.ctx, thread.ID, oldSourceTime, oldSequence)
	if err != nil || oldObservation == nil || oldObservation.ObservationSequence != oldSequence {
		t.Fatalf("old observation = (%+v, %v)", oldObservation, err)
	}
	if _, err := fixture.svc.corpus.GetThreadObservationRevision(fixture.ctx, thread.ID, oldSourceTime, oldSequence+999); !errors.Is(err, corpus.ErrThreadObservationRevisionNotFound) {
		t.Fatalf("missing exact revision error = %v", err)
	}
}

func assertThreadStartResult(t *testing.T, result *cli.ThreadInvestigationResult, thread *corpus.Thread, observation *corpus.ThreadObservation) {
	t.Helper()
	if !result.Created || result.Investigation.ThreadBaseline.ObservationID != observation.ID ||
		result.Investigation.ThreadBaseline.ObservationSequence != observation.ObservationSequence ||
		result.Investigation.ThreadBaseline.SourceUpdatedAt != formatTime(observation.SourceUpdatedAt) {
		t.Fatalf("first baseline = %+v", result)
	}
	if result.Hypothesis.Title != thread.Title || result.Hypothesis.Description != strings.TrimSpace(thread.Body) ||
		result.Hypothesis.Category != string(investigation.CategoryOther) || len(result.Hypothesis.SourceRefs) != 1 || len(result.Hypothesis.AuditTrail) != 1 {
		t.Fatalf("first hypothesis = %+v", result.Hypothesis)
	}
}

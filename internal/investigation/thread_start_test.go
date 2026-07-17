package investigation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestStartFromThreadCreatesAuditedIdempotentPair(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo, &fakeEvidenceStore{})
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	baseline := ThreadBaseline{
		Repo: domain.RepoRef{Owner: "Owner", Repo: "Repo"}, Kind: domain.IssueKind, Number: 42,
		ObservationID: 7, ObservationSequence: 3, SourceUpdatedAt: now.Add(-time.Hour), ObservedAt: now,
		Source: domain.SourceRef{Source: "github:rest", URL: "https://api.github.com/repos/Owner/Repo/issues/42", ObservedAt: now, AsOf: now.Add(-time.Hour)},
	}
	first, err := service.StartFromThread(context.Background(), StartFromThreadInput{
		Baseline: baseline, Title: " Fix retries ", Description: "stored body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.Investigation.ThreadBaseline.ObservationID != 7 || first.Investigation.SeedHypothesisID != first.Hypothesis.ID {
		t.Fatalf("first start = %+v", first)
	}
	if first.Hypothesis.Title != "Fix retries" || first.Hypothesis.Description != "stored body" || first.Hypothesis.Category != CategoryOther {
		t.Fatalf("seed hypothesis = %+v", first.Hypothesis)
	}
	if len(first.Hypothesis.SourceRefs) != 1 || len(first.Hypothesis.Links) != 1 || len(first.Hypothesis.AuditTrail) != 1 ||
		!strings.Contains(first.Hypothesis.AuditTrail[0].Rationale, "observation 7") {
		t.Fatalf("seed provenance/audit = %+v", first.Hypothesis)
	}

	second, err := service.StartFromThread(context.Background(), StartFromThreadInput{
		Baseline: baseline, Title: "new title must not replace baseline", Description: "new body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.Investigation.ID != first.Investigation.ID || second.Hypothesis.ID != first.Hypothesis.ID || second.Hypothesis.Title != "Fix retries" {
		t.Fatalf("repeated start = %+v", second)
	}
}

func TestStartFromThreadRejectsInvalidBaselineAndCancellation(t *testing.T) {
	service := NewService(newFakeRepo(), &fakeEvidenceStore{})
	_, err := service.StartFromThread(context.Background(), StartFromThreadInput{Title: "title"})
	if !errors.Is(err, ErrInvalidThreadBaseline) {
		t.Fatalf("invalid baseline error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.StartFromThread(ctx, StartFromThreadInput{
		Baseline: ThreadBaseline{
			Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Kind: domain.IssueKind, Number: 1,
			ObservationID: 1, ObservationSequence: 1,
			Source: domain.SourceRef{Source: "github:rest", URL: "https://api.github.com/repos/o/r/issues/1"},
		},
		Title: "title",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start error = %v", err)
	}
}

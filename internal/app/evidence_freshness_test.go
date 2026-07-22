package app

import (
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/research"
)

func TestEvidenceFreshnessFromThreadInvestigationBaseline(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	ref := research.ThreadRef{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 1}
	started, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, ref)
	if err != nil {
		t.Fatalf("start from thread: %v", err)
	}

	recorded, err := fixture.svc.RecordEvidence(fixture.ctx, RecordEvidenceInput{
		InvestigationID: started.Investigation.ID,
		Type:            string(evidence.EvidenceTypeGitHubSource),
		Relation:        string(evidence.RelationSupporting),
		Description:     "maintainer requested a cancellation regression test",
	})
	if err != nil {
		t.Fatalf("record github evidence: %v", err)
	}
	if len(recorded.SourceRefs) != 1 || len(recorded.SourceProvenance) != 1 {
		t.Fatalf("baseline source was not inherited: %+v", recorded)
	}
	if got := recorded.SourceProvenance[0]; got.Subject.Kind != evidence.SourceSubjectThread ||
		got.Subject.ThreadKind != string(domain.IssueKind) || got.Subject.Number != 1 ||
		got.ObservationSequence != started.Investigation.ThreadBaseline.ObservationSequence {
		t.Fatalf("unexpected inherited provenance: %+v", got)
	}

	shown, err := fixture.svc.ShowEvidence(fixture.ctx, started.Investigation.ID)
	if err != nil {
		t.Fatalf("show evidence: %v", err)
	}
	initial := evidenceItemByID(t, shown.Evidence, recorded.ID)
	if initial.Freshness != string(evidence.FreshnessFresh) || !strings.Contains(initial.FreshnessReason, "match current projections") {
		t.Fatalf("initial freshness = %+v", initial)
	}
	if len(initial.SourceRefs) != 1 || len(initial.SourceProvenance) != 1 {
		t.Fatalf("source trace missing from show output: %+v", initial)
	}

	thread, err := fixture.svc.corpus.GetThreadByNumber(fixture.ctx, fixture.repoID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
		RepositoryID: fixture.repoID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open",
		Title: "Retry parser cancellation updated", Body: thread.Body, Author: thread.Author,
		AuthorAssociation: thread.AuthorAssociation, Labels: thread.Labels,
		SourceCreatedAt: thread.SourceCreatedAt, SourceUpdatedAt: fixture.now.Add(time.Hour),
	}, `{"revision":"newer"}`); err != nil {
		t.Fatal(err)
	}

	shown, err = fixture.svc.ShowEvidence(fixture.ctx, started.Investigation.ID)
	if err != nil {
		t.Fatalf("show stale evidence: %v", err)
	}
	stale := evidenceItemByID(t, shown.Evidence, recorded.ID)
	if stale.Freshness != string(evidence.FreshnessStale) || !strings.Contains(stale.FreshnessReason, "advanced from") {
		t.Fatalf("stale freshness = %+v", stale)
	}
}

func TestEvidenceFreshnessManualObservationIsNotApplicable(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	started, err := fixture.svc.StartInvestigationFromThread(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 1,
	})
	if err != nil {
		t.Fatalf("start from thread: %v", err)
	}
	recorded, err := fixture.svc.RecordEvidence(fixture.ctx, RecordEvidenceInput{
		InvestigationID: started.Investigation.ID,
		Type:            string(evidence.EvidenceTypeManualObservation),
		Relation:        string(evidence.RelationSupporting),
		Description:     "local reproduction still fails on main",
	})
	if err != nil {
		t.Fatalf("record manual evidence: %v", err)
	}

	shown, err := fixture.svc.ShowEvidence(fixture.ctx, started.Investigation.ID)
	if err != nil {
		t.Fatalf("show evidence: %v", err)
	}
	item := evidenceItemByID(t, shown.Evidence, recorded.ID)
	if item.Freshness != string(evidence.FreshnessNotApplicable) || len(item.SourceProvenance) != 0 {
		t.Fatalf("manual evidence freshness = %+v", item)
	}
}

func evidenceItemByID(t *testing.T, items []cli.EvidenceItem, id string) cli.EvidenceItem {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("missing evidence item %q in %+v", id, items)
	return cli.EvidenceItem{}
}

package corpus

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/evidence"
)

func TestEvidenceThreadFreshnessAndProvenancePersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "freshness.db")
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := c.ApplyRepositoryObservation(ctx, "Owner", "Repo", "R1", time.Unix(10, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 42, "open", "bug", "body", "alice", time.Unix(20, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	subject := evidence.SourceSubject{Kind: evidence.SourceSubjectThread, Owner: "owner", Repo: "repo", ThreadKind: ThreadKindIssue, Number: 42}
	revision, err := c.CurrentSourceRevision(ctx, subject)
	if err != nil || revision == nil {
		t.Fatalf("CurrentSourceRevision = (%+v, %v)", revision, err)
	}
	if revision.ObservationSequence != thread.ObservationSequence || revision.ObservedAt.IsZero() {
		t.Fatalf("unexpected thread revision: %+v", revision)
	}
	item := &evidence.Evidence{
		ID: "ev-thread", Type: evidence.EvidenceTypeGitHubSource, Relation: evidence.RelationSupporting,
		Description: "stored issue confirms the behavior", SourceProvenance: []evidence.SourceRevision{*revision},
		CreatedAt: time.Unix(30, 0).UTC(),
	}
	if err := c.SaveEvidence(ctx, item); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	items, err := c.ListEvidence(ctx, evidence.EvidenceFilter{})
	if err != nil || len(items) != 1 || len(items[0].SourceProvenance) != 1 {
		t.Fatalf("ListEvidence = (%+v, %v)", items, err)
	}
	evaluator := evidence.NewFreshnessEvaluator(c)
	freshness, err := evaluator.Evaluate(ctx, items[0])
	if err != nil || freshness.Status != evidence.FreshnessFresh {
		t.Fatalf("initial freshness = (%+v, %v)", freshness, err)
	}

	// The same source timestamp with a later local observation sequence wins.
	if _, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 42, "open", "updated bug", "body", "alice", time.Unix(20, 0).UTC(), `{}`); err != nil {
		t.Fatal(err)
	}
	freshness, err = evaluator.Evaluate(ctx, items[0])
	if err != nil || freshness.Status != evidence.FreshnessStale {
		t.Fatalf("updated freshness = (%+v, %v)", freshness, err)
	}
}

func TestEvidenceFacetFreshnessIgnoresUnrelatedFacet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	repo, _ := c.ApplyRepositoryObservation(ctx, "owner", "repo", "R1", time.Unix(10, 0).UTC(), `{}`)
	thread, _ := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindIssue, 1, "open", "bug", "", "alice", time.Unix(20, 0).UTC(), `{}`)
	threadID := thread.ID
	commentsAt := time.Unix(30, 0).UTC()
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &threadID, "issue_comments", commentsAt, []FacetObservationInput{{SourceUpdatedAt: commentsAt, Payload: `{}`}}, true, 0); err != nil {
		t.Fatal(err)
	}
	subject := evidence.SourceSubject{
		Kind: evidence.SourceSubjectFacet, Owner: "owner", Repo: "repo", ThreadKind: ThreadKindIssue, Number: 1, Facet: "issue_comments",
	}
	recorded, err := c.CurrentSourceRevision(ctx, subject)
	if err != nil || recorded == nil {
		t.Fatalf("recorded revision = (%+v, %v)", recorded, err)
	}
	item := &evidence.Evidence{Type: evidence.EvidenceTypeGitHubSource, SourceProvenance: []evidence.SourceRevision{*recorded}}
	evaluator := evidence.NewFreshnessEvaluator(c)

	reviewsAt := time.Unix(40, 0).UTC()
	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &threadID, "pr_reviews", reviewsAt, nil, true, 0); err != nil {
		t.Fatal(err)
	}
	freshness, err := evaluator.Evaluate(ctx, item)
	if err != nil || freshness.Status != evidence.FreshnessFresh {
		t.Fatalf("unrelated facet freshness = (%+v, %v)", freshness, err)
	}

	if err := c.ApplyFacetObservationSet(ctx, repo.ID, &threadID, "issue_comments", commentsAt, nil, true, 0); err != nil {
		t.Fatal(err)
	}
	freshness, err = evaluator.Evaluate(ctx, item)
	if err != nil || freshness.Status != evidence.FreshnessStale {
		t.Fatalf("same facet freshness = (%+v, %v)", freshness, err)
	}
}

func TestEvidenceFreshnessMissingRevisionIsUnknownAndReadOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	item := &evidence.Evidence{
		Type: evidence.EvidenceTypeGitHubSource,
		SourceProvenance: []evidence.SourceRevision{{
			Subject:         evidence.SourceSubject{Kind: evidence.SourceSubjectRepository, Owner: "missing", Repo: "repo"},
			SourceUpdatedAt: time.Unix(10, 0).UTC(), ObservationSequence: 1, ObservedAt: time.Unix(11, 0).UTC(),
		}},
	}
	var before, after int64
	if err := c.db.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	freshness, err := evidence.NewFreshnessEvaluator(c).Evaluate(ctx, item)
	if err != nil || freshness.Status != evidence.FreshnessUnknown {
		t.Fatalf("freshness = (%+v, %v)", freshness, err)
	}
	if err := c.db.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("freshness read changed database: before=%d after=%d", before, after)
	}
}

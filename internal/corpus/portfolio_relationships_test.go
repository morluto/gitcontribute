package corpus

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPortfolioLinksAreExplicitAndDeterministic(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	prID := insertPortfolioFixture(t, ctx, c)

	created := time.Unix(500, 0).UTC()
	first, err := c.SavePortfolioLink(ctx, PortfolioLink{
		PullRequestThreadID: prID, OpportunityID: "opp-1", WorkspaceID: "ws-1", CreatedAt: created,
	})
	if err != nil {
		t.Fatalf("save portfolio link: %v", err)
	}
	second, err := c.SavePortfolioLink(ctx, PortfolioLink{
		PullRequestThreadID: prID, OpportunityID: "opp-1", WorkspaceID: "ws-1", CreatedAt: created.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("save duplicate portfolio link: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate link ids = %d and %d", first.ID, second.ID)
	}

	links, err := c.ListPortfolioLinks(ctx)
	if err != nil {
		t.Fatalf("list portfolio links: %v", err)
	}
	if len(links) != 1 || links[0].PullRequestThreadID != prID || links[0].OpportunityID != "opp-1" || links[0].WorkspaceID != "ws-1" {
		t.Fatalf("links = %#v", links)
	}
	results, err := c.FindPortfolioOverlaps(ctx, []PortfolioSubject{{Kind: PortfolioSubjectOpportunity, Ref: "opp-1"}}, []int64{prID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "overlap" || len(results[0].Matches) != 1 || results[0].Matches[0].Evidence[0].Kind != "explicit_link" {
		t.Fatalf("explicit link overlap = %#v", results)
	}
}

func TestPortfolioSignalsRejectMissingSourceObservation(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	prID := insertPortfolioFixture(t, ctx, c)
	_, err := c.ReplacePortfolioSignals(ctx, PortfolioSignalSnapshot{
		Subject: PortfolioSubject{Kind: PortfolioSubjectPullRequest, Ref: fmt.Sprint(prID)}, Facet: PortfolioFacetChangedFiles,
		Signals: []PortfolioSignal{{Kind: PortfolioSignalFilePath, Value: "main.go"}}, SourceUpdatedAt: time.Unix(300, 0).UTC(),
		SourceObservationRefs: []ObservationRef{{Kind: "facet", ID: 999999}},
	})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing observation error = %v", err)
	}
}

func TestFindPortfolioOverlapsUsesOnlyCoveredObservedSignals(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	prID := insertPortfolioFixture(t, ctx, c)
	pr := PortfolioSubject{Kind: PortfolioSubjectPullRequest, Ref: fmt.Sprint(prID)}
	candidate := PortfolioSubject{Kind: PortfolioSubjectOpportunity, Ref: "opp-1"}
	unknown := PortfolioSubject{Kind: PortfolioSubjectOpportunity, Ref: "opp-missing"}
	newer := time.Unix(300, 0).UTC()

	replacePortfolioFixture(t, ctx, c, candidate, PortfolioFacetChangedFiles, newer,
		PortfolioSignal{Kind: PortfolioSignalFilePath, Value: `internal\\store\\record.go`})
	replacePortfolioFixture(t, ctx, c, candidate, PortfolioFacetLinkedIssues, newer,
		PortfolioSignal{Kind: PortfolioSignalLinkedIssue, Value: "owner/repo#7"})
	replacePortfolioFixture(t, ctx, c, candidate, PortfolioFacetOpportunitySimilarity, newer,
		PortfolioSignal{Kind: PortfolioSignalOpportunitySimilarity, TargetKind: PortfolioSubjectPullRequest, TargetRef: pr.Ref, Score: 0.86})
	replacePortfolioFixture(t, ctx, c, pr, PortfolioFacetChangedFiles, newer,
		PortfolioSignal{Kind: PortfolioSignalFilePath, Value: "internal/store/record.go"})
	replacePortfolioFixture(t, ctx, c, pr, PortfolioFacetLinkedIssues, newer)

	// The stale snapshot remains immutable history but cannot replace the newer
	// path projection used by offline overlap reads.
	replacePortfolioFixture(t, ctx, c, candidate, PortfolioFacetChangedFiles, newer.Add(-time.Hour),
		PortfolioSignal{Kind: PortfolioSignalFilePath, Value: "unrelated.go"})

	results, err := c.FindPortfolioOverlaps(ctx, []PortfolioSubject{candidate, unknown}, []int64{prID})
	if err != nil {
		t.Fatalf("find portfolio overlaps: %v", err)
	}
	if len(results) != 2 || results[0].Candidate != candidate || results[1].Candidate != unknown {
		t.Fatalf("input ordering not preserved: %#v", results)
	}
	if results[0].Status != "overlap" || len(results[0].Matches) != 1 {
		t.Fatalf("covered overlap = %#v", results[0])
	}
	evidence := results[0].Matches[0].Evidence
	if len(evidence) != 2 || evidence[0].Kind != PortfolioSignalFilePath || evidence[0].Value != "internal/store/record.go" || evidence[1].Kind != PortfolioSignalOpportunitySimilarity {
		t.Fatalf("overlap evidence = %#v", evidence)
	}
	if results[1].Status != "unknown" || results[1].Coverage["candidate.changed_files"] != "missing" {
		t.Fatalf("missing coverage converted to no-overlap: %#v", results[1])
	}

	var snapshots int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM portfolio_signal_snapshots WHERE subject_kind=? AND subject_ref=? AND facet=?`, candidate.Kind, candidate.Ref, PortfolioFacetChangedFiles).Scan(&snapshots); err != nil {
		t.Fatalf("count signal snapshots: %v", err)
	}
	if snapshots != 2 {
		t.Fatalf("signal snapshot count = %d, want append-only history of 2", snapshots)
	}
}

func TestFindPortfolioOverlapsRequiresCompleteNegativeCoverage(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	prID := insertPortfolioFixture(t, ctx, c)
	pr := PortfolioSubject{Kind: PortfolioSubjectPullRequest, Ref: fmt.Sprint(prID)}
	candidate := PortfolioSubject{Kind: PortfolioSubjectOpportunity, Ref: "opp-1"}
	at := time.Unix(300, 0).UTC()
	for _, facet := range portfolioFacets {
		replacePortfolioFixture(t, ctx, c, candidate, facet, at)
	}
	for _, facet := range []string{PortfolioFacetChangedFiles, PortfolioFacetLinkedIssues} {
		replacePortfolioFixture(t, ctx, c, pr, facet, at)
	}
	results, err := c.FindPortfolioOverlaps(ctx, []PortfolioSubject{candidate}, []int64{prID})
	if err != nil {
		t.Fatalf("find portfolio overlaps: %v", err)
	}
	if results[0].Status != "no_overlap" {
		t.Fatalf("fully covered negative = %#v", results[0])
	}
}

func TestFindPortfolioOverlapsPullRequestNegativeDoesNotRequireSimilarityFacet(t *testing.T) {
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	firstID := insertPortfolioFixture(t, ctx, c)
	var repositoryID int64
	if err := c.db.QueryRowContext(ctx, `SELECT repository_id FROM threads WHERE id=?`, firstID).Scan(&repositoryID); err != nil {
		t.Fatal(err)
	}
	second, err := c.ApplyThreadObservation(ctx, repositoryID, ThreadKindPullRequest, 4, "open", "second PR", "body", "author", time.Unix(201, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	first := PortfolioSubject{Kind: PortfolioSubjectPullRequest, Ref: fmt.Sprint(firstID)}
	secondSubject := PortfolioSubject{Kind: PortfolioSubjectPullRequest, Ref: fmt.Sprint(second.ID)}
	for _, subject := range []PortfolioSubject{first, secondSubject} {
		replacePortfolioFixture(t, ctx, c, subject, PortfolioFacetChangedFiles, time.Unix(300, 0).UTC())
		replacePortfolioFixture(t, ctx, c, subject, PortfolioFacetLinkedIssues, time.Unix(300, 0).UTC())
	}
	results, err := c.FindPortfolioOverlaps(ctx, []PortfolioSubject{first}, []int64{second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "no_overlap" {
		t.Fatalf("pull-request no-overlap = %#v", results)
	}
}

func replacePortfolioFixture(t *testing.T, ctx context.Context, c *Corpus, subject PortfolioSubject, facet string, at time.Time, signals ...PortfolioSignal) {
	t.Helper()
	var observationID int64
	if err := c.db.QueryRowContext(ctx, `SELECT id FROM thread_observations ORDER BY id LIMIT 1`).Scan(&observationID); err != nil {
		t.Fatalf("read fixture source observation: %v", err)
	}
	if _, err := c.ReplacePortfolioSignals(ctx, PortfolioSignalSnapshot{
		Subject: subject, Facet: facet, Signals: signals, SourceUpdatedAt: at,
		SourceObservationRefs: []ObservationRef{{Kind: "thread", ID: observationID}},
	}); err != nil {
		t.Fatalf("replace %s/%s signals: %v", subject.Ref, facet, err)
	}
}

func insertPortfolioFixture(t *testing.T, ctx context.Context, c *Corpus) int64 {
	t.Helper()
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(100, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("insert repository: %v", err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, ThreadKindPullRequest, 3, "open", "PR", "body", "author", time.Unix(200, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("insert pull request: %v", err)
	}
	now := encodeTime(time.Unix(250, 0).UTC())
	for _, statement := range []string{
		`INSERT INTO investigations (id, repo_owner, repo_name, status, payload, created_at, updated_at) VALUES ('inv-1', 'owner', 'repo', 'open', '{}', ?, ?)`,
		`INSERT INTO hypotheses (id, investigation_id, category, status, payload, created_at, updated_at) VALUES ('hyp-1', 'inv-1', 'bug', 'promoted', '{}', ?, ?)`,
		`INSERT INTO opportunities (id, investigation_id, hypothesis_id, category, status, payload, created_at, updated_at) VALUES ('opp-1', 'inv-1', 'hyp-1', 'bug', 'validated', '{}', ?, ?)`,
	} {
		if _, err := c.db.ExecContext(ctx, statement, now, now); err != nil {
			t.Fatalf("insert portfolio fixture: %v", err)
		}
	}
	if _, err := c.db.ExecContext(ctx, `INSERT INTO workspaces (id, investigation_id, payload, created_at) VALUES ('ws-1', 'inv-1', '{}', ?)`, now); err != nil {
		t.Fatalf("insert workspace fixture: %v", err)
	}
	return thread.ID
}

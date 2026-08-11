package corpus

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestPortfolioSubjectCanonicalizesBoundaryIdentity(t *testing.T) {
	t.Parallel()
	subject, err := ParsePortfolioSubject(" pull_request ", " 001 ")
	if err != nil {
		t.Fatal(err)
	}
	if subject.Kind() != PortfolioSubjectPullRequest || subject.Ref() != "1" {
		t.Fatalf("subject = %s:%s", subject.Kind(), subject.Ref())
	}
	encoded, err := json.Marshal(subject)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"kind":"pull_request","ref":"1"}` {
		t.Fatalf("encoded subject = %s", encoded)
	}
	var decoded PortfolioSubject
	if err := json.Unmarshal([]byte(`{"kind":"opportunity","ref":" opp-1 "}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind() != PortfolioSubjectOpportunity || decoded.Ref() != "opp-1" {
		t.Fatalf("decoded subject = %s:%s", decoded.Kind(), decoded.Ref())
	}
	if _, err := ParsePortfolioSubject(PortfolioSubjectPullRequest, "0"); err == nil {
		t.Fatal("expected invalid pull-request identity to be rejected")
	}
}

func TestPortfolioSignalVariantsRejectMixedRepresentations(t *testing.T) {
	t.Parallel()
	pathSignal, err := NewPortfolioFilePathSignal(` internal\store\record.go `)
	if err != nil {
		t.Fatal(err)
	}
	if pathSignal.Kind() != PortfolioSignalFilePath || pathSignal.Value() != "internal/store/record.go" {
		t.Fatalf("path signal = %s:%s", pathSignal.Kind(), pathSignal.Value())
	}
	var mixed PortfolioSignal
	if err := json.Unmarshal([]byte(`{"kind":"file_path","value":"main.go","target_kind":"pull_request","target_ref":"1"}`), &mixed); err == nil {
		t.Fatal("expected mixed scalar and target signal to be rejected")
	}
	opportunity := mustPortfolioSubject(t, PortfolioSubjectOpportunity, "opp-1")
	if _, err := NewPortfolioOpportunitySimilaritySignal(opportunity, 0.8); err == nil {
		t.Fatal("expected similarity target outside the pull-request domain to be rejected")
	}
}

func TestPortfolioLinksAreExplicitAndDeterministic(t *testing.T) {
	t.Parallel()
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
	results, err := c.FindPortfolioOverlaps(ctx, []PortfolioSubject{mustPortfolioSubject(t, PortfolioSubjectOpportunity, "opp-1")}, []int64{prID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status() != "overlap" || len(results[0].Matches) != 1 || results[0].Matches[0].Evidence[0].Kind != "explicit_link" {
		t.Fatalf("explicit link overlap = %#v", results)
	}
}

func TestPortfolioSignalsRejectMissingSourceObservation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	prID := insertPortfolioFixture(t, ctx, c)
	_, err := c.ReplacePortfolioSignals(ctx, PortfolioSignalSnapshot{
		Subject: mustPullRequestPortfolioSubject(t, prID), Facet: PortfolioFacetChangedFiles,
		Signals: []PortfolioSignal{mustPortfolioFilePathSignal(t, "main.go")}, SourceUpdatedAt: time.Unix(300, 0).UTC(),
		SourceObservationRefs: []ObservationRef{mustObservationRef(t, "facet", 999999)},
	})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing observation error = %v", err)
	}
}

func TestFindPortfolioOverlapsUsesOnlyCoveredObservedSignals(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	prID := insertPortfolioFixture(t, ctx, c)
	pr := mustPullRequestPortfolioSubject(t, prID)
	candidate := mustPortfolioSubject(t, PortfolioSubjectOpportunity, "opp-1")
	unknown := mustPortfolioSubject(t, PortfolioSubjectOpportunity, "opp-missing")
	newer := time.Unix(300, 0).UTC()

	replacePortfolioFixture(t, ctx, c, candidate, PortfolioFacetChangedFiles, newer,
		mustPortfolioFilePathSignal(t, `internal\\store\\record.go`))
	replacePortfolioFixture(t, ctx, c, candidate, PortfolioFacetLinkedIssues, newer,
		mustPortfolioLinkedIssueSignal(t, "owner/repo#7"))
	replacePortfolioFixture(t, ctx, c, candidate, PortfolioFacetOpportunitySimilarity, newer,
		mustPortfolioSimilaritySignal(t, pr, 0.86))
	replacePortfolioFixture(t, ctx, c, pr, PortfolioFacetChangedFiles, newer,
		mustPortfolioFilePathSignal(t, "internal/store/record.go"))
	replacePortfolioFixture(t, ctx, c, pr, PortfolioFacetLinkedIssues, newer)

	// The stale snapshot remains immutable history but cannot replace the newer
	// path projection used by offline overlap reads.
	replacePortfolioFixture(t, ctx, c, candidate, PortfolioFacetChangedFiles, newer.Add(-time.Hour),
		mustPortfolioFilePathSignal(t, "unrelated.go"))

	results, err := c.FindPortfolioOverlaps(ctx, []PortfolioSubject{candidate, unknown}, []int64{prID})
	if err != nil {
		t.Fatalf("find portfolio overlaps: %v", err)
	}
	if len(results) != 2 || results[0].Candidate != candidate || results[1].Candidate != unknown {
		t.Fatalf("input ordering not preserved: %#v", results)
	}
	if results[0].Status() != "overlap" || len(results[0].Matches) != 1 {
		t.Fatalf("covered overlap = %#v", results[0])
	}
	evidence := results[0].Matches[0].Evidence
	if len(evidence) != 2 || evidence[0].Kind != PortfolioSignalFilePath || evidence[0].Value != "internal/store/record.go" || evidence[1].Kind != PortfolioSignalOpportunitySimilarity {
		t.Fatalf("overlap evidence = %#v", evidence)
	}
	if results[1].Status() != "unknown" || results[1].Coverage()["candidate.changed_files"] != "missing" {
		t.Fatalf("missing coverage converted to no-overlap: %#v", results[1])
	}

	var snapshots int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM portfolio_signal_snapshots WHERE subject_kind=? AND subject_ref=? AND facet=?`, candidate.Kind(), candidate.Ref(), PortfolioFacetChangedFiles).Scan(&snapshots); err != nil {
		t.Fatalf("count signal snapshots: %v", err)
	}
	if snapshots != 2 {
		t.Fatalf("signal snapshot count = %d, want append-only history of 2", snapshots)
	}
}

func TestFindPortfolioOverlapsRequiresCompleteNegativeCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	prID := insertPortfolioFixture(t, ctx, c)
	pr := mustPullRequestPortfolioSubject(t, prID)
	candidate := mustPortfolioSubject(t, PortfolioSubjectOpportunity, "opp-1")
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
	if results[0].Status() != "no_overlap" {
		t.Fatalf("fully covered negative = %#v", results[0])
	}
}

func TestFindPortfolioOverlapsPullRequestNegativeDoesNotRequireSimilarityFacet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	firstID := insertPortfolioFixture(t, ctx, c)
	var repositoryID int64
	if err := c.db.QueryRowContext(ctx, `SELECT repository_id FROM threads WHERE id=?`, firstID).Scan(&repositoryID); err != nil {
		t.Fatal(err)
	}
	second, err := c.ApplyThreadObservation(ctx, repositoryID, domain.PullRequestKind, 4, "open", "second PR", "body", "author", time.Unix(201, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	first := mustPullRequestPortfolioSubject(t, firstID)
	secondSubject := mustPullRequestPortfolioSubject(t, second.ID)
	for _, subject := range []PortfolioSubject{first, secondSubject} {
		replacePortfolioFixture(t, ctx, c, subject, PortfolioFacetChangedFiles, time.Unix(300, 0).UTC())
		replacePortfolioFixture(t, ctx, c, subject, PortfolioFacetLinkedIssues, time.Unix(300, 0).UTC())
	}
	results, err := c.FindPortfolioOverlaps(ctx, []PortfolioSubject{first}, []int64{second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status() != "no_overlap" {
		t.Fatalf("pull-request no-overlap = %#v", results)
	}
}

func TestListPullRequestIssueLinksDistinguishesCoveredEmptyAndBoundsPopulation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _ := openTestCorpus(t)
	firstID := insertPortfolioFixture(t, ctx, c)
	var repoID int64
	if err := c.db.QueryRowContext(ctx, `SELECT repository_id FROM threads WHERE id=?`, firstID).Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	second, err := c.ApplyThreadObservation(ctx, repoID, domain.PullRequestKind, 4, "open", "newer", "body", "author", time.Unix(201, 0).UTC(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	first := mustPullRequestPortfolioSubject(t, firstID)
	secondSubject := mustPullRequestPortfolioSubject(t, second.ID)
	replacePortfolioFixture(t, ctx, c, first, PortfolioFacetLinkedIssues, time.Unix(300, 0).UTC(),
		mustPortfolioLinkedIssueSignal(t, "owner/repo#7"))
	replacePortfolioFixture(t, ctx, c, secondSubject, PortfolioFacetLinkedIssues, time.Unix(301, 0).UTC())

	links, capped, err := c.ListPullRequestIssueLinks(ctx, repoID, OpenThreadState(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if capped || len(links) != 2 || links[0].Number != 4 || !links[0].Covered || len(links[0].LinkedIssues) != 0 {
		t.Fatalf("links = %+v, capped=%v", links, capped)
	}
	if links[1].Number != 3 || !links[1].Covered || len(links[1].LinkedIssues) != 1 || links[1].LinkedIssues[0] != "owner/repo#7" {
		t.Fatalf("linked projection = %+v", links[1])
	}
	bounded, capped, err := c.ListPullRequestIssueLinks(ctx, repoID, OpenThreadState(), 1)
	if err != nil || !capped || len(bounded) != 1 || bounded[0].Number != 4 {
		t.Fatalf("bounded = %+v, capped=%v, err=%v", bounded, capped, err)
	}
}

//nolint:revive // Test helpers conventionally put *testing.T first.
func replacePortfolioFixture(t *testing.T, ctx context.Context, c *Corpus, subject PortfolioSubject, facet string, at time.Time, signals ...PortfolioSignal) {
	t.Helper()
	var observationID int64
	if err := c.db.QueryRowContext(ctx, `SELECT id FROM thread_observations ORDER BY id LIMIT 1`).Scan(&observationID); err != nil {
		t.Fatalf("read fixture source observation: %v", err)
	}
	if _, err := c.ReplacePortfolioSignals(ctx, PortfolioSignalSnapshot{
		Subject: subject, Facet: facet, Signals: signals, SourceUpdatedAt: at,
		SourceObservationRefs: []ObservationRef{mustObservationRef(t, "thread", observationID)},
	}); err != nil {
		t.Fatalf("replace %s/%s signals: %v", subject.Ref(), facet, err)
	}
}

func mustPortfolioSubject(t *testing.T, kind, ref string) PortfolioSubject {
	t.Helper()
	subject, err := ParsePortfolioSubject(kind, ref)
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func mustPullRequestPortfolioSubject(t *testing.T, threadID int64) PortfolioSubject {
	t.Helper()
	subject, err := NewPullRequestPortfolioSubject(threadID)
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func mustPortfolioFilePathSignal(t *testing.T, value string) PortfolioSignal {
	t.Helper()
	signal, err := NewPortfolioFilePathSignal(value)
	if err != nil {
		t.Fatal(err)
	}
	return signal
}

func mustPortfolioLinkedIssueSignal(t *testing.T, value string) PortfolioSignal {
	t.Helper()
	signal, err := NewPortfolioLinkedIssueSignal(value)
	if err != nil {
		t.Fatal(err)
	}
	return signal
}

func mustPortfolioSimilaritySignal(t *testing.T, target PortfolioSubject, score float64) PortfolioSignal {
	t.Helper()
	signal, err := NewPortfolioOpportunitySimilaritySignal(target, score)
	if err != nil {
		t.Fatal(err)
	}
	return signal
}

func mustObservationRef(t *testing.T, kind string, id int64) ObservationRef {
	t.Helper()
	ref, err := ParseObservationRef(kind, id)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

//nolint:revive // Test helpers conventionally put *testing.T first.
func insertPortfolioFixture(t *testing.T, ctx context.Context, c *Corpus) int64 {
	t.Helper()
	repo, err := c.ApplyRepositoryObservation(ctx, "owner", "repo", "1", time.Unix(100, 0).UTC(), `{}`)
	if err != nil {
		t.Fatalf("insert repository: %v", err)
	}
	thread, err := c.ApplyThreadObservation(ctx, repo.ID, domain.PullRequestKind, 3, "open", "PR", "body", "author", time.Unix(200, 0).UTC(), `{}`)
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

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/radar"
	"github.com/morluto/gitcontribute/internal/relatedwork"
)

type panicRadarReader struct{ github.Reader }

type radarTestFixture struct {
	ctx      context.Context
	svc      *Service
	now      time.Time
	repoID   int64
	issueID  int64
	issue2ID int64
}

func newRadarTestFixture(t *testing.T) radarTestFixture {
	t.Helper()
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	// Any accidental network read will dispatch through a nil embedded Reader
	// and panic, making the side-effect boundary visible to this test.
	svc.SetGitHubReader(panicRadarReader{})

	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: "owner", Name: "repo", SourceUpdatedAt: now.Add(-time.Hour),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	issue1, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open",
		Title: "Focused starter bug", Body: strings.Repeat("Steps to reproduce and expected behavior. ", 8) + "\n- [ ] add a regression test",
		Labels: []string{"good first issue", "help wanted"}, SourceUpdatedAt: now.Add(-24 * time.Hour),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	issue2, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 2, State: "open",
		Title: "Assigned refactor", Body: "Refactor this package.", Assignees: []string{"alice"}, SourceUpdatedAt: now.Add(-48 * time.Hour),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 9, State: "open",
		Title: "Implement starter bug", Body: "Fixes #1", SourceUpdatedAt: now.Add(-30 * time.Minute),
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, repo.ID, nil, "threads", now.Add(-time.Hour), true, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.AdvanceFacet(ctx, repo.ID, nil, "metadata", now.Add(-time.Hour), true, 0); err != nil {
		t.Fatal(err)
	}
	commentPayload, err := json.Marshal([]github.IssueComment{{
		ID: 1, Author: "maintainer", AuthorAssociation: "MEMBER", HTMLURL: "https://github.com/owner/repo/issues/1#issuecomment-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &issue1.ID, FacetIssueComments, now.Add(-time.Hour), []corpus.FacetObservationInput{{
		SourceUpdatedAt: now.Add(-time.Hour), Payload: string(commentPayload),
	}}, true, 0); err != nil {
		t.Fatal(err)
	}
	return radarTestFixture{ctx: ctx, svc: svc, now: now, repoID: repo.ID, issueID: issue1.ID, issue2ID: issue2.ID}
}

func TestContributionRadarUsesOnlyStoredEvidence(t *testing.T) {
	fixture := newRadarTestFixture(t)
	before, err := fixture.svc.corpus.ListFacetObservations(fixture.ctx, fixture.repoID, &fixture.issueID, FacetIssueComments)
	if err != nil {
		t.Fatal(err)
	}

	report, err := fixture.svc.ContributionRadar(fixture.ctx, cli.RadarOptions{Repo: cli.RepoRef{Owner: "owner", Repo: "repo"}, Limit: 20})
	if err != nil {
		t.Fatalf("contribution radar: %v", err)
	}
	after, err := fixture.svc.corpus.ListFacetObservations(fixture.ctx, fixture.repoID, &fixture.issueID, FacetIssueComments)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("radar mutated facet observations: before=%d after=%d", len(before), len(after))
	}
	if report.TotalOpenIssues != 2 || report.CandidatePopulation != 2 || len(report.Candidates) != 2 {
		t.Fatalf("unexpected report population: %+v", report)
	}
	assertContributionRadarEvidence(t, report, fixture.now)
}

func assertContributionRadarEvidence(t *testing.T, report *radar.Report, now time.Time) {
	t.Helper()
	starter := radarCandidate(report, 1)
	if starter == nil {
		t.Fatal("missing issue #1")
	}
	if starter.Eligibility != radar.EligibilityBlocked || !radarSignal(starter.Blockers, "active_implementation") {
		t.Fatalf("starter blocker = %+v", starter)
	}
	if !radarSignal(starter.PositiveSignals, "maintainer_response") || starter.Confidence != "medium" {
		t.Fatalf("starter evidence = %+v", starter)
	}
	if len(starter.LinkedPullRequests) != 1 || !starter.LinkedPullRequests[0].Closing {
		t.Fatalf("starter linked PRs = %+v", starter.LinkedPullRequests)
	}
	related := radarRelatedWork(starter, "pull_request:owner/repo#9")
	if related == nil || related.Relation != "claims_to_close" || related.Direction != "inbound" || !radarRelatedEvidence(*related, "pull_request_text") {
		t.Fatalf("starter related work = %+v", starter.RelatedWork)
	}
	if !starter.SourceAsOf.Equal(now.Add(-30*time.Minute)) || !report.SourceAsOf.Equal(starter.SourceAsOf) {
		t.Fatalf("source as-of candidate=%s report=%s", starter.SourceAsOf, report.SourceAsOf)
	}
	if len(starter.Coverage) != 3 || starter.Coverage[0].Scope != "repository" || starter.Coverage[1].Scope != "repository" || starter.Coverage[2].Scope != "thread" {
		t.Fatalf("starter coverage = %+v", starter.Coverage)
	}

	assigned := radarCandidate(report, 2)
	if assigned == nil || assigned.Eligibility != radar.EligibilityNeedsCoordination || !radarSignal(assigned.Risks, "assigned") {
		t.Fatalf("assigned candidate = %+v", assigned)
	}
	if len(assigned.Unknowns) != 2 || assigned.Unknowns[0].Code != "comments_not_hydrated" || assigned.Unknowns[1].Code != "contribution_guidance_unknown" {
		t.Fatalf("assigned unknowns = %+v", assigned.Unknowns)
	}
}

func TestContributionRadarDistinguishesMissingRepositoryAndEmptyResult(t *testing.T) {
	ctx := context.Background()
	paths := config.NewPaths(&config.Env{Home: t.TempDir()})
	svc, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}

	opts := cli.RadarOptions{Repo: cli.RepoRef{Owner: "owner", Repo: "repo"}}
	if _, err := svc.ContributionRadar(ctx, opts); !errors.Is(err, errRepositoryNotFound) {
		t.Fatalf("missing repository error = %v", err)
	}
	if _, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{Owner: "owner", Name: "repo", SourceUpdatedAt: time.Unix(1, 0).UTC()}, `{}`); err != nil {
		t.Fatal(err)
	}
	report, err := svc.ContributionRadar(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalOpenIssues != 0 || report.CandidatePopulation != 0 || report.Candidates == nil || len(report.Candidates) != 0 {
		t.Fatalf("empty repository report = %+v", report)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.ContributionRadar(cancelled, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled radar error = %v", err)
	}
}

func TestContributionRadarReadsStoredDuplicateCluster(t *testing.T) {
	fixture := newRadarTestFixture(t)
	repo, err := fixture.svc.corpus.GetRepository(fixture.ctx, "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 2, State: "open",
		Title: "same starter bug", Body: "duplicate of #1", Assignees: []string{"alice"}, SourceUpdatedAt: fixture.now,
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	refresh, err := fixture.svc.RefreshClusters(fixture.ctx, cli.RepoRef{Owner: "owner", Repo: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if refresh.Stats.ClusterCount != 1 {
		t.Fatalf("refresh = %+v", refresh)
	}

	report, err := fixture.svc.ContributionRadar(fixture.ctx, cli.RadarOptions{Repo: cli.RepoRef{Owner: "owner", Repo: "repo"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate := radarCandidate(report, 2)
	if candidate == nil || candidate.DuplicateCluster == nil {
		t.Fatalf("candidate duplicate evidence = %+v", candidate)
	}
	if candidate.DuplicateCluster.CanonicalRef != "issue:owner/repo#1" || candidate.DuplicateCluster.CandidateCount != 2 {
		t.Fatalf("duplicate fact = %+v", candidate.DuplicateCluster)
	}
	if !radarSignal(candidate.Risks, "duplicate_candidates") {
		t.Fatalf("duplicate risk missing: %+v", candidate.Risks)
	}
}

func TestRadarPullRequestClosingReferenceIsPrecise(t *testing.T) {
	fixture := newRadarTestFixture(t)
	ref := domain.RepoRef{Owner: "owner", Repo: "repo"}
	stored, err := fixture.svc.corpus.GetRepository(fixture.ctx, ref.Owner, ref.Repo)
	if err != nil {
		t.Fatal(err)
	}
	issues, err := fixture.svc.corpus.ListThreadsFiltered(fixture.ctx, stored.ID, corpus.ThreadKindIssue, "open", 500)
	if err != nil {
		t.Fatal(err)
	}
	links, _, err := radarPullRequestRelatedWork(fixture.ctx, fixture.svc.corpus, stored, ref, issues, []corpus.Thread{{
		Kind: corpus.ThreadKindPullRequest, Number: 8, Title: "Handle both reports",
		Body: "Fixes #1 and also discusses #2. Fixes other/project#3.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(links[1]) != 1 || links[1][0].Relation != "claims_to_close" {
		t.Fatalf("issue 1 links = %+v", links[1])
	}
	if len(links[2]) != 1 || links[2][0].Relation != "mentions" {
		t.Fatalf("issue 2 links = %+v", links[2])
	}
	if _, ok := links[3]; ok {
		t.Fatalf("cross-repository reference leaked into links: %+v", links[3])
	}
}

func TestContributionRadarUsesAuthoritativeClosingIssueProjection(t *testing.T) {
	fixture := newRadarTestFixture(t)
	pr, err := fixture.svc.corpus.GetThread(fixture.ctx, fixture.repoID, corpus.ThreadKindPullRequest, 9)
	if err != nil || pr == nil {
		t.Fatalf("get PR: %+v, %v", pr, err)
	}
	pr.Body = "Implementation without a closing keyword."
	pr.SourceUpdatedAt = fixture.now.Add(-10 * time.Minute)
	pr, err = fixture.svc.corpus.UpsertThread(fixture.ctx, *pr, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := fixture.svc.corpus.LatestThreadObservation(fixture.ctx, pr.ID)
	if err != nil || observation == nil {
		t.Fatalf("latest PR observation: %+v, %v", observation, err)
	}
	if _, err := fixture.svc.corpus.ReplacePortfolioSignals(fixture.ctx, corpus.PortfolioSignalSnapshot{
		Subject: corpus.PortfolioSubject{Kind: corpus.PortfolioSubjectPullRequest, Ref: strconv.FormatInt(pr.ID, 10)},
		Facet:   corpus.PortfolioFacetLinkedIssues, Signals: []corpus.PortfolioSignal{{Kind: corpus.PortfolioSignalLinkedIssue, Value: "owner/repo#1"}},
		SourceUpdatedAt: pr.SourceUpdatedAt, SourceObservationRefs: []corpus.ObservationRef{{Kind: "thread", ID: observation.ID}},
	}); err != nil {
		t.Fatal(err)
	}

	report, err := fixture.svc.ContributionRadar(fixture.ctx, cli.RadarOptions{Repo: cli.RepoRef{Owner: "owner", Repo: "repo"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate := radarCandidate(report, 1)
	work := radarRelatedWork(candidate, "pull_request:owner/repo#9")
	if candidate == nil || candidate.Eligibility != radar.EligibilityBlocked || work == nil || work.Relation != "claims_to_close" || !radarRelatedEvidence(*work, "github_closing_issue") {
		t.Fatalf("candidate = %+v", candidate)
	}
}

func TestContributionRadarUnifiesCommentDependenciesAndTimelineCrossReferences(t *testing.T) {
	fixture := newRadarTestFixture(t)
	for _, number := range []int{10, 11} {
		if _, err := fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
			RepositoryID: fixture.repoID, Kind: corpus.ThreadKindPullRequest, Number: number, State: "open",
			Title: fmt.Sprintf("Related PR %d", number), Body: "No issue link in PR text.", SourceUpdatedAt: fixture.now.Add(time.Duration(number) * time.Minute),
		}, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	comments, err := json.Marshal([]github.IssueComment{{
		ID: 20, Author: "maintainer", AuthorAssociation: "MEMBER",
		Body:      "This depends on https://github.com/owner/repo/pull/10.\n> Fixes #99\n`Fixes #98`",
		CreatedAt: fixture.now.Add(-20 * time.Minute), UpdatedAt: fixture.now.Add(-19 * time.Minute),
		HTMLURL: "https://github.com/owner/repo/issues/2#issuecomment-20",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.svc.corpus.ApplyFacetObservationSet(fixture.ctx, fixture.repoID, &fixture.issue2ID, FacetIssueComments, fixture.now.Add(-19*time.Minute), []corpus.FacetObservationInput{{
		SourceUpdatedAt: fixture.now.Add(-19 * time.Minute), Payload: string(comments),
	}}, true, 0); err != nil {
		t.Fatal(err)
	}
	timeline, err := json.Marshal([]github.IssueTimelineEvent{{
		ID: 21, Event: "cross-referenced", SourceOwner: "owner", SourceRepository: "repo",
		SourceNumber: 11, SourceIsPullRequest: true, CreatedAt: fixture.now.Add(-18 * time.Minute),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.svc.corpus.ApplyFacetObservationSet(fixture.ctx, fixture.repoID, &fixture.issue2ID, FacetIssueTimeline, fixture.now.Add(-18*time.Minute), []corpus.FacetObservationInput{{
		SourceUpdatedAt: fixture.now.Add(-18 * time.Minute), Payload: string(timeline),
	}}, true, 0); err != nil {
		t.Fatal(err)
	}

	report, err := fixture.svc.ContributionRadar(fixture.ctx, cli.RadarOptions{Repo: cli.RepoRef{Owner: "owner", Repo: "repo"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate := radarCandidate(report, 2)
	dependency := radarRelatedWork(candidate, "pull_request:owner/repo#10")
	crossReference := radarRelatedWork(candidate, "pull_request:owner/repo#11")
	if dependency == nil || dependency.Relation != "depends_on" || dependency.Direction != "outbound" || !radarRelatedEvidence(*dependency, "issue_comment") {
		t.Fatalf("dependency = %+v; candidate=%+v", dependency, candidate)
	}
	if crossReference == nil || crossReference.Relation != "cross_reference" || crossReference.Direction != "inbound" || !radarRelatedEvidence(*crossReference, "issue_timeline") {
		t.Fatalf("cross reference = %+v; candidate=%+v", crossReference, candidate)
	}
	if !radarSignal(candidate.Risks, "open_dependency") || !radarSignal(candidate.Risks, "linked_open_pr") || radarRelatedWork(candidate, "thread:owner/repo#98") != nil || radarRelatedWork(candidate, "thread:owner/repo#99") != nil {
		t.Fatalf("candidate risks/quoted refs = %+v / %+v", candidate.Risks, candidate.RelatedWork)
	}
}

func TestNormalizeRadarRelatedWorkReportsEvidenceTruncation(t *testing.T) {
	values := make([]radar.RelatedWork, 0, maxRadarEvidencePerRelation+1)
	for i := 0; i <= maxRadarEvidencePerRelation; i++ {
		values = append(values, radar.RelatedWork{
			Ref: "pull_request:owner/repo#9", Kind: "pull_request", Number: 9, Relation: "mentions",
			Evidence: []radar.RelatedWorkEvidence{{Kind: "issue_comment", SourceURL: fmt.Sprintf("https://example.test/%d", i)}},
		})
	}
	normalized, capped := normalizeRadarRelatedWork(values, maxRadarRelatedWork)
	if !capped || len(normalized) != 1 || len(normalized[0].Evidence) != maxRadarEvidencePerRelation {
		t.Fatalf("normalized = %+v, capped=%v", normalized, capped)
	}
}

func TestRadarWorkAccumulatorKeepsStrongLateRelationshipsWithinBound(t *testing.T) {
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	accumulator := newRadarWorkAccumulator(repo, 1)
	for number := 2; number < 2+maxRadarRelatedWork; number++ {
		accumulator.append(relatedwork.Reference{
			Repo: repo, Number: number, Relation: relatedwork.RelationExplicitReference,
		}, "outbound", radar.RelatedWorkEvidence{Kind: "issue_comment"})
	}
	accumulator.append(relatedwork.Reference{
		Repo: repo, Number: 1000, Relation: relatedwork.RelationDependsOn,
	}, "outbound", radar.RelatedWorkEvidence{Kind: "issue_comment"})
	if !accumulator.capped || len(accumulator.distinct) != maxRadarRelatedWork {
		t.Fatalf("accumulator = distinct:%d capped:%v", len(accumulator.distinct), accumulator.capped)
	}
	if _, ok := accumulator.distinct[radarReferenceKey(relatedwork.Reference{Repo: repo, Number: 1000})]; !ok {
		t.Fatalf("strong dependency was discarded: %+v", accumulator.distinct)
	}
}

func radarCandidate(report *radar.Report, number int) *radar.Candidate {
	for i := range report.Candidates {
		if report.Candidates[i].Number == number {
			return &report.Candidates[i]
		}
	}
	return nil
}

func radarSignal(signals []radar.Signal, code string) bool {
	for _, signal := range signals {
		if signal.Code == code {
			return true
		}
	}
	return false
}

func radarRelatedWork(candidate *radar.Candidate, ref string) *radar.RelatedWork {
	if candidate == nil {
		return nil
	}
	for i := range candidate.RelatedWork {
		if candidate.RelatedWork[i].Ref == ref {
			return &candidate.RelatedWork[i]
		}
	}
	return nil
}

func radarRelatedEvidence(work radar.RelatedWork, kind string) bool {
	for _, evidence := range work.Evidence {
		if evidence.Kind == kind {
			return true
		}
	}
	return false
}

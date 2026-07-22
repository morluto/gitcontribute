package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/codeindex"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/research"
)

type panicResearchReader struct{ github.Reader }

type researchFixture struct {
	ctx     context.Context
	svc     *Service
	now     time.Time
	repoID  int64
	issueID int64
}

func newResearchFixture(t *testing.T) researchFixture {
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
	svc.SetGitHubReader(panicResearchReader{})

	repo, err := svc.corpus.UpsertRepository(ctx, corpus.Repository{
		Owner: "owner", Name: "repo", DefaultBranch: "main", Stars: 10,
		SourceCreatedAt: now.Add(-365 * 24 * time.Hour), SourceUpdatedAt: now.Add(-6 * time.Hour),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, facet := range []string{"metadata", "threads"} {
		if err := svc.corpus.AdvanceFacet(ctx, repo.ID, nil, facet, now.Add(-6*time.Hour), true, 0); err != nil {
			t.Fatal(err)
		}
	}
	issue, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 1, State: "open",
		Title: "Retry parser cancellation", Body: "## Expected behavior\n- [ ] cancellation remains bounded\nRelated to #2",
		Author: "alice", AuthorAssociation: "CONTRIBUTOR", Labels: []string{"bug", "help wanted"},
		SourceCreatedAt: now.Add(-10 * 24 * time.Hour), SourceUpdatedAt: now.Add(-4 * time.Hour),
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 2, State: "closed",
		Title: "Older parser report", Author: "bob", SourceCreatedAt: now.Add(-20 * 24 * time.Hour),
		SourceUpdatedAt: now.Add(-8 * time.Hour),
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.corpus.UpsertThread(ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindPullRequest, Number: 9, State: "open",
		Title: "Implement retry cancellation", Body: "Fixes #1", Author: "carol", AuthorAssociation: "NONE",
		SourceCreatedAt: now.Add(-2 * 24 * time.Hour), SourceUpdatedAt: now.Add(-2 * time.Hour),
	}, `{}`); err != nil {
		t.Fatal(err)
	}
	comments, err := json.Marshal([]github.IssueComment{{
		ID: 101, Body: "Please add a cancellation regression test; this must remain bounded.",
		Author: "maintainer", AuthorAssociation: "MEMBER", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Hour),
		HTMLURL: "https://github.com/owner/repo/issues/1#issuecomment-101",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.corpus.ApplyFacetObservationSet(ctx, repo.ID, &issue.ID, FacetIssueComments, now.Add(-time.Hour), []corpus.FacetObservationInput{{
		SourceUpdatedAt: now.Add(-time.Hour), Payload: string(comments),
	}}, true, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.corpus.StoreCodeSnapshot(ctx, domain.RepoRef{Owner: "owner", Repo: "repo"}, codeindex.Snapshot{
		RepoPath: "/repo", Commit: "abc123", CreatedAt: now.Add(-30 * time.Minute), TotalBytes: 64,
		Documents: []codeindex.Document{{
			Path: "internal/parser/retry.go", Content: "func retryParserWithCancellation() {}", Bytes: 41, LanguageHint: "go",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return researchFixture{ctx: ctx, svc: svc, now: now, repoID: repo.ID, issueID: issue.ID}
}

func TestThreadResearchBriefUsesOnlyStoredEvidence(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	before, err := fixture.svc.corpus.ListFacetObservations(fixture.ctx, fixture.repoID, &fixture.issueID, FacetIssueComments)
	if err != nil {
		t.Fatal(err)
	}
	brief, err := fixture.svc.ThreadResearchBrief(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Number: 1,
	})
	if err != nil {
		t.Fatalf("thread research brief: %v", err)
	}
	after, err := fixture.svc.corpus.ListFacetObservations(fixture.ctx, fixture.repoID, &fixture.issueID, FacetIssueComments)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("research brief mutated observations: before=%d after=%d", len(before), len(after))
	}
	if err := brief.ValidateProvenance(); err != nil {
		t.Fatalf("provenance: %v", err)
	}
	if brief.Target.Ref != "issue:owner/repo#1" || brief.Target.Kind != "issue" || brief.SchemaVersion != research.SchemaVersion {
		t.Fatalf("target = %+v", brief.Target)
	}
	if brief.SourceAsOf != fixture.now.Add(-30*time.Minute) {
		t.Fatalf("source_as_of = %s", brief.SourceAsOf)
	}
	if brief.Sections.Acceptance.Status != research.StatusAvailable || len(brief.Sections.Acceptance.Checklist) != 1 || len(brief.Sections.Acceptance.MaintainerStatements) != 1 {
		t.Fatalf("acceptance = %+v", brief.Sections.Acceptance)
	}
	if !hasRelated(brief.Sections.Duplicates.Candidates, 2, "explicit_reference") {
		t.Fatalf("explicit references = %+v", brief.Sections.Duplicates.Candidates)
	}
	if !hasRelated(brief.Sections.PullRequests.PullRequests, 9, "claims_to_close") {
		t.Fatalf("linked PRs = %+v", brief.Sections.PullRequests.PullRequests)
	}
	if brief.Sections.Code.Status != research.StatusAvailable || len(brief.Sections.Code.Hits) != 1 || brief.Sections.Code.Hits[0].Path != "internal/parser/retry.go" {
		t.Fatalf("code section = %+v", brief.Sections.Code)
	}
	if brief.Sections.Guidance.Status != research.StatusUnknown || !containsString(brief.Sections.Coverage.Gaps, "repository:contribution_guidance") {
		t.Fatalf("guidance/coverage = %+v / %+v", brief.Sections.Guidance, brief.Sections.Coverage)
	}
	assertResearchHealth(t, brief.Sections.Health)
}

func TestThreadResearchBriefPRCoverageAndErrors(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	repo := domain.RepoRef{Owner: "owner", Repo: "repo"}
	brief, err := fixture.svc.ThreadResearchBrief(fixture.ctx, research.ThreadRef{Repo: repo, Kind: domain.PullRequestKind, Number: 9})
	if err != nil {
		t.Fatalf("PR brief: %v", err)
	}
	if brief.Target.Kind != "pull_request" || brief.Sections.Timeline.Status != research.StatusPartial {
		t.Fatalf("PR target/timeline = %+v / %+v", brief.Target, brief.Sections.Timeline)
	}
	next := brief.Sections.Next.Commands
	if !researchCommandsContain(next, "issue_comments,pr_details,pr_review_comments,pr_reviews") {
		t.Fatalf("PR hydration command = %+v", next)
	}

	_, err = fixture.svc.ThreadResearchBrief(fixture.ctx, research.ThreadRef{Repo: repo, Kind: domain.IssueKind, Number: 9})
	var cliErr *cli.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != cli.ExitNotFound || !errors.Is(err, research.ErrThreadKindMismatch) {
		t.Fatalf("kind mismatch error = %v", err)
	}
	_, err = fixture.svc.ThreadResearchBrief(fixture.ctx, research.ThreadRef{Repo: repo, Number: 404})
	if !errors.As(err, &cliErr) || cliErr.Code != cli.ExitNotFound || !errors.Is(err, research.ErrThreadNotFound) {
		t.Fatalf("missing thread error = %v", err)
	}
	cancelled, cancel := context.WithCancel(fixture.ctx)
	cancel()
	_, err = fixture.svc.ThreadResearchBrief(cancelled, research.ThreadRef{Repo: repo, Number: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
}

func TestThreadResearchBriefBoundsStoredFacetPages(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	pages := make([]corpus.FacetObservationInput, 4)
	for i := range pages {
		payload, err := json.Marshal([]github.IssueComment{{
			ID: int64(i + 1), Body: "bounded comment", Author: "user",
			CreatedAt: fixture.now.Add(time.Duration(i) * time.Minute), UpdatedAt: fixture.now.Add(time.Duration(i) * time.Minute),
		}})
		if err != nil {
			t.Fatal(err)
		}
		pages[i] = corpus.FacetObservationInput{SourceUpdatedAt: fixture.now.Add(time.Duration(i) * time.Minute), Payload: string(payload)}
	}
	if err := fixture.svc.corpus.ApplyFacetObservationSet(fixture.ctx, fixture.repoID, &fixture.issueID, FacetIssueComments, fixture.now, pages, true, 0); err != nil {
		t.Fatal(err)
	}
	brief, err := fixture.svc.ThreadResearchBrief(fixture.ctx, research.ThreadRef{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if brief.Sections.Acceptance.Status != research.StatusPartial || !brief.Sections.Timeline.Truncated {
		t.Fatalf("bounded discussion was not explicit: acceptance=%+v timeline=%+v", brief.Sections.Acceptance.SectionMeta, brief.Sections.Timeline)
	}
	for _, facet := range brief.Sections.Coverage.Facets {
		if facet.Scope == "thread" && facet.Facet == FacetIssueComments {
			if !facet.Truncated || facet.Count != maxResearchFacetPages {
				t.Fatalf("bounded facet = %+v", facet)
			}
			return
		}
	}
	t.Fatal("missing issue comment coverage")
}

func hasRelated(values []research.RelatedThread, number int, relation string) bool {
	for _, value := range values {
		if value.Number == number && value.Relation == relation {
			return true
		}
	}
	return false
}

func researchCommandsContain(commands []research.NextCommand, value string) bool {
	for _, command := range commands {
		if strings.Contains(command.Command, value) {
			return true
		}
	}
	return false
}

func assertResearchHealth(t *testing.T, section research.HealthSection) {
	t.Helper()
	if section.Status != research.StatusPartial || section.ThreadSampleSize != 3 ||
		section.ExternalPRSampleSize != 1 || section.IssueResponseSampleSize != 1 ||
		section.PullRequestResponseSampleSize != 0 || !strings.Contains(section.UnknownReason, "response metrics") {
		t.Fatalf("health = %+v", section)
	}
}

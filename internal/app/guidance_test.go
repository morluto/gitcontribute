package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/github"
	"github.com/morluto/gitcontribute/internal/radar"
)

func TestExplicitSyncPersistsSourceBackedContributionGuidance(t *testing.T) {
	t.Parallel()
	const guidance = "We accept pull requests for issues labelled `help wanted`."
	base := &testServer{owner: "octocat", repo: "guided"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/repos/octocat/guided/contents/.github/CONTRIBUTING.md" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"type": "file", "path": ".github/CONTRIBUTING.md", "sha": "policy-sha",
				"html_url": "https://github.com/octocat/guided/blob/main/.github/CONTRIBUTING.md",
				"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(guidance)),
			})
			return
		}
		base.handler(w, r)
	}))
	defer srv.Close()

	ctx := context.Background()
	svc := newTestService(t, srv)
	defer func() { _ = svc.Close() }()
	if _, err := svc.Sync(ctx, cli.RepoRef{Owner: "octocat", Repo: "guided"}); err != nil {
		t.Fatal(err)
	}

	text, refs, err := (&corpusReader{s: svc}).ReadContributionGuidance(ctx, domain.RepoRef{Owner: "octocat", Repo: "guided"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "## .github/CONTRIBUTING.md") || !strings.Contains(text, guidance) {
		t.Fatalf("guidance = %q", text)
	}
	if len(refs) != 1 || refs[0].CommitSHA != "policy-sha" || refs[0].URL == "" {
		t.Fatalf("refs = %+v", refs)
	}

	repo, err := svc.corpus.GetRepository(ctx, "octocat", "guided")
	if err != nil || repo == nil {
		t.Fatalf("repository = %+v err=%v", repo, err)
	}
	coverage, err := svc.corpus.GetCoverage(ctx, repo.ID, nil, FacetContributionGuidance)
	if err != nil || coverage == nil || !coverage.Complete {
		t.Fatalf("coverage = %+v err=%v", coverage, err)
	}
}

func TestRadarClassifiesStoredPolicyAndNaturalLanguageClaimOffline(t *testing.T) {
	t.Parallel()
	fixture := newRadarTestFixture(t)
	repo, err := fixture.svc.corpus.GetRepository(fixture.ctx, "owner", "repo")
	if err != nil || repo == nil {
		t.Fatalf("repository = %+v err=%v", repo, err)
	}

	issue, err := fixture.svc.corpus.UpsertThread(fixture.ctx, corpus.Thread{
		RepositoryID: repo.ID, Kind: corpus.ThreadKindIssue, Number: 3, State: "open",
		Title: "Claimed help-wanted work", Body: "Steps to reproduce and expected behavior are documented.",
		Labels: []string{"help wanted"}, SourceUpdatedAt: fixture.now,
	}, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	file := github.RepositoryFile{
		Path: ".github/CONTRIBUTING.md", SHA: "policy-sha",
		HTMLURL: "https://github.com/owner/repo/blob/main/.github/CONTRIBUTING.md",
		Content: "We accept pull requests for issues labelled help wanted.",
	}
	payload, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.svc.corpus.ApplyFacetObservationSet(fixture.ctx, repo.ID, nil, FacetContributionGuidance, fixture.now, []corpus.FacetObservationInput{{
		SourceUpdatedAt: fixture.now, Payload: string(payload),
	}}, true, 0); err != nil {
		t.Fatal(err)
	}
	comments, err := json.Marshal([]github.IssueComment{{
		ID: 3, Author: "alice", AuthorAssociation: "NONE", Body: "I can pick this up.",
		CreatedAt: fixture.now.Add(-time.Hour), UpdatedAt: fixture.now.Add(-time.Hour),
		HTMLURL: "https://github.com/owner/repo/issues/3#issuecomment-3",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.svc.corpus.ApplyFacetObservationSet(fixture.ctx, repo.ID, &issue.ID, FacetIssueComments, fixture.now, []corpus.FacetObservationInput{{
		SourceUpdatedAt: fixture.now, Payload: string(comments),
	}}, true, 0); err != nil {
		t.Fatal(err)
	}

	report, err := fixture.svc.ContributionRadar(fixture.ctx, cli.RadarOptions{Repo: cli.RepoRef{Owner: "owner", Repo: "repo"}, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	candidate := radarCandidate(report, 3)
	if candidate == nil || candidate.Eligibility != radar.EligibilityNeedsCoordination {
		t.Fatalf("candidate = %+v", candidate)
	}
	if !radarSignal(candidate.PositiveSignals, "policy_allows_issue") || !radarSignal(candidate.Risks, "active_claim") {
		t.Fatalf("candidate signals = %+v / %+v", candidate.PositiveSignals, candidate.Risks)
	}
}

type interruptedGuidanceReader struct {
	github.Reader
}

func (interruptedGuidanceReader) GetRepositoryFile(_ context.Context, _, _, path string) (github.RepositoryFile, github.RateInfo, error) {
	if path == contributionGuidancePaths[0] {
		return github.RepositoryFile{Path: path, Content: "new guidance"}, github.RateInfo{}, nil
	}
	return github.RepositoryFile{}, github.RateInfo{}, errors.New("interrupted guidance retrieval")
}

func TestGuidanceRetrievalReplacesSnapshotOnlyAfterAllPathsComplete(t *testing.T) {
	t.Parallel()
	fixture := newRadarTestFixture(t)
	repo, err := fixture.svc.corpus.GetRepository(fixture.ctx, "owner", "repo")
	if err != nil || repo == nil {
		t.Fatalf("repository = %+v err=%v", repo, err)
	}
	oldFile := github.RepositoryFile{Path: "CONTRIBUTING.md", Content: "old complete guidance"}
	payload, err := json.Marshal(oldFile)
	if err != nil {
		t.Fatal(err)
	}
	oldAt := fixture.now.Add(-time.Hour)
	if err := fixture.svc.corpus.ApplyFacetObservationSet(fixture.ctx, repo.ID, nil, FacetContributionGuidance, oldAt, []corpus.FacetObservationInput{{
		SourceUpdatedAt: oldAt, Payload: string(payload),
	}}, true, 0); err != nil {
		t.Fatal(err)
	}

	err = syncRepositoryGuidance(fixture.ctx, fixture.svc.corpus, interruptedGuidanceReader{}, *repo, domain.RepoRef{Owner: "owner", Repo: "repo"}, fixture.now, 0, newSyncRequestBudget(maxSyncRequests))
	if err == nil || !strings.Contains(err.Error(), "interrupted guidance retrieval") {
		t.Fatalf("sync error = %v", err)
	}
	documents, err := readContributionGuidanceDocuments(fixture.ctx, fixture.svc.corpus, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || documents[0].File.Content != oldFile.Content {
		t.Fatalf("documents = %+v", documents)
	}
	coverage, err := fixture.svc.corpus.GetCoverage(fixture.ctx, repo.ID, nil, FacetContributionGuidance)
	if err != nil || coverage == nil || !coverage.Complete || !coverage.SourceUpdatedAt.Equal(oldAt) {
		t.Fatalf("coverage = %+v err=%v", coverage, err)
	}
}

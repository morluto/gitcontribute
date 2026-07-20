package radar

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/morluto/gitcontribute/internal/domain"
)

func TestRankOrdersEligibilityAndExplainsScore(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	repo := RepositorySnapshot{
		Repo:           domain.RepoRef{Owner: "owner", Repo: "repo"},
		SourceUpdated:  now.Add(-time.Hour),
		GuidanceStatus: "available",
		Coverage: []Coverage{
			{Facet: "metadata", Present: true, Complete: true, AsOf: now.Add(-time.Hour)},
			{Facet: "threads", Present: true, Complete: true, AsOf: now.Add(-time.Hour)},
		},
	}
	report, err := Rank(repo, []IssueSnapshot{
		{
			Number: 2, State: "open", Title: "Assigned work", Body: "A short description.",
			Assignees: []string{"alice"}, SourceUpdated: now.Add(-24 * time.Hour),
			URL: "https://github.com/owner/repo/issues/2",
		},
		{
			Number: 3, State: "open", Title: "Duplicate", Body: "Already handled.",
			Labels: []string{"duplicate"}, SourceUpdated: now.Add(-24 * time.Hour),
			URL: "https://github.com/owner/repo/issues/3",
		},
		{
			Number: 1, State: "open", Title: "Focused bug", Labels: []string{"good first issue", "help wanted"},
			Body:          strings.Repeat("Detailed reproduction and expected behavior. ", 8) + "\n- [ ] add regression test",
			SourceUpdated: now.Add(-24 * time.Hour),
			Discussion: SummarizeDiscussion([]DiscussionComment{{
				Author: "maintainer", AuthorAssociation: "MEMBER", Body: "Thanks for the detailed report.",
				URL: "https://github.com/owner/repo/issues/1#issuecomment-1", CreatedAt: now.Add(-time.Hour),
			}}, now),
			Coverage: []Coverage{{Facet: "issue_comments", Present: true, Complete: true, AsOf: now.Add(-time.Hour)}},
			URL:      "https://github.com/owner/repo/issues/1",
		},
	}, Options{Limit: 10, Now: now, TotalOpenIssues: 3})
	if err != nil {
		t.Fatal(err)
	}

	if got := []int{report.Candidates[0].Number, report.Candidates[1].Number, report.Candidates[2].Number}; !cmp.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("candidate order = %v", got)
	}
	if report.Candidates[0].Eligibility != EligibilityReadyToCode || report.Candidates[1].Eligibility != EligibilityNeedsCoordination || report.Candidates[2].Eligibility != EligibilityBlocked {
		t.Fatalf("eligibility order = %v, %v, %v", report.Candidates[0].Eligibility, report.Candidates[1].Eligibility, report.Candidates[2].Eligibility)
	}
	if report.Candidates[0].ScoreVersion != ScoreVersion || report.ScoreVersion != ScoreVersion {
		t.Fatalf("score versions = candidate %q report %q", report.Candidates[0].ScoreVersion, report.ScoreVersion)
	}
	if report.Candidates[0].Ref != "issue:owner/repo#1" {
		t.Fatalf("candidate ref = %q", report.Candidates[0].Ref)
	}
	for _, code := range []string{"beginner_label", "help_wanted", "acceptance_checklist", "maintainer_response"} {
		if !hasSignal(report.Candidates[0].PositiveSignals, code) {
			t.Fatalf("missing positive signal %q: %+v", code, report.Candidates[0].PositiveSignals)
		}
	}
	if report.Candidates[0].Confidence != "high" {
		t.Fatalf("confidence = %q, want high", report.Candidates[0].Confidence)
	}
}

func TestMissingCoverageIsUnknownNotPenalty(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	base := IssueSnapshot{
		Number: 1, State: "open", Title: "Issue", Body: "Description",
		SourceUpdated: now.Add(-24 * time.Hour), URL: "https://github.com/owner/repo/issues/1",
	}
	repo := RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, GuidanceStatus: "available"}
	missing, err := Rank(repo, []IssueSnapshot{base}, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	completeIssue := base
	completeIssue.Coverage = []Coverage{{Facet: "issue_comments", Present: true, Complete: true}}
	completeRepo := repo
	completeRepo.GuidanceStatus = "available"
	completeRepo.Coverage = []Coverage{
		{Facet: "metadata", Present: true, Complete: true},
		{Facet: "threads", Present: true, Complete: true},
	}
	complete, err := Rank(completeRepo, []IssueSnapshot{completeIssue}, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Candidates[0].Score != complete.Candidates[0].Score {
		t.Fatalf("missing coverage changed score: missing=%d complete=%d", missing.Candidates[0].Score, complete.Candidates[0].Score)
	}
	if len(missing.Candidates[0].Unknowns) != 3 {
		t.Fatalf("candidate unknowns = %+v", missing.Candidates[0].Unknowns)
	}
	if !strings.Contains(missing.Candidates[0].Unknowns[2].Remediation, "--max-pages 3") {
		t.Fatalf("comments remediation is not explicitly bounded: %+v", missing.Candidates[0].Unknowns)
	}
	if missing.Candidates[0].Eligibility != EligibilityNeedsDiagnosis || complete.Candidates[0].Eligibility != EligibilityReadyToCode {
		t.Fatalf("coverage eligibility: missing=%s complete=%s", missing.Candidates[0].Eligibility, complete.Candidates[0].Eligibility)
	}
	if len(complete.Candidates[0].Unknowns) != 0 {
		t.Fatalf("complete candidate unknowns = %+v", complete.Candidates[0].Unknowns)
	}
	candidate := complete.Candidates[0]
	reconstructed := candidate.BaseScore
	for _, signal := range append(candidate.PositiveSignals, candidate.Risks...) {
		reconstructed += signal.Weight
	}
	if reconstructed != candidate.Score {
		t.Fatalf("score is not reconstructable: base=%d signals=%d score=%d", candidate.BaseScore, reconstructed-candidate.BaseScore, candidate.Score)
	}
	payload, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"labels":[]`, `"risks":[]`, `"blockers":[]`, `"linked_pull_requests":[]`, `"related_work":[]`} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("candidate JSON missing stable empty array %s: %s", want, payload)
		}
	}
}

func TestClosingPullRequestBlocksCandidate(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}},
		[]IssueSnapshot{{
			Number: 7, State: "open", Title: "Bug", Body: "Description", SourceUpdated: now,
			URL:                "https://github.com/owner/repo/issues/7",
			LinkedPullRequests: []LinkedPullRequest{{Number: 9, Title: "Fix bug", URL: "https://github.com/owner/repo/pull/9", Closing: true}},
		}},
		Options{Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := report.Candidates[0]
	if got.Eligibility != EligibilityBlocked || !hasSignal(got.Blockers, "active_implementation") || !hasSignal(got.Risks, "active_closing_pr") {
		t.Fatalf("candidate = %+v", got)
	}
}

func TestOpenDependencyRequiresCoordinationWithoutBecomingBlocker(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, GuidanceStatus: "available", Coverage: []Coverage{
			{Facet: "metadata", Present: true, Complete: true}, {Facet: "threads", Present: true, Complete: true},
		}},
		[]IssueSnapshot{{
			Number: 7, State: "open", Title: "Bug", Body: "Description", SourceUpdated: now,
			URL:      "https://github.com/owner/repo/issues/7",
			Coverage: []Coverage{{Facet: "issue_comments", Present: true, Complete: true}},
			RelatedWork: []RelatedWork{{
				Ref: "pull_request:owner/repo#9", Kind: "pull_request", Number: 9, State: "open",
				Relation: "depends_on", Direction: "outbound", URL: "https://github.com/owner/repo/pull/9", Evidence: []RelatedWorkEvidence{},
			}},
		}},
		Options{Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := report.Candidates[0]
	if got.Eligibility != EligibilityNeedsCoordination || !hasSignal(got.Risks, "open_dependency") || len(got.Blockers) != 0 {
		t.Fatalf("candidate = %+v", got)
	}
}

func TestCappedRelatedWorkPreventsReadyToCodeClaim(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, GuidanceStatus: "available", Coverage: []Coverage{
			{Facet: "metadata", Present: true, Complete: true}, {Facet: "threads", Present: true, Complete: true},
		}},
		[]IssueSnapshot{{Number: 7, State: "open", Title: "Bug", Body: "Description", SourceUpdated: now,
			Coverage: []Coverage{{Facet: "issue_comments", Present: true, Complete: true}}, RelatedWorkCapped: true}},
		Options{Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := report.Candidates[0]
	if got.Eligibility != EligibilityNeedsCoordination || got.Confidence != "medium" || got.Unknowns[len(got.Unknowns)-1].Code != "related_work_scan_capped" {
		t.Fatalf("candidate = %+v", got)
	}
}

func TestUnknownRelatedPullRequestStatePreventsReadyToCodeClaim(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, GuidanceStatus: "available", Coverage: []Coverage{
			{Facet: "metadata", Present: true, Complete: true}, {Facet: "threads", Present: true, Complete: true},
		}},
		[]IssueSnapshot{{Number: 7, State: "open", Title: "Bug", Body: "Description", SourceUpdated: now,
			Coverage: []Coverage{{Facet: "issue_comments", Present: true, Complete: true}}, RelatedWork: []RelatedWork{{
				Ref: "pull_request:owner/repo#9", Kind: "pull_request", Number: 9, Relation: "explicit_reference", Direction: "outbound", Evidence: []RelatedWorkEvidence{},
			}}}},
		Options{Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := report.Candidates[0]
	if got.Eligibility != EligibilityNeedsCoordination || got.Confidence != "medium" || got.Unknowns[len(got.Unknowns)-1].Code != "related_pull_request_state_unknown" {
		t.Fatalf("candidate = %+v", got)
	}
	if len(got.Risks) != 0 || len(got.Blockers) != 0 {
		t.Fatalf("unknown state became negative evidence: risks=%+v blockers=%+v", got.Risks, got.Blockers)
	}
}

func TestClosedRelatedPullRequestIsBackgroundOnly(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, GuidanceStatus: "available", Coverage: []Coverage{
			{Facet: "metadata", Present: true, Complete: true}, {Facet: "threads", Present: true, Complete: true},
		}},
		[]IssueSnapshot{{Number: 7, State: "open", Title: "Bug", Body: "Description", SourceUpdated: now,
			Coverage: []Coverage{{Facet: "issue_comments", Present: true, Complete: true}}, RelatedWork: []RelatedWork{{
				Ref: "pull_request:owner/repo#9", Kind: "pull_request", Number: 9, State: "closed",
				Relation: "claims_to_close", Direction: "inbound", Evidence: []RelatedWorkEvidence{},
			}}}},
		Options{Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := report.Candidates[0]
	if got.Eligibility != EligibilityReadyToCode || len(got.Risks) != 0 || len(got.Blockers) != 0 || len(got.Unknowns) != 0 {
		t.Fatalf("closed related work affected eligibility: %+v", got)
	}
}

func TestRankUsesStableFinalTieBreakAndLimit(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	repo := RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}}
	issues := []IssueSnapshot{
		{Number: 8, State: "open", Title: "Same", Body: "Same", SourceUpdated: now},
		{Number: 2, State: "open", Title: "Same", Body: "Same", SourceUpdated: now},
	}
	report, err := Rank(repo, issues, Options{Limit: 1, Now: now, TotalOpenIssues: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].Number != 2 || report.Candidates[0].Rank != 1 {
		t.Fatalf("candidates = %+v", report.Candidates)
	}
	if report.CandidatePopulation != 2 || report.TotalOpenIssues != 2 {
		t.Fatalf("population = %+v", report)
	}
}

func TestRankRejectsUnsafeLimit(t *testing.T) {
	_, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}},
		nil,
		Options{Limit: MaxLimit + 1, Now: time.Now()},
	)
	if err == nil {
		t.Fatal("expected limit error")
	}
}

func TestRankReturnsMaximumBoundedPopulation(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	issues := make([]IssueSnapshot, MaxLimit)
	for i := range issues {
		issues[i] = IssueSnapshot{
			Number: i + 1, State: "open", Title: fmt.Sprintf("Issue %d", i+1), SourceUpdated: now,
		}
	}
	report, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}},
		issues,
		Options{Limit: MaxLimit, Now: now, TotalOpenIssues: MaxLimit},
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Limit != MaxLimit || report.CandidatePopulation != MaxLimit || len(report.Candidates) != MaxLimit {
		t.Fatalf("maximum report bounds = limit:%d population:%d returned:%d", report.Limit, report.CandidatePopulation, len(report.Candidates))
	}
}

func TestObjectiveStateUsesBlockersNotScore(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Archived: true},
		[]IssueSnapshot{{Number: 1, State: "closed", Title: "Finished", SourceUpdated: now}},
		Options{Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate := report.Candidates[0]
	if candidate.Eligibility != EligibilityBlocked || !hasSignal(candidate.Blockers, "issue_not_open") || !hasSignal(candidate.Blockers, "repository_archived") {
		t.Fatalf("objective blockers = %+v", candidate)
	}
	if candidate.Score == 0 {
		t.Fatal("objective eligibility was hidden behind a zero score")
	}
}

func TestRankReportsBoundedEvidenceScans(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := Rank(
		RepositorySnapshot{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, GuidanceStatus: "available"},
		nil,
		Options{Now: now, PopulationCapped: true, LinkedPullRequestScanCapped: true, DuplicateClusterScanCapped: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"candidate_population_capped", "linked_pull_request_scan_capped", "duplicate_cluster_scan_may_be_capped"} {
		found := false
		for _, unknown := range report.Unknowns {
			if unknown.Code == code {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing report unknown %q: %+v", code, report.Unknowns)
		}
	}
}

func TestCappedCollisionEvidenceCannotClaimEligibility(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	report, err := Rank(
		RepositorySnapshot{
			Repo: domain.RepoRef{Owner: "owner", Repo: "repo"},
			Coverage: []Coverage{
				{Facet: "metadata", Present: true, Complete: true},
				{Facet: "threads", Present: true, Complete: true},
			},
		},
		[]IssueSnapshot{{
			Number: 1, State: "open", Title: "Issue", Body: "Description", SourceUpdated: now,
			Coverage: []Coverage{{Facet: "issue_comments", Present: true, Complete: true}},
		}},
		Options{Now: now, LinkedPullRequestScanCapped: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate := report.Candidates[0]
	if candidate.Eligibility != EligibilityNeedsCoordination || candidate.Confidence != "medium" {
		t.Fatalf("capped candidate = %+v", candidate)
	}
	if candidate.Unknowns[len(candidate.Unknowns)-1].Code != "linked_pull_request_scan_capped" {
		t.Fatalf("candidate unknowns = %+v", candidate.Unknowns)
	}
}

func hasSignal(signals []Signal, code string) bool {
	for _, signal := range signals {
		if signal.Code == code {
			return true
		}
	}
	return false
}

package radar

import (
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

func TestContributionPolicyRequiresHelpWantedLabel(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	repo := completeEligibilityRepo(now)
	policyURL := "https://github.com/owner/repo/blob/main/.github/CONTRIBUTING.md"
	repo.Guidance = []GuidanceDocument{{
		Path: ".github/CONTRIBUTING.md", URL: policyURL,
		Content: "We accept pull requests for issues labelled `help wanted`. Do not open a pull request for issues without the `help wanted` label.",
	}}

	report, err := Rank(repo, []IssueSnapshot{
		completeEligibilityIssue(now, 1, nil, nil),
		completeEligibilityIssue(now, 2, []string{"help wanted"}, nil),
	}, Options{Now: now, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	allowed := candidateByNumber(t, report, 2)
	if allowed.Eligibility != EligibilityReadyToCode || !hasSignal(allowed.PositiveSignals, "policy_allows_issue") {
		t.Fatalf("allowed candidate = %+v", allowed)
	}
	denied := candidateByNumber(t, report, 1)
	if denied.Eligibility != EligibilityBlocked || !hasSignal(denied.Blockers, "contribution_policy_mismatch") {
		t.Fatalf("denied candidate = %+v", denied)
	}
	if denied.Blockers[0].SourceURL != policyURL {
		t.Fatalf("policy source = %q", denied.Blockers[0].SourceURL)
	}
}

func TestRestrictionLabelsSelectExplicitEligibility(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		label  string
		want   Eligibility
		signal string
	}{
		{name: "internal", label: "internal", want: EligibilityBlocked, signal: "external_contribution_restricted"},
		{name: "core", label: "core", want: EligibilityBlocked, signal: "external_contribution_restricted"},
		{name: "diagnosis", label: "status:needs-info", want: EligibilityNeedsDiagnosis, signal: "diagnosis_label"},
		{name: "triage", label: "needs-triage", want: EligibilityNeedsDiagnosis, signal: "diagnosis_label"},
		{name: "stale", label: "stale", want: EligibilityNeedsCoordination, signal: "coordination_label"},
		{name: "decision", label: "needs:decision", want: EligibilityNeedsCoordination, signal: "coordination_label"},
		{name: "tracking", label: "tracking issue", want: EligibilityNeedsCoordination, signal: "coordination_label"},
		{name: "gsoc", label: "gsoc", want: EligibilityNeedsCoordination, signal: "coordination_label"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := Rank(completeEligibilityRepo(now), []IssueSnapshot{
				completeEligibilityIssue(now, 1, []string{test.label}, nil),
			}, Options{Now: now})
			if err != nil {
				t.Fatal(err)
			}
			candidate := report.Candidates[0]
			if candidate.Eligibility != test.want || (!hasSignal(candidate.Risks, test.signal) && !hasSignal(candidate.Blockers, test.signal)) {
				t.Fatalf("candidate = %+v", candidate)
			}
		})
	}
}

func TestMaintainerDirectionOverridesGenericResponseBoost(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		body    string
		want    Eligibility
		signal  string
		section string
	}{
		{
			name: "declined", body: "Not accepting implementations for this. Otherwise, this is wontfix.",
			want: EligibilityBlocked, signal: "maintainer_declined", section: "blocker",
		},
		{
			name: "diagnosis", body: "Can you reproduce this on main and provide the checkhealth output?",
			want: EligibilityNeedsDiagnosis, signal: "maintainer_requested_diagnosis", section: "risk",
		},
		{
			name: "approved", body: "Pull requests are welcome; please send a PR.",
			want: EligibilityReadyToCode, signal: "maintainer_invitation", section: "positive",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			comments := []DiscussionComment{{
				Author: "maintainer", AuthorAssociation: "MEMBER", Body: test.body,
				URL: "https://github.com/owner/repo/issues/1#issuecomment-1", CreatedAt: now.Add(-time.Hour),
			}}
			report, err := Rank(completeEligibilityRepo(now), []IssueSnapshot{
				completeEligibilityIssue(now, 1, nil, comments),
			}, Options{Now: now})
			if err != nil {
				t.Fatal(err)
			}
			candidate := report.Candidates[0]
			var found bool
			switch test.section {
			case "blocker":
				found = hasSignal(candidate.Blockers, test.signal)
			case "risk":
				found = hasSignal(candidate.Risks, test.signal)
			case "positive":
				found = hasSignal(candidate.PositiveSignals, test.signal)
			}
			if candidate.Eligibility != test.want || !found {
				t.Fatalf("candidate = %+v", candidate)
			}
		})
	}
}

func TestRecentNaturalLanguageClaimRequiresCoordination(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		comments []DiscussionComment
		want     Eligibility
	}{
		{
			name: "active", comments: []DiscussionComment{{Author: "alice", Body: "I can pick this up.", CreatedAt: now.Add(-24 * time.Hour), URL: "claim"}},
			want: EligibilityNeedsCoordination,
		},
		{
			name: "released", comments: []DiscussionComment{
				{Author: "alice", Body: "I can pick this up.", CreatedAt: now.Add(-48 * time.Hour)},
				{Author: "alice", Body: "I am no longer working on this; feel free to take it.", CreatedAt: now.Add(-24 * time.Hour)},
			}, want: EligibilityReadyToCode,
		},
		{
			name: "expired", comments: []DiscussionComment{{Author: "alice", Body: "I will work on this.", CreatedAt: now.Add(-activeClaimWindow - time.Hour)}},
			want: EligibilityReadyToCode,
		},
		{
			name: "bot", comments: []DiscussionComment{{Author: "triage[bot]", Body: "I will work on this.", CreatedAt: now.Add(-time.Hour)}},
			want: EligibilityReadyToCode,
		},
		{
			name: "quoted", comments: []DiscussionComment{{Author: "bob", Body: "> I can pick this up.\nIs this still relevant?", CreatedAt: now.Add(-time.Hour)}},
			want: EligibilityReadyToCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := Rank(completeEligibilityRepo(now), []IssueSnapshot{
				completeEligibilityIssue(now, 1, nil, test.comments),
			}, Options{Now: now})
			if err != nil {
				t.Fatal(err)
			}
			candidate := report.Candidates[0]
			if candidate.Eligibility != test.want {
				t.Fatalf("eligibility = %s, candidate=%+v", candidate.Eligibility, candidate)
			}
			if test.want == EligibilityNeedsCoordination && !hasSignal(candidate.Risks, "active_claim") {
				t.Fatalf("active claim missing: %+v", candidate)
			}
		})
	}
}

func TestAIPolicyIsAnExplicitGate(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		guidance string
		want     Eligibility
		signal   string
	}{
		{name: "prohibited", guidance: "We do not accept contributions generated using AI.", want: EligibilityBlocked, signal: "ai_policy_block"},
		{name: "disclosure", guidance: "Contributors must disclose any generative AI use.", want: EligibilityNeedsCoordination, signal: "ai_disclosure_required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := completeEligibilityRepo(now)
			repo.Guidance = []GuidanceDocument{{Path: "AI_POLICY.md", Content: test.guidance, URL: "policy"}}
			report, err := Rank(repo, []IssueSnapshot{completeEligibilityIssue(now, 1, nil, nil)}, Options{Now: now})
			if err != nil {
				t.Fatal(err)
			}
			candidate := report.Candidates[0]
			if candidate.Eligibility != test.want || (!hasSignal(candidate.Risks, test.signal) && !hasSignal(candidate.Blockers, test.signal)) {
				t.Fatalf("candidate = %+v", candidate)
			}
		})
	}
}

func completeEligibilityRepo(now time.Time) RepositorySnapshot {
	return RepositorySnapshot{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, SourceUpdated: now,
		GuidanceStatus: "available", Guidance: []GuidanceDocument{{
			Path: "CONTRIBUTING.md", Content: "Contributions are welcome.", URL: "https://github.com/owner/repo/blob/main/CONTRIBUTING.md",
		}},
		Coverage: []Coverage{
			{Facet: "metadata", Scope: "repository", Present: true, Complete: true, AsOf: now},
			{Facet: "threads", Scope: "repository", Present: true, Complete: true, AsOf: now},
			{Facet: "contribution_guidance", Scope: "repository", Present: true, Complete: true, AsOf: now},
		},
	}
}

func completeEligibilityIssue(now time.Time, number int, labels []string, comments []DiscussionComment) IssueSnapshot {
	return IssueSnapshot{
		Number: number, State: "open", Title: "Focused bug", Body: "Steps to reproduce. Expected behavior differs from actual behavior.",
		Labels: labels, SourceUpdated: now, URL: "https://github.com/owner/repo/issues/1", Discussion: SummarizeDiscussion(comments, now),
		Coverage: []Coverage{{Facet: "issue_comments", Scope: "thread", Present: true, Complete: true, AsOf: now}},
	}
}

func candidateByNumber(t *testing.T, report *Report, number int) Candidate {
	t.Helper()
	for _, candidate := range report.Candidates {
		if candidate.Number == number {
			return candidate
		}
	}
	t.Fatalf("candidate #%d not found", number)
	return Candidate{}
}

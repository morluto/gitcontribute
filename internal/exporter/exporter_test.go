package exporter

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
)

var now = time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)

var (
	syntheticGitHubTokenA = "ghp_" + strings.Repeat("1", 36)
	syntheticGitHubTokenB = "ghp_" + strings.Repeat("0", 36)
)

func sampleDossier() *domain.Dossier {
	return &domain.Dossier{
		Repo: domain.MustRepoRef("owner", "repo"),
		Repository: domain.Repository{
			Ref:           domain.MustRepoRef("owner", "repo"),
			Description:   "token=" + syntheticGitHubTokenA + " secret=keep-quiet A test repository with Authorization: Bearer " + syntheticGitHubTokenB,
			Languages:     []string{"Go"},
			DefaultBranch: "main",
			License:       "MIT",
			Stars:         42,
			Watchers:      7,
			Forks:         3,
		},
		CommitSHA:                      "abc123",
		AsOf:                           now,
		OpenIssueCount:                 3,
		ClosedIssueCount:               7,
		OpenPullRequestCount:           2,
		MergedPullRequestCount:         5,
		ClosedUnmergedPullRequestCount: 1,
		ContributionGuidance:           "Please open an issue first. password=hunter2",
		SourceRefs: []domain.SourceRef{
			{Source: "github:graphql", URL: "https://api.github.com/graphql", ObservedAt: now.Add(-time.Hour), AsOf: now.Add(-time.Hour)},
			{Source: "github:rest", URL: "https://api.github.com/repos/owner/repo", ObservedAt: now, AsOf: now},
		},
		Coverage: domain.Coverage{
			AsOf: now,
			Facets: []domain.FacetCoverage{
				domain.MustFacetCoverage("threads", false, now.Add(-time.Hour), 0),
				domain.MustFacetCoverage("metadata", true, now, 0),
			},
		},
		RecentMergedPullRequests: []domain.DossierThread{
			{Number: 9, Title: "Second merged", Author: "bob", State: domain.ClosedState, UpdatedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-10 * time.Hour), MergedAt: now.Add(-3 * time.Hour)},
			{Number: 5, Title: "First merged", Author: "alice", State: domain.ClosedState, UpdatedAt: now.Add(-time.Hour), CreatedAt: now.Add(-12 * time.Hour), MergedAt: now.Add(-2 * time.Hour)},
		},
		RecentOpenPullRequests: []domain.DossierThread{
			{Number: 11, Title: "Open PR", Author: "charlie", State: domain.OpenState, UpdatedAt: now, CreatedAt: now.Add(-time.Hour)},
		},
		RecentClosedUnmergedPullRequests: []domain.DossierThread{
			{Number: 3, Title: "Closed unmerged", Author: "dave", State: domain.ClosedState, UpdatedAt: now.Add(-3 * time.Hour), CreatedAt: now.Add(-20 * time.Hour)},
		},
		RecentIssues: []domain.DossierThread{
			{Number: 7, Title: "Old issue", State: domain.ClosedState, UpdatedAt: now.Add(-4 * time.Hour), CreatedAt: now.Add(-24 * time.Hour)},
			{Number: 42, Title: "Recent issue", State: domain.OpenState, UpdatedAt: now.Add(-30 * time.Minute), CreatedAt: now.Add(-2 * time.Hour)},
		},
	}
}

func sampleEvidence() *contracts.EvidenceResult {
	return &contracts.EvidenceResult{
		InvestigationID: "inv-1",
		Evidence: []contracts.EvidenceItem{
			{
				ID: "ev-2", Type: "manual_observation", Relation: "supporting",
				Description:     " observed with Authorization: token " + syntheticGitHubTokenB,
				ValidationRunID: "run-1", OpportunityID: "opp-1", Freshness: "not_applicable",
				FreshnessReason: "local evidence has no corpus source revision", CreatedAt: now.Format(time.RFC3339),
			},
			{
				ID: "ev-1", Type: "github_source", Relation: "supporting",
				Description:     "Linked issue. api_key=supersecret",
				Freshness:       "stale",
				FreshnessReason: "thread issue:owner/repo#1 advanced from source_updated_at=2026-07-16T10:00:00Z",
				SourceProvenance: []contracts.EvidenceSourceRevisionResult{{
					Subject: contracts.EvidenceSourceSubjectResult{
						Kind: "thread", Owner: "owner", Repo: "repo", ThreadKind: "issue", Number: 1,
					},
					SourceUpdatedAt:     now.Add(-2 * time.Hour).Format(time.RFC3339),
					ObservationSequence: 7,
					ObservedAt:          now.Add(-90 * time.Minute).Format(time.RFC3339),
				}},
				CreatedAt: now.Add(-time.Hour).Format(time.RFC3339),
			},
		},
	}
}

func TestDossierJSONDeterministic(t *testing.T) {
	t.Parallel()
	d := sampleDossier()
	var a, b bytes.Buffer
	if err := ExportDossierJSON(&a, d); err != nil {
		t.Fatalf("first export: %v", err)
	}
	if err := ExportDossierJSON(&b, d); err != nil {
		t.Fatalf("second export: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatalf("dossier JSON export is not deterministic:\n%s\n---\n%s", a.String(), b.String())
	}
}

func TestDossierMarkdownDeterministic(t *testing.T) {
	t.Parallel()
	d := sampleDossier()
	var a, b bytes.Buffer
	if err := ExportDossierMarkdown(&a, d); err != nil {
		t.Fatalf("first export: %v", err)
	}
	if err := ExportDossierMarkdown(&b, d); err != nil {
		t.Fatalf("second export: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatalf("dossier markdown export is not deterministic:\n%s\n---\n%s", a.String(), b.String())
	}
}

func TestEvidenceJSONDeterministic(t *testing.T) {
	t.Parallel()
	e := sampleEvidence()
	var a, b bytes.Buffer
	if err := ExportEvidenceJSON(&a, e); err != nil {
		t.Fatalf("first export: %v", err)
	}
	if err := ExportEvidenceJSON(&b, e); err != nil {
		t.Fatalf("second export: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatalf("evidence JSON export is not deterministic:\n%s\n---\n%s", a.String(), b.String())
	}
}

func TestEvidenceMarkdownDeterministic(t *testing.T) {
	t.Parallel()
	e := sampleEvidence()
	var a, b bytes.Buffer
	if err := ExportEvidenceMarkdown(&a, e); err != nil {
		t.Fatalf("first export: %v", err)
	}
	if err := ExportEvidenceMarkdown(&b, e); err != nil {
		t.Fatalf("second export: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatalf("evidence markdown export is not deterministic:\n%s\n---\n%s", a.String(), b.String())
	}
}

func TestEvidenceMarkdownIncludesFreshnessAndSourceRevisions(t *testing.T) {
	t.Parallel()
	e := sampleEvidence()
	var buf bytes.Buffer
	if err := ExportEvidenceMarkdown(&buf, e); err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"freshness `stale`",
		"Freshness reason: thread issue:owner/repo#1 advanced",
		"Source revision: thread:owner/repo:issue:1:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown missing %q:\n%s", want, out)
		}
	}
}

func TestDossierRedaction(t *testing.T) {
	t.Parallel()
	d := sampleDossier()
	var buf bytes.Buffer
	if err := ExportDossierJSON(&buf, d); err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, syntheticGitHubTokenA) {
		t.Fatalf("GitHub token was not redacted in JSON output")
	}
	if strings.Contains(out, "hunter2") {
		t.Fatalf("password value was not redacted in JSON output")
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] placeholder in JSON output")
	}
	if strings.Contains(out, syntheticGitHubTokenB) {
		t.Fatalf("second GitHub token was not redacted in JSON output")
	}

	buf.Reset()
	if err := ExportDossierMarkdown(&buf, d); err != nil {
		t.Fatalf("markdown export: %v", err)
	}
	md := buf.String()
	if strings.Contains(md, "hunter2") {
		t.Fatalf("password value was not redacted in Markdown output")
	}
	if !strings.Contains(md, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] placeholder in Markdown output")
	}
}

func TestEvidenceRedaction(t *testing.T) {
	t.Parallel()
	e := sampleEvidence()
	var buf bytes.Buffer
	if err := ExportEvidenceJSON(&buf, e); err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, syntheticGitHubTokenB) {
		t.Fatalf("GitHub token in evidence description was not redacted")
	}
	if strings.Contains(out, "supersecret") {
		t.Fatalf("api_key value in evidence was not redacted")
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] placeholder in evidence JSON")
	}
}

func TestRedactStringCoversQuotedAndMultiTokenValues(t *testing.T) {
	t.Parallel()
	tests := []string{
		`{"api_key":"supersecret"}`,
		`auth_token: Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature`,
		`password = "two word value"`,
	}
	for _, input := range tests {
		got := redactString(input)
		for _, leaked := range []string{"supersecret", "eyJhbGciOiJIUzI1NiJ9", "two word value"} {
			if strings.Contains(got, leaked) {
				t.Fatalf("redactString(%q) leaked %q: %q", input, leaked, got)
			}
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Fatalf("redactString(%q) = %q, missing marker", input, got)
		}
	}
}

func TestEvidenceOrderByID(t *testing.T) {
	t.Parallel()
	e := sampleEvidence()
	var buf bytes.Buffer
	if err := ExportEvidenceMarkdown(&buf, e); err != nil {
		t.Fatalf("export: %v", err)
	}
	lines := strings.Split(buf.String(), "\n")
	var ids []string
	for _, line := range lines {
		if strings.HasPrefix(line, "- **ev-") {
			ids = append(ids, line[4:8])
		}
	}
	want := []string{"ev-1", "ev-2"}
	if len(ids) != len(want) {
		t.Fatalf("unexpected evidence lines: %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected evidence ordered %v, got %v", want, ids)
		}
	}
}

func TestNilInputs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := ExportDossierJSON(&buf, nil); err == nil {
		t.Fatal("expected error for nil dossier JSON")
	}
	if err := ExportDossierMarkdown(&buf, nil); err == nil {
		t.Fatal("expected error for nil dossier markdown")
	}
	if err := ExportEvidenceJSON(&buf, nil); err == nil {
		t.Fatal("expected error for nil evidence JSON")
	}
	if err := ExportEvidenceMarkdown(&buf, nil); err == nil {
		t.Fatal("expected error for nil evidence markdown")
	}
}

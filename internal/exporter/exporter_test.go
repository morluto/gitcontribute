package exporter

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
)

var now = time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)

func sampleDossier() *domain.Dossier {
	return &domain.Dossier{
		Repo: domain.RepoRef{Owner: "owner", Repo: "repo"},
		Repository: domain.Repository{
			RepoRef:       domain.RepoRef{Owner: "owner", Repo: "repo"},
			Description:   "token=ghp_123456789012345678901234567890123456 secret=keep-quiet A test repository with Authorization: Bearer ghp_000000000000000000000000000000000000",
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
				{Facet: "threads", Present: true, Complete: false, Freshness: domain.Freshness{Status: domain.Stale, AsOf: now.Add(-time.Hour)}},
				{Facet: "metadata", Present: true, Complete: true, Freshness: domain.Freshness{Status: domain.Fresh, AsOf: now}},
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

func sampleEvidence() *cli.EvidenceResult {
	return &cli.EvidenceResult{
		InvestigationID: "inv-1",
		Evidence: []cli.EvidenceItem{
			{ID: "ev-2", Type: "manual_observation", Relation: "supporting", Description: " observed with Authorization: token ghp_000000000000000000000000000000000000", ValidationRunID: "run-1", OpportunityID: "opp-1", CreatedAt: now.Format(time.RFC3339)},
			{ID: "ev-1", Type: "github_source", Relation: "supporting", Description: "Linked issue. api_key=supersecret", CreatedAt: now.Add(-time.Hour).Format(time.RFC3339)},
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

func TestDossierRedaction(t *testing.T) {
	t.Parallel()
	d := sampleDossier()
	var buf bytes.Buffer
	if err := ExportDossierJSON(&buf, d); err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "ghp_123456789012345678901234567890123456") {
		t.Fatalf("GitHub token was not redacted in JSON output")
	}
	if strings.Contains(out, "hunter2") {
		t.Fatalf("password value was not redacted in JSON output")
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] placeholder in JSON output")
	}
	if strings.Contains(out, "ghp_000000000000000000000000000000000000") {
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
	if strings.Contains(out, "ghp_000000000000000000000000000000000000") {
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

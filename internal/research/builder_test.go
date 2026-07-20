package research

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

type fakeResearchReader struct {
	repo            domain.Repository
	repoSources     []domain.SourceRef
	repoCoverage    domain.Coverage
	guidance        string
	guidanceSources []domain.SourceRef
	thread          ThreadEvidence
	relations       RelationshipEvidence
	code            CodeEvidence
	health          HealthEvidence
}

func (f *fakeResearchReader) ReadRepository(context.Context, domain.RepoRef) (domain.Repository, []domain.SourceRef, error) {
	return f.repo, f.repoSources, nil
}

func (f *fakeResearchReader) ReadCoverage(context.Context, domain.RepoRef) (domain.Coverage, error) {
	return f.repoCoverage, nil
}

func (f *fakeResearchReader) ReadContributionGuidance(context.Context, domain.RepoRef) (string, []domain.SourceRef, error) {
	return f.guidance, f.guidanceSources, nil
}

func (f *fakeResearchReader) ReadResearchThread(context.Context, ThreadRef) (ThreadEvidence, error) {
	return f.thread, nil
}

func (f *fakeResearchReader) ReadResearchRelationships(context.Context, ThreadRef, []Reference) (RelationshipEvidence, error) {
	return f.relations, nil
}

func (f *fakeResearchReader) ReadResearchCode(context.Context, domain.RepoRef, []string) (CodeEvidence, error) {
	return f.code, nil
}

func (f *fakeResearchReader) ReadResearchHealth(context.Context, domain.RepoRef) (HealthEvidence, error) {
	return f.health, nil
}

func TestBuilderMakesCoverageAndUnknownsExplicit(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	ref := ThreadRef{Repo: domain.RepoRef{Owner: "owner", Repo: "repo"}, Kind: domain.IssueKind, Number: 42}
	threadSource := SourceRef{Source: "github:rest", URL: "https://api.github.com/repos/owner/repo/issues/42", ObservedAt: now.Add(-time.Hour), AsOf: now.Add(-2 * time.Hour)}
	commentSource := SourceRef{Source: "github:rest", URL: "https://github.com/owner/repo/issues/42#issuecomment-1", ObservedAt: now.Add(-30 * time.Minute), AsOf: now.Add(-time.Hour)}
	localSource := SourceRef{Source: "local:relationships", URL: "local://relationships/owner/repo/42", AsOf: now.Add(-time.Hour)}
	reader := &fakeResearchReader{
		repo: domain.Repository{RepoRef: ref.Repo},
		repoSources: []domain.SourceRef{{
			Source: "github:rest", URL: "https://api.github.com/repos/owner/repo", ObservedAt: now.Add(-3 * time.Hour), AsOf: now.Add(-4 * time.Hour),
		}},
		repoCoverage: domain.Coverage{Facets: []domain.FacetCoverage{
			{Facet: "metadata", Present: true, Complete: true, Freshness: domain.Freshness{AsOf: now.Add(-4 * time.Hour)}},
			{Facet: "threads", Present: true, Complete: true, Freshness: domain.Freshness{AsOf: now.Add(-3 * time.Hour)}},
		}},
		thread: ThreadEvidence{
			Thread: ThreadSnapshot{
				Ref: ref, Title: "Parser <script> regression", Author: "alice", AuthorAssociation: "CONTRIBUTOR",
				Body:  "## Expected behavior\n- [ ] keep retries bounded\n# injected heading\ntoken=super-secret-value\nSee #7",
				State: "open", Labels: []string{"bug", "help wanted"}, CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-2 * time.Hour), Source: threadSource,
			},
			Discussion: []DiscussionItem{{
				ID: 1, Kind: "issue_comment", Body: "Please add a cancellation test; this must stay bounded.",
				Author: "maintainer", AuthorAssociation: "MEMBER", CreatedAt: now.Add(-time.Hour), Source: commentSource,
			}},
			Coverage: []FacetCoverage{{
				Facet: "issue_comments", Present: true, Complete: false, AsOf: now.Add(-time.Hour), Count: 1, Source: commentSource,
			}},
		},
		relations: RelationshipEvidence{
			DuplicateThreads: []RelatedThread{{
				Ref: "issue:owner/repo#7", Kind: "issue", Number: 7, Relation: "explicit_reference",
				Basis: "stored source text explicitly references this thread", URL: "https://github.com/owner/repo/issues/7", Source: threadSource,
			}},
			PullRequests: []RelatedThread{{
				Ref: "pull_request:owner/repo#9", Kind: "pull_request", Number: 9, State: "open", Relation: "claims_to_close",
				Basis: "open pull request uses a closing keyword for the target", URL: "https://github.com/owner/repo/pull/9", Source: localSource,
			}},
			Sources: []SourceRef{localSource},
		},
		code: CodeEvidence{Present: false},
		health: HealthEvidence{
			Available: true, OpenIssues: 4, OpenPullRequests: 1, ThreadSampleSize: 5,
			Sources: []SourceRef{{Source: "local:health", URL: "local://health/owner/repo", AsOf: now.Add(-3 * time.Hour)}},
		},
	}

	brief, err := NewBuilder(reader, func() time.Time { return now }).Build(context.Background(), ThreadRef{Repo: ref.Repo, Number: ref.Number})
	if err != nil {
		t.Fatalf("build brief: %v", err)
	}
	if brief.SchemaVersion != SchemaVersion || brief.Target.Kind != "issue" || brief.SourceAsOf != now.Add(-time.Hour) {
		t.Fatalf("unexpected brief header: %+v", brief)
	}
	if err := brief.ValidateProvenance(); err != nil {
		t.Fatalf("provenance: %v", err)
	}
	if brief.Sections.Acceptance.Status != StatusPartial || len(brief.Sections.Acceptance.Checklist) != 1 || len(brief.Sections.Acceptance.MaintainerStatements) != 1 {
		t.Fatalf("acceptance section = %+v", brief.Sections.Acceptance)
	}
	if brief.Sections.Code.Status != StatusUnknown || brief.Sections.Guidance.Status != StatusUnknown {
		t.Fatalf("missing code/guidance not explicit: code=%+v guidance=%+v", brief.Sections.Code, brief.Sections.Guidance)
	}
	if !containsCommand(brief.Sections.Next.Commands, "archive hydrate") || !containsCommand(brief.Sections.Next.Commands, "--max-pages 3") || !containsCommand(brief.Sections.Next.Commands, "gitcontribute index") {
		t.Fatalf("missing remediation commands: %+v", brief.Sections.Next.Commands)
	}

	var first, second bytes.Buffer
	if err := RenderMarkdown(&first, brief); err != nil {
		t.Fatalf("render markdown: %v", err)
	}
	if err := RenderMarkdown(&second, brief); err != nil {
		t.Fatalf("render markdown again: %v", err)
	}
	if first.String() != second.String() {
		t.Fatal("markdown output is not deterministic")
	}
	for _, leaked := range []string{"super-secret-value", "<script>"} {
		if strings.Contains(first.String(), leaked) {
			t.Fatalf("markdown leaked %q:\n%s", leaked, first.String())
		}
	}
	for _, want := range []string{"[REDACTED]", "&lt;script&gt;", "> # injected heading", "12. Next explicit commands"} {
		if !strings.Contains(first.String(), want) {
			t.Fatalf("markdown missing %q:\n%s", want, first.String())
		}
	}

	jsonA, err := json.Marshal(brief)
	if err != nil {
		t.Fatal(err)
	}
	jsonB, _ := json.Marshal(brief)
	if !bytes.Equal(jsonA, jsonB) || !strings.Contains(string(jsonA), `"schema_version":"research-brief.v1"`) {
		t.Fatalf("JSON contract is not deterministic: %s", jsonA)
	}
}

func TestParseThreadRef(t *testing.T) {
	tests := []struct {
		input string
		kind  domain.ThreadKind
	}{
		{"owner/repo#7", ""},
		{"issue:owner/repo#7", domain.IssueKind},
		{"pr:owner/repo#7", domain.PullRequestKind},
		{"PULL_REQUEST:owner/repo#7", domain.PullRequestKind},
	}
	for _, test := range tests {
		ref, err := ParseThreadRef(test.input)
		if err != nil {
			t.Fatalf("ParseThreadRef(%q): %v", test.input, err)
		}
		if ref.Kind != test.kind || ref.Repo.String() != "owner/repo" || ref.Number != 7 {
			t.Fatalf("ParseThreadRef(%q) = %+v", test.input, ref)
		}
	}
	for _, input := range []string{"owner/repo", "owner/repo#0", "issue:nope#1", "unknown:owner/repo#1"} {
		if _, err := ParseThreadRef(input); err == nil {
			t.Fatalf("ParseThreadRef(%q) unexpectedly succeeded", input)
		}
	}
}

func TestBuilderHonorsCancellationAndProvenanceValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewBuilder(&fakeResearchReader{}, time.Now).Build(ctx, ThreadRef{Repo: domain.RepoRef{Owner: "o", Repo: "r"}, Number: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled build error = %v", err)
	}
	brief := &Brief{}
	if err := brief.ValidateProvenance(); err == nil || !strings.Contains(err.Error(), "current_state") {
		t.Fatalf("missing provenance error = %v", err)
	}
}

func TestMarkdownScalarSanitizersDoNotAllowLineBreakout(t *testing.T) {
	if got := safeURL("https://example.test/path\n# heading"); strings.ContainsAny(got, "\r\n\t ") {
		t.Fatalf("safeURL retained whitespace: %q", got)
	}
	if got := code("path\n```shell"); strings.Contains(got, "\n") || strings.Contains(got, "`") {
		t.Fatalf("code retained fence characters: %q", got)
	}
}

func containsCommand(commands []NextCommand, text string) bool {
	for _, command := range commands {
		if strings.Contains(command.Command, text) {
			return true
		}
	}
	return false
}

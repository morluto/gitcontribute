package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/research"
)

type researchFakeService struct {
	*fakeService
	called bool
	ref    research.ThreadRef
	result *research.Brief
}

func (f *researchFakeService) ThreadResearchBrief(_ context.Context, ref research.ThreadRef) (*research.Brief, error) {
	f.called = true
	f.ref = ref
	return f.result, f.err
}

func TestResearchBriefMarkdownAndTypedRef(t *testing.T) {
	t.Parallel()
	svc := &researchFakeService{fakeService: &fakeService{}, result: researchCLIBrief()}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"research", "brief", "pr:o/r#7", "--format", "markdown"}))
	if !svc.called || svc.ref.Kind != domain.PullRequestKind || svc.ref.Repo.String() != "o/r" || svc.ref.Number != 7 {
		t.Fatalf("research call = called:%v ref:%+v", svc.called, svc.ref)
	}
	for _, want := range []string{"# Thread research brief: pull\\_request:o/r\\#7", "1. Current state", "12. Next explicit commands"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("markdown missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestResearchBriefJSONAndValidation(t *testing.T) {
	t.Parallel()
	svc := &researchFakeService{fakeService: &fakeService{}, result: researchCLIBrief()}
	c, stdout, _ := newTestCLI(svc, nil)
	requireNoErr(t, c.Run(context.Background(), []string{"research", "brief", "o/r#7", "--json"}))
	var got research.Brief
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid research JSON: %v\n%s", err, stdout.String())
	}
	if got.SchemaVersion != research.SchemaVersion || svc.ref.Kind != "" {
		t.Fatalf("brief/ref = %+v / %+v", got, svc.ref)
	}

	for _, args := range [][]string{
		{"research", "brief", "bad-ref"},
		{"research", "brief", "o/r#0"},
		{"research", "brief", "o/r#1", "--format", "yaml"},
	} {
		plain := &fakeService{}
		candidate, _, _ := newTestCLI(plain, nil)
		err := candidate.Run(context.Background(), args)
		if err == nil {
			t.Fatalf("expected usage error for %v", args)
		}
		requireCLIError(t, err, cli.ExitUsage)
	}
}

func TestResearchBriefRequiresCapability(t *testing.T) {
	t.Parallel()
	c, _, _ := newTestCLI(&fakeService{}, nil)
	err := c.Run(context.Background(), []string{"research", "brief", "o/r#1"})
	if err == nil {
		t.Fatal("expected not-wired error")
	}
	requireCLIError(t, err, cli.ExitNotWired)
}

func researchCLIBrief() *research.Brief {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	source := research.SourceRef{Source: "github:rest", URL: "https://api.github.com/repos/o/r/pulls/7", AsOf: now}
	available := research.SectionMeta{Status: research.StatusAvailable, Sources: []research.SourceRef{source}}
	unknown := research.SectionMeta{Status: research.StatusUnknown, Sources: []research.SourceRef{}, UnknownReason: "not stored"}
	return &research.Brief{
		SchemaVersion: research.SchemaVersion, GeneratedAt: now, SourceAsOf: now,
		Target: research.Target{Ref: "pull_request:o/r#7", Repository: "o/r", Kind: "pull_request", Number: 7, URL: "https://github.com/o/r/pull/7"},
		Sections: research.Sections{
			CurrentState: research.CurrentStateSection{SectionMeta: available, State: "open", Labels: []string{}},
			Problem:      research.ProblemSection{SectionMeta: available, Title: "Fix parser", Labels: []string{}, Assignees: []string{}},
			Acceptance: research.AcceptanceSection{
				SectionMeta: available, Checklist: []research.ChecklistHint{}, RelevantHeadings: []research.TextHint{},
				MaintainerStatements: []research.TextHint{}, Caveat: "Hints are not a complete contract.",
			},
			Participants: research.ParticipantsSection{SectionMeta: available, Participants: []research.Participant{}},
			Timeline:     research.TimelineSection{SectionMeta: available, Events: []research.TimelineEvent{}},
			Duplicates: research.DuplicateSection{
				SectionMeta: available, Candidates: []research.RelatedThread{}, Caveat: "Candidates are not decisions.",
			},
			PullRequests: research.PullRequestSection{SectionMeta: available, PullRequests: []research.RelatedThread{}},
			Code:         research.CodeSection{SectionMeta: unknown, Queries: []string{}, Hits: []research.CodeHit{}},
			Guidance:     research.GuidanceSection{SectionMeta: unknown},
			Health:       research.HealthSection{SectionMeta: available},
			Coverage:     research.CoverageSection{SectionMeta: available, Facets: []research.CoverageFact{}, Gaps: []string{}},
			Next: research.NextSection{SectionMeta: available, Commands: []research.NextCommand{{
				Reason: "inspect neighbors", Command: "gitcontribute neighbors o/r#7 --kind pull_request --limit 20",
			}}},
		},
	}
}

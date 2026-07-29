package app

import (
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/tuicontract"
)

func TestTUIResearchBriefProjectsStoredEvidenceWithoutNetwork(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)

	brief, err := fixture.svc.ResearchBrief(fixture.ctx, tuicontract.Item{
		Kind: "candidate",
		Ref:  "issue:owner/repo#1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if brief.Ref != "issue:owner/repo#1" || brief.Title != "Retry parser cancellation" {
		t.Fatalf("target = %+v", brief)
	}
	if !strings.Contains(brief.Problem, "Expected behavior") {
		t.Fatalf("problem = %q", brief.Problem)
	}
	if len(brief.ExpectedBehavior) == 0 || !strings.Contains(brief.ExpectedBehavior[0].Summary, "cancellation remains bounded") {
		t.Fatalf("expected behavior = %+v", brief.ExpectedBehavior)
	}
	if len(brief.Discussion) == 0 || !briefFactsContain(brief.Discussion, "regression test") {
		t.Fatalf("discussion = %+v", brief.Discussion)
	}
	if !briefFactsContain(brief.RelatedWork, "Older parser report") ||
		!briefFactsContain(brief.RelatedWork, "Implement retry cancellation") {
		t.Fatalf("related work = %+v", brief.RelatedWork)
	}
	if len(brief.MissingEvidence) == 0 || len(brief.SuggestedNext) == 0 {
		t.Fatalf("missing/next = %+v / %+v", brief.MissingEvidence, brief.SuggestedNext)
	}
}

func briefFactsContain(facts []tuicontract.BriefFact, text string) bool {
	for _, fact := range facts {
		if strings.Contains(fact.Summary, text) {
			return true
		}
	}
	return false
}

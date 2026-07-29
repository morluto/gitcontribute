package app

import (
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/tuicontract"
)

func TestTUIActionsExposeOnlyContextualApplicationOperations(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)

	candidate := tuicontract.Item{
		Kind: "candidate", ID: "owner/repo#1", Ref: "owner/repo#1", Title: "Retry parser cancellation",
	}
	actions, err := fixture.svc.Actions(fixture.ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].ID != tuiActionStartInvestigation ||
		actions[0].Capability != tuicontract.CapabilityLocalWrite || !actions[0].RequiresConfirmation {
		t.Fatalf("candidate actions = %+v", actions)
	}

	first, err := fixture.svc.ExecuteAction(fixture.ctx, tuicontract.ActionRequest{
		ActionID: tuiActionStartInvestigation,
		Item:     candidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Reload || !strings.Contains(first.Message, "Started investigation") {
		t.Fatalf("first action result = %+v", first)
	}

	investigations, err := fixture.svc.corpus.ListInvestigations(fixture.ctx)
	if err != nil || len(investigations) != 1 {
		t.Fatalf("investigations = (%+v, %v)", investigations, err)
	}
	hypotheses, err := fixture.svc.corpus.ListHypotheses(fixture.ctx, investigations[0].ID)
	if err != nil || len(hypotheses) != 1 {
		t.Fatalf("hypotheses = (%+v, %v)", hypotheses, err)
	}

	hypothesis := tuicontract.Item{Kind: "hypothesis", ID: hypotheses[0].ID, Title: hypotheses[0].Title}
	actions, err = fixture.svc.Actions(fixture.ctx, hypothesis)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 {
		t.Fatalf("hypothesis actions = %+v", actions)
	}
	for _, action := range actions {
		if action.Capability != tuicontract.CapabilityOfflineRead || action.RequiresConfirmation {
			t.Fatalf("research action crosses unexpected boundary: %+v", action)
		}
	}
	result, err := fixture.svc.ExecuteAction(fixture.ctx, tuicontract.ActionRequest{
		ActionID: tuiActionCheckDuplicates,
		Item:     hypothesis,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reload || !strings.Contains(result.Message, "similar local threads") {
		t.Fatalf("offline action result = %+v", result)
	}
	if result.Title != "Duplicate check complete" || len(result.Facts) == 0 ||
		result.SourceRevision == "" || len(result.Items) == 0 {
		t.Fatalf("duplicate action omitted structured result data: %+v", result)
	}

	result, err = fixture.svc.ExecuteAction(fixture.ctx, tuicontract.ActionRequest{
		ActionID: tuiActionCheckCollisions,
		Item:     hypothesis,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reload || !strings.Contains(result.Message, "competing local pull requests") {
		t.Fatalf("collision action result = %+v", result)
	}
	if result.Title != "Competing-work check complete" || len(result.Facts) == 0 ||
		result.SourceRevision == "" || len(result.Items) == 0 {
		t.Fatalf("collision action omitted structured result data: %+v", result)
	}

	opportunity, err := fixture.svc.PromoteOpportunity(
		fixture.ctx,
		hypotheses[0].ID,
		"parser cancellation is unbounded",
		"parser",
		"hang",
		"small",
		0.8,
	)
	if err != nil {
		t.Fatal(err)
	}
	opportunityItem := tuicontract.Item{Kind: "opportunity", ID: opportunity.ID, Title: opportunity.Title}
	actions, err = fixture.svc.Actions(fixture.ctx, opportunityItem)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 3 || actions[0].ID != tuiActionCheckReadiness {
		t.Fatalf("opportunity actions = %+v", actions)
	}
	for _, actionID := range []string{tuiActionCheckReadiness, tuiActionCheckDuplicates, tuiActionCheckCollisions} {
		result, err = fixture.svc.ExecuteAction(fixture.ctx, tuicontract.ActionRequest{
			ActionID: actionID,
			Item:     opportunityItem,
		})
		if err != nil {
			t.Fatalf("execute opportunity action %s: %v", actionID, err)
		}
		if result.Reload || result.Message == "" {
			t.Fatalf("opportunity action %s result = %+v", actionID, result)
		}
		if result.Title == "" || len(result.Facts) == 0 {
			t.Fatalf("opportunity action %s omitted structured result: %+v", actionID, result)
		}
	}

	repository := tuicontract.Item{Kind: "repository", ID: "R_1", Ref: "owner/repo", Title: "owner/repo"}
	actions, err = fixture.svc.Actions(fixture.ctx, repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].ID != tuiActionRefreshClusters ||
		actions[0].Capability != tuicontract.CapabilityLocalWrite || !actions[0].RequiresConfirmation {
		t.Fatalf("repository actions = %+v", actions)
	}
	result, err = fixture.svc.ExecuteAction(fixture.ctx, tuicontract.ActionRequest{
		ActionID: tuiActionRefreshClusters,
		Item:     repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reload || !strings.Contains(result.Message, "Cluster projection") {
		t.Fatalf("cluster refresh action result = %+v", result)
	}
	if result.Title != "Related-work refresh complete" || len(result.Facts) == 0 || result.SourceRevision == "" {
		t.Fatalf("cluster action omitted structured result data: %+v", result)
	}

	for _, kind := range []string{"investigation", "contribution", "cluster"} {
		actions, err = fixture.svc.Actions(fixture.ctx, tuicontract.Item{Kind: kind})
		if err != nil || len(actions) != 0 {
			t.Fatalf("%s actions = (%+v, %v), want none", kind, actions, err)
		}
	}
}

func TestTUIActionRejectsMismatchedItem(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	_, err := fixture.svc.ExecuteAction(fixture.ctx, tuicontract.ActionRequest{
		ActionID: tuiActionStartInvestigation,
		Item:     tuicontract.Item{Kind: "repository", Ref: "owner/repo"},
	})
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("mismatched action error = %v", err)
	}
}

func TestTUILocalWriteRunsAfterFreshServiceReadOnlyLoad(t *testing.T) {
	t.Parallel()
	fixture := newResearchFixture(t)
	paths := fixture.svc.paths
	if err := fixture.svc.Close(); err != nil {
		t.Fatal(err)
	}

	fresh, err := New(paths, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	data, err := fresh.Load(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Candidates) == 0 {
		t.Fatal("fixture produced no candidate")
	}
	result, err := fresh.ExecuteAction(fixture.ctx, tuicontract.ActionRequest{
		ActionID: tuiActionStartInvestigation,
		Item:     data.Candidates[0],
	})
	if err != nil {
		t.Fatalf("local write after read-only TUI load: %v", err)
	}
	if result.Target == nil || result.Target.Stage != "research" || !result.Reload {
		t.Fatalf("action result=%+v", result)
	}
}

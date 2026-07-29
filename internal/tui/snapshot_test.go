package tui

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

var updateTUIGolden = flag.Bool("update-tui", false, "update deterministic TUI golden snapshots")

func TestRendererSnapshots(t *testing.T) {
	testCases := []struct {
		name  string
		build func(t *testing.T) Model
	}{
		{"wide_workbench", func(t *testing.T) Model { return snapshotModel(t, 118, 36) }},
		{"medium_workbench", func(t *testing.T) Model { return snapshotModel(t, 88, 30) }},
		{"narrow_workbench", func(t *testing.T) Model { return snapshotModel(t, 60, 30) }},
		{"research_brief", snapshotBriefModel},
		{"action_palette", snapshotActionPaletteModel},
		{"confirmation", snapshotConfirmationModel},
		{"action_success", snapshotActionSuccessModel},
		{"action_failure", snapshotActionFailureModel},
		{"empty_corpus", snapshotEmptyModel},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			model := tc.build(t)
			got := snapshotText(model.View().Content)
			if lines := strings.Count(got, "\n"); lines != model.height {
				t.Fatalf("renderer emitted %d lines for a %d-row terminal:\n%s", lines, model.height, got)
			}
			path := filepath.Join("testdata", tc.name+".golden")
			if *updateTUIGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run go test ./internal/tui -run TestRendererSnapshots -update-tui)", path, err)
			}
			if got != string(want) {
				t.Fatalf("renderer snapshot differs from %s; review, then update with -update-tui\n%s", path, firstSnapshotDifference(string(want), got))
			}
		})
	}
}

func snapshotModel(t *testing.T, width, height int) Model {
	t.Helper()
	data := snapshotData()
	provider := snapshotActionProvider()
	briefs := &fakeBriefProvider{brief: snapshotBrief()}
	m := loadModel(t, &fakeReader{data: data}, WithActionProvider(provider), WithBriefProvider(briefs))
	model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return model.(Model)
}

func snapshotBriefModel(t *testing.T) Model {
	m := snapshotModel(t, 118, 36)
	model, cmd := m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected research brief command")
	}
	model, _ = m.Update(cmd())
	return model.(Model)
}

func snapshotActionPaletteModel(t *testing.T) Model {
	m := snapshotModel(t, 118, 36)
	model, cmd := m.Update(keyPress('a'))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected action discovery command")
	}
	model, _ = m.Update(cmd())
	return model.(Model)
}

func snapshotConfirmationModel(t *testing.T) Model {
	m := snapshotActionPaletteModel(t)
	model, _ := m.Update(keyPress(tea.KeyEnter))
	return model.(Model)
}

func snapshotActionSuccessModel(t *testing.T) Model {
	m := snapshotActionPaletteModel(t)
	model, _ := m.Update(keyPress(tea.KeyDown))
	m = model.(Model)
	model, cmd := m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected action execution command")
	}
	model, _ = m.Update(cmd())
	return model.(Model)
}

func snapshotActionFailureModel(t *testing.T) Model {
	data := snapshotData()
	provider := snapshotActionProvider()
	provider.executeErr = context.DeadlineExceeded
	m := loadModel(t, &fakeReader{data: data}, WithActionProvider(provider), WithBriefProvider(&fakeBriefProvider{brief: snapshotBrief()}))
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 36})
	m = model.(Model)
	model, cmd := m.Update(keyPress('a'))
	m = model.(Model)
	model, _ = m.Update(cmd())
	m = model.(Model)
	model, _ = m.Update(keyPress(tea.KeyDown))
	m = model.(Model)
	model, cmd = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	model, _ = m.Update(cmd())
	return model.(Model)
}

func snapshotEmptyModel(t *testing.T) Model {
	m := loadModel(t, &fakeReader{data: tuicontract.Data{}}, WithActionProvider(&fakeActionProvider{}), WithBriefProvider(&fakeBriefProvider{}))
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 36})
	return model.(Model)
}

func snapshotData() tuicontract.Data {
	data := sampleData()
	data.Candidates[0] = tuicontract.Item{
		Kind: "candidate", ID: "issue:openclaw/openclaw#45311", Ref: "issue:openclaw/openclaw#45311",
		Title: "Slack socket mode receives no inbound events", Status: "needs_diagnosis",
		Score: 82, Confidence: "medium", Detail: "A clear regression window is stored, but no local reproduction has been recorded.",
		Source: "https://github.com/openclaw/openclaw/issues/45311", AsOf: "2026-07-17T12:00:00Z",
		Assessment: &tuicontract.Assessment{
			Positive: []tuicontract.Fact{
				{Summary: "Clear regression window from 2026.3.11 to 2026.3.12."},
				{Summary: "A maintainer acknowledged the report."},
			},
			Risks: []tuicontract.Fact{
				{Summary: "No reproduction has been recorded."},
				{Summary: "Comments are not fully synchronized."},
			},
			Related: []tuicontract.Fact{{Summary: "PR #45287 · non-closing"}},
		},
		Actions: []tuicontract.Action{
			{ID: "start_investigation", Label: "Start investigation", Description: "Create a local investigation and initial hypothesis.", Capability: tuicontract.CapabilityLocalWrite, RequiresConfirmation: true},
			{ID: "check_duplicates", Label: "Check duplicates", Description: "Review stored issue and pull-request matches.", Capability: tuicontract.CapabilityOfflineRead},
		},
	}
	data.Candidates[1] = tuicontract.Item{
		Kind: "candidate", ID: "issue:openclaw/openclaw#49012", Ref: "issue:openclaw/openclaw#49012",
		Title: "Control UI authentication disconnects", Status: "needs_coordination",
		Score: 67, Confidence: "high", AsOf: "2026-07-16T12:00:00Z",
	}
	data.Repositories[0].Ref = "openclaw/openclaw"
	data.Repositories[0].Title = "openclaw/openclaw"
	data.SyncStatuses[0].Ref = "openclaw/openclaw"
	data.SyncStatuses[0].Title = "openclaw/openclaw"
	data.SyncStatuses[0].Commands = []string{"gitcontribute archive sync openclaw/openclaw"}
	return data
}

func snapshotActionProvider() *fakeActionProvider {
	return &fakeActionProvider{
		actions: snapshotData().Candidates[0].Actions,
		result: tuicontract.ActionResult{
			Title:   "Duplicate check complete",
			Message: "Stored related work was compared without network access.",
			Facts: []tuicontract.ActionResultFact{
				{Label: "Possible matches", Value: "3"},
				{Label: "Competing pull requests", Value: "1"},
			},
			Items: []tuicontract.ActionResultItem{
				{Ref: "openclaw/openclaw#45287", Title: "Restore Slack socket event delivery", Status: "open", Source: "local corpus"},
				{Ref: "openclaw/openclaw#56653", Title: "Slack reactions are dropped", Status: "open", Source: "local corpus"},
			},
			SourceRevision: "abc123",
		},
	}
}

func snapshotBrief() tuicontract.ResearchBrief {
	return tuicontract.ResearchBrief{
		Ref: "issue:openclaw/openclaw#45311", Title: "Slack socket mode receives no inbound events",
		SourceAsOf: "2026-07-17T12:00:00Z",
		Problem:    "After upgrading from 2026.3.11 to 2026.3.12, Slack socket mode connects successfully but receives no inbound message events.",
		ExpectedBehavior: []tuicontract.BriefFact{
			{Summary: "Connected socket-mode sessions forward inbound Slack events to the gateway.", Source: "issue body"},
		},
		Discussion: []tuicontract.BriefFact{
			{Summary: "A maintainer confirmed the affected release window.", Source: "maintainer comment"},
		},
		ReproductionStatus: tuicontract.BriefFact{Summary: "No successful local reproduction is stored.", Source: "local corpus"},
		RelatedWork: []tuicontract.BriefFact{
			{Summary: "PR #45287 may overlap but does not close the issue.", Source: "local pull-request index"},
		},
		MissingEvidence: []string{"Complete synchronized comments", "A minimal reproduction on 2026.3.12"},
		SuggestedNext:   []string{"Start an investigation and reproduce the socket event path locally."},
	}
}

func snapshotText(content string) string {
	content = ansi.Strip(content)
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func firstSnapshotDifference(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	limit := min(len(wantLines), len(gotLines))
	for i := 0; i < limit; i++ {
		if wantLines[i] != gotLines[i] {
			return "first difference at line " + fmt.Sprint(i+1) + "\nwant: " + wantLines[i] + "\n got: " + gotLines[i]
		}
	}
	return fmt.Sprintf("line counts differ: want %d, got %d", len(wantLines), len(gotLines))
}

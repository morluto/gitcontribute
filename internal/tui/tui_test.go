package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/morluto/gitcontribute/internal/tuicontract"

	tea "charm.land/bubbletea/v2"
)

type fakeReader struct {
	data      tuicontract.Data
	err       error
	loadCount int
}

type fakeActionProvider struct {
	actions      []tuicontract.Action
	result       tuicontract.ActionResult
	actionsErr   error
	executeErr   error
	executeCount int
	request      tuicontract.ActionRequest
}

type fakeBriefProvider struct {
	brief tuicontract.ResearchBrief
	err   error
	calls int
	item  tuicontract.Item
}

func (f *fakeBriefProvider) ResearchBrief(_ context.Context, item tuicontract.Item) (tuicontract.ResearchBrief, error) {
	f.calls++
	f.item = item
	return f.brief, f.err
}

func (f *fakeActionProvider) Actions(context.Context, tuicontract.Item) ([]tuicontract.Action, error) {
	return f.actions, f.actionsErr
}

func (f *fakeActionProvider) ExecuteAction(_ context.Context, request tuicontract.ActionRequest) (tuicontract.ActionResult, error) {
	f.executeCount++
	f.request = request
	return f.result, f.executeErr
}

func (f *fakeReader) Load(ctx context.Context) (tuicontract.Data, error) {
	f.loadCount++
	return f.data, f.err
}

func sampleData() tuicontract.Data {
	return tuicontract.Data{
		Candidates: []tuicontract.Item{
			{
				Kind: "candidate", ID: "owner/repo#1", Ref: "owner/repo#1", Title: "Fix bug",
				Status: "ready_to_code", Score: 76, Confidence: "high",
				Assessment: &tuicontract.Assessment{
					Positive: []tuicontract.Fact{{Code: "clear_scope", Summary: "The requested behavior is explicit."}},
					Risks:    []tuicontract.Fact{{Code: "coverage", Summary: "Discussion coverage is incomplete."}},
				},
			},
			{Kind: "candidate", ID: "owner/repo#2", Ref: "owner/repo#2", Title: "Add feature", Status: "needs_coordination", Score: 61},
		},
		Hypotheses: []tuicontract.Item{
			{Kind: "hypothesis", ID: "h1", Ref: "owner/repo:hypothesis:h1", Title: "Parser can panic", Status: "proposed"},
		},
		SyncStatuses: []tuicontract.Item{
			{
				Kind: "sync_status", ID: "owner/repo", Ref: "owner/repo", Title: "owner/repo",
				Status: "partial", AsOf: "2026-07-17T00:00:00Z",
				Detail: "Candidate ranking evidence is incomplete.",
				Coverage: []tuicontract.Facet{
					{Name: "metadata", Present: true, Complete: true, AsOf: "2026-07-17T00:00:00Z"},
					{Name: "threads", Present: true, Complete: false, AsOf: "2026-07-16T00:00:00Z"},
					{Name: "contribution_guidance", Present: false, Complete: false},
				},
				Assessment: &tuicontract.Assessment{
					Risks: []tuicontract.Fact{{Summary: "The latest thread sync is partial."}},
				},
				Commands: []string{"gitcontribute archive sync owner/repo"},
			},
		},
		Repositories: []tuicontract.Item{
			{
				Kind:     "repo",
				ID:       "1",
				Ref:      "owner/repo",
				Title:    "owner/repo",
				Subtitle: "Go · 100 stars",
				Source:   "github:rest",
				AsOf:     "2026-07-17T00:00:00Z",
				Coverage: []tuicontract.Facet{
					{Name: "metadata", Present: true, Complete: true, AsOf: "2026-07-17T00:00:00Z"},
					{Name: "threads", Present: true, Complete: false, AsOf: "2026-07-16T00:00:00Z"},
				},
			},
		},
		Threads: []tuicontract.Item{
			{Kind: "thread", ID: "2", Ref: "owner/repo#1", Title: "Fix bug", Subtitle: "open", Source: "github:rest", AsOf: "2026-07-17T00:00:00Z"},
			{Kind: "thread", ID: "3", Ref: "owner/repo#2", Title: "Add feature", Subtitle: "closed", Source: "github:rest", AsOf: "2026-07-16T00:00:00Z"},
		},
		Clusters: []tuicontract.Item{
			{Kind: "cluster", ID: "c1", Title: "Duplicate reports", Subtitle: "2 members"},
		},
		Investigations: []tuicontract.Item{
			{Kind: "investigation", ID: "i1", Title: "Investigate crash", Subtitle: "open"},
		},
		Opportunities: []tuicontract.Item{
			{Kind: "opportunity", ID: "o1", Title: "Refactor parser", Subtitle: "validated", Stage: "ready"},
			{Kind: "opportunity", ID: "o2", Title: "Bound retries", Subtitle: "reproduced", Stage: "validate"},
		},
		Contributions: []tuicontract.Item{
			{Kind: "contribution", ID: "p1", Title: "Fix parser panic", Status: "submitted"},
			{Kind: "contribution", ID: "p2", Title: "Document retry limits", Status: "prepared"},
		},
	}
}

func loadModel(t *testing.T, r tuicontract.Reader, opts ...Option) Model {
	t.Helper()
	m := New(context.Background(), r, opts...)

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init command")
	}

	msg := cmd()
	if _, ok := msg.(loadMsg); !ok {
		t.Fatalf("expected loadMsg, got %T", msg)
	}

	model, cmd := m.Update(msg)
	m = model.(Model)

	if cmd == nil {
		t.Fatal("expected load command after loadMsg")
	}

	loaded := cmd()
	model, _ = m.Update(loaded)
	return model.(Model)
}

func TestInitLoadsData(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	if !m.loaded {
		t.Fatal("expected model to be loaded")
	}
	if fake.loadCount != 1 {
		t.Fatalf("expected one Load call, got %d", fake.loadCount)
	}
	if len(m.filtered) != 2 {
		t.Fatalf("expected 2 candidates visible, got %d", len(m.filtered))
	}
}

func TestSearchFiltersLoadedData(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	// focus search
	model, _ := m.Update(keyPress('/'))
	m = model.(Model)

	// type "feature"
	for _, r := range "feature" {
		model, _ := m.Update(keyPress(r))
		m = model.(Model)
	}

	if got := len(m.filtered); got != 1 {
		t.Fatalf("expected 1 filtered thread, got %d", got)
	}
	item, _ := m.selectedItem()
	if item.Title != "Add feature" {
		t.Fatalf("expected 'Add feature', got %q", item.Title)
	}

	// search does not trigger another reader Load
	if fake.loadCount != 1 {
		t.Fatalf("search must not call Load; got %d calls", fake.loadCount)
	}
}

func TestViewSwitch(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	model, _ := m.Update(keyPress('9'))
	m = model.(Model)
	if m.view != viewSyncStatus {
		t.Fatalf("expected sync-status view, got %s", m.view)
	}
	model, _ = m.Update(keyPress('0'))
	m = model.(Model)
	if m.view != viewRelatedWork {
		t.Fatalf("expected related-work view from key 0, got %s", m.view)
	}

	// switch does not call reader
	if fake.loadCount != 1 {
		t.Fatalf("view switch must not call Load; got %d calls", fake.loadCount)
	}
}

func TestDetailShowsCoverageAndSource(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	model, _ := m.Update(keyPress('8'))
	m = model.(Model)
	model, _ = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)

	if m.focus != focusDetail {
		t.Fatal("expected detail pane to be focused")
	}

	out := m.renderDetail(36, 100)
	for _, want := range []string{"owner/repo", "Coverage", "metadata", "threads", "github:rest", "2026-07-17"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, out)
		}
	}
}

func TestDetailMakesLongTitlesAndSourcesFullyInspectable(t *testing.T) {
	data := sampleData()
	data.Repositories[0].Title = "An intentionally long repository title whose unique ending must remain visible"
	data.Repositories[0].Ref = "owner/repository-with-a-very-long-name-and-distinct-ref-ending"
	data.Repositories[0].Source = "https://example.invalid/owner/repository/blob/main/a/very/long/path/with-distinct-source-ending"
	m := loadModel(t, &fakeReader{data: data})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 36})
	m = model.(Model)
	model, _ = m.Update(keyPress('8'))
	m = model.(Model)
	model, _ = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)

	out := m.renderDetail(36, 100)
	for _, want := range []string{
		"An intentionally long repository",
		"title whose unique ending must",
		"remain visible",
		"distinct-ref-ending",
		"distinct-source-ending",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("detail omitted long-value segment %q:\n%s", want, out)
		}
	}
}

func TestEmptyState(t *testing.T) {
	fake := &fakeReader{data: tuicontract.Data{}}
	m := loadModel(t, fake)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 36})
	m = model.(Model)

	out := m.View().Content
	for _, want := range []string{
		"No contribution candidates yet",
		"The local corpus contains no repositories",
		"gitcontribute source add repos OWNER/REPO",
		"gitcontribute archive sync OWNER/REPO",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected empty state to contain %q, got:\n%s", want, out)
		}
	}
}

func TestEmptyCandidatesWithRepositoryPointsToSyncStatus(t *testing.T) {
	data := sampleData()
	data.Candidates = nil
	m := loadModel(t, &fakeReader{data: data})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 36})
	m = model.(Model)

	out := m.View().Content
	for _, want := range []string{
		"No contribution candidates yet",
		"Open Sync status [9]",
		"gitcontribute archive sync OWNER/REPO",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("candidate empty state omitted %q:\n%s", want, out)
		}
	}
}

func TestErrorState(t *testing.T) {
	fake := &fakeReader{err: context.Canceled}
	m := loadModel(t, fake)

	if m.err == nil {
		t.Fatal("expected error state")
	}
	out := m.View().Content
	if !strings.Contains(out, "Error") {
		t.Fatalf("expected error message, got:\n%s", out)
	}
}

func TestKeyboardHelp(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 50})
	m = model.(Model)

	model, _ = m.Update(keyPress('?'))
	m = model.(Model)

	if !m.help {
		t.Fatal("expected help to be visible")
	}

	out := m.View().Content
	for _, want := range []string{"Keyboard help", "quit", "filter"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, out)
		}
	}
	for _, want := range []string{
		"Discover · ranked issues",
		"Research · proposed hypotheses",
		"Active · investigations in progress",
		"Validate · opportunities needing evidence",
		"Ready · opportunities passing local checks",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected stage definition %q, got:\n%s", want, out)
		}
	}
}

func TestLocalWriteActionRequiresConfirmation(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)
	actions := &fakeActionProvider{
		actions: []tuicontract.Action{{
			ID: "start", Label: "Start investigation", Description: "Create local records.",
			Capability: tuicontract.CapabilityLocalWrite, RequiresConfirmation: true,
		}},
		result: tuicontract.ActionResult{Message: "Started investigation", Reload: true},
	}
	m.actionProvider = actions

	model, cmd := m.Update(keyPress('a'))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected action discovery command")
	}
	model, _ = m.Update(cmd())
	m = model.(Model)

	out := m.View().Content
	if !strings.Contains(out, "Start investigation") || !strings.Contains(out, "local corpus write") {
		t.Fatalf("expected typed action palette, got:\n%s", out)
	}

	model, cmd = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	if cmd != nil || !m.actionConfirm || actions.executeCount != 0 {
		t.Fatal("local write must wait for confirmation")
	}
	if !strings.Contains(m.View().Content, "No network access or GitHub mutation") {
		t.Fatalf("expected side-effect boundary in confirmation, got:\n%s", m.View().Content)
	}

	model, cmd = m.Update(keyPress('y'))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected confirmed action command")
	}
	model, reload := m.Update(cmd())
	m = model.(Model)
	if actions.executeCount != 1 || actions.request.ActionID != "start" {
		t.Fatalf("unexpected action execution: count=%d request=%+v", actions.executeCount, actions.request)
	}
	if reload == nil || m.actionMsg != "Started investigation" {
		t.Fatalf("expected successful action to request reload, message=%q", m.actionMsg)
	}
}

func TestOfflineReadActionRunsWithoutConfirmation(t *testing.T) {
	m := loadModel(t, &fakeReader{data: sampleData()})
	actions := &fakeActionProvider{
		actions: []tuicontract.Action{{
			ID: "check", Label: "Find similar threads",
			Capability: tuicontract.CapabilityOfflineRead,
		}},
		result: tuicontract.ActionResult{Message: "Found 2 similar threads"},
	}
	m.actionProvider = actions

	model, cmd := m.Update(keyPress('a'))
	m = model.(Model)
	model, _ = m.Update(cmd())
	m = model.(Model)
	model, cmd = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	if cmd == nil || m.actionConfirm {
		t.Fatal("offline read should execute without confirmation")
	}
	model, reload := m.Update(cmd())
	m = model.(Model)
	if actions.executeCount != 1 || reload != nil || m.actionMsg != "Found 2 similar threads" {
		t.Fatalf("unexpected offline action outcome: count=%d message=%q", actions.executeCount, m.actionMsg)
	}
}

func TestActionCompletionOpensStructuredResultAndEscReturnsToWorkbench(t *testing.T) {
	data := sampleData()
	data.Candidates[0].Actions = []tuicontract.Action{{
		ID: "duplicates", Label: "Check duplicates", Capability: tuicontract.CapabilityOfflineRead,
	}}
	provider := &fakeActionProvider{
		actions: data.Candidates[0].Actions,
		result: tuicontract.ActionResult{
			Title:   "Duplicate check complete",
			Message: "Found possible overlapping work.",
			Facts: []tuicontract.ActionResultFact{
				{Label: "Possible matches", Value: "3"},
				{Label: "Competing pull requests", Value: "1"},
			},
			Items: []tuicontract.ActionResultItem{{
				Ref: "pr:owner/repo#9", Title: "Fix retry cancellation", Status: "open",
			}},
			SourceRevision: "abc123",
		},
	}
	m := loadModel(t, &fakeReader{data: data}, WithActionProvider(provider))
	origin := m.String()

	model, cmd := m.Update(keyPress('a'))
	m = model.(Model)
	model, _ = m.Update(cmd())
	m = model.(Model)
	model, cmd = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	model, _ = m.Update(cmd())
	m = model.(Model)

	out := m.View().Content
	for _, want := range []string{
		"ACTION RESULT", "Duplicate check complete", "Possible matches", "3",
		"Competing pull requests", "Source revision", "abc123", "Fix retry cancellation",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("structured action result omitted %q:\n%s", want, out)
		}
	}

	model, _ = m.Update(keyPress(tea.KeyEsc))
	m = model.(Model)
	if got := m.String(); got != origin || !strings.Contains(m.View().Content, "DISCOVER") {
		t.Fatalf("result return lost workbench origin: got %q, want %q", got, origin)
	}
}

func TestActionResultCanOpenCreatedWorkflowItemAfterReload(t *testing.T) {
	data := sampleData()
	data.Candidates[0].Actions = []tuicontract.Action{{
		ID: "start", Label: "Start investigation",
		Capability: tuicontract.CapabilityLocalWrite, RequiresConfirmation: true,
	}}
	provider := &fakeActionProvider{
		actions: data.Candidates[0].Actions,
		result: tuicontract.ActionResult{
			Title:   "Investigation started",
			Message: "Created a seed hypothesis.",
			Target:  &tuicontract.ActionTarget{Stage: "research", ID: "h1"},
			Reload:  true,
		},
	}
	reader := &fakeReader{data: data}
	m := loadModel(t, reader, WithActionProvider(provider))

	model, cmd := m.Update(keyPress('a'))
	m = model.(Model)
	model, _ = m.Update(cmd())
	m = model.(Model)
	model, _ = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	model, cmd = m.Update(keyPress('y'))
	m = model.(Model)
	model, reload := m.Update(cmd())
	m = model.(Model)
	if reload == nil || !m.resultOpen {
		t.Fatal("successful local write must show its result while reloading")
	}
	model, _ = m.Update(reload())
	m = model.(Model)
	model, _ = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)

	if got := m.String(); got != "hypothesis:h1" || m.view != viewResearch {
		t.Fatalf("result target = %q view=%q, want hypothesis:h1 in research", got, m.view)
	}
}

func TestActionPaletteNavigationCancellationAndErrors(t *testing.T) {
	t.Parallel()
	t.Run("no provider", func(t *testing.T) {
		m := loadModel(t, &fakeReader{data: sampleData()})
		model, cmd := m.Update(keyPress('a'))
		m = model.(Model)
		if cmd != nil || m.actionOpen {
			t.Fatal("action key must be inert without an application provider")
		}
	})

	t.Run("no contextual actions", func(t *testing.T) {
		m := loadModel(t, &fakeReader{data: sampleData()})
		m.actionProvider = &fakeActionProvider{}
		model, cmd := m.Update(keyPress('a'))
		m = model.(Model)
		model, _ = m.Update(cmd())
		m = model.(Model)
		if m.actionOpen || m.actionMsg != "No actions available for this item" {
			t.Fatalf("empty action outcome = open:%v message:%q", m.actionOpen, m.actionMsg)
		}
	})

	t.Run("choose second action", func(t *testing.T) {
		provider := &fakeActionProvider{
			actions: []tuicontract.Action{
				{ID: "first", Label: "First", Capability: tuicontract.CapabilityOfflineRead},
				{ID: "second", Label: "Second", Capability: tuicontract.CapabilityOfflineRead},
			},
			result: tuicontract.ActionResult{Message: "done"},
		}
		m := loadModel(t, &fakeReader{data: sampleData()})
		m.actionProvider = provider
		model, cmd := m.Update(keyPress('a'))
		m = model.(Model)
		model, _ = m.Update(cmd())
		m = model.(Model)
		model, _ = m.Update(keyPress(tea.KeyDown))
		m = model.(Model)
		model, cmd = m.Update(keyPress(tea.KeyEnter))
		m = model.(Model)
		model, _ = m.Update(cmd())
		m = model.(Model)
		if provider.request.ActionID != "second" {
			t.Fatalf("executed %q, want second", provider.request.ActionID)
		}
	})

	t.Run("cancel confirmation", func(t *testing.T) {
		provider := &fakeActionProvider{actions: []tuicontract.Action{{
			ID: "write", Label: "Write", Capability: tuicontract.CapabilityLocalWrite, RequiresConfirmation: true,
		}}}
		m := loadModel(t, &fakeReader{data: sampleData()})
		m.actionProvider = provider
		model, cmd := m.Update(keyPress('a'))
		m = model.(Model)
		model, _ = m.Update(cmd())
		m = model.(Model)
		model, _ = m.Update(keyPress(tea.KeyEnter))
		m = model.(Model)
		model, _ = m.Update(keyPress('n'))
		m = model.(Model)
		if m.actionConfirm || !m.actionOpen || provider.executeCount != 0 {
			t.Fatal("cancel must return to the palette without executing")
		}
		model, _ = m.Update(keyPress(tea.KeyEsc))
		m = model.(Model)
		if m.actionOpen {
			t.Fatal("escape must close the action palette")
		}
	})

	t.Run("discovery failure", func(t *testing.T) {
		m := loadModel(t, &fakeReader{data: sampleData()})
		provider := &fakeActionProvider{actionsErr: context.DeadlineExceeded}
		m.actionProvider = provider
		model, cmd := m.Update(keyPress('a'))
		m = model.(Model)
		model, _ = m.Update(cmd())
		m = model.(Model)
		if !m.actionOpen || !strings.Contains(m.View().Content, "ACTION FAILED") {
			t.Fatalf("expected visible discovery failure, got:\n%s", m.View().Content)
		}
		provider.actionsErr = nil
		provider.actions = []tuicontract.Action{{
			ID: "read", Label: "Check duplicates", Capability: tuicontract.CapabilityOfflineRead,
		}}
		model, cmd = m.Update(keyPress(tea.KeyEnter))
		m = model.(Model)
		if cmd == nil || !m.actionLoading {
			t.Fatal("Enter must retry failed action discovery")
		}
		model, _ = m.Update(cmd())
		m = model.(Model)
		if m.actionErr != nil || len(m.actions) != 1 || !strings.Contains(m.View().Content, "Check duplicates") {
			t.Fatalf("action discovery retry did not recover:\n%s", m.View().Content)
		}
	})

	t.Run("execution failure", func(t *testing.T) {
		provider := &fakeActionProvider{
			actions:    []tuicontract.Action{{ID: "read", Label: "Check duplicates", Capability: tuicontract.CapabilityOfflineRead}},
			executeErr: context.Canceled,
			result:     tuicontract.ActionResult{Title: "Duplicate check complete", Message: "No matches."},
		}
		m := loadModel(t, &fakeReader{data: sampleData()})
		m.actionProvider = provider
		model, cmd := m.Update(keyPress('a'))
		m = model.(Model)
		model, _ = m.Update(cmd())
		m = model.(Model)
		model, cmd = m.Update(keyPress(tea.KeyEnter))
		m = model.(Model)
		model, _ = m.Update(cmd())
		m = model.(Model)
		for _, want := range []string{"ACTION FAILED", "Check duplicates", "context canceled", "Recovery", "Enter"} {
			if !m.actionOpen || !strings.Contains(m.View().Content, want) {
				t.Fatalf("expected visible execution failure containing %q, got:\n%s", want, m.View().Content)
			}
		}
		provider.executeErr = nil
		model, cmd = m.Update(keyPress(tea.KeyEnter))
		m = model.(Model)
		model, _ = m.Update(cmd())
		m = model.(Model)
		if !m.resultOpen || !strings.Contains(m.View().Content, "Duplicate check complete") {
			t.Fatalf("expected visible execution failure, got:\n%s", m.View().Content)
		}
	})
}

func TestSearchExitAndDetailClose(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	// open search
	model, _ := m.Update(keyPress('/'))
	m = model.(Model)
	if !m.searching {
		t.Fatal("expected search to be active")
	}

	// close search
	model, _ = m.Update(keyPress(tea.KeyEsc))
	m = model.(Model)
	if m.searching {
		t.Fatal("expected search to be inactive")
	}

	// Open detail for a non-candidate item. Candidate Enter opens its research
	// brief instead.
	model, _ = m.Update(keyPress('8'))
	m = model.(Model)
	model, _ = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	if m.focus != focusDetail {
		t.Fatal("expected detail focus")
	}

	// close detail
	model, _ = m.Update(keyPress(tea.KeyEsc))
	m = model.(Model)
	if m.focus != focusList {
		t.Fatal("expected list focus")
	}
}

func TestCandidateEnterOpensResearchBriefAndEscRestoresSelection(t *testing.T) {
	m := loadModel(t, &fakeReader{data: sampleData()})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 36})
	m = model.(Model)
	model, _ = m.Update(keyPress(tea.KeyDown))
	m = model.(Model)
	before := m.String()

	model, _ = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	out := m.View().Content
	for _, want := range []string{"RESEARCH BRIEF", "Problem", "Add feature"} {
		if !strings.Contains(out, want) {
			t.Fatalf("candidate Enter did not open a research brief containing %q:\n%s", want, out)
		}
	}

	model, _ = m.Update(keyPress(tea.KeyEsc))
	m = model.(Model)
	if got := m.String(); got != before {
		t.Fatalf("selection changed after closing brief: got %q, want %q", got, before)
	}
	if !strings.Contains(m.View().Content, "CONTRIBUTIONS") {
		t.Fatalf("Esc did not restore the workbench:\n%s", m.View().Content)
	}
}

func TestCandidateEnterOpensResearchBriefAtEveryResponsiveSize(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{"wide", 118, 36},
		{"medium", 88, 30},
		{"narrow", 60, 30},
	} {
		size := size
		t.Run(size.name, func(t *testing.T) {
			m := loadModel(t, &fakeReader{data: sampleData()})
			model, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			m = model.(Model)
			model, _ = m.Update(keyPress(tea.KeyEnter))
			m = model.(Model)
			if !m.briefOpen || !strings.Contains(m.View().Content, "RESEARCH BRIEF") {
				t.Fatalf("candidate Enter did not open the brief at %dx%d:\n%s", size.width, size.height, m.View().Content)
			}
		})
	}
}

func TestResearchBriefLoadingFailureAndRetryHaveRecoveryGuidance(t *testing.T) {
	briefs := &fakeBriefProvider{err: context.DeadlineExceeded}
	m := loadModel(t, &fakeReader{data: sampleData()}, WithBriefProvider(briefs))
	model, cmd := m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	if cmd == nil || !strings.Contains(m.View().Content, "Loading stored research brief") {
		t.Fatalf("brief loading state is not visible:\n%s", m.View().Content)
	}
	model, _ = m.Update(cmd())
	m = model.(Model)
	for _, want := range []string{"Research brief unavailable", "Recovery", "Enter to retry", "Esc to return"} {
		if !strings.Contains(m.View().Content, want) {
			t.Fatalf("brief failure omitted %q:\n%s", want, m.View().Content)
		}
	}
	briefs.err = nil
	briefs.brief = snapshotBrief()
	model, cmd = m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("brief retry did not issue a local read")
	}
	model, _ = m.Update(cmd())
	m = model.(Model)
	if m.briefErr != nil || !strings.Contains(m.View().Content, "Expected behavior") {
		t.Fatalf("brief retry did not recover:\n%s", m.View().Content)
	}
}

func TestCandidateResearchBriefRendersStoredEvidenceSections(t *testing.T) {
	briefs := &fakeBriefProvider{brief: tuicontract.ResearchBrief{
		Ref:        "issue:owner/repo#1",
		Title:      "Fix bug",
		Problem:    "Requests can hang after cancellation.",
		SourceAsOf: "2026-07-17T00:00:00Z",
		ExpectedBehavior: []tuicontract.BriefFact{{
			Summary: "Cancellation should stop the request.", Source: "issue:owner/repo#1",
		}},
		Discussion: []tuicontract.BriefFact{{
			Summary: "A maintainer requested a regression test.", Source: "comment:17",
		}},
		ReproductionStatus: tuicontract.BriefFact{Summary: "No stored reproduction is available."},
		RelatedWork: []tuicontract.BriefFact{{
			Summary: "PR #9 may overlap.", Source: "pr:owner/repo#9",
		}},
		MissingEvidence: []string{"issue_comments"},
		SuggestedNext:   []string{"gitcontribute hydrate owner/repo#1 --facets issue_comments"},
	}}
	m := loadModel(t, &fakeReader{data: sampleData()}, WithBriefProvider(briefs))
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 48})
	m = model.(Model)
	model, cmd := m.Update(keyPress(tea.KeyEnter))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("candidate Enter must request the stored research brief")
	}
	model, _ = m.Update(cmd())
	m = model.(Model)

	out := m.View().Content
	for _, want := range []string{
		"Problem", "Requests can hang", "Expected behavior", "Cancellation should stop",
		"Relevant discussion", "maintainer requested", "Reproduction status",
		"Related work", "PR #9", "Missing evidence", "issue_comments",
		"Suggested next step", "gitcontribute hydrate",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("research brief omitted %q:\n%s", want, out)
		}
	}
	if briefs.calls != 1 || briefs.item.Ref != "owner/repo#1" {
		t.Fatalf("brief provider calls=%d item=%+v", briefs.calls, briefs.item)
	}
}

func TestWideWorkbenchKeepsWorkflowListAndEvidenceVisible(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)
	m.actionProvider = &fakeActionProvider{}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 36})
	m = model.(Model)

	out := m.View().Content
	for _, want := range []string{"CONTRIBUTIONS", "DISCOVER", "READY TO CODE", "Why it matters", "Risks", "1/2", "a actions"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected workbench to contain %q, got:\n%s", want, out)
		}
	}
}

func TestDetailShowsSelectedItemsPrimaryContextualAction(t *testing.T) {
	data := sampleData()
	data.Candidates[0].Actions = []tuicontract.Action{{
		ID: "start_investigation", Label: "Start investigation",
		Capability: tuicontract.CapabilityLocalWrite, RequiresConfirmation: true,
	}}
	data.Hypotheses[0].Actions = []tuicontract.Action{{
		ID: "check_duplicates", Label: "Check duplicates",
		Capability: tuicontract.CapabilityOfflineRead,
	}}
	provider := &fakeActionProvider{}
	m := loadModel(t, &fakeReader{data: data}, WithActionProvider(provider))

	if out := m.View().Content; !strings.Contains(out, "[a] Start investigation") {
		t.Fatalf("candidate detail omitted its primary action:\n%s", out)
	}
	model, _ := m.Update(keyPress('2'))
	m = model.(Model)
	if out := m.View().Content; !strings.Contains(out, "[a] Check duplicates") {
		t.Fatalf("hypothesis detail omitted its primary action:\n%s", out)
	}
}

func TestDetailKeepsPrimaryActionVisibleWhenEvidenceOverflows(t *testing.T) {
	data := sampleData()
	data.Candidates[0].Actions = []tuicontract.Action{{
		ID: "start_investigation", Label: "Start investigation",
		Capability: tuicontract.CapabilityLocalWrite, RequiresConfirmation: true,
	}}
	data.Candidates[0].Assessment = &tuicontract.Assessment{}
	for i := 0; i < 20; i++ {
		data.Candidates[0].Assessment.Risks = append(data.Candidates[0].Assessment.Risks, tuicontract.Fact{
			Summary: fmt.Sprintf("Stored risk %02d needs review before work starts.", i),
		})
	}
	m := loadModel(t, &fakeReader{data: data}, WithActionProvider(&fakeActionProvider{}))
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 24})
	m = model.(Model)

	if out := m.View().Content; !strings.Contains(out, "[a] Start investigation") {
		t.Fatalf("overflowing evidence hid the primary next action:\n%s", out)
	}
}

func TestSyncStatusExplainsCoverageFreshnessAndExplicitRecovery(t *testing.T) {
	m := loadModel(t, &fakeReader{data: sampleData()})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 36})
	m = model.(Model)
	model, _ = m.Update(keyPress('9'))
	m = model.(Model)

	out := m.View().Content
	for _, want := range []string{
		"SYNC STATUS", "owner/repo", "PARTIAL", "Candidate ranking evidence is incomplete",
		"metadata", "threads", "contribution_guidance", "latest thread sync is partial",
		"gitcontribute archive sync owner/repo",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("sync status omitted %q:\n%s", want, out)
		}
	}
}

func TestListUsesViewportAndKeepsCursorVisible(t *testing.T) {
	data := sampleData()
	data.Candidates = nil
	for i := 0; i < 20; i++ {
		data.Candidates = append(data.Candidates, tuicontract.Item{
			Kind: "candidate", ID: fmt.Sprint(i), Ref: fmt.Sprintf("owner/repo#%d", i),
			Title: fmt.Sprintf("Candidate %02d", i), Status: "needs_diagnosis",
		})
	}
	m := loadModel(t, &fakeReader{data: data})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	m = model.(Model)
	for range 12 {
		model, _ = m.Update(keyPress(tea.KeyDown))
		m = model.(Model)
	}

	out := m.View().Content
	if !strings.Contains(out, "Candidate 12") || strings.Contains(out, "Candidate 00") {
		t.Fatalf("expected viewport to follow cursor, got:\n%s", out)
	}
}

func TestWorkbenchNeverExceedsTerminalViewport(t *testing.T) {
	t.Parallel()
	for _, size := range []struct {
		width  int
		height int
	}{
		{40, 12},
		{60, 20},
		{72, 24},
		{107, 30},
		{108, 30},
		{118, 36},
		{160, 48},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := loadModel(t, &fakeReader{data: sampleData()})
			model, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			m = model.(Model)
			assertViewportBounds(t, m.View().Content, size.width, size.height)

			m.actionProvider = &fakeActionProvider{actions: []tuicontract.Action{{
				ID: "start", Label: "Start investigation", Description: "Create local records.",
				Capability: tuicontract.CapabilityLocalWrite, RequiresConfirmation: true,
			}}}
			model, cmd := m.Update(keyPress('a'))
			m = model.(Model)
			model, _ = m.Update(cmd())
			m = model.(Model)
			assertViewportBounds(t, m.View().Content, size.width, size.height)
		})
	}
}

func assertViewportBounds(t *testing.T, content string, width, height int) {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		t.Fatalf("rendered %d lines into %d-row terminal", len(lines), height)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d rendered width %d into %d-column terminal: %q", i+1, got, width, line)
		}
	}
}

func TestQuit(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	_, cmd := m.Update(keyPress('q'))
	if cmd == nil {
		t.Fatal("expected quit command")
	}

	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected quit message, got %T", msg)
	}
}

func keyPress(code rune) tea.KeyPressMsg {
	key := tea.Key{Code: code}
	if unicode.IsPrint(code) {
		key.Text = string(code)
	}
	return tea.KeyPressMsg(key)
}

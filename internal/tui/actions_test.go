package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

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

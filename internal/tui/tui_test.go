package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeReader struct {
	data      Data
	err       error
	loadCount int
}

func (f *fakeReader) Load(ctx context.Context) (Data, error) {
	f.loadCount++
	return f.data, f.err
}

func sampleData() Data {
	return Data{
		Repositories: []Item{
			{
				Kind:     "repo",
				ID:       "1",
				Ref:      "owner/repo",
				Title:    "owner/repo",
				Subtitle: "Go · 100 stars",
				Source:   "github:rest",
				AsOf:     "2026-07-17T00:00:00Z",
				Coverage: []Facet{
					{Name: "metadata", Present: true, Complete: true, AsOf: "2026-07-17T00:00:00Z"},
					{Name: "threads", Present: true, Complete: false, AsOf: "2026-07-16T00:00:00Z"},
				},
			},
		},
		Threads: []Item{
			{Kind: "thread", ID: "2", Ref: "owner/repo#1", Title: "Fix bug", Subtitle: "open", Source: "github:rest", AsOf: "2026-07-17T00:00:00Z"},
			{Kind: "thread", ID: "3", Ref: "owner/repo#2", Title: "Add feature", Subtitle: "closed", Source: "github:rest", AsOf: "2026-07-16T00:00:00Z"},
		},
		Clusters: []Item{
			{Kind: "cluster", ID: "c1", Title: "Duplicate reports", Subtitle: "2 members"},
		},
		Investigations: []Item{
			{Kind: "investigation", ID: "i1", Title: "Investigate crash", Subtitle: "open"},
		},
		Opportunities: []Item{
			{Kind: "opportunity", ID: "o1", Title: "Refactor parser", Subtitle: "validated"},
		},
	}
}

func loadModel(t *testing.T, r Reader) Model {
	t.Helper()
	m := New(r)

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
	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 repository visible, got %d", len(m.filtered))
	}
}

func TestSearchFiltersLoadedData(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	// switch to threads view
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = model.(Model)

	// focus search
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = model.(Model)

	// type "feature"
	for _, r := range "feature" {
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
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

	for i := 0; i < len(viewOrder); i++ {
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = model.(Model)
	}

	if m.view != viewRepositories {
		t.Fatalf("expected to cycle back to repositories, got %s", m.view)
	}

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = model.(Model)
	if m.view != viewClusters {
		t.Fatalf("expected clusters view, got %s", m.view)
	}

	// switch does not call reader
	if fake.loadCount != 1 {
		t.Fatalf("view switch must not call Load; got %d calls", fake.loadCount)
	}
}

func TestDetailShowsCoverageAndSource(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)

	if m.detail == nil {
		t.Fatal("expected detail to be set")
	}

	out := m.View()
	for _, want := range []string{"owner/repo", "Coverage:", "metadata", "threads", "github:rest", "2026-07-17"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, out)
		}
	}
}

func TestEmptyState(t *testing.T) {
	fake := &fakeReader{data: Data{}}
	m := loadModel(t, fake)

	out := m.View()
	if !strings.Contains(out, "No items") {
		t.Fatalf("expected empty state message, got:\n%s", out)
	}
}

func TestErrorState(t *testing.T) {
	fake := &fakeReader{err: context.Canceled}
	m := loadModel(t, fake)

	if m.err == nil {
		t.Fatal("expected error state")
	}
	out := m.View()
	if !strings.Contains(out, "Error:") {
		t.Fatalf("expected error message, got:\n%s", out)
	}
}

func TestKeyboardHelp(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = model.(Model)

	if !m.help {
		t.Fatal("expected help to be visible")
	}

	out := m.View()
	for _, want := range []string{"Keyboard help", "quit", "filter", "refresh/hydration"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRefreshActionIntent(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = model.(Model)

	if !strings.Contains(m.actionMsg, "Refresh requested") {
		t.Fatalf("expected refresh action intent, got %q", m.actionMsg)
	}
	if fake.loadCount != 1 {
		t.Fatalf("refresh action must not call Load; got %d calls", fake.loadCount)
	}
}

func TestSearchExitAndDetailClose(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	// open search
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = model.(Model)
	if !m.searching {
		t.Fatal("expected search to be active")
	}

	// close search
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	if m.searching {
		t.Fatal("expected search to be inactive")
	}

	// open detail
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if m.detail == nil {
		t.Fatal("expected detail")
	}

	// close detail
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	if m.detail != nil {
		t.Fatal("expected detail to be closed")
	}
}

func TestQuit(t *testing.T) {
	fake := &fakeReader{data: sampleData()}
	m := loadModel(t, fake)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command")
	}

	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected quit message, got %T", msg)
	}
}

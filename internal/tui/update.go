package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type loadMsg struct{}

type loadedMsg struct {
	data Data
	err  error
}

// Update processes messages and returns the updated model and any commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadMsg:
		m.loading = true
		m.err = nil
		return m, m.loadCmd()

	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.loaded = false
			m.items = make(map[view][]Item)
			m.filtered = nil
			return m, nil
		}
		m.loaded = true
		m.items = map[view][]Item{
			viewRepositories:   msg.data.Repositories,
			viewThreads:        msg.data.Threads,
			viewClusters:       msg.data.Clusters,
			viewInvestigations: msg.data.Investigations,
			viewOpportunities:  msg.data.Opportunities,
		}
		m.applyFilter()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 4 {
			m.search.Width = m.width - 4
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.search.Focused() {
		switch msg.String() {
		case "esc":
			m.searching = false
			m.search.Blur()
			return m, nil
		case "enter":
			m.searching = false
			m.search.Blur()
			return m, nil
		case "up":
			m.cursorUp()
			return m, nil
		case "down":
			m.cursorDown()
			return m, nil
		}

		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.applyFilter()
		return m, cmd
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		m.nextView()
		return m, nil
	case "shift+tab":
		m.prevView()
		return m, nil
	case "1", "2", "3", "4", "5":
		if v, ok := viewKeys[rune(msg.String()[0])]; ok {
			m.switchView(v)
		}
		return m, nil
	case "/":
		m.searching = true
		cmd := m.search.Focus()
		return m, cmd
	case "up", "k":
		m.cursorUp()
		return m, nil
	case "down", "j":
		m.cursorDown()
		return m, nil
	case "enter":
		m.showDetail()
		return m, nil
	case "esc":
		if m.detail != nil {
			m.detail = nil
		} else if m.help {
			m.help = false
		}
		return m, nil
	case "?":
		m.help = !m.help
		return m, nil
	case "r":
		return m.requestAction()
	}

	return m, nil
}

// String returns a short human-readable identifier for the current selection.
func (m Model) String() string {
	if m.detail != nil {
		return fmt.Sprintf("%s:%s", m.detail.Kind, m.detail.ID)
	}
	if it, ok := m.selectedItem(); ok {
		return fmt.Sprintf("%s:%s", it.Kind, it.ID)
	}
	return string(m.view)
}

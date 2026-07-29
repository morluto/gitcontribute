package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

type loadMsg struct{}

type loadedMsg struct {
	data tuicontract.Data
	err  error
}

type actionsLoadedMsg struct {
	actions []tuicontract.Action
	err     error
}

type actionCompletedMsg struct {
	result tuicontract.ActionResult
	err    error
}

type briefLoadedMsg struct {
	itemRef string
	brief   tuicontract.ResearchBrief
	err     error
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
			m.items = make(map[view][]tuicontract.Item)
			m.windows = make(map[view]tuicontract.Window)
			m.filtered = nil
			return m, nil
		}
		m.loaded = true
		m.loadData(msg.data)
		m.applyFilter()
		return m, nil

	case actionsLoadedMsg:
		m.actionLoading = false
		m.actionErr = msg.err
		m.actions = msg.actions
		if msg.err == nil && len(msg.actions) == 0 {
			m.actionOpen = false
			m.actionMsg = "No actions available for this item"
		}
		return m, nil

	case actionCompletedMsg:
		m.actionExecuting = false
		if msg.err != nil {
			m.actionErr = msg.err
			return m, nil
		}
		m.actionOpen = false
		m.actionMsg = msg.result.Message
		m.actionResult = msg.result
		m.resultOpen = true
		m.resultTop = 0
		if msg.result.Reload {
			return m, m.loadCmd()
		}
		return m, nil

	case briefLoadedMsg:
		if msg.itemRef != m.briefItem.Ref {
			return m, nil
		}
		m.briefLoading = false
		m.briefErr = msg.err
		m.brief = msg.brief
		return m, nil

	case tea.WindowSizeMsg:
		m.width = max(40, msg.Width)
		m.height = max(12, msg.Height)
		if m.width < wideLayoutMinimum && m.focus == focusNavigation {
			m.focus = focusList
		}
		m.resizeSearch()
		m.ensureCursorVisible()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) loadData(data tuicontract.Data) {
	validate, ready := splitByStage(data.Opportunities, "ready")
	submitted, needsYou := splitByStatus(data.Contributions, "submitted")
	m.items = map[view][]tuicontract.Item{
		viewDiscover:     data.Candidates,
		viewResearch:     data.Hypotheses,
		viewActive:       data.Investigations,
		viewValidate:     validate,
		viewReady:        ready,
		viewSubmitted:    submitted,
		viewNeedsYou:     needsYou,
		viewRepositories: data.Repositories,
		viewSyncStatus:   data.SyncStatuses,
		viewRelatedWork:  data.Clusters,
	}
	m.windows = make(map[view]tuicontract.Window)
	for _, spec := range viewSpecs {
		window := data.Windows[spec.key]
		if spec.view == viewValidate || spec.view == viewReady ||
			spec.view == viewSubmitted || spec.view == viewNeedsYou {
			window.Total = len(m.items[spec.view])
		}
		m.windows[spec.view] = window
	}
}

func splitByStage(items []tuicontract.Item, stage string) (other, matching []tuicontract.Item) {
	for _, item := range items {
		if item.Stage == stage {
			matching = append(matching, item)
		} else {
			other = append(other, item)
		}
	}
	return other, matching
}

func splitByStatus(items []tuicontract.Item, status string) (matching, other []tuicontract.Item) {
	for _, item := range items {
		if item.Status == status {
			matching = append(matching, item)
		} else {
			other = append(other, item)
		}
	}
	return matching, other
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.resultOpen {
		switch msg.String() {
		case "esc", "q":
			m.resultOpen = false
		case "enter":
			m.resultOpen = false
			m.focusActionTarget(m.actionResult.Target)
		case "up", "k":
			m.resultTop = max(0, m.resultTop-1)
		case "down", "j":
			m.resultTop++
		case "pgup":
			m.resultTop = max(0, m.resultTop-max(1, m.height-6))
		case "pgdown":
			m.resultTop += max(1, m.height-6)
		case "home":
			m.resultTop = 0
		}
		return m, nil
	}

	if m.briefOpen {
		switch msg.String() {
		case "esc", "q":
			m.briefOpen = false
		case "enter":
			if m.briefErr != nil {
				return m.openBrief(m.briefItem)
			}
		case "up", "k":
			m.briefTop = max(0, m.briefTop-1)
		case "down", "j":
			m.briefTop++
		case "pgup":
			m.briefTop = max(0, m.briefTop-max(1, m.height-6))
		case "pgdown":
			m.briefTop += max(1, m.height-6)
		case "home":
			m.briefTop = 0
		}
		return m, nil
	}

	if m.actionOpen {
		return m.handleActionKey(msg)
	}

	if m.search.Focused() {
		switch msg.String() {
		case "esc", "enter":
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

	if m.help {
		switch msg.String() {
		case "?", "esc", "enter":
			m.help = false
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		m.cycleFocus(false)
	case "shift+tab":
		m.cycleFocus(true)
	case "[", "h":
		if m.focus == focusDetail {
			m.focus = focusList
		} else {
			m.prevView()
		}
	case "]", "l":
		switch m.focus {
		case focusNavigation:
			m.focus = focusList
		case focusList:
			m.focus = focusDetail
		default:
			m.nextView()
		}
	case "/":
		m.searching = true
		m.focus = focusList
		return m, m.search.Focus()
	case "up", "k":
		m.moveUp()
	case "down", "j":
		m.moveDown()
	case "pgup":
		m.movePage(-1)
	case "pgdown":
		m.movePage(1)
	case "home":
		m.moveHome()
	case "end":
		m.moveEnd()
	case "enter":
		if m.focus == focusNavigation {
			m.focus = focusList
		} else if item, ok := m.selectedItem(); ok && item.Kind == "candidate" {
			return m.openBrief(item)
		} else if m.focus == focusList {
			m.focus = focusDetail
			m.detailTop = 0
		}
	case "esc":
		if m.focus == focusDetail {
			m.focus = focusList
		}
	case "?":
		m.help = true
	case "a":
		return m.openActions()
	default:
		if len(msg.String()) == 1 {
			key := msg.String()[0]
			if key >= '1' && key <= '9' {
				m.switchView(viewOrder[int(key-'1')])
			} else if key == '0' && len(viewOrder) >= 10 {
				m.switchView(viewOrder[9])
			}
		}
	}
	return m, nil
}

func (m Model) handleActionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.actionLoading || m.actionExecuting {
		if msg.String() == "esc" && m.actionLoading {
			m.actionOpen = false
		}
		return m, nil
	}
	if m.actionErr != nil {
		switch msg.String() {
		case "esc", "q":
			m.actionOpen = false
		case "enter":
			if len(m.actions) == 0 {
				return m.retryActionDiscovery()
			}
			return m.executeSelectedAction()
		}
		return m, nil
	}
	if m.actionConfirm {
		switch msg.String() {
		case "y", "enter":
			return m.executeSelectedAction()
		case "n", "esc":
			m.actionConfirm = false
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "q":
		m.actionOpen = false
	case "up", "k":
		m.actionCursor = max(0, m.actionCursor-1)
	case "down", "j":
		m.actionCursor = min(max(0, len(m.actions)-1), m.actionCursor+1)
	case "enter":
		action, ok := m.selectedAction()
		if !ok {
			return m, nil
		}
		if action.RequiresConfirmation {
			m.actionConfirm = true
			return m, nil
		}
		return m.executeSelectedAction()
	}
	return m, nil
}

func (m *Model) moveUp() {
	switch m.focus {
	case focusNavigation:
		m.prevView()
	case focusList:
		m.cursorUp()
	case focusDetail:
		m.detailTop = max(0, m.detailTop-1)
	}
}

func (m *Model) moveDown() {
	switch m.focus {
	case focusNavigation:
		m.nextView()
	case focusList:
		m.cursorDown()
	case focusDetail:
		m.detailTop++
	}
}

func (m *Model) movePage(direction int) {
	page := max(1, m.listPageSize())
	if m.focus == focusDetail {
		m.detailTop = max(0, m.detailTop+direction*page)
		return
	}
	if m.focus == focusNavigation {
		return
	}
	m.cursor = max(0, min(len(m.filtered)-1, m.cursor+direction*page))
	m.detailTop = 0
	m.ensureCursorVisible()
}

func (m *Model) moveHome() {
	if m.focus == focusDetail {
		m.detailTop = 0
		return
	}
	if m.focus == focusList {
		m.cursor = 0
		m.ensureCursorVisible()
	}
}

func (m *Model) moveEnd() {
	if m.focus == focusList && len(m.filtered) > 0 {
		m.cursor = len(m.filtered) - 1
		m.ensureCursorVisible()
	}
}

// String returns a short human-readable identifier for the current selection.
func (m Model) String() string {
	if item, ok := m.selectedItem(); ok {
		return fmt.Sprintf("%s:%s", item.Kind, item.ID)
	}
	return string(m.view)
}

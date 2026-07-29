// Package tui provides an offline contribution workbench over local corpus data.
package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

const wideLayoutMinimum = 108

type view string

const (
	viewDiscover     view = "discover"
	viewResearch     view = "research"
	viewActive       view = "active"
	viewValidate     view = "validate"
	viewReady        view = "ready"
	viewSubmitted    view = "submitted"
	viewNeedsYou     view = "needs_you"
	viewRepositories view = "repositories"
	viewSyncStatus   view = "sync_status"
	viewRelatedWork  view = "related_work"
)

type viewSpec struct {
	view  view
	group string
	label string
	key   string
}

var viewSpecs = []viewSpec{
	{viewDiscover, "CONTRIBUTIONS", "Discover", "candidates"},
	{viewResearch, "CONTRIBUTIONS", "Research", "hypotheses"},
	{viewActive, "CONTRIBUTIONS", "Active", "investigations"},
	{viewValidate, "CONTRIBUTIONS", "Validate", "opportunities"},
	{viewReady, "CONTRIBUTIONS", "Ready", "opportunities"},
	{viewSubmitted, "PORTFOLIO", "Submitted", "contributions"},
	{viewNeedsYou, "PORTFOLIO", "Needs you", "needs_you"},
	{viewRepositories, "SOURCES", "Repositories", "repositories"},
	{viewSyncStatus, "SOURCES", "Sync status", "sync_statuses"},
	{viewRelatedWork, "SOURCES", "Related work", "clusters"},
}

var viewOrder = func() []view {
	out := make([]view, 0, len(viewSpecs))
	for _, spec := range viewSpecs {
		out = append(out, spec.view)
	}
	return out
}()

type paneFocus int

const (
	focusNavigation paneFocus = iota
	focusList
	focusDetail
)

// Option customizes a Model.
type Option func(*Model)

// WithSize sets the initial terminal size.
func WithSize(w, h int) Option {
	return func(m *Model) { m.width, m.height = w, h }
}

// WithActionProvider registers typed contextual application operations.
// Loading, navigation, filtering, and detail inspection remain local.
func WithActionProvider(provider tuicontract.ActionProvider) Option {
	return func(m *Model) { m.actionProvider = provider }
}

// WithBriefProvider registers the offline research-brief capability.
func WithBriefProvider(provider tuicontract.BriefProvider) Option {
	return func(m *Model) { m.briefProvider = provider }
}

// Model is the TUI state.
type Model struct {
	reader tuicontract.Reader
	ctx    context.Context

	width  int
	height int

	view      view
	focus     paneFocus
	loading   bool
	loaded    bool
	err       error
	items     map[view][]tuicontract.Item
	windows   map[view]tuicontract.Window
	filtered  []int
	cursor    int
	listStart int
	detailTop int

	search        textinput.Model
	searching     bool
	help          bool
	briefOpen     bool
	briefTop      int
	briefProvider tuicontract.BriefProvider
	briefLoading  bool
	briefErr      error
	briefItem     tuicontract.Item
	brief         tuicontract.ResearchBrief

	actionMsg       string
	actionProvider  tuicontract.ActionProvider
	actionOpen      bool
	actionLoading   bool
	actionExecuting bool
	actionConfirm   bool
	actionErr       error
	actionItem      tuicontract.Item
	actions         []tuicontract.Action
	actionCursor    int
	resultOpen      bool
	resultTop       int
	actionResult    tuicontract.ActionResult
}

// New creates a Model for the given reader and lifecycle context.
func New(ctx context.Context, reader tuicontract.Reader, opts ...Option) Model {
	m := Model{
		reader: reader,
		ctx:    ctx,
		view:   viewDiscover,
		focus:  focusList,
		items:  make(map[view][]tuicontract.Item),
		width:  80,
		height: 24,
	}
	for _, opt := range opts {
		opt(&m)
	}

	ti := textinput.New()
	ti.Placeholder = "Filter this stage…"
	ti.Prompt = "/ "
	ti.CharLimit = 120
	ti.ShowSuggestions = false
	m.search = ti
	m.resizeSearch()
	return m
}

// Init triggers an asynchronous local load. No network I/O is performed here.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return loadMsg{} }
}

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		data, err := m.reader.Load(m.ctx)
		return loadedMsg{data: data, err: err}
	}
}

func (m Model) itemCount() int {
	return len(m.items[m.view])
}

func (m Model) selectedItem() (tuicontract.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return tuicontract.Item{}, false
	}
	idx := m.filtered[m.cursor]
	items := m.items[m.view]
	if idx < 0 || idx >= len(items) {
		return tuicontract.Item{}, false
	}
	return items[idx], true
}

func (m *Model) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	items := m.items[m.view]
	m.filtered = m.filtered[:0]
	for i, item := range items {
		text := strings.ToLower(strings.Join([]string{
			item.Kind, item.Ref, item.Title, item.Subtitle, item.Status,
		}, " "))
		if query == "" || strings.Contains(text, query) {
			m.filtered = append(m.filtered, i)
		}
	}
	m.capCursor()
}

func (m *Model) capCursor() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.listStart = 0
		return
	}
	m.cursor = max(0, min(m.cursor, len(m.filtered)-1))
	m.ensureCursorVisible()
}

func (m *Model) cursorUp() {
	if m.cursor > 0 {
		m.cursor--
		m.detailTop = 0
		m.ensureCursorVisible()
	}
}

func (m *Model) cursorDown() {
	if m.cursor < len(m.filtered)-1 {
		m.cursor++
		m.detailTop = 0
		m.ensureCursorVisible()
	}
}

func (m *Model) ensureCursorVisible() {
	page := max(1, m.listPageSize())
	if m.cursor < m.listStart {
		m.listStart = m.cursor
	}
	if m.cursor >= m.listStart+page {
		m.listStart = m.cursor - page + 1
	}
	maxStart := max(0, len(m.filtered)-page)
	m.listStart = min(m.listStart, maxStart)
}

func (m *Model) switchView(next view) {
	m.view = next
	m.cursor = 0
	m.listStart = 0
	m.detailTop = 0
	m.briefOpen = false
	m.briefTop = 0
	m.resultOpen = false
	m.resultTop = 0
	m.actionMsg = ""
	m.applyFilter()
}

func (m *Model) focusActionTarget(target *tuicontract.ActionTarget) {
	if target == nil || target.ID == "" {
		return
	}
	targetView := view(target.Stage)
	if _, ok := m.items[targetView]; !ok {
		return
	}
	m.search.SetValue("")
	m.switchView(targetView)
	for position, index := range m.filtered {
		if index >= 0 && index < len(m.items[m.view]) && m.items[m.view][index].ID == target.ID {
			m.cursor = position
			m.ensureCursorVisible()
			break
		}
	}
}

func (m *Model) nextView() {
	for i, current := range viewOrder {
		if current == m.view {
			m.switchView(viewOrder[(i+1)%len(viewOrder)])
			return
		}
	}
}

func (m *Model) prevView() {
	for i, current := range viewOrder {
		if current == m.view {
			m.switchView(viewOrder[(i-1+len(viewOrder))%len(viewOrder)])
			return
		}
	}
}

func (m *Model) cycleFocus(reverse bool) {
	focuses := []paneFocus{focusList, focusDetail}
	if m.width >= wideLayoutMinimum {
		focuses = []paneFocus{focusNavigation, focusList, focusDetail}
	}
	for i, current := range focuses {
		if current != m.focus {
			continue
		}
		step := 1
		if reverse {
			step = -1
		}
		m.focus = focuses[(i+step+len(focuses))%len(focuses)]
		return
	}
	m.focus = focuses[0]
}

func (m *Model) resizeSearch() {
	width := m.listPanelWidth() - 6
	m.search.SetWidth(max(12, width))
}

func (m Model) openActions() (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok || m.actionProvider == nil {
		return m, nil
	}
	m.actionOpen = true
	m.actionLoading = true
	m.actionExecuting = false
	m.actionConfirm = false
	m.actionErr = nil
	m.actionItem = item
	m.actions = nil
	m.actionCursor = 0
	return m, func() tea.Msg {
		actions, err := m.actionProvider.Actions(m.ctx, item)
		return actionsLoadedMsg{actions: actions, err: err}
	}
}

func (m Model) retryActionDiscovery() (Model, tea.Cmd) {
	if m.actionProvider == nil || m.actionItem.Kind == "" {
		return m, nil
	}
	m.actionLoading = true
	m.actionErr = nil
	m.actions = nil
	m.actionCursor = 0
	item := m.actionItem
	return m, func() tea.Msg {
		actions, err := m.actionProvider.Actions(m.ctx, item)
		return actionsLoadedMsg{actions: actions, err: err}
	}
}

func (m Model) selectedAction() (tuicontract.Action, bool) {
	if m.actionCursor < 0 || m.actionCursor >= len(m.actions) {
		return tuicontract.Action{}, false
	}
	return m.actions[m.actionCursor], true
}

func (m Model) executeSelectedAction() (Model, tea.Cmd) {
	action, ok := m.selectedAction()
	if !ok || m.actionProvider == nil {
		return m, nil
	}
	m.actionExecuting = true
	m.actionConfirm = false
	m.actionErr = nil
	request := tuicontract.ActionRequest{ActionID: action.ID, Item: m.actionItem}
	return m, func() tea.Msg {
		result, err := m.actionProvider.ExecuteAction(m.ctx, request)
		return actionCompletedMsg{result: result, err: err}
	}
}

func (m Model) openBrief(item tuicontract.Item) (Model, tea.Cmd) {
	m.briefOpen = true
	m.briefTop = 0
	m.briefErr = nil
	m.briefItem = item
	m.brief = tuicontract.ResearchBrief{}
	if m.briefProvider == nil {
		return m, nil
	}
	m.briefLoading = true
	return m, func() tea.Msg {
		brief, err := m.briefProvider.ResearchBrief(m.ctx, item)
		return briefLoadedMsg{itemRef: item.Ref, brief: brief, err: err}
	}
}

func currentViewSpec(current view) viewSpec {
	for _, spec := range viewSpecs {
		if spec.view == current {
			return spec
		}
	}
	return viewSpecs[0]
}

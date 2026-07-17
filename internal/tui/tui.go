// Package tui provides an offline terminal UI for browsing local corpus data.
package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Reader is a narrow local data source for the TUI. Implementations must not
// perform network I/O.
type Reader interface {
	Load(ctx context.Context) (Data, error)
}

// Data is the offline dataset loaded by the reader.
type Data struct {
	Repositories   []Item
	Threads        []Item
	Clusters       []Item
	Investigations []Item
	Opportunities  []Item
}

// Item is one browsable record.
type Item struct {
	Kind     string
	ID       string
	Ref      string
	Title    string
	Subtitle string
	Detail   string
	Source   string
	AsOf     string
	Coverage []Facet
}

// Facet describes coverage for one data facet.
type Facet struct {
	Name     string
	Present  bool
	Complete bool
	AsOf     string
}

// view is the current browse category.
type view string

const (
	viewRepositories   view = "repositories"
	viewThreads        view = "threads"
	viewClusters       view = "clusters"
	viewInvestigations view = "investigations"
	viewOpportunities  view = "opportunities"
)

var (
	viewOrder = []view{
		viewRepositories,
		viewThreads,
		viewClusters,
		viewInvestigations,
		viewOpportunities,
	}

	viewLabels = map[view]string{
		viewRepositories:   "Repositories",
		viewThreads:        "Threads",
		viewClusters:       "Clusters",
		viewInvestigations: "Investigations",
		viewOpportunities:  "Opportunities",
	}

	viewKeys = map[rune]view{
		'1': viewRepositories,
		'2': viewThreads,
		'3': viewClusters,
		'4': viewInvestigations,
		'5': viewOpportunities,
	}
)

// Option customizes a Model.
type Option func(*Model)

// WithSize sets the initial terminal size.
func WithSize(w, h int) Option {
	return func(m *Model) { m.width = w; m.height = h }
}

// WithActionHandler registers an optional handler for explicit side-effecting
// actions such as refresh/hydration. The TUI never invokes it on initial load
// or search.
func WithActionHandler(fn func(context.Context, Item) tea.Cmd) Option {
	return func(m *Model) { m.actionHandler = fn }
}

// Model is the TUI state.
type Model struct {
	reader Reader
	ctx    context.Context

	width  int
	height int

	view     view
	loading  bool
	loaded   bool
	err      error
	items    map[view][]Item
	filtered []int
	cursor   int

	search    textinput.Model
	searching bool

	detail *Item
	help   bool

	actionMsg     string
	actionHandler func(context.Context, Item) tea.Cmd
}

// New creates a Model for the given reader.
func New(reader Reader, opts ...Option) Model {
	m := Model{
		reader: reader,
		view:   viewRepositories,
		items:  make(map[view][]Item),
		width:  80,
		height: 24,
	}
	for _, opt := range opts {
		opt(&m)
	}

	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.Prompt = ""
	ti.CharLimit = 120
	ti.ShowSuggestions = false
	if m.width > 4 {
		ti.Width = m.width - 4
	}
	m.search = ti

	return m
}

// Init triggers an asynchronous local load. No network I/O is performed here.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return loadMsg{} }
}

func (m Model) ctxOrBackground() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		data, err := m.reader.Load(m.ctxOrBackground())
		return loadedMsg{data: data, err: err}
	}
}

// itemCount returns the total number of items in the current view.
func (m Model) itemCount() int {
	return len(m.items[m.view])
}

// filteredItems returns the currently visible items for the current view.
func (m Model) filteredItems() []Item {
	out := make([]Item, 0, len(m.filtered))
	items := m.items[m.view]
	for _, idx := range m.filtered {
		if idx >= 0 && idx < len(items) {
			out = append(out, items[idx])
		}
	}
	return out
}

// selectedItem returns the currently selected item, if any.
func (m Model) selectedItem() (Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return Item{}, false
	}
	idx := m.filtered[m.cursor]
	items := m.items[m.view]
	if idx < 0 || idx >= len(items) {
		return Item{}, false
	}
	return items[idx], true
}

// applyFilter recomputes the visible indices from the search value.
func (m *Model) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	items := m.items[m.view]

	if query == "" {
		m.filtered = make([]int, len(items))
		for i := range items {
			m.filtered[i] = i
		}
		m.capCursor()
		return
	}

	m.filtered = nil
	for i, it := range items {
		text := strings.ToLower(it.Kind + " " + it.Ref + " " + it.Title + " " + it.Subtitle)
		if strings.Contains(text, query) {
			m.filtered = append(m.filtered, i)
		}
	}
	m.capCursor()
}

func (m *Model) capCursor() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *Model) cursorUp() {
	if m.cursor > 0 {
		m.cursor--
	}
}

func (m *Model) cursorDown() {
	if m.cursor < len(m.filtered)-1 {
		m.cursor++
	}
}

func (m *Model) switchView(v view) {
	m.view = v
	m.detail = nil
	m.cursor = 0
	m.applyFilter()
}

func (m *Model) nextView() {
	for i, v := range viewOrder {
		if v == m.view {
			m.switchView(viewOrder[(i+1)%len(viewOrder)])
			return
		}
	}
	m.switchView(viewOrder[0])
}

func (m *Model) prevView() {
	for i, v := range viewOrder {
		if v == m.view {
			m.switchView(viewOrder[(i-1+len(viewOrder))%len(viewOrder)])
			return
		}
	}
	m.switchView(viewOrder[0])
}

func (m *Model) showDetail() {
	it, ok := m.selectedItem()
	if !ok {
		return
	}
	m.detail = &it
}

func (m Model) requestAction() (Model, tea.Cmd) {
	it, ok := m.selectedItem()
	if m.detail != nil {
		it = *m.detail
		ok = true
	}
	if !ok {
		return m, nil
	}

	m.actionMsg = "Refresh requested for " + it.Kind + ": " + it.Title
	if m.actionHandler != nil {
		return m, m.actionHandler(m.ctxOrBackground(), it)
	}
	return m, nil
}

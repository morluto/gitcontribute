package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

var (
	accentColor  = lipgloss.Color("#67D5B5")
	warningColor = lipgloss.Color("#EBCB8B")
	errorColor   = lipgloss.Color("#E06C75")
	mutedColor   = lipgloss.Color("244")
	borderColor  = lipgloss.Color("240")

	brandStyle    = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	headingStyle  = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	dimStyle      = lipgloss.NewStyle().Foreground(mutedColor)
	warningStyle  = lipgloss.NewStyle().Foreground(warningColor)
	errorStyle    = lipgloss.NewStyle().Foreground(errorColor)
)

// View renders the current model state.
func (m Model) View() tea.View {
	width := max(40, m.width)
	height := max(12, m.height)
	bodyHeight := max(8, height-2)

	var body string
	switch {
	case m.resultOpen:
		body = m.renderActionResult(width, bodyHeight)
	case m.briefOpen:
		body = m.renderResearchBrief(width, bodyHeight)
	case m.actionOpen:
		body = m.renderActions(width, bodyHeight)
	case m.help:
		body = m.renderHelp(width, bodyHeight)
	case m.loading:
		body = panel("Loading local contribution corpus…", width, bodyHeight, true)
	case m.err != nil:
		body = panel(errorStyle.Render("Error · could not open local corpus\n\n"+m.err.Error()), width, bodyHeight, true)
	default:
		body = m.renderWorkbench(width, bodyHeight)
	}

	content := strings.Join([]string{
		m.renderHeader(width),
		body,
		m.renderFooter(width),
	}, "\n")
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m Model) renderWorkbench(width, height int) string {
	if width >= wideLayoutMinimum {
		navWidth := max(22, width*22/100)
		listWidth := max(38, width*39/100)
		detailWidth := width - navWidth - listWidth
		return lipgloss.JoinHorizontal(
			lipgloss.Top,
			panel(m.renderNavigation(navWidth-4, height-2), navWidth, height, m.focus == focusNavigation),
			panel(m.renderList(listWidth-4, height-2), listWidth, height, m.focus == focusList),
			panel(m.renderDetail(detailWidth-4, height-2), detailWidth, height, m.focus == focusDetail),
		)
	}

	if width >= 72 {
		listWidth := width * 48 / 100
		detailWidth := width - listWidth
		return lipgloss.JoinHorizontal(
			lipgloss.Top,
			panel(m.renderList(listWidth-4, height-2), listWidth, height, m.focus == focusList),
			panel(m.renderDetail(detailWidth-4, height-2), detailWidth, height, m.focus == focusDetail),
		)
	}

	if m.focus == focusDetail {
		return panel(m.renderDetail(width-4, height-2), width, height, true)
	}
	return panel(m.renderList(width-4, height-2), width, height, true)
}

func (m Model) renderHeader(width int) string {
	scope := m.scopeLabel()
	left := brandStyle.Render("GitContribute") + "  " + scope
	status := "OFFLINE · local corpus"
	if item, ok := m.selectedItem(); ok && item.AsOf != "" {
		status = "OFFLINE · as of " + shortDate(item.AsOf)
	}
	right := "/ search"
	if m.actionMsg != "" {
		status = m.actionMsg
	}
	return distribute(width, left, dimStyle.Render(status), dimStyle.Render(right))
}

func (m Model) renderNavigation(width, height int) string {
	var lines []string
	group := ""
	for _, spec := range viewSpecs {
		if spec.group != group {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			group = spec.group
			lines = append(lines, headingStyle.Render(group))
		}
		count := len(m.items[spec.view])
		labelWidth := max(1, width-7)
		label := truncate(spec.label, labelWidth)
		line := fmt.Sprintf("  %-*s %3d", labelWidth, label, count)
		if spec.view == m.view {
			line = "› " + line[2:]
			line = selectedStyle.Render(line)
		} else {
			line = dimStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return fitLines(lines, height)
}

func (m Model) renderList(width, height int) string {
	spec := currentViewSpec(m.view)
	title := strings.ToUpper(spec.label)
	window := m.windows[m.view]
	total := len(m.items[m.view])
	if window.Total > total {
		total = window.Total
	}
	titleLine := headingStyle.Render(title) + dimStyle.Render(fmt.Sprintf("  %d", total))
	if window.Truncated {
		titleLine += warningStyle.Render("  partial")
	}
	lines := []string{titleLine}
	if m.width < wideLayoutMinimum {
		lines[0] = dimStyle.Render("[ / ] stage  ") + titleLine
	}
	if m.searching || strings.TrimSpace(m.search.Value()) != "" {
		lines = append(lines, m.search.View())
	} else {
		lines = append(lines, "")
	}

	if len(m.filtered) == 0 {
		lines = append(lines, "")
		for _, paragraph := range strings.Split(m.emptyMessage(), "\n") {
			for _, line := range strings.Split(wrapPlain(paragraph, width), "\n") {
				lines = append(lines, dimStyle.Render(line))
			}
		}
		return fitLines(lines, height)
	}

	page := max(1, (height-4)/3)
	start := min(m.listStart, max(0, len(m.filtered)-page))
	end := min(len(m.filtered), start+page)
	items := m.items[m.view]
	for position := start; position < end; position++ {
		index := m.filtered[position]
		if index < 0 || index >= len(items) {
			continue
		}
		item := items[index]
		marker := "  "
		titleStyle := lipgloss.NewStyle()
		if position == m.cursor {
			marker = "› "
			titleStyle = selectedStyle
		}
		lines = append(lines, titleStyle.Render(truncate(marker+item.Title, width)))

		meta := displayRef(item.Ref)
		if item.Status != "" {
			meta = strings.ReplaceAll(item.Status, "_", " ") + " · " + meta
		}
		if item.Score != 0 {
			meta = fmt.Sprintf("score %d · %s", item.Score, meta)
		}
		lines = append(lines, dimStyle.Render(truncate("  "+meta, width)), "")
	}

	current := m.cursor + 1
	percentage := current * 100 / max(1, len(m.filtered))
	progress := fmt.Sprintf("%d/%d  %d%%", current, len(m.filtered), percentage)
	lines = append(lines, lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(dimStyle.Render(progress)))
	return fitLines(lines, height)
}

func (m Model) renderDetail(width, height int) string {
	item, ok := m.selectedItem()
	if !ok {
		lines := []string{
			headingStyle.Render("DETAIL"),
			"",
		}
		for _, line := range strings.Split(wrapPlain("Select an item to inspect its evidence and next action.", width), "\n") {
			lines = append(lines, dimStyle.Render(line))
		}
		return fitLines(lines, height)
	}

	titleLines := strings.Split(wrapPlain(item.Title, width), "\n")
	lines := make([]string, 0, height+8)
	for _, line := range titleLines {
		lines = append(lines, headingStyle.Render(line))
	}
	lines = append(lines, "")
	if item.Status != "" {
		lines = append(lines, selectedStyle.Render(strings.ToUpper(strings.ReplaceAll(item.Status, "_", " "))))
	}
	if item.Confidence != "" {
		lines = append(lines, "Confidence: "+item.Confidence)
	}
	if item.Score != 0 {
		lines = append(lines, fmt.Sprintf("Radar score: %d", item.Score))
	}

	if item.Detail != "" {
		lines = append(lines, "", headingStyle.Render("Summary"))
		lines = appendWrapped(lines, "", item.Detail, width)
	}

	if item.Assessment != nil {
		lines = appendFacts(lines, "Why it matters", "+", item.Assessment.Positive, width, selectedStyle)
		lines = appendFacts(lines, "Risks", "!", item.Assessment.Risks, width, warningStyle)
		lines = appendFacts(lines, "Blockers", "×", item.Assessment.Blockers, width, errorStyle)
		lines = appendFacts(lines, "Unknowns", "?", item.Assessment.Unknowns, width, dimStyle)
		lines = appendFacts(lines, "Related work", "○", item.Assessment.Related, width, dimStyle)
	}

	if len(item.Coverage) > 0 {
		lines = append(lines, "", headingStyle.Render("Coverage"))
		for _, facet := range item.Coverage {
			marker := "×"
			state := "missing"
			if facet.Present {
				marker = "!"
				state = "partial"
			}
			if facet.Present && facet.Complete {
				marker = "✓"
				state = "complete"
			}
			summary := fmt.Sprintf("%s %s · %s", marker, facet.Name, state)
			if facet.AsOf != "" {
				summary += " · " + shortDate(facet.AsOf)
			}
			lines = append(lines, summary)
		}
	}

	if len(item.Commands) > 0 {
		lines = append(lines, "", headingStyle.Render("Refresh explicitly"))
		for _, command := range item.Commands {
			lines = appendWrapped(lines, "$ ", command, width)
		}
	}

	lines = append(lines, "", headingStyle.Render("Source"))
	if item.Ref != "" {
		lines = appendPlainWrapped(lines, "", displayRef(item.Ref), width)
	}
	if item.Source != "" {
		sourceLines := appendPlainWrapped(nil, "", item.Source, width)
		for _, line := range sourceLines {
			lines = append(lines, dimStyle.Render(line))
		}
	}
	if item.AsOf != "" {
		lines = append(lines, dimStyle.Render("Observed "+shortDate(item.AsOf)))
	}

	next := []string{"", headingStyle.Render("Next")}
	for _, action := range nextActions(item, m.actionProvider != nil) {
		next = appendWrapped(next, "", action, width)
	}

	contentHeight := max(1, height-len(next))
	top := min(m.detailTop, max(0, len(lines)-contentHeight))
	content := fitLines(lines[top:], contentHeight)
	return content + "\n" + strings.Join(next, "\n")
}

func (m Model) renderFooter(width int) string {
	var text string
	switch {
	case m.resultOpen:
		if m.actionResult.Target != nil {
			text = "enter view result   esc return   ↑↓ scroll"
		} else {
			text = "enter return   esc return   ↑↓ scroll"
		}
	case m.briefOpen:
		if m.briefErr != nil {
			text = "enter retry   esc return"
		} else {
			text = "↑↓ scroll   pgup/pgdown page   home top   esc return"
		}
	case m.actionOpen && m.actionConfirm:
		text = "y / enter confirm   n / esc cancel   local corpus only · no GitHub mutation"
	case m.actionOpen && m.actionErr != nil:
		text = "enter retry   esc return"
	case m.actionOpen:
		text = "↑↓ move   enter run   esc close"
	case m.help:
		text = "esc close help"
	case m.searching:
		text = "type to filter   ↑↓ move   enter apply   esc close"
	case m.focus == focusDetail:
		text = "tab focus   ↑↓ scroll   esc back   [ ] stage   / filter   ? help   q quit"
	default:
		text = "tab focus   ↑↓ move   enter inspect   [ ] stage   / filter   ? help   q quit"
	}
	if m.actionProvider != nil && !m.help && !m.searching && !m.actionOpen {
		text = strings.Replace(text, "   / filter", "   a actions   / filter", 1)
	}
	return truncate(dimStyle.Render(text), width)
}

func (m Model) listPanelWidth() int {
	if m.width >= wideLayoutMinimum {
		return max(38, m.width*39/100)
	}
	if m.width >= 72 {
		return m.width * 48 / 100
	}
	return m.width
}

func (m Model) listPageSize() int {
	bodyHeight := max(8, m.height-2)
	return max(1, (bodyHeight-6)/3)
}

func (m Model) scopeLabel() string {
	repositories := m.items[viewRepositories]
	if len(repositories) == 1 {
		return repositories[0].Title
	}
	if len(repositories) > 1 {
		return fmt.Sprintf("%d repositories", len(repositories))
	}
	return "local corpus"
}

func panel(content string, width, height int, focused bool) string {
	color := borderColor
	if focused {
		color = accentColor
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(color).
		Width(max(3, width)).
		Height(max(3, height)).
		Render(content)
}

func appendFacts(
	lines []string,
	title, marker string,
	facts []tuicontract.Fact,
	width int,
	style lipgloss.Style,
) []string {
	if len(facts) == 0 {
		return lines
	}
	lines = append(lines, "", headingStyle.Render(title))
	for _, fact := range facts {
		lines = appendWrapped(lines, style.Render(marker)+" ", fact.Summary, width)
	}
	return lines
}

func appendWrapped(lines []string, prefix, text string, width int) []string {
	wrapped := strings.Split(wrap(text, max(8, width-lipgloss.Width(prefix))), "\n")
	for i, line := range wrapped {
		if i == 0 {
			lines = append(lines, prefix+line)
		} else {
			lines = append(lines, strings.Repeat(" ", lipgloss.Width(prefix))+line)
		}
	}
	return lines
}

func appendPlainWrapped(lines []string, prefix, text string, width int) []string {
	wrapped := strings.Split(wrapPlain(text, max(8, width-lipgloss.Width(prefix))), "\n")
	for i, line := range wrapped {
		if i == 0 {
			lines = append(lines, prefix+line)
		} else {
			lines = append(lines, strings.Repeat(" ", lipgloss.Width(prefix))+line)
		}
	}
	return lines
}

func nextActions(item tuicontract.Item, canAct bool) []string {
	var actions []string
	switch item.Kind {
	case "candidate":
		actions = []string{"[Enter] Open research brief"}
	case "hypothesis":
		actions = []string{"[Enter] Review hypothesis"}
	case "investigation":
		actions = []string{"[Enter] Inspect investigation"}
	case "opportunity":
		actions = []string{"[Enter] Review readiness"}
	case "contribution":
		actions = []string{"[Enter] Inspect prepared contribution"}
	case "repository":
		actions = []string{"[Enter] Inspect repository coverage"}
	default:
		actions = []string{"[Enter] Inspect details"}
	}
	if canAct && len(item.Actions) > 0 {
		label := item.Actions[0].Label
		if len(item.Actions) > 1 {
			label += fmt.Sprintf(" (+%d)", len(item.Actions)-1)
		}
		actions = append(actions, "[a] "+label)
	}
	return actions
}

func (m Model) emptyMessage() string {
	switch m.view {
	case viewDiscover:
		if len(m.items[viewRepositories]) == 0 {
			return strings.Join([]string{
				"No contribution candidates yet.",
				"",
				"The local corpus contains no repositories.",
				"Run: gitcontribute source add repos OWNER/REPO",
				"Then: gitcontribute archive sync OWNER/REPO",
			}, "\n")
		}
		return strings.Join([]string{
			"No contribution candidates yet.",
			"",
			"Open Sync status [9] to inspect local evidence.",
			"Refresh explicitly with:",
			"gitcontribute archive sync OWNER/REPO",
		}, "\n")
	case viewResearch:
		return "No proposed hypotheses are waiting for research."
	case viewActive:
		return "No investigations are active."
	case viewValidate:
		return "No opportunities currently need validation."
	case viewReady:
		return "No opportunities pass the local readiness gate."
	case viewSubmitted:
		return "No submitted contributions are recorded locally."
	case viewNeedsYou:
		return "No prepared contributions are waiting for submission."
	case viewRepositories:
		return "No repositories are stored in the local corpus."
	case viewSyncStatus:
		return "No repository sync status is available in the local corpus."
	default:
		return "No related work is available."
	}
}

func fitLines(lines []string, height int) string {
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func distribute(width int, left, center, right string) string {
	plainWidth := lipgloss.Width(left) + lipgloss.Width(center) + lipgloss.Width(right)
	if plainWidth+4 > width {
		return truncate(left+"  "+center, width)
	}
	remaining := width - plainWidth
	leftGap := remaining / 2
	rightGap := remaining - leftGap
	return left + strings.Repeat(" ", leftGap) + center + strings.Repeat(" ", rightGap) + right
}

func shortDate(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.Format("2006-01-02")
}

func displayRef(value string) string {
	for _, prefix := range []string{"issue:", "pr:", "pull_request:"} {
		value = strings.TrimPrefix(value, prefix)
	}
	return value
}

func truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func wrap(s string, width int) string {
	if width <= 0 {
		width = 78
	}
	var lines []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		var line strings.Builder
		for _, word := range words {
			candidate := word
			if line.Len() > 0 {
				candidate = line.String() + " " + word
			}
			if lipgloss.Width(candidate) > width && line.Len() > 0 {
				lines = append(lines, line.String())
				line.Reset()
				line.WriteString(word)
				continue
			}
			if line.Len() > 0 {
				line.WriteString(" ")
			}
			line.WriteString(word)
		}
		if line.Len() > 0 {
			lines = append(lines, line.String())
		}
	}
	return strings.Join(lines, "\n")
}

func wrapPlain(s string, width int) string {
	if width <= 0 {
		width = 78
	}
	var lines []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, word := range words {
			if line != "" && lipgloss.Width(line+" "+word) <= width {
				line += " " + word
				continue
			}
			if line != "" {
				lines = append(lines, line)
			}
			for lipgloss.Width(word) > width {
				chunk, rest := splitDisplayWidth(word, width)
				lines = append(lines, chunk)
				word = rest
			}
			line = word
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func splitDisplayWidth(value string, width int) (string, string) {
	runes := []rune(value)
	end := 0
	for end < len(runes) && lipgloss.Width(string(runes[:end+1])) <= width {
		end++
	}
	if end == 0 && len(runes) > 0 {
		end = 1
	}
	return string(runes[:end]), string(runes[end:])
}

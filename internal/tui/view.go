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

func (m Model) renderActionResult(width, height int) string {
	result := m.actionResult
	title := result.Title
	if title == "" {
		title = "Action complete"
	}
	contentWidth := max(12, width-4)
	lines := []string{
		headingStyle.Render("ACTION RESULT"),
		"",
		headingStyle.Render(title),
	}
	if result.Message != "" {
		lines = append(lines, "")
		lines = appendWrapped(lines, "", result.Message, contentWidth)
	}
	if len(result.Facts) > 0 {
		lines = append(lines, "", headingStyle.Render("Summary"))
		for _, fact := range result.Facts {
			lines = appendWrapped(lines, "", fact.Label+": "+fact.Value, contentWidth)
		}
	}
	if result.SourceRevision != "" {
		lines = append(lines, "", headingStyle.Render("Source revision"))
		lines = appendWrapped(lines, "", result.SourceRevision, contentWidth)
	}
	if len(result.Items) > 0 {
		lines = append(lines, "", headingStyle.Render("Matches"))
		for _, item := range result.Items {
			summary := item.Title
			if item.Status != "" {
				summary += " · " + item.Status
			}
			lines = appendWrapped(lines, "• ", summary, contentWidth)
			if item.Ref != "" {
				lines = appendWrapped(lines, "  ", dimStyle.Render(item.Ref), contentWidth)
			}
			if item.Source != "" {
				lines = appendWrapped(lines, "  ", dimStyle.Render(item.Source), contentWidth)
			}
		}
	}
	if result.Target != nil {
		lines = append(lines, "", selectedStyle.Render("[Enter] View "+strings.ReplaceAll(result.Target.Stage, "_", " ")))
	}
	lines = append(lines, "", dimStyle.Render("[Esc] Return to the workbench"))
	top := min(m.resultTop, max(0, len(lines)-(height-2)))
	return panel(fitLines(lines[top:], height-2), width, height, true)
}

func (m Model) renderResearchBrief(width, height int) string {
	item := m.briefItem
	if item.Ref == "" {
		selected, ok := m.selectedItem()
		if ok {
			item = selected
		}
	}
	if item.Ref == "" {
		return panel(errorStyle.Render("Research brief unavailable\n\nThe selected candidate is no longer visible."), width, height, true)
	}
	if m.briefLoading {
		return panel("Loading stored research brief…\n\n"+dimStyle.Render("Offline · no corpus writes"), width, height, true)
	}
	if m.briefErr != nil {
		lines := []string{
			errorStyle.Render("Research brief unavailable"),
			"",
			wrap(m.briefErr.Error(), max(12, width-4)),
			"",
			headingStyle.Render("Recovery"),
			"Press Enter to retry the local read.",
			"Press Esc to return to the workbench.",
		}
		return panel(fitLines(lines, height-2), width, height, true)
	}
	contentWidth := max(12, width-4)
	brief := m.brief
	if brief.Title == "" {
		brief.Title = item.Title
		brief.Ref = item.Ref
		brief.Problem = item.Detail
	}
	lines := []string{
		headingStyle.Render("RESEARCH BRIEF"),
		"",
		headingStyle.Render(brief.Title),
	}
	if brief.Ref != "" {
		lines = appendWrapped(lines, "", brief.Ref, contentWidth)
	}
	if brief.SourceAsOf != "" {
		lines = append(lines, dimStyle.Render("Source as of "+shortDate(brief.SourceAsOf)))
	}
	lines = append(lines, "", headingStyle.Render("Problem"))
	if strings.TrimSpace(brief.Problem) == "" {
		lines = append(lines, dimStyle.Render("No problem statement is available in the loaded local evidence."))
	} else {
		lines = appendWrapped(lines, "", brief.Problem, contentWidth)
	}
	lines = appendBriefFacts(lines, "Expected behavior", brief.ExpectedBehavior, contentWidth)
	lines = appendBriefFacts(lines, "Relevant discussion", brief.Discussion, contentWidth)
	lines = appendBriefFacts(lines, "Reproduction status", []tuicontract.BriefFact{brief.ReproductionStatus}, contentWidth)
	lines = appendBriefFacts(lines, "Related work", brief.RelatedWork, contentWidth)
	if len(brief.MissingEvidence) > 0 {
		lines = append(lines, "", headingStyle.Render("Missing evidence"))
		for _, gap := range brief.MissingEvidence {
			lines = appendWrapped(lines, warningStyle.Render("! "), gap, contentWidth)
		}
	}
	if len(brief.SuggestedNext) > 0 {
		lines = append(lines, "", headingStyle.Render("Suggested next step"))
		for _, command := range brief.SuggestedNext {
			lines = appendWrapped(lines, selectedStyle.Render("› "), command, contentWidth)
		}
	}
	top := min(m.briefTop, max(0, len(lines)-(height-2)))
	return panel(fitLines(lines[top:], height-2), width, height, true)
}

func appendBriefFacts(lines []string, title string, facts []tuicontract.BriefFact, width int) []string {
	visible := make([]tuicontract.BriefFact, 0, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact.Summary) != "" {
			visible = append(visible, fact)
		}
	}
	if len(visible) == 0 {
		return lines
	}
	lines = append(lines, "", headingStyle.Render(title))
	for _, fact := range visible {
		lines = appendWrapped(lines, "• ", fact.Summary, width)
		if fact.Source != "" {
			lines = appendWrapped(lines, "  ", dimStyle.Render(fact.Source), width)
		}
	}
	return lines
}

func (m Model) renderHelp(width, height int) string {
	lines := []string{
		headingStyle.Render("Keyboard help"),
		"",
		headingStyle.Render("Navigate"),
		"  ↑↓ / jk       move in the focused pane",
		"  tab           focus next pane",
		"  shift+tab     focus previous pane",
		"  [ / ]         previous / next workflow stage",
		"  1–9 / 0       jump to a workflow stage",
		"  enter         inspect the selected item",
		"  esc           return to the contribution list",
		"",
		headingStyle.Render("Find and inspect"),
		"  /             filter the current stage",
		"  home / end    first / last item",
		"  pgup / pgdown move by one viewport",
		"",
		headingStyle.Render("Contribution stages"),
		"  Discover · ranked issues worth evaluating",
		"  Research · proposed hypotheses awaiting investigation",
		"  Active · investigations in progress",
		"  Validate · opportunities needing evidence",
		"  Ready · opportunities passing local checks",
		"  Submitted · contributions already submitted",
		"  Needs you · prepared contributions awaiting your submission",
		"  Repositories · locally stored sources",
		"  Sync status · stored coverage and freshness; never syncs on open",
		"  Related work · locally inferred issue and pull-request clusters",
	}
	if m.actionProvider != nil {
		lines = append(lines, "  a             open contextual actions")
	}
	lines = append(lines,
		"",
		headingStyle.Render("Application"),
		"  ?             close this help",
		"  q / ctrl+c    quit",
		"",
		dimStyle.Render("Navigation and inspection use only the local corpus."),
		dimStyle.Render("Network reads and process execution require explicit actions."),
	)
	return panel(fitLines(lines, height-2), width, height, true)
}

func (m Model) renderActions(width, height int) string {
	contentWidth := min(72, max(32, width-8))
	lines := []string{
		headingStyle.Render("Actions"),
		dimStyle.Render(truncate(displayRef(m.actionItem.Ref)+" · "+m.actionItem.Title, contentWidth)),
		"",
	}
	switch {
	case m.actionLoading:
		lines = append(lines, "Loading available actions…")
	case m.actionExecuting:
		lines = append(lines, "Running action…")
	case m.actionErr != nil:
		action, selected := m.selectedAction()
		label := action.Label
		recovery := "Press Enter to retry this action."
		if !selected {
			label = "Load contextual actions"
			recovery = "Press Enter to retry loading available actions."
		}
		lines = append(lines,
			errorStyle.Render("ACTION FAILED"),
			"",
			headingStyle.Render(label),
			wrap(m.actionErr.Error(), contentWidth),
			"",
			headingStyle.Render("Recovery"),
			recovery,
			"Press Esc to return to the workbench.",
		)
	case m.actionConfirm:
		action, _ := m.selectedAction()
		lines = append(lines,
			warningStyle.Render("Confirm local write"),
			"",
			action.Label,
			wrap(action.Description, contentWidth),
			"",
			"Capability: "+capabilityLabel(action.Capability),
			"No network access or GitHub mutation.",
			"",
			"Continue? [y/N]",
		)
	default:
		for i, action := range m.actions {
			marker := "  "
			style := lipgloss.NewStyle()
			if i == m.actionCursor {
				marker = "› "
				style = selectedStyle
			}
			lines = append(lines,
				style.Render(truncate(marker+action.Label, contentWidth)),
				dimStyle.Render(truncate("  "+capabilityLabel(action.Capability)+" · "+action.Description, contentWidth)),
				"",
			)
		}
	}
	card := panel(fitLines(lines, max(8, height-4)), contentWidth+4, max(10, height-2), true)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, card)
}

func capabilityLabel(capability tuicontract.Capability) string {
	switch capability {
	case tuicontract.CapabilityOfflineRead:
		return "offline read"
	case tuicontract.CapabilityLocalWrite:
		return "local corpus write"
	default:
		return string(capability)
	}
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
				line = ""
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

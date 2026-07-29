package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

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

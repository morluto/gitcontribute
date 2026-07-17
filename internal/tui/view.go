package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#04B575"))
	headerStyle    = lipgloss.NewStyle().Bold(true)
	itemStyle      = lipgloss.NewStyle()
	selectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F57"))
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	detailBoxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).BorderForeground(lipgloss.Color("241"))
)

// View renders the current model state.
func (m Model) View() string {
	if m.width <= 0 {
		m.width = 80
	}
	if m.height <= 0 {
		m.height = 24
	}

	var lines []string
	lines = append(lines, m.renderHeader())

	if m.help {
		lines = append(lines, m.renderHelp())
	} else {
		switch {
		case m.loading:
			lines = append(lines, m.renderLoading())
		case m.err != nil:
			lines = append(lines, m.renderError())
		case m.detail != nil:
			lines = append(lines, m.renderDetail())
		case len(m.filtered) == 0:
			lines = append(lines, m.renderEmpty())
		default:
			lines = append(lines, m.renderList())
		}
	}

	lines = append(lines, m.renderFooter())
	return strings.Join(lines, "\n")
}

func (m Model) renderHeader() string {
	label := viewLabels[m.view]
	total := m.itemCount()
	visible := len(m.filtered)

	header := titleStyle.Render("GitContribute")
	viewLine := headerStyle.Render(fmt.Sprintf(" %s ", label))
	count := dimStyle.Render(fmt.Sprintf(" %d/%d ", visible, total))

	line := header + viewLine + count
	if m.actionMsg != "" {
		line += " | " + dimStyle.Render(m.actionMsg)
	}
	return line
}

func (m Model) renderSearch() string {
	return m.search.View()
}

func (m Model) renderLoading() string {
	return "\nLoading local corpus...\n"
}

func (m Model) renderError() string {
	return errorStyle.Render("Error: " + m.err.Error())
}

func (m Model) renderEmpty() string {
	return m.renderSearch() + "\nNo items.\n"
}

func (m Model) renderList() string {
	var b strings.Builder
	b.WriteString(m.renderSearch())
	b.WriteString("\n")

	items := m.items[m.view]
	for i, idx := range m.filtered {
		if idx < 0 || idx >= len(items) {
			continue
		}
		it := items[idx]
		marker := "  "
		style := itemStyle
		if i == m.cursor {
			marker = "> "
			style = selectedStyle
		}

		text := it.Title
		if it.Subtitle != "" {
			text += "  " + dimStyle.Render(it.Subtitle)
		}
		line := marker + text
		if lipgloss.Width(line) > m.width-2 {
			line = truncate(line, m.width-2)
		}
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderDetail() string {
	it := m.detail
	if it == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(m.renderSearch())
	b.WriteString("\n")
	b.WriteString(titleStyle.Render(it.Title))
	b.WriteString("\n")

	if it.Subtitle != "" {
		b.WriteString(dimStyle.Render(it.Subtitle))
		b.WriteString("\n")
	}
	if it.Ref != "" {
		b.WriteString(dimStyle.Render("Ref: " + it.Ref))
		b.WriteString("\n")
	}
	if it.Detail != "" {
		b.WriteString(wrap(it.Detail, m.width-4))
		b.WriteString("\n")
	}

	if it.Source != "" || it.AsOf != "" {
		b.WriteString(dimStyle.Render("Source: " + it.Source + "  As of: " + it.AsOf))
		b.WriteString("\n")
	}

	if len(it.Coverage) > 0 {
		b.WriteString(renderCoverage(it.Coverage))
	}

	b.WriteString(dimStyle.Render("Press 'r' to request refresh/hydration, esc to close."))

	box := detailBoxStyle.Render(b.String())
	if lipgloss.Width(box) > m.width {
		return wrap(box, m.width)
	}
	return box
}

func (m Model) renderFooter() string {
	return helpStyle.Render("q quit | 1-5 views | / filter | enter detail | ? help")
}

func (m Model) renderHelp() string {
	keys := []string{
		"q / ctrl+c  quit",
		"1-5         switch view",
		"tab         next view",
		"shift+tab   previous view",
		"/           focus filter",
		"esc         close filter / detail / help",
		"up/k        move up",
		"down/j      move down",
		"enter       show detail",
		"r           request refresh/hydration",
		"?           toggle this help",
	}
	return "Keyboard help\n" + strings.Join(keys, "\n")
}

func renderCoverage(coverage []Facet) string {
	var lines []string
	lines = append(lines, "Coverage:")
	for _, f := range coverage {
		status := " "
		if f.Present {
			status = "x"
		}
		complete := ""
		if f.Complete {
			complete = " complete"
		}
		asOf := f.AsOf
		if asOf == "" {
			asOf = "-"
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s%s (as of %s)", status, f.Name, complete, asOf))
	}
	return strings.Join(lines, "\n") + "\n"
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > w-3 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "..."
}

func wrap(s string, width int) string {
	if width <= 0 {
		width = 78
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		var line strings.Builder
		for _, w := range words {
			candidate := line.String()
			if candidate != "" {
				candidate += " "
			}
			candidate += w
			if lipgloss.Width(candidate) > width && line.Len() > 0 {
				lines = append(lines, line.String())
				line.Reset()
				line.WriteString(w)
			} else {
				if line.Len() > 0 {
					line.WriteString(" ")
				}
				line.WriteString(w)
			}
		}
		if line.Len() > 0 {
			lines = append(lines, line.String())
		}
	}
	return strings.Join(lines, "\n")
}

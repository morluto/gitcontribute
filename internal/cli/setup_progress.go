package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type setupStartedMsg SetupPhase
type setupCompletedMsg SetupStep
type setupFinishedMsg struct{}

type setupProgressModel struct {
	spinner   spinner.Model
	active    string
	completed []string
}

func newSetupProgressModel() setupProgressModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return setupProgressModel{spinner: s}
}

func (m setupProgressModel) Init() tea.Cmd { return m.spinner.Tick }

func (m setupProgressModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case setupStartedMsg:
		m.active = string(message)
		return m, nil
	case setupCompletedMsg:
		step := SetupStep(message)
		m.active = ""
		m.completed = append(m.completed, fmt.Sprintf("%s %s — %s", setupProgressSymbol(step.Status), setupStepLabel(step.Name), step.Status))
		return m, nil
	case setupFinishedMsg:
		return m, tea.Quit
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		return m, command
	}
	return m, nil
}

func (m setupProgressModel) View() string {
	lines := append([]string(nil), m.completed...)
	if m.active != "" {
		lines = append(lines, fmt.Sprintf("%s %s…", m.spinner.View(), setupProgressAction(m.active)))
	}
	return strings.Join(lines, "\n")
}

type setupProgressRenderer struct {
	program *tea.Program
	done    <-chan error
}

func startSetupProgress(output io.Writer) *setupProgressRenderer {
	program := tea.NewProgram(newSetupProgressModel(), tea.WithInput(nil), tea.WithOutput(output))
	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()
	return &setupProgressRenderer{program: program, done: done}
}

func (r *setupProgressRenderer) SetupStarted(phase SetupPhase) {
	r.program.Send(setupStartedMsg(phase))
}

func (r *setupProgressRenderer) SetupCompleted(step SetupStep) {
	r.program.Send(setupCompletedMsg(step))
}

func (r *setupProgressRenderer) Close() error {
	r.program.Send(setupFinishedMsg{})
	return <-r.done
}

func setupProgressSymbol(status string) string {
	if status == "failed" {
		return "✗"
	}
	if status == "not installed" || strings.HasPrefix(status, "skipped") {
		return "○"
	}
	return "✓"
}

func setupProgressAction(phase string) string {
	switch phase {
	case "terminal":
		return "Installing terminal command"
	case "configuration":
		return "Writing configuration"
	case "corpus":
		return "Initializing local corpus"
	case "clients":
		return "Configuring coding agents"
	case "repository":
		return "Adding repository source"
	case "verification":
		return "Verifying setup"
	default:
		return setupStepLabel(phase)
	}
}

func interactiveWriter(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

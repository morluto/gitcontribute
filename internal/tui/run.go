package tui

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI program with the provided local reader and I/O.
//
// It is suitable for later CLI wiring: the caller supplies a context, a
// Reader, and input/output streams. The TUI loads local data on start and
// never performs network I/O on its own.
func Run(ctx context.Context, reader Reader, input io.Reader, output io.Writer) (Model, error) {
	m := New(reader)
	m.ctx = ctx

	p := tea.NewProgram(
		m,
		tea.WithInput(input),
		tea.WithOutput(output),
		tea.WithContext(ctx),
	)

	final, err := p.Run()
	if err != nil {
		return Model{}, err
	}
	return final.(Model), nil
}

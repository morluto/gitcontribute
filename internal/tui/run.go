package tui

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/morluto/gitcontribute/internal/cli"
)

// Runner adapts the TUI to the CLI boundary.
type Runner struct {
	reader Reader
	input  io.Reader
	output io.Writer
}

func NewRunner(reader Reader, input io.Reader, output io.Writer) *Runner {
	return &Runner{reader: reader, input: input, output: output}
}

func (r *Runner) Run(ctx context.Context, opts cli.TUIOptions) error {
	if r == nil || r.reader == nil {
		return cli.ErrNotWired
	}
	if opts.JSON || opts.Repo.Owner != "" {
		data, err := r.reader.Load(ctx)
		if err != nil {
			return err
		}
		data = filterData(data, opts.Repo.String())
		if opts.JSON {
			enc := json.NewEncoder(r.output)
			enc.SetIndent("", "  ")
			return enc.Encode(data)
		}
		_, err = Run(ctx, staticReader{data: data}, r.input, r.output)
		return err
	}
	_, err := Run(ctx, r.reader, r.input, r.output)
	return err
}

type staticReader struct{ data Data }

func (r staticReader) Load(context.Context) (Data, error) { return r.data, nil }

func filterData(data Data, repo string) Data {
	if repo == "" || repo == "/" {
		return data
	}
	keep := func(items []Item) []Item {
		out := make([]Item, 0, len(items))
		for _, item := range items {
			ref := strings.ToLower(item.Ref)
			want := strings.ToLower(repo)
			if ref == want || strings.HasPrefix(ref, want+"#") || strings.HasPrefix(ref, want+":") {
				out = append(out, item)
			}
		}
		return out
	}
	data.Repositories = keep(data.Repositories)
	data.Threads = keep(data.Threads)
	data.Clusters = keep(data.Clusters)
	data.Investigations = keep(data.Investigations)
	data.Opportunities = keep(data.Opportunities)
	return data
}

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

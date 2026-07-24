package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

var errReaderNotConfigured = errors.New("tui reader is not configured")

// Runner implements the product-owned TUI runner contract.
type Runner struct {
	reader tuicontract.Reader
	input  io.Reader
	output io.Writer
}

func NewRunner(reader tuicontract.Reader, input io.Reader, output io.Writer) *Runner {
	return &Runner{reader: reader, input: input, output: output}
}

func (r *Runner) Run(ctx context.Context, opts contracts.TUIOptions) error {
	if r == nil || r.reader == nil {
		return errReaderNotConfigured
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

type staticReader struct{ data tuicontract.Data }

func (r staticReader) Load(context.Context) (tuicontract.Data, error) { return r.data, nil }

func filterData(data tuicontract.Data, repo string) tuicontract.Data {
	if repo == "" || repo == "/" {
		return data
	}
	keep := func(items []tuicontract.Item) []tuicontract.Item {
		out := make([]tuicontract.Item, 0, len(items))
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
func Run(ctx context.Context, reader tuicontract.Reader, input io.Reader, output io.Writer) (Model, error) {
	m := New(ctx, reader)

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
	model, ok := final.(Model)
	if !ok {
		return Model{}, fmt.Errorf("unexpected terminal model %T", final)
	}
	return model, nil
}

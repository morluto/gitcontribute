package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

var errReaderNotConfigured = errors.New("tui reader is not configured")

// Runner implements the product-owned TUI runner contract.
type Runner struct {
	reader  tuicontract.Reader
	actions tuicontract.ActionProvider
	briefs  tuicontract.BriefProvider
	input   io.Reader
	output  io.Writer
}

func NewRunner(
	reader tuicontract.Reader,
	actions tuicontract.ActionProvider,
	briefs tuicontract.BriefProvider,
	input io.Reader,
	output io.Writer,
) *Runner {
	return &Runner{reader: reader, actions: actions, briefs: briefs, input: input, output: output}
}

func (r *Runner) Run(ctx context.Context, opts contracts.TUIOptions) error {
	if r == nil || r.reader == nil {
		return errReaderNotConfigured
	}
	if opts.JSON {
		data, err := r.reader.Load(ctx)
		if err != nil {
			return err
		}
		data = filterData(data, opts.Repo.String())
		enc := json.NewEncoder(r.output)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}
	if opts.Repo.Owner != "" {
		_, err := run(ctx, filteredReader{reader: r.reader, repo: opts.Repo.String()}, r.actions, r.briefs, r.input, r.output)
		return err
	}
	_, err := run(ctx, r.reader, r.actions, r.briefs, r.input, r.output)
	return err
}

type filteredReader struct {
	reader tuicontract.Reader
	repo   string
}

func (r filteredReader) Load(ctx context.Context) (tuicontract.Data, error) {
	data, err := r.reader.Load(ctx)
	if err != nil {
		return tuicontract.Data{}, err
	}
	return filterData(data, r.repo), nil
}

func filterData(data tuicontract.Data, repo string) tuicontract.Data {
	if repo == "" || repo == "/" {
		return data
	}
	keep := func(items []tuicontract.Item) []tuicontract.Item {
		out := make([]tuicontract.Item, 0, len(items))
		for _, item := range items {
			ref := strings.ToLower(item.Ref)
			want := strings.ToLower(repo)
			for _, prefix := range []string{"issue:", "pr:", "pull_request:"} {
				ref = strings.TrimPrefix(ref, prefix)
			}
			if ref == want || strings.HasPrefix(ref, want+"#") || strings.HasPrefix(ref, want+":") {
				out = append(out, item)
			}
		}
		return out
	}
	data.Repositories = keep(data.Repositories)
	data.SyncStatuses = keep(data.SyncStatuses)
	data.Threads = keep(data.Threads)
	data.Clusters = keep(data.Clusters)
	data.Investigations = keep(data.Investigations)
	data.Opportunities = keep(data.Opportunities)
	data.Candidates = keep(data.Candidates)
	data.Hypotheses = keep(data.Hypotheses)
	data.Contributions = keep(data.Contributions)
	return data
}

// Run starts the TUI program with the provided local reader and I/O.
//
// It is suitable for later CLI wiring: the caller supplies a context, a
// Reader, and input/output streams. The TUI loads local data on start and
// never performs network I/O on its own.
func Run(ctx context.Context, reader tuicontract.Reader, input io.Reader, output io.Writer) (Model, error) {
	return run(ctx, reader, nil, nil, input, output)
}

func run(
	ctx context.Context,
	reader tuicontract.Reader,
	actions tuicontract.ActionProvider,
	briefs tuicontract.BriefProvider,
	input io.Reader,
	output io.Writer,
) (Model, error) {
	m := New(ctx, reader, WithActionProvider(actions), WithBriefProvider(briefs))

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

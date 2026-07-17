package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/lens"
)

// AddLens validates and stores a lens definition.
func (s *Service) AddLens(ctx context.Context, name string, def lens.Definition) (*cli.LensResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("lens name is required")
	}
	def.Name = name
	if err := lens.Validate(def); err != nil {
		return nil, err
	}

	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	record, err := c.SaveLens(ctx, def)
	if err != nil {
		return nil, fmt.Errorf("save lens: %w", err)
	}
	return lensResult(record), nil
}

// ListLenses returns all saved lenses in stable order.
func (s *Service) ListLenses(ctx context.Context) (*cli.LensListResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	records, err := c.ListLenses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list lenses: %w", err)
	}
	result := &cli.LensListResult{Lenses: make([]cli.LensResult, len(records))}
	for i, r := range records {
		result.Lenses[i] = *lensResult(&r)
	}
	return result, nil
}

// ShowLens returns a saved lens by name.
func (s *Service) ShowLens(ctx context.Context, name string) (*cli.LensResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("lens name is required")
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	record, err := c.GetLens(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get lens: %w", err)
	}
	if record == nil {
		return nil, cli.NewCLIError(cli.ExitNotFound, fmt.Errorf("lens %q not found", name))
	}
	return lensResult(record), nil
}

func lensResult(r *corpus.LensRecord) *cli.LensResult {
	return &cli.LensResult{
		Name:       r.Definition.Name,
		Definition: r.Definition,
		CreatedAt:  formatTime(r.CreatedAt),
		UpdatedAt:  formatTime(r.UpdatedAt),
	}
}

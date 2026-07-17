package app

import (
	"context"
	"errors"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/research"
)

// ThreadResearchBrief builds a deterministic brief from the local corpus. It
// performs no network access, local mutation, or process execution.
func (s *Service) ThreadResearchBrief(ctx context.Context, ref research.ThreadRef) (*research.Brief, error) {
	brief, err := research.NewBuilder(&corpusReader{s: s}, s.now).Build(ctx, ref)
	if err == nil {
		return brief, nil
	}
	if errors.Is(err, errRepositoryNotFound) || errors.Is(err, research.ErrThreadNotFound) || errors.Is(err, research.ErrThreadKindMismatch) {
		return nil, cli.NewCLIError(cli.ExitNotFound, err)
	}
	return nil, err
}

package cli

import (
	"context"

	"github.com/morluto/gitcontribute/internal/research"
)

// ResearchService exposes deterministic local thread briefs as an optional
// offline-read capability.
type ResearchService interface {
	ThreadResearchBrief(ctx context.Context, ref research.ThreadRef) (*research.Brief, error)
}

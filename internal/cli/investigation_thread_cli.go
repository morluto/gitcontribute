package cli

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/research"
)

type startThreadInvestigationCmd struct {
	Thread string `arg:"" name:"thread" help:"Thread as OWNER/REPO#NUMBER, issue:OWNER/REPO#NUMBER, or pr:OWNER/REPO#NUMBER"`
	JSON   bool   `name:"json" help:"Print the result as JSON"`
}

// ThreadInvestigationService is the optional local-write capability for
// starting an investigation and seed hypothesis from one stored thread.
type ThreadInvestigationService interface {
	StartInvestigationFromThread(ctx context.Context, ref research.ThreadRef) (*ThreadInvestigationResult, error)
}

// ThreadInvestigationResult contains the atomically created or reused pair.
type ThreadInvestigationResult struct {
	Created       bool                 `json:"created"`
	Investigation *InvestigationResult `json:"investigation"`
	Hypothesis    *HypothesisResult    `json:"hypothesis"`
}

// ThreadBaselineResult is the immutable observation revision saved at start.
type ThreadBaselineResult struct {
	Ref                  string                  `json:"ref"`
	Repository           string                  `json:"repository"`
	Kind                 string                  `json:"kind"`
	Number               int                     `json:"number"`
	ObservationID        int64                   `json:"observation_id"`
	SourceUpdatedAt      string                  `json:"source_updated_at,omitempty"`
	ObservationSequence  int64                   `json:"observation_sequence"`
	ObservedAt           string                  `json:"observed_at,omitempty"`
	Source               WorkflowSourceRefResult `json:"source"`
	DescriptionTruncated bool                    `json:"description_truncated"`
}

// WorkflowSourceRefResult is a transport-stable workflow provenance record.
type WorkflowSourceRefResult struct {
	Source     string `json:"source"`
	URL        string `json:"url,omitempty"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	ObservedAt string `json:"observed_at,omitempty"`
	AsOf       string `json:"as_of,omitempty"`
}

// WorkflowLinkResult is an explicit hypothesis source link.
type WorkflowLinkResult struct {
	Kind   string                  `json:"kind"`
	Ref    string                  `json:"ref"`
	Source WorkflowSourceRefResult `json:"source"`
}

// WorkflowAuditResult records why a local workflow object changed state.
type WorkflowAuditResult struct {
	From      string `json:"from,omitempty"`
	To        string `json:"to"`
	Rationale string `json:"rationale"`
	At        string `json:"at"`
}

func (c *CLI) runStartThreadInvestigation(ctx context.Context, cmd *startThreadInvestigationCmd) error {
	ref, err := research.ParseThreadRef(cmd.Thread)
	if err != nil {
		return NewCLIError(ExitUsage, err)
	}
	service, ok := c.svc.(ThreadInvestigationService)
	if !ok {
		return NewCLIError(ExitNotWired, ErrNotWired)
	}
	if _, err := fmt.Fprintf(c.stderr, "starting investigation from stored thread %s...\n", ref); err != nil {
		return c.mapError(err)
	}
	result, err := service.StartInvestigationFromThread(ctx, ref)
	if err != nil {
		return c.mapError(err)
	}
	return c.render(cmd.JSON, result)
}

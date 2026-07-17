package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/tracking"
)

var _ cli.TrackingService = (*Service)(nil)

// RecordTriageEvent records a local triage or feedback outcome for a typed target.
func (s *Service) RecordTriageEvent(ctx context.Context, opts cli.RecordTriageEventOptions) (*cli.TriageEventResult, error) {
	kind, ref, err := parseTrackingTarget(opts.Target)
	if err != nil {
		return nil, err
	}
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	event, err := tracking.NewService(c).RecordTriageEvent(ctx, &tracking.TriageEvent{
		TargetKind: tracking.TargetKind(kind),
		TargetRef:  ref,
		Outcome:    tracking.Outcome(opts.Outcome),
		Reason:     opts.Reason,
		Lens:       opts.Lens,
	})
	if err != nil {
		return nil, err
	}
	return triageEventResult(event), nil
}

// ListTriageEvents returns local triage outcomes in source-event order.
func (s *Service) ListTriageEvents(ctx context.Context, opts cli.ListTriageEventsOptions) (*cli.TriageEventListResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	events, err := tracking.NewService(c).ListTriageEvents(ctx, tracking.TriageEventFilter{
		TargetKind: tracking.TargetKind(opts.TargetKind),
		TargetRef:  opts.TargetRef,
		Outcome:    tracking.Outcome(opts.Outcome),
		Lens:       opts.Lens,
		Limit:      opts.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]cli.TriageEventResult, len(events))
	for i, e := range events {
		out[i] = *triageEventResult(e)
	}
	return &cli.TriageEventListResult{Events: out, Limit: opts.Limit, Total: len(out)}, nil
}

// RecordContribution stores prepared or submitted contribution metadata for an
// opportunity, keeping it separate from live GitHub state.
func (s *Service) RecordContribution(ctx context.Context, opts cli.RecordContributionOptions) (*cli.ContributionResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	item, err := tracking.NewService(c).RecordContribution(ctx, &tracking.Contribution{
		OpportunityID: opts.OpportunityID,
		Kind:          normalizeContributionKind(opts.Kind),
		Title:         opts.Title,
		Body:          opts.Body,
		Reference:     opts.Reference,
		ReferenceURL:  opts.ReferenceURL,
	})
	if err != nil {
		return nil, err
	}
	return contributionResult(item), nil
}

// GetContribution returns contribution metadata by durable id.
func (s *Service) GetContribution(ctx context.Context, id string) (*cli.ContributionResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	item, err := tracking.NewService(c).GetContribution(ctx, id)
	if err != nil {
		return nil, err
	}
	return contributionResult(item), nil
}

// ListContributions returns contribution metadata in prepared-at order.
func (s *Service) ListContributions(ctx context.Context, opts cli.ListContributionsOptions) (*cli.ContributionListResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	items, err := tracking.NewService(c).ListContributions(ctx, tracking.ContributionFilter{
		OpportunityID: opts.OpportunityID,
		Kind:          normalizeContributionKind(opts.Kind),
		Limit:         opts.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]cli.ContributionResult, len(items))
	for i, item := range items {
		out[i] = *contributionResult(item)
	}
	return &cli.ContributionListResult{Contributions: out, Limit: opts.Limit, Total: len(out)}, nil
}

// RecordContributionOutcome stores a lifecycle outcome for a contribution.
func (s *Service) RecordContributionOutcome(ctx context.Context, opts cli.RecordContributionOutcomeOptions) (*cli.ContributionOutcomeResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	outcome, err := tracking.NewService(c).RecordContributionOutcome(ctx, &tracking.ContributionOutcome{
		ContributionID: opts.ContributionID,
		Outcome:        tracking.Outcome(opts.Outcome),
		Reason:         opts.Reason,
	})
	if err != nil {
		return nil, err
	}
	return contributionOutcomeResult(outcome), nil
}

// ListContributionOutcomes returns lifecycle outcomes for a contribution.
func (s *Service) ListContributionOutcomes(ctx context.Context, contributionID string) (*cli.ContributionOutcomeListResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	outcomes, err := tracking.NewService(c).ListContributionOutcomes(ctx, contributionID)
	if err != nil {
		return nil, err
	}
	out := make([]cli.ContributionOutcomeResult, len(outcomes))
	for i, o := range outcomes {
		out[i] = *contributionOutcomeResult(o)
	}
	return &cli.ContributionOutcomeListResult{ContributionID: contributionID, Outcomes: out}, nil
}

// ExportLocalMetadata returns a bounded, redacted, deterministic JSON export of
// local tracking metadata. It does not include tokens, credentials, or absolute
// local paths.
func (s *Service) ExportLocalMetadata(ctx context.Context, opts cli.MetadataExportOptions) (*cli.MetadataExportResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	bundle, err := tracking.NewService(c).ExportLocalMetadata(ctx, tracking.ExportOptions{Limit: opts.Limit})
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal local metadata: %w", err)
	}
	return &cli.MetadataExportResult{
		SchemaVersion:        bundle.SchemaVersion,
		Data:                 json.RawMessage(data),
		TriageEvents:         len(bundle.TriageEvents),
		Contributions:        len(bundle.Contributions),
		ContributionOutcomes: len(bundle.ContributionOutcomes),
		Evidence:             len(bundle.Evidence),
	}, nil
}

// ImportLocalMetadata imports a bounded JSON bundle of local tracking metadata
// idempotently.
func (s *Service) ImportLocalMetadata(ctx context.Context, opts cli.MetadataImportOptions) (*cli.MetadataImportResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	var bundle tracking.Bundle
	if err := json.Unmarshal(opts.Data, &bundle); err != nil {
		return nil, fmt.Errorf("parse local metadata: %w", err)
	}
	if err := tracking.NewService(c).ImportLocalMetadata(ctx, &bundle); err != nil {
		return nil, err
	}
	version, err := tracking.ResolveBundleVersion(&bundle)
	if err != nil {
		return nil, err
	}
	return &cli.MetadataImportResult{
		SchemaVersion:        version,
		TriageEvents:         len(bundle.TriageEvents),
		Contributions:        len(bundle.Contributions),
		ContributionOutcomes: len(bundle.ContributionOutcomes),
		Evidence:             len(bundle.Evidence),
	}, nil
}

func parseTrackingTarget(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	kind, ref, ok := strings.Cut(raw, ":")
	if !ok || kind == "" || ref == "" {
		return "", "", fmt.Errorf("invalid target %q: expected kind:ref", raw)
	}
	switch strings.ToLower(kind) {
	case "repo", "repository":
		kind = "repository"
	case "issue", "issues":
		kind = "issue"
	case "pr", "pull_request", "pullrequest":
		kind = "pull_request"
	case "thread":
		kind = "thread"
	case "opportunity", "opp":
		kind = "opportunity"
	case "investigation", "inv":
		kind = "investigation"
	}
	return kind, strings.TrimSpace(ref), nil
}

func normalizeContributionKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "pr", "pull_request", "pullrequest":
		return "pull_request"
	default:
		return strings.TrimSpace(kind)
	}
}

func triageEventResult(e *tracking.TriageEvent) *cli.TriageEventResult {
	if e == nil {
		return nil
	}
	return &cli.TriageEventResult{
		ID:            e.ID,
		TargetKind:    string(e.TargetKind),
		TargetRef:     e.TargetRef,
		Outcome:       string(e.Outcome),
		Reason:        e.Reason,
		Lens:          e.Lens,
		SourceEventAt: formatTime(e.SourceEventAt),
		CreatedAt:     formatTime(e.CreatedAt),
		UpdatedAt:     formatTime(e.UpdatedAt),
	}
}

func contributionResult(c *tracking.Contribution) *cli.ContributionResult {
	if c == nil {
		return nil
	}
	var submitted string
	if c.SubmittedAt != nil {
		submitted = formatTime(*c.SubmittedAt)
	}
	return &cli.ContributionResult{
		ID:            c.ID,
		OpportunityID: c.OpportunityID,
		Kind:          c.Kind,
		Title:         c.Title,
		Body:          c.Body,
		Reference:     c.Reference,
		ReferenceURL:  c.ReferenceURL,
		PreparedAt:    formatTime(c.PreparedAt),
		SubmittedAt:   submitted,
		CreatedAt:     formatTime(c.CreatedAt),
		UpdatedAt:     formatTime(c.UpdatedAt),
		Metadata:      c.Metadata,
	}
}

func contributionOutcomeResult(o *tracking.ContributionOutcome) *cli.ContributionOutcomeResult {
	if o == nil {
		return nil
	}
	return &cli.ContributionOutcomeResult{
		ID:             o.ID,
		ContributionID: o.ContributionID,
		Outcome:        string(o.Outcome),
		Reason:         o.Reason,
		SourceEventAt:  formatTime(o.SourceEventAt),
		CreatedAt:      formatTime(o.CreatedAt),
	}
}

package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/morluto/gitcontribute/internal/tracking"
)

// RecordOutcome records a local triage or feedback outcome for a typed target.
func (s *Service) RecordOutcome(ctx context.Context, event *tracking.TriageEvent) (*tracking.TriageEvent, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return tracking.NewService(c).RecordTriageEvent(ctx, event)
}

// ListOutcomes returns local triage outcomes in source-event order.
func (s *Service) ListOutcomes(ctx context.Context, filter tracking.TriageEventFilter) ([]*tracking.TriageEvent, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return tracking.NewService(c).ListTriageEvents(ctx, filter)
}

// RecordContribution stores prepared or submitted contribution metadata for an
// opportunity, keeping it separate from live GitHub state.
func (s *Service) RecordContribution(ctx context.Context, item *tracking.Contribution) (*tracking.Contribution, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return tracking.NewService(c).RecordContribution(ctx, item)
}

// GetContribution returns contribution metadata by durable id.
func (s *Service) GetContribution(ctx context.Context, id string) (*tracking.Contribution, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return tracking.NewService(c).GetContribution(ctx, id)
}

// ListContributions returns contribution metadata in prepared-at order.
func (s *Service) ListContributions(ctx context.Context, filter tracking.ContributionFilter) ([]*tracking.Contribution, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return tracking.NewService(c).ListContributions(ctx, filter)
}

// RecordContributionOutcome stores a lifecycle outcome for a contribution.
func (s *Service) RecordContributionOutcome(ctx context.Context, outcome *tracking.ContributionOutcome) (*tracking.ContributionOutcome, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return tracking.NewService(c).RecordContributionOutcome(ctx, outcome)
}

// ListContributionOutcomes returns lifecycle outcomes for a contribution.
func (s *Service) ListContributionOutcomes(ctx context.Context, contributionID string) ([]*tracking.ContributionOutcome, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return tracking.NewService(c).ListContributionOutcomes(ctx, contributionID)
}

// ExportLocalMetadata returns a bounded, redacted, deterministic JSON export of
// local tracking metadata. It does not include tokens, credentials, or absolute
// local paths.
func (s *Service) ExportLocalMetadata(ctx context.Context, opts tracking.ExportOptions) ([]byte, *tracking.Bundle, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, nil, err
	}
	bundle, err := tracking.NewService(c).ExportLocalMetadata(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	b, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal local metadata: %w", err)
	}
	return b, bundle, nil
}

// ImportLocalMetadata imports a bounded JSON bundle of local tracking metadata
// idempotently.
func (s *Service) ImportLocalMetadata(ctx context.Context, data []byte) error {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return err
	}
	var bundle tracking.Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return fmt.Errorf("parse local metadata: %w", err)
	}
	return tracking.NewService(c).ImportLocalMetadata(ctx, &bundle)
}

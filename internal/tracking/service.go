package tracking

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Repository is the persistence boundary for local tracking and feedback.
type Repository interface {
	RecordTriageEvent(ctx context.Context, e *TriageEvent) error
	ListTriageEvents(ctx context.Context, filter TriageEventFilter) ([]*TriageEvent, error)

	SaveContribution(ctx context.Context, c *Contribution) error
	GetContribution(ctx context.Context, id string) (*Contribution, error)
	ListContributions(ctx context.Context, filter ContributionFilter) ([]*Contribution, error)

	RecordContributionOutcome(ctx context.Context, o *ContributionOutcome) error
	ListContributionOutcomes(ctx context.Context, contributionID string) ([]*ContributionOutcome, error)

	ExportLocalMetadata(ctx context.Context, opts ExportOptions) (*Bundle, error)
	ImportLocalMetadata(ctx context.Context, bundle *Bundle) error
}

// Service validates and records local triage decisions and contribution outcomes.
type Service struct {
	repo  Repository
	clock func() time.Time
}

// NewService returns a tracking service backed by repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, clock: time.Now}
}

// SetClock overrides the time source. It is intended for tests.
func (s *Service) SetClock(clock func() time.Time) {
	s.clock = clock
}

// RecordTriageEvent stores a local triage decision after validating it.
func (s *Service) RecordTriageEvent(ctx context.Context, e *TriageEvent) (*TriageEvent, error) {
	if e == nil {
		return nil, errors.New("triage event is required")
	}
	if err := validateTriageEvent(e); err != nil {
		return nil, err
	}
	now := s.now()
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	if e.SourceEventAt.IsZero() {
		e.SourceEventAt = now
	}
	if err := s.repo.RecordTriageEvent(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// ListTriageEvents returns triage events ordered by source event time and id.
func (s *Service) ListTriageEvents(ctx context.Context, filter TriageEventFilter) ([]*TriageEvent, error) {
	return s.repo.ListTriageEvents(ctx, filter)
}

// RecordContribution stores prepared or submitted contribution metadata.
func (s *Service) RecordContribution(ctx context.Context, c *Contribution) (*Contribution, error) {
	if c == nil {
		return nil, errors.New("contribution is required")
	}
	c.OpportunityID = strings.TrimSpace(c.OpportunityID)
	if c.OpportunityID == "" {
		return nil, errors.New("contribution opportunity id is required")
	}
	c.Kind = strings.TrimSpace(c.Kind)
	if c.Kind != "issue" && c.Kind != "pull_request" {
		return nil, fmt.Errorf("unsupported contribution kind %q", c.Kind)
	}
	c.Title = strings.TrimSpace(c.Title)
	if c.Title == "" {
		return nil, errors.New("contribution title is required")
	}
	now := s.now()
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	if c.PreparedAt.IsZero() {
		c.PreparedAt = now
	}
	if err := s.repo.SaveContribution(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetContribution returns a contribution by durable id.
func (s *Service) GetContribution(ctx context.Context, id string) (*Contribution, error) {
	return s.repo.GetContribution(ctx, id)
}

// ListContributions returns contributions ordered by creation time.
func (s *Service) ListContributions(ctx context.Context, filter ContributionFilter) ([]*Contribution, error) {
	return s.repo.ListContributions(ctx, filter)
}

// RecordContributionOutcome stores a lifecycle event for a contribution.
func (s *Service) RecordContributionOutcome(ctx context.Context, o *ContributionOutcome) (*ContributionOutcome, error) {
	if o == nil {
		return nil, errors.New("contribution outcome is required")
	}
	o.ContributionID = strings.TrimSpace(o.ContributionID)
	if o.ContributionID == "" {
		return nil, errors.New("contribution id is required")
	}
	if !isContributionOutcome(o.Outcome) {
		return nil, fmt.Errorf("invalid contribution outcome %q", o.Outcome)
	}
	now := s.now()
	if o.ID == "" {
		o.ID = uuid.NewString()
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = now
	}
	if o.SourceEventAt.IsZero() {
		o.SourceEventAt = now
	}
	if err := s.repo.RecordContributionOutcome(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// ListContributionOutcomes returns outcomes for a contribution.
func (s *Service) ListContributionOutcomes(ctx context.Context, contributionID string) ([]*ContributionOutcome, error) {
	return s.repo.ListContributionOutcomes(ctx, contributionID)
}

// ExportLocalMetadata returns a redacted, deterministic bundle of local metadata.
func (s *Service) ExportLocalMetadata(ctx context.Context, opts ExportOptions) (*Bundle, error) {
	return s.repo.ExportLocalMetadata(ctx, opts)
}

// ImportLocalMetadata imports a bounded bundle idempotently.
func (s *Service) ImportLocalMetadata(ctx context.Context, bundle *Bundle) error {
	if bundle == nil {
		return errors.New("bundle is required")
	}
	return s.repo.ImportLocalMetadata(ctx, bundle)
}

func (s *Service) now() time.Time {
	if s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock().UTC()
}

func validateTriageEvent(e *TriageEvent) error {
	e.TargetRef = strings.TrimSpace(e.TargetRef)
	if e.TargetRef == "" {
		return errors.New("triage target reference is required")
	}
	if !isValidTargetKind(e.TargetKind) {
		return fmt.Errorf("unsupported triage target kind %q", e.TargetKind)
	}
	if !isValidOutcome(e.Outcome) {
		return fmt.Errorf("unsupported triage outcome %q", e.Outcome)
	}
	return nil
}

func isValidTargetKind(k TargetKind) bool {
	switch k {
	case TargetRepository, TargetIssue, TargetPullRequest, TargetThread,
		TargetOpportunity, TargetInvestigation:
		return true
	}
	return false
}

func isValidOutcome(o Outcome) bool {
	switch o {
	case OutcomeViewed, OutcomeIgnored, OutcomeSaved, OutcomeInvestigated,
		OutcomeImplemented, OutcomeSubmitted, OutcomeMerged, OutcomeRejected,
		OutcomeAbandoned:
		return true
	}
	return false
}

func isContributionOutcome(o Outcome) bool {
	switch o {
	case OutcomeSubmitted, OutcomeMerged, OutcomeRejected, OutcomeAbandoned:
		return true
	}
	return false
}

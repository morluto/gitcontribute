package investigation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/domain"
)

// ThreadBaseline identifies the immutable stored observation from which an
// investigation and its seed hypothesis were created.
type ThreadBaseline struct {
	Repo                 domain.RepoRef
	Kind                 domain.ThreadKind
	Number               int
	ObservationID        int64
	SourceUpdatedAt      time.Time
	ObservationSequence  int64
	ObservedAt           time.Time
	Source               domain.SourceRef
	DescriptionTruncated bool
}

// Ref returns a stable explicit thread reference.
func (b ThreadBaseline) Ref() string {
	return fmt.Sprintf("%s:%s#%d", b.Kind, b.Repo, b.Number)
}

// OriginKey returns the case-insensitive identity used to prevent duplicate
// open investigations for the same source thread.
func (b ThreadBaseline) OriginKey() string {
	return strings.ToLower(b.Ref())
}

// Validate ensures the baseline can identify both a source thread and one
// immutable local observation.
func (b ThreadBaseline) Validate() error {
	if err := b.Repo.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidThreadBaseline, err)
	}
	if b.Kind != domain.IssueKind && b.Kind != domain.PullRequestKind {
		return fmt.Errorf("%w: unsupported thread kind %q", ErrInvalidThreadBaseline, b.Kind)
	}
	if b.Number <= 0 || b.ObservationID <= 0 || b.ObservationSequence <= 0 {
		return fmt.Errorf("%w: thread number and observation identity must be positive", ErrInvalidThreadBaseline)
	}
	if strings.TrimSpace(b.Source.Source) == "" || strings.TrimSpace(b.Source.URL) == "" {
		return fmt.Errorf("%w: traceable observation source is required", ErrInvalidThreadBaseline)
	}
	if !b.Source.AsOf.Equal(b.SourceUpdatedAt) || !b.Source.ObservedAt.Equal(b.ObservedAt) {
		return fmt.Errorf("%w: source reference does not match observation revision", ErrInvalidThreadBaseline)
	}
	return nil
}

// StartFromThreadInput contains source-backed values for the atomic start
// operation. Description may be bounded by the application adapter, which must
// record that fact on Baseline.
type StartFromThreadInput struct {
	Baseline    ThreadBaseline
	Title       string
	Description string
}

// StartFromThreadResult reports whether a new pair was created or an existing
// open investigation for the same thread was returned.
type StartFromThreadResult struct {
	Investigation *Investigation
	Hypothesis    *Hypothesis
	Created       bool
}

// StartFromThread atomically creates an investigation and proposed hypothesis
// from an immutable thread baseline. Repeated requests return the existing open
// pair without changing its baseline.
func (s *Service) StartFromThread(ctx context.Context, input StartFromThreadInput) (*StartFromThreadResult, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidThreadBaseline)
	}
	if err := input.Baseline.Validate(); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, ErrMissingTitle
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	investigationID := uuid.NewString()
	hypothesisID := uuid.NewString()
	auditReason := fmt.Sprintf("started from stored thread baseline %s observation %d", input.Baseline.Ref(), input.Baseline.ObservationID)
	inv := &Investigation{
		ID: investigationID, Repo: input.Baseline.Repo, Status: InvestigationOpen,
		ThreadBaseline: &input.Baseline, SeedHypothesisID: hypothesisID,
		AuditTrail: []StatusChange{{From: "", To: string(InvestigationOpen), Rationale: auditReason, At: now}},
		CreatedAt:  now, UpdatedAt: now,
	}
	hypothesis := &Hypothesis{
		ID: hypothesisID, InvestigationID: investigationID, Title: title,
		Description: input.Description, Category: CategoryOther,
		SourceRefs: []domain.SourceRef{input.Baseline.Source},
		Links:      []Link{{Kind: string(input.Baseline.Kind), Ref: input.Baseline.Ref(), Source: input.Baseline.Source}},
		Status:     HypothesisProposed,
		AuditTrail: []StatusChange{{From: "", To: string(HypothesisProposed), Rationale: auditReason, At: now}},
		CreatedAt:  now, UpdatedAt: now,
	}

	storedInvestigation, storedHypothesis, created, err := s.repo.StartThreadInvestigation(ctx, inv, hypothesis)
	if err != nil {
		return nil, err
	}
	return &StartFromThreadResult{Investigation: storedInvestigation, Hypothesis: storedHypothesis, Created: created}, nil
}

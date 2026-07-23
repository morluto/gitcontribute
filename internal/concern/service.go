package concern

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
)

const (
	maxListLimit = 100
	maxItems     = 100
	maxTextBytes = 256 << 10
)

// Service validates local concern lifecycle operations.
type Service struct{ repo Repository }

// NewService returns a concern service backed by repo.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Create validates and stores one local concern.
func (s *Service) Create(ctx context.Context, item *Concern) (*Concern, error) {
	if item == nil {
		return nil, errors.New("concern is required")
	}
	itemCopy := cloneConcern(item)
	if err := normalizeConcern(&itemCopy); err != nil {
		return nil, err
	}
	if itemCopy.ID == "" {
		itemCopy.ID = uuid.NewString()
	}
	if itemCopy.Status == "" {
		itemCopy.Status = StatusUntriaged
	}
	if itemCopy.Status != StatusUntriaged {
		return nil, fmt.Errorf("%w: new concerns must be untriaged", ErrInvalidStatus)
	}
	now := time.Now().UTC()
	if itemCopy.CreatedAt.IsZero() {
		itemCopy.CreatedAt = now
	}
	itemCopy.UpdatedAt = itemCopy.CreatedAt
	if err := s.repo.SaveConcern(ctx, &itemCopy); err != nil {
		return nil, err
	}
	return &itemCopy, nil
}

// Get returns one concern.
func (s *Service) Get(ctx context.Context, id string) (*Concern, error) {
	return s.repo.GetConcern(ctx, strings.TrimSpace(id))
}

// List returns at most 100 concerns and performs no external reads.
func (s *Service) List(ctx context.Context, filter Filter) ([]*Concern, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > maxListLimit {
		return nil, errors.New("concern list limit cannot exceed 100")
	}
	if filter.Status != "" && !validStatus(filter.Status) {
		return nil, ErrInvalidStatus
	}
	if filter.Repo.Owner != "" || filter.Repo.Repo != "" {
		if err := filter.Repo.Validate(); err != nil {
			return nil, err
		}
	}
	return s.repo.ListConcerns(ctx, filter)
}

// Update replaces editable concern fields while preserving identity and audit.
func (s *Service) Update(ctx context.Context, item *Concern) (*Concern, error) {
	if item == nil || strings.TrimSpace(item.ID) == "" {
		return nil, errors.New("concern id is required")
	}
	stored, err := s.repo.GetConcern(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	itemCopy := cloneConcern(item)
	itemCopy.ID, itemCopy.CreatedAt, itemCopy.Status = stored.ID, stored.CreatedAt, stored.Status
	itemCopy.AuditTrail, itemCopy.Promotion = stored.AuditTrail, stored.Promotion
	if err := normalizeConcern(&itemCopy); err != nil {
		return nil, err
	}
	itemCopy.UpdatedAt = time.Now().UTC()
	if err := s.repo.SaveConcern(ctx, &itemCopy); err != nil {
		return nil, err
	}
	return &itemCopy, nil
}

// SetStatus applies an explicit lifecycle transition with rationale.
func (s *Service) SetStatus(ctx context.Context, id string, next Status, rationale string) (*Concern, error) {
	item, err := s.repo.GetConcern(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	rationale = strings.TrimSpace(rationale)
	if rationale == "" {
		return nil, errors.New("status rationale is required")
	}
	if !allowedTransition(item.Status, next) {
		return nil, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, item.Status, next)
	}
	now := time.Now().UTC()
	item.AuditTrail = append(item.AuditTrail, StatusChange{From: item.Status, To: next, Rationale: rationale, At: now})
	item.Status, item.UpdatedAt = next, now
	if err := s.repo.SaveConcern(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

// Link records an explicit relationship. Duplicate candidates remain links;
// they do not silently change concern status or assert a root cause.
func (s *Service) Link(ctx context.Context, id string, link Link) error {
	link.TargetType, link.TargetID, link.Note = strings.TrimSpace(link.TargetType), strings.TrimSpace(link.TargetID), strings.TrimSpace(link.Note)
	if !validLinkKind(link.Kind) || link.TargetType == "" || link.TargetID == "" {
		return ErrInvalidLink
	}
	if link.CreatedAt.IsZero() {
		link.CreatedAt = time.Now().UTC()
	}
	return s.repo.AddConcernLink(ctx, strings.TrimSpace(id), link)
}

func normalizeConcern(item *Concern) error {
	if err := item.Repo.Validate(); err != nil {
		return fmt.Errorf("invalid concern repository: %w", err)
	}
	item.CommitSHA = strings.TrimSpace(item.CommitSHA)
	item.WorkspaceID = strings.TrimSpace(item.WorkspaceID)
	item.Title = strings.TrimSpace(item.Title)
	item.ProblemStatement = strings.TrimSpace(item.ProblemStatement)
	item.SuspectedOwner = strings.TrimSpace(item.SuspectedOwner)
	item.SuccessCriterion = strings.TrimSpace(item.SuccessCriterion)
	item.Notes = strings.TrimSpace(item.Notes)
	if item.CommitSHA == "" && item.WorkspaceID == "" {
		return errors.New("commit SHA or workspace ID is required")
	}
	if item.Title == "" || item.ProblemStatement == "" {
		return errors.New("concern title and problem statement are required")
	}
	if math.IsNaN(item.Confidence) || math.IsInf(item.Confidence, 0) || item.Confidence < 0 || item.Confidence > 1 {
		return errors.New("concern confidence must be between 0 and 1")
	}
	if len(item.Unknowns) > maxItems || len(item.EvidenceIDs) > maxItems || len(item.SourceRefs) > maxItems || len(item.SourceProvenance) > maxItems {
		return errors.New("concern collections cannot exceed 100 items")
	}
	if len(item.Title)+len(item.ProblemStatement)+len(item.SuspectedOwner)+len(item.SuccessCriterion)+len(item.Notes) > maxTextBytes {
		return errors.New("concern text exceeds 256 KiB")
	}
	item.Unknowns = normalizeStrings(item.Unknowns)
	item.EvidenceIDs = normalizeStrings(item.EvidenceIDs)
	revisions, err := evidence.NormalizeSourceRevisions(item.SourceProvenance)
	if err != nil {
		return err
	}
	item.SourceProvenance = revisions
	return nil
}

func normalizeStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}

func cloneConcern(item *Concern) Concern {
	result := *item
	result.Unknowns = append([]string(nil), item.Unknowns...)
	result.EvidenceIDs = append([]string(nil), item.EvidenceIDs...)
	result.SourceRefs = append([]domain.SourceRef(nil), item.SourceRefs...)
	result.SourceProvenance = append([]evidence.SourceRevision(nil), item.SourceProvenance...)
	result.Links = append([]Link(nil), item.Links...)
	result.AuditTrail = append([]StatusChange(nil), item.AuditTrail...)
	if item.Promotion != nil {
		promotion := *item.Promotion
		result.Promotion = &promotion
	}
	return result
}

func validStatus(status Status) bool {
	return status == StatusUntriaged || status == StatusAccepted || status == StatusInvestigating || status == StatusDeferred || status == StatusPromoted || status == StatusResolved
}

func allowedTransition(from, to Status) bool {
	if !validStatus(to) || from == to {
		return false
	}
	switch from {
	case StatusUntriaged:
		return to == StatusAccepted || to == StatusDeferred || to == StatusResolved
	case StatusAccepted:
		return to == StatusInvestigating || to == StatusDeferred || to == StatusResolved
	case StatusInvestigating:
		return to == StatusDeferred || to == StatusResolved
	case StatusDeferred, StatusResolved:
		return to == StatusAccepted
	case StatusPromoted:
		return to == StatusResolved
	default:
		return false
	}
}

func validLinkKind(kind LinkKind) bool {
	return kind == LinkRelated || kind == LinkDuplicateCandidate || kind == LinkHotspot || kind == LinkInvestigation || kind == LinkOpportunity
}

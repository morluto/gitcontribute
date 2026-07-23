package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/concern"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func (s *Service) writeConcernService(ctx context.Context) (*concern.Service, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return concern.NewService(c), nil
}

func (s *Service) readConcernService(ctx context.Context) (*concern.Service, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	return concern.NewService(c), nil
}

// CreateConcern records one local concern without external access.
func (s *Service) CreateConcern(ctx context.Context, opts cli.ConcernCreateOptions) (*cli.ConcernResult, error) {
	return s.createConcern(ctx, &concern.Concern{
		Repo: domain.RepoRef{Owner: opts.Repo.Owner, Repo: opts.Repo.Repo}, CommitSHA: opts.CommitSHA, WorkspaceID: opts.WorkspaceID,
		Title: opts.Title, ProblemStatement: opts.ProblemStatement, SuspectedOwner: opts.SuspectedOwner,
		Confidence: opts.Confidence, Unknowns: opts.Unknowns, SuccessCriterion: opts.SuccessCriterion,
		Notes: opts.Notes, EvidenceIDs: opts.EvidenceIDs,
	})
}

func (s *Service) createConcern(ctx context.Context, input *concern.Concern) (*cli.ConcernResult, error) {
	svc, err := s.writeConcernService(ctx)
	if err != nil {
		return nil, err
	}
	item, err := svc.Create(ctx, input)
	if err != nil {
		return nil, mapConcernError(err)
	}
	return s.concernResult(ctx, item)
}

// ListConcerns performs a bounded offline concern list or search.
func (s *Service) ListConcerns(ctx context.Context, opts cli.ConcernListOptions) (*cli.ConcernListResult, error) {
	svc, err := s.readConcernService(ctx)
	if err != nil {
		return nil, err
	}
	items, err := svc.List(ctx, concern.Filter{
		Repo: domain.RepoRef{Owner: opts.Repo.Owner, Repo: opts.Repo.Repo}, Status: concern.Status(opts.Status), Query: opts.Query, Limit: opts.Limit,
	})
	if err != nil {
		return nil, mapConcernError(err)
	}
	result := &cli.ConcernListResult{Concerns: make([]cli.ConcernResult, 0, len(items)), Total: len(items)}
	for _, item := range items {
		converted, err := s.concernResult(ctx, item)
		if err != nil {
			return nil, err
		}
		result.Concerns = append(result.Concerns, *converted)
	}
	return result, nil
}

// ShowConcern returns one local concern with derived freshness.
func (s *Service) ShowConcern(ctx context.Context, id string) (*cli.ConcernResult, error) {
	svc, err := s.readConcernService(ctx)
	if err != nil {
		return nil, err
	}
	item, err := svc.Get(ctx, id)
	if err != nil {
		return nil, mapConcernError(err)
	}
	return s.concernResult(ctx, item)
}

// UpdateConcern changes editable concern content without changing status.
func (s *Service) UpdateConcern(ctx context.Context, id string, opts cli.ConcernUpdateOptions) (*cli.ConcernResult, error) {
	svc, err := s.writeConcernService(ctx)
	if err != nil {
		return nil, err
	}
	item, err := svc.Get(ctx, id)
	if err != nil {
		return nil, mapConcernError(err)
	}
	if opts.Title != nil {
		item.Title = *opts.Title
	}
	if opts.ProblemStatement != nil {
		item.ProblemStatement = *opts.ProblemStatement
	}
	if opts.SuspectedOwner != nil {
		item.SuspectedOwner = *opts.SuspectedOwner
	}
	if opts.Confidence != nil {
		item.Confidence = *opts.Confidence
	}
	if opts.Unknowns != nil {
		item.Unknowns = opts.Unknowns
	}
	if opts.SuccessCriterion != nil {
		item.SuccessCriterion = *opts.SuccessCriterion
	}
	if opts.Notes != nil {
		item.Notes = *opts.Notes
	}
	if opts.EvidenceIDs != nil {
		item.EvidenceIDs = opts.EvidenceIDs
	}
	item, err = svc.Update(ctx, item)
	if err != nil {
		return nil, mapConcernError(err)
	}
	return s.concernResult(ctx, item)
}

// SetConcernStatus applies one validated non-promotion transition.
func (s *Service) SetConcernStatus(ctx context.Context, id, status, rationale string) (*cli.ConcernResult, error) {
	svc, err := s.writeConcernService(ctx)
	if err != nil {
		return nil, err
	}
	item, err := svc.SetStatus(ctx, id, concern.Status(strings.TrimSpace(status)), rationale)
	if err != nil {
		return nil, mapConcernError(err)
	}
	return s.concernResult(ctx, item)
}

// LinkConcern attaches one explicit typed relationship.
func (s *Service) LinkConcern(ctx context.Context, id string, opts cli.ConcernLinkOptions) (*cli.ConcernResult, error) {
	svc, err := s.writeConcernService(ctx)
	if err != nil {
		return nil, err
	}
	if err := svc.Link(ctx, id, concern.Link{Kind: concern.LinkKind(opts.Kind), TargetType: opts.TargetType, TargetID: opts.TargetID, Note: opts.Note}); err != nil {
		return nil, mapConcernError(err)
	}
	return s.ShowConcern(ctx, id)
}

// PromoteConcern atomically creates downstream investigation workflow records.
func (s *Service) PromoteConcern(ctx context.Context, id string, opts cli.ConcernPromoteOptions) (*cli.ConcernResult, error) {
	c, err := s.openCorpus(ctx)
	if err != nil {
		return nil, err
	}
	item, err := c.GetConcern(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, mapConcernError(err)
	}
	category := investigation.Category(strings.TrimSpace(opts.Category))
	if !investigation.ValidCategory(category) {
		return nil, investigation.ErrInvalidCategory
	}
	kind := strings.TrimSpace(opts.Kind)
	if kind != "investigation" && kind != "opportunity" {
		return nil, errors.New("promotion kind must be investigation or opportunity")
	}
	now := time.Now().UTC()
	hypothesisID := uuid.NewString()
	inv := &investigation.Investigation{
		ID: uuid.NewString(), Repo: item.Repo, CommitSHA: item.CommitSHA, Status: investigation.InvestigationOpen,
		SeedHypothesisID: hypothesisID, CreatedAt: now, UpdatedAt: now,
	}
	hypothesis := &investigation.Hypothesis{
		ID: hypothesisID, InvestigationID: inv.ID, Title: item.Title, Description: item.ProblemStatement,
		Category: category, OpenQuestions: append([]string(nil), item.Unknowns...), SourceRefs: append([]domain.SourceRef(nil), item.SourceRefs...),
		Links: []investigation.Link{{Kind: "concern", Ref: item.ID}}, Status: investigation.HypothesisProposed, CreatedAt: now, UpdatedAt: now,
	}
	var opportunity *investigation.Opportunity
	if kind == "opportunity" {
		if strings.TrimSpace(opts.Scope) == "" || strings.TrimSpace(opts.Impact) == "" || strings.TrimSpace(opts.ExpectedEffort) == "" {
			return nil, errors.New("scope, impact, and expected effort are required for opportunity promotion")
		}
		hypothesis.Status = investigation.HypothesisPromoted
		hypothesis.AuditTrail = []investigation.StatusChange{{From: string(investigation.HypothesisProposed), To: string(investigation.HypothesisPromoted), Rationale: "promoted from concern", At: now}}
		opportunity = &investigation.Opportunity{
			ID: uuid.NewString(), InvestigationID: inv.ID, HypothesisID: hypothesis.ID, Title: item.Title,
			ProblemStatement: item.ProblemStatement, Category: category, Scope: opts.Scope, Impact: opts.Impact,
			Confidence: item.Confidence, ExpectedEffort: opts.ExpectedEffort, CollisionStatus: investigation.CollisionUnknown,
			SourceRefs: append([]domain.SourceRef(nil), item.SourceRefs...), EvidenceIDs: append([]string(nil), item.EvidenceIDs...),
			Status: investigation.OpportunityHypothesis, CreatedAt: now, UpdatedAt: now,
		}
	}
	item, err = c.PromoteConcern(ctx, item.ID, inv, hypothesis, opportunity)
	if err != nil {
		return nil, mapConcernError(err)
	}
	return s.concernResult(ctx, item)
}

func (s *Service) concernResult(ctx context.Context, item *concern.Concern) (*cli.ConcernResult, error) {
	freshness := evidence.Freshness{Status: evidence.FreshnessUnknown, Reason: "concern has no recorded corpus source revision"}
	if len(item.SourceProvenance) > 0 {
		c, err := s.openReadOnlyCorpus(ctx)
		if err != nil {
			return nil, err
		}
		freshness, err = evidence.NewFreshnessEvaluator(c).Evaluate(ctx, &evidence.Evidence{Type: evidence.EvidenceTypeGitHubSource, SourceProvenance: item.SourceProvenance})
		if err != nil {
			return nil, err
		}
	}
	result := &cli.ConcernResult{
		ID: item.ID, Repo: cli.RepoRef{Owner: item.Repo.Owner, Repo: item.Repo.Repo}, CommitSHA: item.CommitSHA, WorkspaceID: item.WorkspaceID,
		Title: item.Title, ProblemStatement: item.ProblemStatement, SuspectedOwner: item.SuspectedOwner, Confidence: item.Confidence,
		Unknowns: append([]string(nil), item.Unknowns...), SuccessCriterion: item.SuccessCriterion, Notes: item.Notes,
		EvidenceIDs: append([]string(nil), item.EvidenceIDs...), SourceRefCount: len(item.SourceRefs), Freshness: string(freshness.Status),
		FreshnessReason: freshness.Reason, Status: string(item.Status), CreatedAt: formatTime(item.CreatedAt), UpdatedAt: formatTime(item.UpdatedAt),
	}
	for _, link := range item.Links {
		result.Links = append(result.Links, cli.ConcernLinkResult{Kind: string(link.Kind), TargetType: link.TargetType, TargetID: link.TargetID, Note: link.Note})
	}
	if item.Promotion != nil {
		result.Promotion = &cli.ConcernPromotionResult{Kind: item.Promotion.Kind, InvestigationID: item.Promotion.InvestigationID, HypothesisID: item.Promotion.HypothesisID, OpportunityID: item.Promotion.OpportunityID}
	}
	return result, nil
}

func mapConcernError(err error) error {
	if errors.Is(err, concern.ErrNotFound) {
		return cli.NewCLIError(cli.ExitNotFound, err)
	}
	return err
}

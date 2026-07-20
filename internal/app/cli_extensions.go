package app

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
)

// UpdateHypothesisForCLI maps CLI update options to the structured application input.
func (s *Service) UpdateHypothesisForCLI(ctx context.Context, id string, opts cli.HypothesisUpdateOptions) (any, error) {
	invSvc, err := s.investigationSvc(ctx)
	if err != nil {
		return nil, err
	}
	current, err := invSvc.GetHypothesis(ctx, id)
	if err != nil {
		return nil, mapInvestigationError(err)
	}
	in := investigation.UpdateHypothesisInput{
		Title: current.Title, Description: current.Description, Category: current.Category,
		ExpectedBehavior: current.ExpectedBehavior, ObservedBehavior: current.ObservedBehavior,
		PotentialImpact: current.PotentialImpact, OpenQuestions: append([]string(nil), current.OpenQuestions...),
		AffectedComponents: append([]string(nil), current.AffectedComponents...),
		SourceRefs:         append([]domain.SourceRef(nil), current.SourceRefs...), Links: append([]investigation.Link(nil), current.Links...),
		Rationale: opts.Rationale,
	}
	if opts.Title != nil {
		in.Title = *opts.Title
	}
	if opts.Description != nil {
		in.Description = *opts.Description
	}
	if opts.Category != nil {
		in.Category = investigation.Category(*opts.Category)
	}
	if opts.ExpectedBehavior != nil {
		in.ExpectedBehavior = *opts.ExpectedBehavior
	}
	if opts.ObservedBehavior != nil {
		in.ObservedBehavior = *opts.ObservedBehavior
	}
	if opts.PotentialImpact != nil {
		in.PotentialImpact = *opts.PotentialImpact
	}
	if opts.OpenQuestions != nil {
		in.OpenQuestions = append([]string(nil), opts.OpenQuestions...)
	}
	if opts.AffectedComponents != nil {
		in.AffectedComponents = append([]string(nil), opts.AffectedComponents...)
	}
	return s.UpdateHypothesis(ctx, id, in)
}

// TransitionHypothesisForCLI validates and records a rationale-bearing status change.
func (s *Service) TransitionHypothesisForCLI(ctx context.Context, id, status, rationale string) (any, error) {
	return s.TransitionHypothesis(ctx, id, status, rationale)
}

// CheckDuplicatesForCLI runs local duplicate analysis for a hypothesis or opportunity.
func (s *Service) CheckDuplicatesForCLI(ctx context.Context, target, id string, limit int) (any, error) {
	switch target {
	case "hypothesis":
		return s.CheckHypothesisDuplicates(ctx, id, limit)
	case "opportunity":
		return s.CheckOpportunityDuplicates(ctx, id, limit)
	default:
		return nil, fmt.Errorf("unknown duplicate target %q", target)
	}
}

// CheckCollisionsForCLI runs local open-work collision analysis for a target.
func (s *Service) CheckCollisionsForCLI(ctx context.Context, target, id string, limit int) (any, error) {
	switch target {
	case "hypothesis":
		return s.CheckHypothesisCollisions(ctx, id, limit)
	case "opportunity":
		return s.CheckOpportunityCollisions(ctx, id, limit)
	default:
		return nil, fmt.Errorf("unknown collision target %q", target)
	}
}

// SetCollisionForCLI records a reviewed collision status and rationale.
func (s *Service) SetCollisionForCLI(ctx context.Context, id, status, rationale string) (any, error) {
	return s.UpdateOpportunityCollisionStatus(ctx, id, status, rationale)
}

// RecordEvidenceForCLI maps CLI evidence fields to the application evidence model.
func (s *Service) RecordEvidenceForCLI(ctx context.Context, opts cli.RecordEvidenceOptions) (any, error) {
	return s.RecordEvidence(ctx, RecordEvidenceInput{
		InvestigationID: opts.InvestigationID, HypothesisID: opts.HypothesisID, OpportunityID: opts.OpportunityID,
		Type: opts.Type, Relation: opts.Relation, Description: opts.Description,
	})
}

// WorkspaceDiffForCLI returns bounded diff metadata for a managed workspace.
func (s *Service) WorkspaceDiffForCLI(ctx context.Context, id string) (any, error) {
	return s.WorkspaceDiff(ctx, id)
}

// PrepareReviewForCLI creates a local review report from an opportunity and workspace.
func (s *Service) PrepareReviewForCLI(ctx context.Context, opportunityID, workspaceID string) (any, error) {
	return s.PrepareReviewReport(ctx, PrepareReviewReportInput{OpportunityID: opportunityID, WorkspaceID: workspaceID})
}

// BuildDossierForCLI builds and persists a dossier from local corpus data.
func (s *Service) BuildDossierForCLI(ctx context.Context, repo cli.RepoRef) (any, error) {
	return s.BuildRepositoryDossier(ctx, repo)
}

// GetDossierForCLI returns the latest persisted dossier without network access.
func (s *Service) GetDossierForCLI(ctx context.Context, repo cli.RepoRef) (any, error) {
	return s.GetRepositoryDossier(ctx, repo)
}

// ExtractSeedsForCLI derives bounded contribution seeds from stored threads.
func (s *Service) ExtractSeedsForCLI(ctx context.Context, repo cli.RepoRef, classes, polarities []string, limit int) (any, error) {
	parsed := make([]domain.SeedSourceClass, len(classes))
	for i, class := range classes {
		switch class {
		case "merged-prs", "merged_pr", "merged_prs":
			parsed[i] = domain.SeedSourceClassMergedPR
		case "closed-prs", "closed_unmerged_pr", "closed_unmerged_prs":
			parsed[i] = domain.SeedSourceClassClosedUnmergedPR
		case "issues", "issue":
			parsed[i] = domain.SeedSourceClassIssue
		default:
			return nil, fmt.Errorf("unknown seed source class %q", class)
		}
	}
	parsedPolarities := make([]domain.SeedPolarity, 0, len(polarities))
	for _, polarity := range polarities {
		switch polarity {
		case "positive":
			parsedPolarities = append(parsedPolarities, domain.SeedPolarityPositive)
		case "negative":
			parsedPolarities = append(parsedPolarities, domain.SeedPolarityNegative)
		case "context":
			parsedPolarities = append(parsedPolarities, domain.SeedPolarityContext)
		case "all":
			parsedPolarities = append(parsedPolarities, domain.SeedPolarityPositive, domain.SeedPolarityNegative, domain.SeedPolarityContext)
		default:
			return nil, fmt.Errorf("unknown seed polarity %q", polarity)
		}
	}
	return s.ExtractSeeds(ctx, repo, domain.ExtractSeedsOptions{Classes: parsed, Polarities: parsedPolarities, Limit: limit})
}

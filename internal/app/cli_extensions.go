package app

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/investigation"
)

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

func (s *Service) TransitionHypothesisForCLI(ctx context.Context, id, status, rationale string) (any, error) {
	return s.TransitionHypothesis(ctx, id, status, rationale)
}

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

func (s *Service) SetCollisionForCLI(ctx context.Context, id, status, rationale string) (any, error) {
	return s.UpdateOpportunityCollisionStatus(ctx, id, status, rationale)
}

func (s *Service) RecordEvidenceForCLI(ctx context.Context, opts cli.RecordEvidenceOptions) (any, error) {
	return s.RecordEvidence(ctx, RecordEvidenceInput{
		InvestigationID: opts.InvestigationID, HypothesisID: opts.HypothesisID, OpportunityID: opts.OpportunityID,
		Type: opts.Type, Relation: opts.Relation, Description: opts.Description,
	})
}

func (s *Service) WorkspaceDiffForCLI(ctx context.Context, id string) (any, error) {
	return s.WorkspaceDiff(ctx, id)
}

func (s *Service) PrepareReviewForCLI(ctx context.Context, opportunityID, workspaceID string) (any, error) {
	return s.PrepareReviewReport(ctx, PrepareReviewReportInput{OpportunityID: opportunityID, WorkspaceID: workspaceID})
}

func (s *Service) BuildDossierForCLI(ctx context.Context, repo cli.RepoRef) (any, error) {
	return s.BuildRepositoryDossier(ctx, repo)
}

func (s *Service) GetDossierForCLI(ctx context.Context, repo cli.RepoRef) (any, error) {
	return s.GetRepositoryDossier(ctx, repo)
}

func (s *Service) ExtractSeedsForCLI(ctx context.Context, repo cli.RepoRef, classes []string, limit int) (any, error) {
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
	return s.ExtractSeeds(ctx, repo, domain.ExtractSeedsOptions{Classes: parsed, Limit: limit})
}

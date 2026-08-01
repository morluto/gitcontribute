package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// CheckDuplicates finds duplicate-candidate threads for a hypothesis or opportunity.
func (r *MCPReader) CheckDuplicates(ctx context.Context, in mcpcontract.CheckDuplicatesInput) (mcpcontract.CheckOutput, error) {
	return r.checkRelatedWork(ctx, in.Target, in.ID, in.Limit, "duplicate", func() (mcpcontract.CheckOutput, error) {
		var result *contracts.DuplicateCheckResult
		var err error
		switch in.Target {
		case "hypothesis":
			result, err = r.CheckHypothesisDuplicates(ctx, in.ID, in.Limit)
		case "opportunity":
			result, err = r.CheckOpportunityDuplicates(ctx, in.ID, in.Limit)
		default:
			return mcpcontract.CheckOutput{}, fmt.Errorf("unknown target %q", in.Target)
		}
		if err != nil {
			return mcpcontract.CheckOutput{}, err
		}
		return duplicateCheckResultToMCP(in.Target, in.ID, result), nil
	})
}

// CheckCollisions finds open pull request collisions for a hypothesis or opportunity.
func (r *MCPReader) CheckCollisions(ctx context.Context, in mcpcontract.CheckCollisionsInput) (mcpcontract.CheckOutput, error) {
	return r.checkRelatedWork(ctx, in.Target, in.ID, in.Limit, "collision", func() (mcpcontract.CheckOutput, error) {
		var result *contracts.CollisionCheckResult
		var err error
		switch in.Target {
		case "hypothesis":
			result, err = r.CheckHypothesisCollisions(ctx, in.ID, in.Limit)
		case "opportunity":
			result, err = r.CheckOpportunityCollisions(ctx, in.ID, in.Limit)
		default:
			return mcpcontract.CheckOutput{}, fmt.Errorf("unknown target %q", in.Target)
		}
		if err != nil {
			return mcpcontract.CheckOutput{}, err
		}
		return collisionCheckResultToMCP(in.Target, in.ID, result), nil
	})
}

func (r *MCPReader) checkRelatedWork(ctx context.Context, target, id string, limit int, kind string, run func() (mcpcontract.CheckOutput, error)) (mcpcontract.CheckOutput, error) {
	repo, err := r.relatedWorkRepository(ctx, target, id)
	if err != nil {
		return mcpcontract.CheckOutput{}, err
	}
	indexed, err := r.relatedWorkRepositoryIndexed(ctx, repo)
	if err != nil {
		return mcpcontract.CheckOutput{}, err
	}
	message := fmt.Sprintf("The repository is absent from the local corpus, so an empty %s result would not be evidence of absence.", kind)
	if !indexed {
		return unavailableRelatedWorkOutput(target, id, repo, limit, "repository_not_indexed", message, syncRepositoryContextCall(repo.Owner, repo.Repo)), nil
	}
	result, err := run()
	if errors.Is(err, errRepositoryNotFound) {
		return unavailableRelatedWorkOutput(target, id, repo, limit, "repository_not_indexed", message, syncRepositoryContextCall(repo.Owner, repo.Repo)), nil
	}
	return result, err
}

func duplicateCheckResultToMCP(target, id string, result *contracts.DuplicateCheckResult) mcpcontract.CheckOutput {
	truncated := result.Total >= result.Limit
	var recovery *mcpcontract.RecoveryPlan
	if truncated {
		recovery = relatedWorkLimitRecovery(target, id, result.Limit, false)
	}
	status := "complete"
	if truncated {
		status = "partial"
	}
	return mcpcontract.CheckOutput{
		Status: status, Coverage: "complete", Truncated: truncated, Recovery: recovery,
		Target:         target,
		ID:             id,
		Repo:           result.Repo.String(),
		Query:          result.Query,
		Total:          result.Total,
		Findings:       evidenceToMCPItems(result.Findings),
		SourceRevision: result.SourceRevision,
		Limit:          result.Limit,
	}
}

func collisionCheckResultToMCP(target, id string, result *contracts.CollisionCheckResult) mcpcontract.CheckOutput {
	findings := make([]evidence.Evidence, len(result.Findings))
	copy(findings, result.Findings)
	truncated := result.Total >= result.Limit
	var recovery *mcpcontract.RecoveryPlan
	if truncated {
		recovery = relatedWorkLimitRecovery(target, id, result.Limit, true)
	}
	status := "complete"
	if truncated {
		status = "partial"
	}
	return mcpcontract.CheckOutput{
		Status: status, Coverage: "complete", Truncated: truncated, Recovery: recovery,
		Target:         target,
		ID:             id,
		Repo:           result.Repo.String(),
		Query:          result.Query,
		Total:          result.Total,
		Findings:       evidenceToMCPItems(findings),
		SourceRevision: result.SourceRevision,
		Limit:          result.Limit,
	}
}

func relatedWorkLimitRecovery(target, id string, limit int, collision bool) *mcpcontract.RecoveryPlan {
	nextLimit := min(100, max(limit*2, limit+1))
	kind := "duplicates"
	if collision {
		kind = "competing_pull_requests"
	}
	return recoveryPlan("related_work_truncated", "The related-work result reached its bound. Rerun workflow.find_related_work with a larger limit before treating the returned findings as exhaustive.", mcpcontract.RecoveryAction(mcpcontract.FindRelatedWorkInput{Target: target, ID: id, Kinds: []string{kind}, Limit: nextLimit}))
}

func (r *MCPReader) relatedWorkRepository(ctx context.Context, target, id string) (domain.RepoRef, error) {
	invSvc, err := r.readInvestigationSvc(ctx)
	if err != nil {
		return domain.RepoRef{}, err
	}
	var investigationID string
	switch target {
	case "hypothesis":
		hypothesis, err := invSvc.GetHypothesis(ctx, id)
		if err != nil {
			return domain.RepoRef{}, mapInvestigationError(err)
		}
		investigationID = hypothesis.InvestigationID
	case "opportunity":
		opportunity, err := invSvc.GetOpportunity(ctx, id)
		if err != nil {
			return domain.RepoRef{}, mapInvestigationError(err)
		}
		investigationID = opportunity.InvestigationID
	default:
		return domain.RepoRef{}, fmt.Errorf("unknown related-work target %q", target)
	}
	investigation, err := invSvc.GetInvestigation(ctx, investigationID)
	if err != nil {
		return domain.RepoRef{}, mapInvestigationError(err)
	}
	return investigation.Repo, nil
}

func (r *MCPReader) relatedWorkRepositoryIndexed(ctx context.Context, repo domain.RepoRef) (bool, error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return false, err
	}
	stored, err := c.GetRepository(ctx, repo.Owner, repo.Repo)
	if err != nil {
		return false, err
	}
	return stored != nil, nil
}

func unavailableRelatedWorkOutput(target, id string, repo domain.RepoRef, limit int, reason, message string, action mcpcontract.ToolCall) mcpcontract.CheckOutput {
	return mcpcontract.CheckOutput{
		Status: "unavailable", Coverage: "unknown", Target: target, ID: id, Repo: repo.String(), Limit: limit,
		Recovery: recoveryPlan(reason, message, action),
	}
}

func evidenceToMCPItems(items []evidence.Evidence) []mcpcontract.EvidenceItem {
	out := make([]mcpcontract.EvidenceItem, len(items))
	for i, e := range items {
		out[i] = mcpcontract.EvidenceItem{
			ID:          e.ID,
			Type:        string(e.Type),
			Relation:    string(e.Relation),
			Description: e.Description,
			SourceRefs:  sourceRefsToMCP(e.SourceRefs),
			CreatedAt:   formatTime(e.CreatedAt),
		}
	}
	return out
}

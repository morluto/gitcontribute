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
	return r.checkRelatedWorkInput(ctx, in.Target, in.ID, in.Limit, duplicateRelatedWork)
}

// CheckCollisions finds open pull request collisions for a hypothesis or opportunity.
func (r *MCPReader) CheckCollisions(ctx context.Context, in mcpcontract.CheckCollisionsInput) (mcpcontract.CheckOutput, error) {
	return r.checkRelatedWorkInput(ctx, in.Target, in.ID, in.Limit, competingPullRequests)
}

type relatedWorkCheckKind uint8

const (
	duplicateRelatedWork relatedWorkCheckKind = iota + 1
	competingPullRequests
)

func (k relatedWorkCheckKind) resultName() string {
	if k == competingPullRequests {
		return "collision"
	}
	return "duplicate"
}

func (r *MCPReader) checkRelatedWorkInput(ctx context.Context, rawTarget, rawID string, limit int, check relatedWorkCheckKind) (mcpcontract.CheckOutput, error) {
	target, err := parseRelatedWorkSubjectKind(rawTarget)
	if err != nil {
		return mcpcontract.CheckOutput{}, err
	}
	subject, err := r.loadRelatedWorkSubject(ctx, target, rawID)
	if err != nil {
		return mcpcontract.CheckOutput{}, err
	}
	return r.checkRelatedWork(ctx, subject, limit, check)
}

func (r *MCPReader) checkRelatedWork(ctx context.Context, subject relatedWorkSubject, limit int, check relatedWorkCheckKind) (mcpcontract.CheckOutput, error) {
	repo := subject.investigation.Repo
	target := subject.kind.String()
	id := subject.id
	indexed, complete, err := r.relatedWorkRepositoryCoverage(ctx, repo)
	if err != nil {
		return mcpcontract.CheckOutput{}, err
	}
	message := fmt.Sprintf("The repository is absent from the local corpus, so an empty %s result would not be evidence of absence.", check.resultName())
	if !indexed {
		return unavailableRelatedWorkOutput(target, id, repo, limit, "repository_not_indexed", message, syncRepositoryContextCall(repo.Owner(), repo.Repo())), nil
	}
	var result mcpcontract.CheckOutput
	switch check {
	case duplicateRelatedWork:
		value, runErr := r.duplicatesForRelatedWorkSubject(ctx, subject, limit)
		err = runErr
		if err == nil {
			result = duplicateCheckResultToMCP(subject, value)
		}
	case competingPullRequests:
		value, runErr := r.collisionsForRelatedWorkSubject(ctx, subject, limit)
		err = runErr
		if err == nil {
			result = collisionCheckResultToMCP(subject, value)
		}
	default:
		return mcpcontract.CheckOutput{}, errors.New("related-work check was not parsed")
	}
	if errors.Is(err, errRepositoryNotFound) {
		return unavailableRelatedWorkOutput(target, id, repo, limit, "repository_not_indexed", message, syncRepositoryContextCall(repo.Owner(), repo.Repo())), nil
	}
	if err == nil && !complete {
		result.Status, result.Coverage = "partial", "unknown"
		result.Recovery = recoveryPlan("thread_coverage_incomplete", "Stored thread coverage is missing or incomplete. Synchronize repository threads, poll the job, and repeat this check before inferring absence.", mcpcontract.RecoveryAction(mcpcontract.SyncThreadsInput{
			Selection: "repositories", Repositories: []mcpcontract.RepositoryRef{{Owner: repo.Owner(), Repo: repo.Repo()}}, Kind: "both", State: "all",
		}))
	}
	return result, err
}

func duplicateCheckResultToMCP(subject relatedWorkSubject, result *contracts.DuplicateCheckResult) mcpcontract.CheckOutput {
	truncated := result.Truncated
	var recovery *mcpcontract.RecoveryPlan
	if truncated {
		recovery = relatedWorkLimitRecovery(subject, result.Limit, duplicateRelatedWork)
	}
	status := "complete"
	if truncated {
		status = "partial"
	}
	return mcpcontract.CheckOutput{
		Status: status, Coverage: "complete", Truncated: truncated, Recovery: recovery,
		Target:         subject.kind.String(),
		ID:             subject.id,
		Repo:           result.Repo.String(),
		Query:          result.Query,
		Total:          result.Total,
		Findings:       evidenceToMCPItems(result.Findings),
		SourceRevision: result.SourceRevision,
		Limit:          result.Limit,
	}
}

func collisionCheckResultToMCP(subject relatedWorkSubject, result *contracts.CollisionCheckResult) mcpcontract.CheckOutput {
	findings := make([]evidence.Evidence, len(result.Findings))
	copy(findings, result.Findings)
	truncated := result.Truncated
	var recovery *mcpcontract.RecoveryPlan
	if truncated {
		recovery = relatedWorkLimitRecovery(subject, result.Limit, competingPullRequests)
	}
	status := "complete"
	if truncated {
		status = "partial"
	}
	return mcpcontract.CheckOutput{
		Status: status, Coverage: "complete", Truncated: truncated, Recovery: recovery,
		Target:         subject.kind.String(),
		ID:             subject.id,
		Repo:           result.Repo.String(),
		Query:          result.Query,
		Total:          result.Total,
		Findings:       evidenceToMCPItems(findings),
		SourceRevision: result.SourceRevision,
		Limit:          result.Limit,
	}
}

func relatedWorkLimitRecovery(subject relatedWorkSubject, limit int, check relatedWorkCheckKind) *mcpcontract.RecoveryPlan {
	if limit >= maxResultLimit {
		return recoveryPlan("related_work_truncated", "The related-work check reached its maximum bound. Use targeted thread search and inspect exact candidates; this result does not establish absence.")
	}
	nextLimit := min(maxResultLimit, max(limit*2, limit+1))
	message := "The related-work result reached its bound. Rerun the same exact check with a larger limit before treating the findings as exhaustive."
	input := mcpcontract.CheckDuplicatesInput{Target: subject.kind.String(), ID: subject.id, Limit: nextLimit}
	if check == competingPullRequests {
		return recoveryPlan("related_work_truncated", message, mcpcontract.RecoveryAction(mcpcontract.CheckCollisionsInput(input)))
	}
	return recoveryPlan("related_work_truncated", message, mcpcontract.RecoveryAction(input))
}

func (r *MCPReader) relatedWorkRepositoryCoverage(ctx context.Context, repo domain.RepoRef) (indexed, complete bool, err error) {
	c, err := r.openReadOnlyCorpus(ctx)
	if err != nil {
		return false, false, err
	}
	stored, err := c.GetRepository(ctx, repo.Owner(), repo.Repo())
	if err != nil || stored == nil {
		return false, false, err
	}
	coverage, err := c.GetCoverage(ctx, stored.ID, nil, "threads")
	if err != nil {
		return true, false, err
	}
	return true, coverage != nil && coverage.Complete, nil
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

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/concern"
	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// CreateConcern implements the MCP local concern-write capability.
func (r *MCPReader) CreateConcern(ctx context.Context, in mcpcontract.CreateConcernInput) (mcpcontract.ConcernOutput, error) {
	provenance, err := concernSourceProvenance(in.SourceProvenance)
	if err != nil {
		return mcpcontract.ConcernOutput{}, err
	}
	result, err := r.createConcern(ctx, &concern.Concern{
		Repo: domain.RepoRef{Owner: in.Owner, Repo: in.Repo}, CommitSHA: in.CommitSHA, WorkspaceID: in.WorkspaceID,
		Title: in.Title, ProblemStatement: in.ProblemStatement, SuspectedOwner: in.SuspectedOwner,
		Confidence: float64(in.Confidence), Unknowns: in.Unknowns, SuccessCriterion: in.SuccessCriterion,
		Notes: in.Notes, EvidenceIDs: in.EvidenceIDs, SourceProvenance: provenance,
	})
	if err != nil {
		return mcpcontract.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// ListConcerns implements the MCP offline concern-read capability.
func (r *MCPReader) ListConcerns(ctx context.Context, in mcpcontract.ListConcernsInput) (mcpcontract.ConcernListOutput, error) {
	result, err := r.Service.ListConcerns(ctx, contracts.ConcernListOptions{
		Repo: contracts.RepoRef{Owner: in.Owner, Repo: in.Repo}, Status: in.Status, Query: in.Query, Limit: in.Limit,
	})
	if err != nil {
		return mcpcontract.ConcernListOutput{}, err
	}
	out := mcpcontract.ConcernListOutput{
		Concerns:  make([]mcpcontract.ConcernSummaryOutput, len(result.Concerns)),
		Limit:     result.Limit,
		Total:     result.Total,
		Truncated: result.Truncated,
	}
	for index := range result.Concerns {
		value := &result.Concerns[index]
		out.Concerns[index] = mcpcontract.ConcernSummaryOutput{
			ID: value.ID, Owner: value.Repo.Owner, Repo: value.Repo.Repo, Title: value.Title,
			Confidence: mcpcontract.Probability(value.Confidence), Freshness: value.Freshness,
			Status: value.Status, UpdatedAt: value.UpdatedAt,
			URI: "gitcontribute://concern/" + value.ID,
		}
	}
	return out, nil
}

// UpdateConcern implements MCP concern content updates.
func (r *MCPReader) UpdateConcern(ctx context.Context, in mcpcontract.UpdateConcernInput) (mcpcontract.ConcernOutput, error) {
	var confidence *float64
	if in.Confidence != nil {
		value := float64(*in.Confidence)
		confidence = &value
	}
	result, err := r.Service.UpdateConcern(ctx, in.ID, contracts.ConcernUpdateOptions{
		Title: in.Title, ProblemStatement: in.ProblemStatement, SuspectedOwner: in.SuspectedOwner,
		Confidence: confidence, Unknowns: in.Unknowns, SuccessCriterion: in.SuccessCriterion,
		Notes: in.Notes, EvidenceIDs: in.EvidenceIDs,
	})
	if err != nil {
		return mcpcontract.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// SetConcernStatus implements MCP concern lifecycle transitions.
func (r *MCPReader) SetConcernStatus(ctx context.Context, in mcpcontract.SetConcernStatusInput) (mcpcontract.ConcernOutput, error) {
	result, err := r.Service.SetConcernStatus(ctx, in.ID, in.Status, in.Rationale)
	if err != nil {
		return mcpcontract.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// LinkConcern implements MCP concern relationship writes.
func (r *MCPReader) LinkConcern(ctx context.Context, in mcpcontract.LinkConcernInput) (mcpcontract.ConcernOutput, error) {
	result, err := r.Service.LinkConcern(ctx, in.ID, contracts.ConcernLinkOptions{Kind: in.Kind, TargetType: in.TargetType, TargetID: in.TargetID, Note: in.Note})
	if err != nil {
		return mcpcontract.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// PromoteConcern implements atomic MCP concern promotion.
func (r *MCPReader) PromoteConcern(ctx context.Context, in mcpcontract.PromoteConcernInput) (mcpcontract.ConcernOutput, error) {
	result, err := r.Service.PromoteConcern(ctx, in.ID, contracts.ConcernPromoteOptions{
		Kind: in.Kind, Category: in.Category, Scope: in.Scope, Impact: in.Impact, ExpectedEffort: in.ExpectedEffort,
	})
	if err != nil {
		return mcpcontract.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

func concernSourceProvenance(values []mcpcontract.EvidenceSourceRevision) ([]evidence.SourceRevision, error) {
	out := make([]evidence.SourceRevision, len(values))
	for index, value := range values {
		sourceUpdatedAt, err := parseTime(value.SourceUpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("source_provenance[%d].source_updated_at: %w", index, err)
		}
		observedAt, err := time.Parse(time.RFC3339, value.ObservedAt)
		if err != nil {
			return nil, fmt.Errorf("source_provenance[%d].observed_at: %w", index, err)
		}
		out[index] = evidence.SourceRevision{
			Subject: evidence.SourceSubject{
				Kind: evidence.SourceSubjectKind(value.Subject.Kind), Owner: value.Subject.Owner, Repo: value.Subject.Repo,
				ThreadKind: value.Subject.ThreadKind, Number: value.Subject.Number, Facet: value.Subject.Facet,
			}, SourceUpdatedAt: sourceUpdatedAt, ObservationSequence: value.ObservationSequence, ObservedAt: observedAt,
		}
	}
	return out, nil
}

func concernResultToMCP(value *contracts.ConcernResult) mcpcontract.ConcernOutput {
	if value == nil {
		return mcpcontract.ConcernOutput{}
	}
	out := mcpcontract.ConcernOutput{
		ID: value.ID, Owner: value.Repo.Owner, Repo: value.Repo.Repo, CommitSHA: value.CommitSHA, WorkspaceID: value.WorkspaceID,
		Title: value.Title, ProblemStatement: value.ProblemStatement, SuspectedOwner: value.SuspectedOwner,
		Confidence: mcpcontract.Probability(value.Confidence), Unknowns: value.Unknowns, SuccessCriterion: value.SuccessCriterion,
		Notes: value.Notes, EvidenceIDs: value.EvidenceIDs, SourceRefCount: value.SourceRefCount,
		Freshness: value.Freshness, FreshnessReason: value.FreshnessReason, Status: value.Status,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	for _, link := range value.Links {
		out.Links = append(out.Links, mcpcontract.ConcernLinkOutput{Kind: link.Kind, TargetType: link.TargetType, TargetID: link.TargetID, Note: link.Note})
	}
	if value.Promotion != nil {
		out.Promotion = &mcpcontract.ConcernPromotionOutput{
			Kind: value.Promotion.Kind, InvestigationID: value.Promotion.InvestigationID,
			HypothesisID: value.Promotion.HypothesisID, OpportunityID: value.Promotion.OpportunityID,
		}
	}
	return out
}

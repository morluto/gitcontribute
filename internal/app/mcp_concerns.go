package app

import (
	"context"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/concern"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/mcpserver"
)

// CreateConcern implements the MCP local concern-write capability.
func (r *MCPReader) CreateConcern(ctx context.Context, in mcpserver.CreateConcernInput) (mcpserver.ConcernOutput, error) {
	provenance, err := concernSourceProvenance(in.SourceProvenance)
	if err != nil {
		return mcpserver.ConcernOutput{}, err
	}
	result, err := r.createConcern(ctx, &concern.Concern{
		Repo: domain.RepoRef{Owner: in.Owner, Repo: in.Repo}, CommitSHA: in.CommitSHA, WorkspaceID: in.WorkspaceID,
		Title: in.Title, ProblemStatement: in.ProblemStatement, SuspectedOwner: in.SuspectedOwner,
		Confidence: in.Confidence, Unknowns: in.Unknowns, SuccessCriterion: in.SuccessCriterion,
		Notes: in.Notes, EvidenceIDs: in.EvidenceIDs, SourceProvenance: provenance,
	})
	if err != nil {
		return mcpserver.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// ListConcerns implements the MCP offline concern-read capability.
func (r *MCPReader) ListConcerns(ctx context.Context, in mcpserver.ListConcernsInput) (mcpserver.ConcernListOutput, error) {
	result, err := r.Service.ListConcerns(ctx, cli.ConcernListOptions{
		Repo: cli.RepoRef{Owner: in.Owner, Repo: in.Repo}, Status: in.Status, Query: in.Query, Limit: in.Limit,
	})
	if err != nil {
		return mcpserver.ConcernListOutput{}, err
	}
	out := mcpserver.ConcernListOutput{Concerns: make([]mcpserver.ConcernOutput, len(result.Concerns)), Total: result.Total}
	for index := range result.Concerns {
		out.Concerns[index] = concernResultToMCP(&result.Concerns[index])
	}
	return out, nil
}

// UpdateConcern implements MCP concern content updates.
func (r *MCPReader) UpdateConcern(ctx context.Context, in mcpserver.UpdateConcernInput) (mcpserver.ConcernOutput, error) {
	result, err := r.Service.UpdateConcern(ctx, in.ID, cli.ConcernUpdateOptions{
		Title: in.Title, ProblemStatement: in.ProblemStatement, SuspectedOwner: in.SuspectedOwner,
		Confidence: in.Confidence, Unknowns: in.Unknowns, SuccessCriterion: in.SuccessCriterion,
		Notes: in.Notes, EvidenceIDs: in.EvidenceIDs,
	})
	if err != nil {
		return mcpserver.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// SetConcernStatus implements MCP concern lifecycle transitions.
func (r *MCPReader) SetConcernStatus(ctx context.Context, in mcpserver.SetConcernStatusInput) (mcpserver.ConcernOutput, error) {
	result, err := r.Service.SetConcernStatus(ctx, in.ID, in.Status, in.Rationale)
	if err != nil {
		return mcpserver.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// LinkConcern implements MCP concern relationship writes.
func (r *MCPReader) LinkConcern(ctx context.Context, in mcpserver.LinkConcernInput) (mcpserver.ConcernOutput, error) {
	result, err := r.Service.LinkConcern(ctx, in.ID, cli.ConcernLinkOptions{Kind: in.Kind, TargetType: in.TargetType, TargetID: in.TargetID, Note: in.Note})
	if err != nil {
		return mcpserver.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

// PromoteConcern implements atomic MCP concern promotion.
func (r *MCPReader) PromoteConcern(ctx context.Context, in mcpserver.PromoteConcernInput) (mcpserver.ConcernOutput, error) {
	result, err := r.Service.PromoteConcern(ctx, in.ID, cli.ConcernPromoteOptions{
		Kind: in.Kind, Category: in.Category, Scope: in.Scope, Impact: in.Impact, ExpectedEffort: in.ExpectedEffort,
	})
	if err != nil {
		return mcpserver.ConcernOutput{}, err
	}
	return concernResultToMCP(result), nil
}

func concernSourceProvenance(values []mcpserver.EvidenceSourceRevision) ([]evidence.SourceRevision, error) {
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

func concernResultToMCP(value *cli.ConcernResult) mcpserver.ConcernOutput {
	if value == nil {
		return mcpserver.ConcernOutput{}
	}
	out := mcpserver.ConcernOutput{
		ID: value.ID, Owner: value.Repo.Owner, Repo: value.Repo.Repo, CommitSHA: value.CommitSHA, WorkspaceID: value.WorkspaceID,
		Title: value.Title, ProblemStatement: value.ProblemStatement, SuspectedOwner: value.SuspectedOwner,
		Confidence: value.Confidence, Unknowns: value.Unknowns, SuccessCriterion: value.SuccessCriterion,
		Notes: value.Notes, EvidenceIDs: value.EvidenceIDs, SourceRefCount: value.SourceRefCount,
		Freshness: value.Freshness, FreshnessReason: value.FreshnessReason, Status: value.Status,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	for _, link := range value.Links {
		out.Links = append(out.Links, mcpserver.ConcernLinkOutput{Kind: link.Kind, TargetType: link.TargetType, TargetID: link.TargetID, Note: link.Note})
	}
	if value.Promotion != nil {
		out.Promotion = &mcpserver.ConcernPromotionOutput{
			Kind: value.Promotion.Kind, InvestigationID: value.Promotion.InvestigationID,
			HypothesisID: value.Promotion.HypothesisID, OpportunityID: value.Promotion.OpportunityID,
		}
	}
	return out
}

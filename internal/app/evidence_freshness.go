package app

import (
	"context"
	"fmt"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func evidenceItemResult(ctx context.Context, c *corpus.Corpus, item *evidence.Evidence) (contracts.EvidenceItem, error) {
	freshness, err := evidence.NewFreshnessEvaluator(c).Evaluate(ctx, item)
	if err != nil {
		return contracts.EvidenceItem{}, err
	}
	return contracts.EvidenceItem{
		ID: item.ID, Type: string(item.Type), Relation: string(item.Relation),
		Description: item.Description, ValidationRunID: item.ValidationRunID,
		OpportunityID: item.OpportunityID, SourceRefs: workflowSourceRefResults(item.SourceRefs),
		SourceProvenance: evidenceSourceRevisionResults(item.SourceProvenance),
		Freshness:        string(freshness.Status), FreshnessReason: freshness.Reason,
		CreatedAt: formatTime(item.CreatedAt),
	}, nil
}

func sourceRevisionFromThreadBaseline(baseline investigation.ThreadBaseline) (evidence.SourceRevision, error) {
	subject, err := evidence.NewThreadSourceSubject(baseline.Repo, baseline.Kind, baseline.Number)
	if err != nil {
		return evidence.SourceRevision{}, fmt.Errorf("parse thread baseline source: %w", err)
	}
	return evidence.SourceRevision{
		Subject:             subject,
		SourceUpdatedAt:     baseline.SourceUpdatedAt,
		ObservationSequence: baseline.ObservationSequence,
		ObservedAt:          baseline.ObservedAt,
	}, nil
}

func evidenceSourceRevisionResults(values []evidence.SourceRevision) []contracts.EvidenceSourceRevisionResult {
	if len(values) == 0 {
		return nil
	}
	result := make([]contracts.EvidenceSourceRevisionResult, len(values))
	for i, value := range values {
		repository := value.Subject.Repository()
		threadKind, number, _ := value.Subject.Thread()
		result[i] = contracts.EvidenceSourceRevisionResult{
			Subject: contracts.EvidenceSourceSubjectResult{
				Kind: value.Subject.Kind().String(), Owner: repository.Owner(), Repo: repository.Repo(),
				ThreadKind: string(threadKind), Number: number, Facet: value.Subject.Facet(),
			},
			SourceUpdatedAt:     formatTime(value.SourceUpdatedAt),
			ObservationSequence: value.ObservationSequence,
			ObservedAt:          formatTime(value.ObservedAt),
		}
	}
	return result
}

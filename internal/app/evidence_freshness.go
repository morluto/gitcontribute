package app

import (
	"context"

	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/investigation"
)

func evidenceItemResult(ctx context.Context, c *corpus.Corpus, item *evidence.Evidence) (cli.EvidenceItem, error) {
	freshness, err := evidence.NewFreshnessEvaluator(c).Evaluate(ctx, item)
	if err != nil {
		return cli.EvidenceItem{}, err
	}
	return cli.EvidenceItem{
		ID: item.ID, Type: string(item.Type), Relation: string(item.Relation),
		Description: item.Description, ValidationRunID: item.ValidationRunID,
		OpportunityID: item.OpportunityID, SourceRefs: workflowSourceRefResults(item.SourceRefs),
		SourceProvenance: evidenceSourceRevisionResults(item.SourceProvenance),
		Freshness:        string(freshness.Status), FreshnessReason: freshness.Reason,
		CreatedAt: formatTime(item.CreatedAt),
	}, nil
}

func sourceRevisionFromThreadBaseline(baseline investigation.ThreadBaseline) evidence.SourceRevision {
	return evidence.SourceRevision{
		Subject: evidence.SourceSubject{
			Kind: evidence.SourceSubjectThread, Owner: baseline.Repo.Owner, Repo: baseline.Repo.Repo,
			ThreadKind: string(baseline.Kind), Number: baseline.Number,
		},
		SourceUpdatedAt:     baseline.SourceUpdatedAt,
		ObservationSequence: baseline.ObservationSequence,
		ObservedAt:          baseline.ObservedAt,
	}
}

func evidenceSourceRevisionResults(values []evidence.SourceRevision) []cli.EvidenceSourceRevisionResult {
	if len(values) == 0 {
		return nil
	}
	result := make([]cli.EvidenceSourceRevisionResult, len(values))
	for i, value := range values {
		result[i] = cli.EvidenceSourceRevisionResult{
			Subject: cli.EvidenceSourceSubjectResult{
				Kind: string(value.Subject.Kind), Owner: value.Subject.Owner, Repo: value.Subject.Repo,
				ThreadKind: value.Subject.ThreadKind, Number: value.Subject.Number, Facet: value.Subject.Facet,
			},
			SourceUpdatedAt:     formatTime(value.SourceUpdatedAt),
			ObservationSequence: value.ObservationSequence,
			ObservedAt:          formatTime(value.ObservedAt),
		}
	}
	return result
}

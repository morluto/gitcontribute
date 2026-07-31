package app

import (
	"context"
	"errors"

	"github.com/morluto/gitcontribute/internal/failure"
	"github.com/morluto/gitcontribute/internal/research"
)

// ThreadResearchBrief builds a deterministic brief from the local corpus. It
// performs no network access, local mutation, or process execution.
func (s *Service) ThreadResearchBrief(ctx context.Context, ref research.ThreadRef) (*research.Brief, error) {
	c, err := s.openReadOnlyCorpus(ctx)
	if err != nil {
		return nil, err
	}
	revision, err := beginCorpusRead(ctx, c, nil)
	if err != nil {
		return nil, err
	}
	brief, err := research.NewBuilder(&corpusReader{s: s}, s.now).Build(ctx, ref)
	if err == nil {
		if err := finishCorpusRead(ctx, c, revision); err != nil {
			return nil, err
		}
		complete, truncated, unknownCoverage := researchBriefCompleteness(brief)
		provenance, err := offlineReadProvenance("research_brief", revision, ref, complete, truncated, unknownCoverage)
		if err != nil {
			return nil, err
		}
		brief.Provenance = research.ReadProvenance{
			SnapshotToken: provenance.SnapshotToken, Durable: provenance.Durable,
			ObservationWatermark: provenance.ObservationWatermark, QueryDigestSHA256: provenance.QueryDigestSHA256,
			Complete: provenance.Complete, Truncated: provenance.Truncated, UnknownCoverage: provenance.UnknownCoverage,
			Limitations: append([]string(nil), provenance.Limitations...),
		}
		return brief, nil
	}
	if errors.Is(err, errRepositoryNotFound) || errors.Is(err, research.ErrThreadNotFound) || errors.Is(err, research.ErrThreadKindMismatch) {
		return nil, failure.NotFound(err)
	}
	return nil, err
}

func researchBriefCompleteness(brief *research.Brief) (complete, truncated, unknownCoverage bool) {
	statuses := []research.SectionStatus{
		brief.Sections.CurrentState.Status, brief.Sections.Problem.Status, brief.Sections.Acceptance.Status,
		brief.Sections.Participants.Status, brief.Sections.Timeline.Status, brief.Sections.Duplicates.Status,
		brief.Sections.PullRequests.Status, brief.Sections.Code.Status, brief.Sections.Guidance.Status,
		brief.Sections.Health.Status, brief.Sections.Coverage.Status, brief.Sections.Next.Status,
	}
	complete = true
	for _, status := range statuses {
		switch status {
		case research.StatusAvailable:
		case research.StatusPartial:
			complete = false
			truncated = true
		case research.StatusUnknown:
			complete = false
			unknownCoverage = true
		default:
			complete = false
			unknownCoverage = true
		}
	}
	return complete, truncated, unknownCoverage
}

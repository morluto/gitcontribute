package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/research"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

var _ tuicontract.BriefProvider = (*Service)(nil)

// ResearchBrief projects the existing source-backed local research package
// into the terminal contract. It performs no network access or mutation.
func (s *Service) ResearchBrief(ctx context.Context, item tuicontract.Item) (tuicontract.ResearchBrief, error) {
	if item.Kind != "candidate" {
		return tuicontract.ResearchBrief{}, fmt.Errorf("research brief is not available for %s", item.Kind)
	}
	ref, err := research.ParseThreadRef(item.Ref)
	if err != nil {
		return tuicontract.ResearchBrief{}, err
	}
	brief, err := s.ThreadResearchBrief(ctx, ref)
	if err != nil {
		return tuicontract.ResearchBrief{}, err
	}
	out := tuicontract.ResearchBrief{
		Ref:        brief.Target.Ref,
		Title:      brief.Sections.Problem.Title,
		SourceAsOf: formatTime(brief.SourceAsOf),
		Problem:    brief.Sections.Problem.BodyExcerpt,
		ReproductionStatus: tuicontract.BriefFact{
			Summary: "Unknown — the local corpus has no explicit reproduction result.",
		},
	}
	for _, hint := range brief.Sections.Acceptance.Checklist {
		summary := hint.Text
		if hint.Checked {
			summary = "Completed source checkbox: " + summary
		}
		out.ExpectedBehavior = append(out.ExpectedBehavior, tuiBriefFact(summary, hint.Source))
	}
	for _, hint := range brief.Sections.Acceptance.RelevantHeadings {
		out.ExpectedBehavior = append(out.ExpectedBehavior, tuiBriefFact(hint.Text, hint.Source))
	}
	for _, hint := range brief.Sections.Acceptance.MaintainerStatements {
		out.Discussion = append(out.Discussion, tuiBriefFact(hint.Text, hint.Source))
	}
	for _, event := range brief.Sections.Timeline.Events {
		out.Discussion = append(out.Discussion, tuiBriefFact(event.Summary, event.Source))
	}
	for _, related := range brief.Sections.Duplicates.Candidates {
		out.RelatedWork = append(out.RelatedWork, tuicontract.BriefFact{
			Summary: relatedBriefSummary(related.Ref, related.Title, related.Relation),
			Source:  related.URL,
		})
	}
	for _, related := range brief.Sections.PullRequests.PullRequests {
		out.RelatedWork = append(out.RelatedWork, tuicontract.BriefFact{
			Summary: relatedBriefSummary(related.Ref, related.Title, related.Relation),
			Source:  related.URL,
		})
	}
	out.MissingEvidence = append(out.MissingEvidence, brief.Sections.Coverage.Gaps...)
	for _, next := range brief.Sections.Next.Commands {
		out.SuggestedNext = append(out.SuggestedNext, next.Command)
	}
	return out, nil
}

func tuiBriefFact(summary string, source research.SourceRef) tuicontract.BriefFact {
	return tuicontract.BriefFact{Summary: summary, Source: source.URL}
}

func relatedBriefSummary(ref, title, relation string) string {
	summary := strings.TrimSpace(ref)
	if strings.TrimSpace(title) != "" {
		summary += " · " + strings.TrimSpace(title)
	}
	if strings.TrimSpace(relation) != "" {
		summary += " · " + strings.ReplaceAll(relation, "_", " ")
	}
	return summary
}

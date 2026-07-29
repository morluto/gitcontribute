package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/tuicontract"
)

var tuiExpectedRepositoryFacets = []string{"metadata", "threads", FacetContributionGuidance}

func buildTUISyncStatus(
	ctx context.Context,
	c *corpus.Corpus,
	repo corpus.Repository,
	ref domain.RepoRef,
	coverage []corpus.Coverage,
) (tuicontract.Item, error) {
	byFacet := make(map[string]corpus.Coverage, len(coverage))
	for _, fact := range coverage {
		byFacet[fact.Facet] = fact
	}

	names := append([]string(nil), tuiExpectedRepositoryFacets...)
	var extra []string
	for name := range byFacet {
		if !containsTUIFacet(names, name) {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	names = append(names, extra...)

	item := tuicontract.Item{
		Kind:       "sync_status",
		ID:         ref.String(),
		Ref:        ref.String(),
		Title:      ref.String(),
		Source:     "local corpus coverage",
		AsOf:       formatTime(repo.SourceUpdatedAt),
		Commands:   []string{"gitcontribute archive sync " + ref.String()},
		Assessment: &tuicontract.Assessment{},
	}

	status := "complete"
	var lastSuccessful time.Time
	for _, name := range names {
		fact, present := byFacet[name]
		facet := tuicontract.Facet{Name: name, Present: present}
		switch {
		case !present:
			status = "partial"
			item.Assessment.Unknowns = append(item.Assessment.Unknowns, tuicontract.Fact{
				Code: "facet_missing", Summary: name + " has not been recorded in the local corpus.",
			})
		default:
			facet.Complete = fact.Complete
			facet.AsOf = formatTime(fact.SourceUpdatedAt)
			if fact.SourceUpdatedAt.After(repo.SourceUpdatedAt) {
				item.AsOf = formatTime(fact.SourceUpdatedAt)
			}
			if fact.Complete {
				if fact.SourceUpdatedAt.After(lastSuccessful) {
					lastSuccessful = fact.SourceUpdatedAt
				}
				item.Assessment.Positive = append(item.Assessment.Positive, tuicontract.Fact{
					Code: "facet_complete", Summary: name + " is complete.",
				})
			} else {
				status = "partial"
				item.Assessment.Risks = append(item.Assessment.Risks, tuicontract.Fact{
					Code: "facet_partial", Summary: name + " coverage is partial.",
				})
			}
		}
		item.Coverage = append(item.Coverage, facet)
	}
	if lastSuccessful.IsZero() {
		item.Assessment.Unknowns = append(item.Assessment.Unknowns, tuicontract.Fact{
			Code: "last_successful_sync_unknown", Summary: "No successful facet sync is recorded.",
		})
	} else {
		item.Assessment.Positive = append([]tuicontract.Fact{{
			Code: "last_successful_sync", Summary: "Last successful sync: " + formatTime(lastSuccessful) + ".",
		}}, item.Assessment.Positive...)
	}

	metadata, hasMetadata := byFacet["metadata"]
	threads, hasThreads := byFacet["threads"]
	switch {
	case !hasMetadata || !hasThreads:
		item.Detail = "Candidate ranking evidence is incomplete because required repository facets are missing."
	case !metadata.Complete || !threads.Complete:
		item.Detail = "Candidate ranking evidence is incomplete."
	case metadata.SourceUpdatedAt.Before(repo.SourceUpdatedAt) || threads.SourceUpdatedAt.Before(repo.SourceUpdatedAt):
		status = "stale"
		item.Detail = "Candidate rankings are based on stale evidence relative to the latest repository observation."
		item.Assessment.Risks = append(item.Assessment.Risks, tuicontract.Fact{
			Code: "ranking_stale", Summary: "Candidate ranking inputs predate the latest repository observation.",
		})
	default:
		item.Detail = "Candidate ranking evidence is current relative to the latest stored repository observation."
	}

	seenRuns := make(map[int64]struct{})
	for _, fact := range coverage {
		if fact.RunID == nil {
			continue
		}
		if _, seen := seenRuns[*fact.RunID]; seen {
			continue
		}
		seenRuns[*fact.RunID] = struct{}{}
		run, err := c.GetRun(ctx, *fact.RunID)
		if err != nil {
			return tuicontract.Item{}, fmt.Errorf("read sync run %d for %s: %w", *fact.RunID, ref, err)
		}
		if run == nil {
			item.Assessment.Unknowns = append(item.Assessment.Unknowns, tuicontract.Fact{
				Code: "run_missing", Summary: fmt.Sprintf("Run %d referenced by coverage is unavailable.", *fact.RunID),
			})
			continue
		}
		summary := syncRunSummary(*run)
		switch run.Status {
		case corpus.RunStatusFailed, corpus.RunStatusPartial:
			status = "partial"
			item.Assessment.Risks = append(item.Assessment.Risks, tuicontract.Fact{
				Code: "sync_" + run.Status, Summary: summary,
			})
		case corpus.RunStatusRunning:
			if status == "complete" {
				status = "partial"
			}
			item.Assessment.Unknowns = append(item.Assessment.Unknowns, tuicontract.Fact{
				Code: "sync_running", Summary: summary,
			})
		}
	}
	item.Status = status
	return item, nil
}

func containsTUIFacet(facets []string, want string) bool {
	for _, facet := range facets {
		if facet == want {
			return true
		}
	}
	return false
}

func syncRunSummary(run corpus.Run) string {
	summary := fmt.Sprintf("%s run %d is %s", run.Kind, run.ID, run.Status)
	if message := boundedTUIMessage(run.Error, 160); message != "" {
		summary += ": " + message
	}
	return summary + "."
}

func boundedTUIMessage(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit-1]) + "…"
}

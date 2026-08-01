package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// Search performs a local-only corpus search through the MCP interface.
func (r *MCPReader) Search(ctx context.Context, in mcpcontract.SearchInput) (mcpcontract.SearchOutput, error) {
	if in.Query == "" {
		return mcpcontract.SearchOutput{}, errors.New("query is required")
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return mcpcontract.SearchOutput{}, errors.New("limit must be between 1 and 100")
	}
	if in.MatchMode == "" {
		in.MatchMode = "all"
	}
	if in.MatchMode != "all" && in.MatchMode != "any" {
		return mcpcontract.SearchOutput{}, errors.New("match_mode must be all or any")
	}
	if in.View == "" {
		in.View = "compact"
	}
	if in.View != "compact" && in.View != "full" {
		return mcpcontract.SearchOutput{}, errors.New("view must be compact or full")
	}
	var updatedAfter time.Time
	if strings.TrimSpace(in.UpdatedAfter) != "" {
		var err error
		updatedAfter, err = time.Parse(time.RFC3339, in.UpdatedAfter)
		if err != nil {
			return mcpcontract.SearchOutput{}, errors.New("updated_after must be RFC 3339")
		}
	}
	var updatedBefore time.Time
	if strings.TrimSpace(in.UpdatedBefore) != "" {
		var err error
		updatedBefore, err = time.Parse(time.RFC3339, in.UpdatedBefore)
		if err != nil {
			return mcpcontract.SearchOutput{}, errors.New("updated_before must be RFC 3339")
		}
	}
	if !updatedAfter.IsZero() && !updatedBefore.IsZero() && updatedBefore.Before(updatedAfter) {
		return mcpcontract.SearchOutput{}, errors.New("updated_before must not be earlier than updated_after")
	}

	repo := ""
	if (in.Owner == "") != (in.Repo == "") {
		return mcpcontract.SearchOutput{}, errors.New("owner and repo must be provided together")
	}
	if in.Owner != "" && in.Repo != "" {
		repo = in.Owner + "/" + in.Repo
	}
	res, err := r.searchCorpus(ctx, in.Query, contracts.SearchOptions{
		Kind:  in.Kind,
		Repo:  repo,
		State: in.State, StateReason: in.StateReason, Merged: in.Merged, Author: in.Author,
		Association: in.Association, Assignee: in.Assignee, Labels: in.Labels, UpdatedAfter: updatedAfter, UpdatedBefore: updatedBefore,
		Limit:  in.Limit,
		Cursor: in.Cursor,
		Sort:   in.Sort, MatchMode: in.MatchMode,
		SnapshotToken: in.SnapshotToken,
	})
	if err != nil {
		return mcpcontract.SearchOutput{}, err
	}

	matches := make([]mcpcontract.ThreadOutput, len(res.Matches))
	for i, m := range res.Matches {
		updatedAt := ""
		if !m.UpdatedAt.IsZero() {
			updatedAt = m.UpdatedAt.Format(time.RFC3339)
		}
		matches[i] = mcpcontract.ThreadOutput{
			Owner:             m.Repo.Owner,
			Repo:              m.Repo.Repo,
			Kind:              m.Kind,
			Number:            m.Number,
			State:             m.State,
			StateReason:       m.StateReason,
			Title:             m.Title,
			Body:              "",
			Author:            m.Author,
			AuthorAssociation: m.AuthorAssociation,
			Labels:            m.Labels,
			Assignees:         m.Assignees,
			Draft:             m.Draft, ClosedAt: formatTime(m.ClosedAt), MergedAt: formatTime(m.MergedAt), Merged: knownMergePointer(m.Merged, m.MergedKnown),
			UpdatedAt:      updatedAt,
			MatchSource:    m.MatchSource,
			MatchExcerpt:   m.MatchExcerpt,
			MatchTruncated: m.MatchTruncated,
			SnapshotToken:  res.SnapshotToken,
		}
		if in.View == "full" {
			matches[i].Body = m.Body
		}
		if m.MatchSource != "" {
			matches[i].MatchUpdatedAt = formatTime(m.Freshness)
		}
	}
	separator := " AND "
	if in.MatchMode == "any" {
		separator = " OR "
	}
	out := mcpcontract.SearchOutput{
		Query: in.Query, QueryInterpretation: strings.Join(strings.Fields(in.Query), separator),
		MatchMode: in.MatchMode, View: in.View, Total: res.Total, Matches: matches, NextCursor: res.NextCursor,
		UnknownMergeCount: res.UnknownMergeCount,
		SnapshotToken:     res.SnapshotToken,
	}
	provenance, err := offlineReadProvenance("thread_search", res.ObservationWatermark, in, res.NextCursor == "", res.NextCursor != "", true)
	if err != nil {
		return mcpcontract.SearchOutput{}, err
	}
	out.Provenance = provenance
	if provenance.UnknownCoverage {
		out.Recovery = localThreadSearchRecovery(in)
		out.Provenance.Recovery = out.Recovery
	}
	if out.UnknownMergeCount > 0 {
		out.Suggestion = "Some otherwise-matching pull requests have unknown merge state. Repeat without the merged filter to identify finalists, then hydrate pr_details before inferring absence."
	} else if out.Total == 0 && in.MatchMode == "all" && len(strings.Fields(in.Query)) > 1 {
		out.Suggestion = "No all-term matches. Retry with match_mode=any or fewer terms; verify corpus coverage before inferring absence."
	}
	return out, nil
}

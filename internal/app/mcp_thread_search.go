package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/contracts"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

type threadSearchView uint8

const (
	compactThreadSearch threadSearchView = iota
	fullThreadSearch
)

func parseThreadSearchView(value string) (threadSearchView, error) {
	switch strings.TrimSpace(value) {
	case "", "compact":
		return compactThreadSearch, nil
	case "full":
		return fullThreadSearch, nil
	default:
		return 0, errors.New("view must be compact or full")
	}
}

func (v threadSearchView) String() string {
	if v == fullThreadSearch {
		return "full"
	}
	return "compact"
}

// Search performs a local-only corpus search through the MCP interface.
func (r *MCPReader) Search(ctx context.Context, in mcpcontract.SearchInput) (mcpcontract.SearchOutput, error) {
	if strings.TrimSpace(in.Query) == "" {
		return mcpcontract.SearchOutput{}, errors.New("query is required")
	}
	if in.Limit < 0 {
		return mcpcontract.SearchOutput{}, errors.New("limit must be between 1 and 100")
	}
	view, err := parseThreadSearchView(in.View)
	if err != nil {
		return mcpcontract.SearchOutput{}, err
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
	repo, err := optionalRepoRef(in.Owner, in.Repo)
	if err != nil {
		return mcpcontract.SearchOutput{}, err
	}
	request, err := parseSearchRequest(in.Query, contracts.SearchOptions{
		Kind:  in.Kind,
		State: in.State, StateReason: in.StateReason, Merged: in.Merged, Author: in.Author,
		Association: in.Association, Assignee: in.Assignee, Labels: in.Labels, UpdatedAfter: updatedAfter, UpdatedBefore: updatedBefore,
		Limit:  in.Limit,
		Cursor: in.Cursor,
		Sort:   in.Sort, MatchMode: in.MatchMode,
		SnapshotToken: in.SnapshotToken,
	}, repo)
	if err != nil {
		return mcpcontract.SearchOutput{}, err
	}
	threadRequest, ok := request.(threadSearchRequest)
	if !ok {
		return mcpcontract.SearchOutput{}, errors.New("kind must be issue or pull_request")
	}
	in.Query = request.read().query
	in.Kind = threadRequest.criteria.kind.corpusThreadKind().String()
	in.State = threadRequest.criteria.state.String()
	in.StateReason = threadRequest.criteria.stateReason.String()
	in.Author = threadRequest.criteria.author
	in.Association = threadRequest.criteria.association
	in.Assignee = threadRequest.criteria.assignee
	in.Labels = append([]string(nil), threadRequest.criteria.labels...)
	in.Limit = request.read().page.Limit()
	in.Sort = threadRequest.criteria.order.String()
	in.MatchMode = threadRequest.criteria.match.String()
	in.View = view.String()
	if !updatedAfter.IsZero() {
		in.UpdatedAfter = updatedAfter.Format(time.RFC3339)
	}
	if !updatedBefore.IsZero() {
		in.UpdatedBefore = updatedBefore.Format(time.RFC3339)
	}
	res, err := r.searchCorpus(ctx, request)
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
			Owner:             m.Repo.Owner(),
			Repo:              m.Repo.Repo(),
			Kind:              m.Kind.String(),
			Number:            m.Number,
			State:             m.State,
			StateReason:       m.StateReason,
			Title:             m.Title,
			Body:              "",
			Author:            m.Author,
			AuthorAssociation: m.AuthorAssociation,
			Labels:            m.Labels,
			Assignees:         m.Assignees,
			Draft:             m.Draft, ClosedAt: formatTime(m.ClosedAt), MergedAt: formatTime(m.Merge.MergedAt()), Merged: mergeStatusPointer(m.Merge),
			UpdatedAt:      updatedAt,
			MatchSource:    m.MatchSource,
			MatchExcerpt:   m.MatchExcerpt,
			MatchTruncated: m.MatchTruncated,
			SnapshotToken:  res.SnapshotToken,
		}
		if view == fullThreadSearch {
			matches[i].Body = m.Body
		}
		if m.MatchSource != "" {
			matches[i].MatchUpdatedAt = formatTime(m.Freshness)
		}
	}
	separator := " AND "
	if threadRequest.criteria.match.IsAny() {
		separator = " OR "
	}
	out := mcpcontract.SearchOutput{
		Query: in.Query, QueryInterpretation: strings.Join(strings.Fields(in.Query), separator),
		MatchMode: in.MatchMode, View: in.View, Total: res.Total, Matches: matches, NextCursor: res.NextCursor,
		UnknownMergeCount: res.UnknownMergeCount,
		SnapshotToken:     res.SnapshotToken,
	}
	provenance, err := offlineReadProvenance("thread_search", res.ObservationWatermark, in, res.NextCursor != "", true)
	if err != nil {
		return mcpcontract.SearchOutput{}, err
	}
	out.Provenance = provenance
	if provenance.UnknownCoverage() {
		out.Recovery = localThreadSearchRecovery(in)
		out.Provenance.Recovery = out.Recovery
	}
	if out.UnknownMergeCount > 0 {
		out.Suggestion = "Some otherwise-matching pull requests have unknown merge state. Repeat without the merged filter to identify finalists, then hydrate pr_details before inferring absence."
	} else if out.Total == 0 && !threadRequest.criteria.match.IsAny() && len(strings.Fields(in.Query)) > 1 {
		out.Suggestion = "No all-term matches. Retry with match_mode=any or fewer terms; verify corpus coverage before inferring absence."
	}
	return out, nil
}

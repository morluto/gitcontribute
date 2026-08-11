package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/morluto/gitcontribute/internal/domain"
)

type projectedPortfolioSignals struct {
	covered bool
	signals []PortfolioSignal
	refs    []ObservationRef
}

func (c *Corpus) projectedSignals(ctx context.Context, subject PortfolioSubject, facet string) (out projectedPortfolioSignals, err error) {
	var refs string
	err = c.db.QueryRowContext(ctx, `
		SELECT s.source_observation_refs
		FROM portfolio_signal_projections p
		JOIN portfolio_signal_snapshots s ON s.id=p.snapshot_id
		WHERE p.subject_kind=? AND p.subject_ref=? AND p.facet=?
	`, subject.Kind(), subject.Ref(), facet).Scan(&refs)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.covered = true
	if err := json.Unmarshal([]byte(refs), &out.refs); err != nil {
		return out, fmt.Errorf("decode portfolio signal observation refs: %w", err)
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT s.kind, s.value, COALESCE(s.target_kind, ''), COALESCE(s.target_ref, ''), COALESCE(s.score, 0)
		FROM portfolio_signal_projections p
		JOIN portfolio_signals s ON s.snapshot_id=p.snapshot_id
		WHERE p.subject_kind=? AND p.subject_ref=? AND p.facet=?
		ORDER BY s.position
	`, subject.Kind(), subject.Ref(), facet)
	if err != nil {
		return projectedPortfolioSignals{}, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close projected portfolio signal rows: %w", closeErr)
			out = projectedPortfolioSignals{}
		}
	}()
	for rows.Next() {
		var kind, value, targetKind, targetRef string
		var score float64
		if err := rows.Scan(&kind, &value, &targetKind, &targetRef, &score); err != nil {
			return out, err
		}
		signal, err := parsePortfolioSignal(kind, value, targetKind, targetRef, score)
		if err != nil {
			return out, fmt.Errorf("parse stored portfolio signal: %w", err)
		}
		out.signals = append(out.signals, signal)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// ListPullRequestIssueLinks returns a bounded, deterministic offline view of
// authoritative closing-issue relationships for stored pull requests. It
// performs one corpus query and preserves selected-thread ordering.
func (c *Corpus) ListPullRequestIssueLinks(ctx context.Context, repoID int64, state ThreadStateFilter, limit int) (out []PullRequestIssueLinks, capped bool, err error) {
	if repoID <= 0 {
		return nil, false, errors.New("repository id must be positive")
	}
	if limit <= 0 || limit > 10_000 {
		return nil, false, errors.New("pull request issue-link limit must be between 1 and 10000")
	}
	stateFilter := ""
	args := []any{repoID, domain.PullRequestKind}
	if !state.IsAny() {
		stateFilter = " AND state = ?"
		args = append(args, state.String())
	}
	args = append(args, limit+1, PortfolioSubjectPullRequest, PortfolioFacetLinkedIssues)
	rows, err := c.db.QueryContext(ctx, `
		WITH selected AS (
			SELECT id, number, source_updated_at
			FROM threads
			WHERE repository_id = ? AND kind = ?`+stateFilter+`
			ORDER BY source_updated_at DESC, number DESC
			LIMIT ?
		)
		SELECT selected.id, selected.number, projection.snapshot_id,
		       snapshot.source_updated_at, snapshot.source_observation_refs,
		       signal.value
		FROM selected
		LEFT JOIN portfolio_signal_projections projection
		  ON projection.subject_kind = ?
		 AND projection.subject_ref = CAST(selected.id AS TEXT)
		 AND projection.facet = ?
		LEFT JOIN portfolio_signal_snapshots snapshot ON snapshot.id = projection.snapshot_id
		LEFT JOIN portfolio_signals signal ON signal.snapshot_id = projection.snapshot_id
		ORDER BY selected.source_updated_at DESC, selected.number DESC, signal.position
	`, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list pull request issue links: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close pull request issue links: %w", closeErr)
			out = nil
		}
	}()
	indexes := map[int64]int{}
	for rows.Next() {
		var threadID int64
		var number int
		var snapshotID, sourceUpdated sql.NullInt64
		var encodedRefs, value sql.NullString
		if err := rows.Scan(&threadID, &number, &snapshotID, &sourceUpdated, &encodedRefs, &value); err != nil {
			return nil, false, err
		}
		index, ok := indexes[threadID]
		if !ok {
			index = len(out)
			indexes[threadID] = index
			out = append(out, PullRequestIssueLinks{ThreadID: threadID, Number: number, Covered: snapshotID.Valid})
			if snapshotID.Valid {
				out[index].SourceUpdatedAt = scanTime(sourceUpdated.Int64)
				if err := json.Unmarshal([]byte(encodedRefs.String), &out[index].SourceObservationRefs); err != nil {
					return nil, false, fmt.Errorf("decode pull request issue-link observation refs: %w", err)
				}
			}
		}
		if value.Valid {
			out[index].LinkedIssues = append(out[index].LinkedIssues, value.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate pull request issue links: %w", err)
	}
	if len(out) > limit {
		out = out[:limit]
		capped = true
	}
	return out, capped, nil
}

// FindPortfolioOverlaps compares candidates with exact authored PR corpus IDs.
// It is an offline read and preserves candidate input order.
func (c *Corpus) FindPortfolioOverlaps(ctx context.Context, candidates []PortfolioSubject, pullRequestThreadIDs []int64) ([]PortfolioOverlapResult, error) {
	if len(candidates) == 0 || len(candidates) > 50 {
		return nil, errors.New("candidates must contain 1 to 50 items")
	}
	if len(pullRequestThreadIDs) == 0 || len(pullRequestThreadIDs) > 100 {
		return nil, errors.New("pull request ids must contain 1 to 100 items")
	}
	prs := append([]int64(nil), pullRequestThreadIDs...)
	sort.Slice(prs, func(i, j int) bool { return prs[i] < prs[j] })
	results := make([]PortfolioOverlapResult, len(candidates))
	for i, candidate := range candidates {
		result, err := c.findCandidateOverlaps(ctx, candidate, prs)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

func (c *Corpus) findCandidateOverlaps(ctx context.Context, candidate PortfolioSubject, prs []int64) (PortfolioOverlapResult, error) {
	if err := validatePortfolioSubject(candidate); err != nil {
		return PortfolioOverlapResult{}, err
	}
	result := PortfolioOverlapResult{Candidate: candidate, status: portfolioOverlapUnknown, coverage: make(map[string]bool)}
	candidateFacets, allCovered, err := c.loadCandidateFacets(ctx, candidate, result.coverage)
	if err != nil {
		return PortfolioOverlapResult{}, err
	}
	for _, prID := range prs {
		covered, err := c.comparePortfolioPullRequest(ctx, candidate, candidateFacets, prID, &result)
		if err != nil {
			return PortfolioOverlapResult{}, err
		}
		allCovered = allCovered && covered
	}
	if len(result.Matches) > 0 {
		result.status = portfolioOverlapFound
	} else if allCovered {
		result.status = portfolioNoOverlap
	}
	return result, nil
}

func (c *Corpus) loadCandidateFacets(ctx context.Context, candidate PortfolioSubject, coverage map[string]bool) (map[string]projectedPortfolioSignals, bool, error) {
	facets := make(map[string]projectedPortfolioSignals)
	allCovered := true
	for _, facet := range requiredPortfolioFacets(candidate) {
		projected, err := c.projectedSignals(ctx, candidate, facet)
		if err != nil {
			return nil, false, err
		}
		facets[facet] = projected
		coverage["candidate."+facet] = projected.covered
		allCovered = allCovered && projected.covered
	}
	return facets, allCovered, nil
}

func (c *Corpus) comparePortfolioPullRequest(ctx context.Context, candidate PortfolioSubject, candidateFacets map[string]projectedPortfolioSignals, prID int64, result *PortfolioOverlapResult) (bool, error) {
	pr, err := NewPullRequestPortfolioSubject(prID)
	if err != nil {
		return false, err
	}
	evidence, err := c.explicitPortfolioEvidence(ctx, candidate, prID)
	if err != nil {
		return false, err
	}
	allCovered := true
	for _, facet := range []string{PortfolioFacetChangedFiles, PortfolioFacetLinkedIssues} {
		projected, err := c.projectedSignals(ctx, pr, facet)
		if err != nil {
			return false, err
		}
		result.coverage["pull_request."+pr.Ref()+"."+facet] = projected.covered
		allCovered = allCovered && projected.covered
		evidence = append(evidence, overlapEvidence(candidate, pr, candidateFacets[facet], projected)...)
	}
	evidence = append(evidence, overlapEvidence(candidate, pr, candidateFacets[PortfolioFacetOpportunitySimilarity], projectedPortfolioSignals{})...)
	if len(evidence) == 0 {
		return allCovered, nil
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].Kind != evidence[j].Kind {
			return evidence[i].Kind < evidence[j].Kind
		}
		return evidence[i].Value < evidence[j].Value
	})
	result.Matches = append(result.Matches, PortfolioOverlapMatch{PullRequestThreadID: prID, Evidence: evidence})
	return allCovered, nil
}

func requiredPortfolioFacets(subject PortfolioSubject) []string {
	if subject.Kind() == PortfolioSubjectPullRequest {
		return []string{PortfolioFacetChangedFiles, PortfolioFacetLinkedIssues}
	}
	return portfolioFacets
}

func (c *Corpus) explicitPortfolioLink(ctx context.Context, candidate PortfolioSubject, pullRequestThreadID int64) (*PortfolioOverlapEvidence, error) {
	column := ""
	switch candidate.Kind() {
	case PortfolioSubjectOpportunity:
		column = "opportunity_id"
	case PortfolioSubjectWorkspace:
		column = "workspace_id"
	default:
		return nil, errPortfolioLinkNotApplicable
	}
	var linkID int64
	err := c.db.QueryRowContext(ctx, `SELECT id FROM portfolio_links WHERE pull_request_thread_id=? AND `+column+`=? ORDER BY id LIMIT 1`, pullRequestThreadID, candidate.Ref()).Scan(&linkID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errPortfolioLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read explicit portfolio link: %w", err)
	}
	ref, err := newPortfolioLinkObservationRef(linkID)
	if err != nil {
		return nil, err
	}
	return &PortfolioOverlapEvidence{Kind: "explicit_link", Value: candidate.Ref() + "->" + strconv.FormatInt(pullRequestThreadID, 10), SourceObservationRefs: []ObservationRef{ref}}, nil
}

func (c *Corpus) explicitPortfolioEvidence(ctx context.Context, candidate PortfolioSubject, pullRequestThreadID int64) ([]PortfolioOverlapEvidence, error) {
	evidence, err := c.explicitPortfolioLink(ctx, candidate, pullRequestThreadID)
	if errors.Is(err, errPortfolioLinkNotApplicable) || errors.Is(err, errPortfolioLinkNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []PortfolioOverlapEvidence{*evidence}, nil
}

func overlapEvidence(candidate, pr PortfolioSubject, candidateSignals, prSignals projectedPortfolioSignals) []PortfolioOverlapEvidence {
	var out []PortfolioOverlapEvidence
	values := make(map[string]struct{}, len(prSignals.signals))
	for _, signal := range prSignals.signals {
		values[signal.Kind()+"\x00"+signal.Value()] = struct{}{}
	}
	for _, signal := range candidateSignals.signals {
		switch signal.Kind() {
		case PortfolioSignalFilePath, PortfolioSignalLinkedIssue:
			if _, ok := values[signal.Kind()+"\x00"+signal.Value()]; ok {
				out = append(out, PortfolioOverlapEvidence{Kind: signal.Kind(), Value: signal.Value(), SourceObservationRefs: mergeObservationRefs(candidateSignals.refs, prSignals.refs)})
			}
		case PortfolioSignalOpportunitySimilarity:
			target, _ := signal.Target()
			if target == pr {
				out = append(out, PortfolioOverlapEvidence{Kind: signal.Kind(), Value: candidate.Ref() + "->" + pr.Ref(), Score: signal.Score(), SourceObservationRefs: candidateSignals.refs})
			}
		}
	}
	return out
}

func mergeObservationRefs(first, second []ObservationRef) []ObservationRef {
	seen := make(map[ObservationRef]struct{}, len(first)+len(second))
	out := make([]ObservationRef, 0, len(first)+len(second))
	for _, refs := range [][]ObservationRef{first, second} {
		for _, ref := range refs {
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			out = append(out, ref)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind() != out[j].Kind() {
			return out[i].Kind() < out[j].Kind()
		}
		return out[i].ID() < out[j].ID()
	})
	return out
}

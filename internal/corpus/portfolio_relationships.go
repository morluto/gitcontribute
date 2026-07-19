package corpus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// PortfolioSubjectPullRequest identifies a corpus pull-request thread.
	PortfolioSubjectPullRequest = "pull_request"
	// PortfolioSubjectOpportunity identifies a local contribution opportunity.
	PortfolioSubjectOpportunity = "opportunity"
	// PortfolioSubjectWorkspace identifies a local contribution workspace.
	PortfolioSubjectWorkspace = "workspace"

	// PortfolioFacetChangedFiles contains normalized changed paths.
	PortfolioFacetChangedFiles = "changed_files"
	// PortfolioFacetLinkedIssues contains normalized issue references.
	PortfolioFacetLinkedIssues = "linked_issues"
	// PortfolioFacetOpportunitySimilarity contains scored PR relationships.
	PortfolioFacetOpportunitySimilarity = "opportunity_similarity"

	// PortfolioSignalFilePath is a normalized changed-path signal.
	PortfolioSignalFilePath = "file_path"
	// PortfolioSignalLinkedIssue is a normalized linked-issue signal.
	PortfolioSignalLinkedIssue = "linked_issue"
	// PortfolioSignalOpportunitySimilarity is a scored subject relationship.
	PortfolioSignalOpportunitySimilarity = "opportunity_similarity"
)

var (
	errPortfolioLinkNotApplicable = errors.New("portfolio link is not applicable to this subject")
	errPortfolioLinkNotFound      = errors.New("portfolio link not found")
)

var portfolioFacets = []string{
	PortfolioFacetChangedFiles,
	PortfolioFacetLinkedIssues,
	PortfolioFacetOpportunitySimilarity,
}

// ObservationRef identifies one immutable corpus observation used to derive a
// local portfolio or resolution fact. Kind is product-owned (for example,
// thread or facet) and ID is the corresponding corpus observation identity.
type ObservationRef struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

// PortfolioSubject is a stable local identity. Pull-request references are
// decimal corpus thread IDs; opportunity and workspace references are IDs.
type PortfolioSubject struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// PortfolioLink explicitly associates an authored PR with local workflow
// state. OpportunityID or WorkspaceID, and possibly both, must be present.
type PortfolioLink struct {
	ID                  int64     `json:"id"`
	PullRequestThreadID int64     `json:"pull_request_thread_id"`
	OpportunityID       string    `json:"opportunity_id,omitempty"`
	WorkspaceID         string    `json:"workspace_id,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// PortfolioSignal is one normalized overlap input. Similarity signals name a
// target subject and carry a score; path and linked-issue signals use Value.
type PortfolioSignal struct {
	Kind       string  `json:"kind"`
	Value      string  `json:"value"`
	TargetKind string  `json:"target_kind,omitempty"`
	TargetRef  string  `json:"target_ref,omitempty"`
	Score      float64 `json:"score,omitempty"`
}

// PortfolioSignalSnapshot is one complete, immutable facet replacement.
type PortfolioSignalSnapshot struct {
	ID                    int64             `json:"id"`
	Subject               PortfolioSubject  `json:"subject"`
	Facet                 string            `json:"facet"`
	Signals               []PortfolioSignal `json:"signals"`
	SourceUpdatedAt       time.Time         `json:"source_updated_at"`
	ObservationSequence   int64             `json:"observation_sequence"`
	SourceObservationRefs []ObservationRef  `json:"source_observation_refs"`
	ObservedAt            time.Time         `json:"observed_at"`
}

// PortfolioOverlapEvidence is an exact observed reason for an overlap.
type PortfolioOverlapEvidence struct {
	Kind                  string           `json:"kind"`
	Value                 string           `json:"value"`
	Score                 float64          `json:"score,omitempty"`
	SourceObservationRefs []ObservationRef `json:"source_observation_refs"`
}

// PortfolioOverlapMatch associates one candidate with an authored PR.
type PortfolioOverlapMatch struct {
	PullRequestThreadID int64                      `json:"pull_request_thread_id"`
	Evidence            []PortfolioOverlapEvidence `json:"evidence"`
}

// PortfolioOverlapResult preserves candidate input order. Status is overlap,
// no_overlap, or unknown. A no_overlap result requires complete coverage of
// every overlap facet for both the candidate and every compared PR.
type PortfolioOverlapResult struct {
	Candidate PortfolioSubject        `json:"candidate"`
	Status    string                  `json:"status"`
	Coverage  map[string]string       `json:"coverage"`
	Matches   []PortfolioOverlapMatch `json:"matches"`
}

// SavePortfolioLink idempotently records an explicit local workflow link.
func (c *Corpus) SavePortfolioLink(ctx context.Context, link PortfolioLink) (*PortfolioLink, error) {
	if link.PullRequestThreadID <= 0 || (strings.TrimSpace(link.OpportunityID) == "" && strings.TrimSpace(link.WorkspaceID) == "") {
		return nil, errors.New("pull request thread and an opportunity or workspace are required")
	}
	var kind string
	if err := c.db.QueryRowContext(ctx, `SELECT kind FROM threads WHERE id=?`, link.PullRequestThreadID).Scan(&kind); err != nil {
		return nil, fmt.Errorf("resolve portfolio pull request: %w", err)
	}
	if kind != ThreadKindPullRequest {
		return nil, errors.New("portfolio link thread is not a pull request")
	}
	if link.CreatedAt.IsZero() {
		link.CreatedAt = time.Now().UTC()
	}
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO portfolio_links (pull_request_thread_id, opportunity_id, workspace_id, created_at)
		VALUES (?, NULLIF(?, ''), NULLIF(?, ''), ?)
		ON CONFLICT DO NOTHING
	`, link.PullRequestThreadID, strings.TrimSpace(link.OpportunityID), strings.TrimSpace(link.WorkspaceID), encodeTime(link.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("save portfolio link: %w", err)
	}
	var createdAt int64
	err = c.db.QueryRowContext(ctx, `
		SELECT id, created_at FROM portfolio_links
		WHERE pull_request_thread_id=? AND COALESCE(opportunity_id, '')=? AND COALESCE(workspace_id, '')=?
	`, link.PullRequestThreadID, strings.TrimSpace(link.OpportunityID), strings.TrimSpace(link.WorkspaceID)).Scan(&link.ID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("read portfolio link: %w", err)
	}
	link.CreatedAt = scanTime(createdAt)
	return &link, nil
}

// ListPortfolioLinks returns explicit links in stable PR/opportunity/workspace order.
func (c *Corpus) ListPortfolioLinks(ctx context.Context) (out []PortfolioLink, err error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, pull_request_thread_id, COALESCE(opportunity_id, ''), COALESCE(workspace_id, ''), created_at
		FROM portfolio_links ORDER BY pull_request_thread_id, opportunity_id, workspace_id, id LIMIT 10000
	`)
	if err != nil {
		return nil, fmt.Errorf("list portfolio links: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close portfolio link rows: %w", closeErr)
		}
	}()
	for rows.Next() {
		var link PortfolioLink
		var created int64
		if err := rows.Scan(&link.ID, &link.PullRequestThreadID, &link.OpportunityID, &link.WorkspaceID, &created); err != nil {
			return nil, err
		}
		link.CreatedAt = scanTime(created)
		out = append(out, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate portfolio links: %w", err)
	}
	return out, nil
}

// ReplacePortfolioSignals stores a complete child snapshot and atomically
// advances its projection only if its source clock is newer.
func (c *Corpus) ReplacePortfolioSignals(ctx context.Context, snapshot PortfolioSignalSnapshot) (saved *PortfolioSignalSnapshot, err error) {
	if err := validatePortfolioSnapshot(snapshot); err != nil {
		return nil, err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin portfolio signal replacement: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("rollback portfolio signal replacement: %w", rollbackErr)
			saved = nil
		}
	}()
	if snapshot.ObservationSequence == 0 {
		snapshot.ObservationSequence, err = c.nextSequence(ctx, tx)
		if err != nil {
			return nil, err
		}
	}
	if snapshot.ObservedAt.IsZero() {
		snapshot.ObservedAt = time.Now().UTC()
	}
	refs, err := json.Marshal(snapshot.SourceObservationRefs)
	if err != nil {
		return nil, fmt.Errorf("encode portfolio observation refs: %w", err)
	}
	if err := validateObservationRefsTx(ctx, tx, snapshot.SourceObservationRefs); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO portfolio_signal_snapshots
			(subject_kind, subject_ref, facet, source_updated_at, observation_sequence, source_observation_refs, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, snapshot.Subject.Kind, snapshot.Subject.Ref, snapshot.Facet, encodeTime(snapshot.SourceUpdatedAt), snapshot.ObservationSequence, string(refs), encodeTime(snapshot.ObservedAt))
	if err != nil {
		return nil, fmt.Errorf("insert portfolio signal snapshot: %w", err)
	}
	snapshot.ID, err = result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("read portfolio signal snapshot id: %w", err)
	}
	for position, signal := range canonicalPortfolioSignals(snapshot.Signals) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO portfolio_signals (snapshot_id, position, kind, value, target_kind, target_ref, score)
			VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)
		`, snapshot.ID, position, signal.Kind, signal.Value, signal.TargetKind, signal.TargetRef, signal.Score); err != nil {
			return nil, fmt.Errorf("insert portfolio signal: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO portfolio_signal_projections
			(subject_kind, subject_ref, facet, snapshot_id, source_updated_at, observation_sequence)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (subject_kind, subject_ref, facet) DO UPDATE SET
			snapshot_id=excluded.snapshot_id,
			source_updated_at=excluded.source_updated_at,
			observation_sequence=excluded.observation_sequence
		WHERE portfolio_signal_projections.source_updated_at < excluded.source_updated_at
		   OR (portfolio_signal_projections.source_updated_at = excluded.source_updated_at
		       AND portfolio_signal_projections.observation_sequence < excluded.observation_sequence)
	`, snapshot.Subject.Kind, snapshot.Subject.Ref, snapshot.Facet, snapshot.ID, encodeTime(snapshot.SourceUpdatedAt), snapshot.ObservationSequence); err != nil {
		return nil, fmt.Errorf("advance portfolio signal projection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit portfolio signal replacement: %w", err)
	}
	return &snapshot, nil
}

func validateObservationRefsTx(ctx context.Context, tx *sql.Tx, refs []ObservationRef) error {
	for _, ref := range refs {
		var exists int
		var err error
		switch ref.Kind {
		case "thread":
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM thread_observations WHERE id=?`, ref.ID).Scan(&exists)
		case "facet":
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM facet_observations WHERE id=?`, ref.ID).Scan(&exists)
		default:
			return fmt.Errorf("unsupported source observation kind %q", ref.Kind)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("source observation %s:%d does not exist", ref.Kind, ref.ID)
		}
		if err != nil {
			return fmt.Errorf("validate source observation %s:%d: %w", ref.Kind, ref.ID, err)
		}
	}
	return nil
}

func validatePortfolioSnapshot(snapshot PortfolioSignalSnapshot) error {
	if err := validatePortfolioSubject(snapshot.Subject); err != nil {
		return err
	}
	wantKind := map[string]string{
		PortfolioFacetChangedFiles:          PortfolioSignalFilePath,
		PortfolioFacetLinkedIssues:          PortfolioSignalLinkedIssue,
		PortfolioFacetOpportunitySimilarity: PortfolioSignalOpportunitySimilarity,
	}[snapshot.Facet]
	if wantKind == "" {
		return errors.New("unknown portfolio signal facet")
	}
	if snapshot.SourceUpdatedAt.IsZero() || len(snapshot.SourceObservationRefs) == 0 {
		return errors.New("portfolio signal source time and observation refs are required")
	}
	for _, ref := range snapshot.SourceObservationRefs {
		if strings.TrimSpace(ref.Kind) == "" || ref.ID <= 0 {
			return errors.New("invalid portfolio source observation reference")
		}
	}
	for _, signal := range snapshot.Signals {
		if signal.Kind != wantKind {
			return fmt.Errorf("signal kind %q does not belong to facet %q", signal.Kind, snapshot.Facet)
		}
		if strings.TrimSpace(signal.Value) == "" && signal.Kind != PortfolioSignalOpportunitySimilarity {
			return errors.New("portfolio signal value is required")
		}
		if signal.Kind == PortfolioSignalOpportunitySimilarity && (signal.TargetKind != PortfolioSubjectPullRequest || signal.TargetRef == "" || signal.Score < 0 || signal.Score > 1) {
			return errors.New("opportunity similarity requires a pull request target and score between zero and one")
		}
	}
	return nil
}

func validatePortfolioSubject(subject PortfolioSubject) error {
	if strings.TrimSpace(subject.Ref) == "" {
		return errors.New("portfolio subject reference is required")
	}
	switch subject.Kind {
	case PortfolioSubjectPullRequest:
		id, err := strconv.ParseInt(subject.Ref, 10, 64)
		if err != nil || id <= 0 {
			return errors.New("pull request subject reference must be a positive corpus thread id")
		}
	case PortfolioSubjectOpportunity, PortfolioSubjectWorkspace:
	default:
		return errors.New("unknown portfolio subject kind")
	}
	return nil
}

func canonicalPortfolioSignals(signals []PortfolioSignal) []PortfolioSignal {
	out := append([]PortfolioSignal(nil), signals...)
	for i := range out {
		out[i].Value = strings.TrimSpace(out[i].Value)
		if out[i].Kind == PortfolioSignalFilePath {
			out[i].Value = path.Clean(strings.ReplaceAll(out[i].Value, `\`, "/"))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		if a.TargetKind != b.TargetKind {
			return a.TargetKind < b.TargetKind
		}
		if a.TargetRef != b.TargetRef {
			return a.TargetRef < b.TargetRef
		}
		return a.Score < b.Score
	})
	return out
}

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
	`, subject.Kind, subject.Ref, facet).Scan(&refs)
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
	`, subject.Kind, subject.Ref, facet)
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
		var signal PortfolioSignal
		if err := rows.Scan(&signal.Kind, &signal.Value, &signal.TargetKind, &signal.TargetRef, &signal.Score); err != nil {
			return out, err
		}
		out.signals = append(out.signals, signal)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
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
	result := PortfolioOverlapResult{Candidate: candidate, Status: "unknown", Coverage: make(map[string]string)}
	candidateFacets, allCovered, err := c.loadCandidateFacets(ctx, candidate, result.Coverage)
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
		result.Status = "overlap"
	} else if allCovered {
		result.Status = "no_overlap"
	}
	return result, nil
}

func (c *Corpus) loadCandidateFacets(ctx context.Context, candidate PortfolioSubject, coverage map[string]string) (map[string]projectedPortfolioSignals, bool, error) {
	facets := make(map[string]projectedPortfolioSignals)
	allCovered := true
	for _, facet := range requiredPortfolioFacets(candidate) {
		projected, err := c.projectedSignals(ctx, candidate, facet)
		if err != nil {
			return nil, false, err
		}
		facets[facet] = projected
		coverage["candidate."+facet] = coverageStatus(projected.covered)
		allCovered = allCovered && projected.covered
	}
	return facets, allCovered, nil
}

func (c *Corpus) comparePortfolioPullRequest(ctx context.Context, candidate PortfolioSubject, candidateFacets map[string]projectedPortfolioSignals, prID int64, result *PortfolioOverlapResult) (bool, error) {
	pr := PortfolioSubject{Kind: PortfolioSubjectPullRequest, Ref: strconv.FormatInt(prID, 10)}
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
		result.Coverage["pull_request."+pr.Ref+"."+facet] = coverageStatus(projected.covered)
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

func coverageStatus(covered bool) string {
	if covered {
		return "complete"
	}
	return "missing"
}

func requiredPortfolioFacets(subject PortfolioSubject) []string {
	if subject.Kind == PortfolioSubjectPullRequest {
		return []string{PortfolioFacetChangedFiles, PortfolioFacetLinkedIssues}
	}
	return portfolioFacets
}

func (c *Corpus) explicitPortfolioLink(ctx context.Context, candidate PortfolioSubject, pullRequestThreadID int64) (*PortfolioOverlapEvidence, error) {
	column := ""
	switch candidate.Kind {
	case PortfolioSubjectOpportunity:
		column = "opportunity_id"
	case PortfolioSubjectWorkspace:
		column = "workspace_id"
	default:
		return nil, errPortfolioLinkNotApplicable
	}
	var linkID int64
	err := c.db.QueryRowContext(ctx, `SELECT id FROM portfolio_links WHERE pull_request_thread_id=? AND `+column+`=? ORDER BY id LIMIT 1`, pullRequestThreadID, candidate.Ref).Scan(&linkID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errPortfolioLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read explicit portfolio link: %w", err)
	}
	return &PortfolioOverlapEvidence{Kind: "explicit_link", Value: candidate.Ref + "->" + strconv.FormatInt(pullRequestThreadID, 10), SourceObservationRefs: []ObservationRef{{Kind: "portfolio_link", ID: linkID}}}, nil
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
		values[signal.Kind+"\x00"+signal.Value] = struct{}{}
	}
	for _, signal := range candidateSignals.signals {
		switch signal.Kind {
		case PortfolioSignalFilePath, PortfolioSignalLinkedIssue:
			if _, ok := values[signal.Kind+"\x00"+signal.Value]; ok {
				out = append(out, PortfolioOverlapEvidence{Kind: signal.Kind, Value: signal.Value, SourceObservationRefs: mergeObservationRefs(candidateSignals.refs, prSignals.refs)})
			}
		case PortfolioSignalOpportunitySimilarity:
			if signal.TargetKind == pr.Kind && signal.TargetRef == pr.Ref {
				out = append(out, PortfolioOverlapEvidence{Kind: signal.Kind, Value: candidate.Ref + "->" + pr.Ref, Score: signal.Score, SourceObservationRefs: candidateSignals.refs})
			}
		}
	}
	for _, signal := range prSignals.signals {
		if signal.Kind == PortfolioSignalOpportunitySimilarity && signal.TargetKind == candidate.Kind && signal.TargetRef == candidate.Ref {
			out = append(out, PortfolioOverlapEvidence{Kind: signal.Kind, Value: pr.Ref + "->" + candidate.Ref, Score: signal.Score, SourceObservationRefs: prSignals.refs})
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
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

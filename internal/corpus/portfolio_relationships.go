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

	"github.com/morluto/gitcontribute/internal/domain"
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
	kind observationRefKind
	id   int64
}

type observationRefKind uint8

const (
	threadObservationRef observationRefKind = iota + 1
	facetObservationRef
	portfolioLinkObservationRef
)

func ParseObservationRef(kind string, id int64) (ObservationRef, error) {
	if id <= 0 {
		return ObservationRef{}, errors.New("observation reference id must be positive")
	}
	switch strings.TrimSpace(kind) {
	case "thread":
		return ObservationRef{kind: threadObservationRef, id: id}, nil
	case "facet":
		return ObservationRef{kind: facetObservationRef, id: id}, nil
	case "portfolio_link":
		return ObservationRef{kind: portfolioLinkObservationRef, id: id}, nil
	default:
		return ObservationRef{}, errors.New("unknown observation reference kind")
	}
}

func NewThreadObservationRef(id int64) (ObservationRef, error) {
	return ParseObservationRef("thread", id)
}

func NewFacetObservationRef(id int64) (ObservationRef, error) {
	return ParseObservationRef("facet", id)
}

func newPortfolioLinkObservationRef(id int64) (ObservationRef, error) {
	return ParseObservationRef("portfolio_link", id)
}

func (r ObservationRef) Kind() string {
	switch r.kind {
	case threadObservationRef:
		return "thread"
	case facetObservationRef:
		return "facet"
	case portfolioLinkObservationRef:
		return "portfolio_link"
	default:
		return ""
	}
}

func (r ObservationRef) ID() int64 { return r.id }

func (r ObservationRef) valid() bool { return r.Kind() != "" && r.id > 0 }

func (r ObservationRef) MarshalJSON() ([]byte, error) {
	if !r.valid() {
		return nil, errors.New("invalid observation reference")
	}
	return json.Marshal(struct {
		Kind string `json:"kind"`
		ID   int64  `json:"id"`
	}{Kind: r.Kind(), ID: r.ID()})
}

func (r *ObservationRef) UnmarshalJSON(data []byte) error {
	var input struct {
		Kind string `json:"kind"`
		ID   int64  `json:"id"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return err
	}
	parsed, err := ParseObservationRef(input.Kind, input.ID)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// PortfolioSubject is a stable local identity. Pull-request references are
// canonical decimal corpus thread IDs; opportunity and workspace references
// are trimmed local IDs. Construct subjects with ParsePortfolioSubject or
// NewPullRequestPortfolioSubject so storage and comparison share one identity.
type PortfolioSubject struct {
	kind portfolioSubjectKind
	ref  string
}

type portfolioSubjectKind uint8

const (
	portfolioPullRequestSubject portfolioSubjectKind = iota + 1
	portfolioOpportunitySubject
	portfolioWorkspaceSubject
)

// ParsePortfolioSubject consumes the transport/storage spelling for one local
// portfolio identity and returns its canonical representation.
func ParsePortfolioSubject(kind, ref string) (PortfolioSubject, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return PortfolioSubject{}, errors.New("portfolio subject reference is required")
	}
	switch strings.TrimSpace(kind) {
	case PortfolioSubjectPullRequest:
		id, err := strconv.ParseInt(ref, 10, 64)
		if err != nil || id <= 0 {
			return PortfolioSubject{}, errors.New("pull request subject reference must be a positive corpus thread id")
		}
		return PortfolioSubject{kind: portfolioPullRequestSubject, ref: strconv.FormatInt(id, 10)}, nil
	case PortfolioSubjectOpportunity:
		return PortfolioSubject{kind: portfolioOpportunitySubject, ref: ref}, nil
	case PortfolioSubjectWorkspace:
		return PortfolioSubject{kind: portfolioWorkspaceSubject, ref: ref}, nil
	default:
		return PortfolioSubject{}, errors.New("unknown portfolio subject kind")
	}
}

// NewPullRequestPortfolioSubject constructs an identity from an authoritative
// corpus thread ID without a string round trip at the call site.
func NewPullRequestPortfolioSubject(threadID int64) (PortfolioSubject, error) {
	if threadID <= 0 {
		return PortfolioSubject{}, errors.New("pull request subject reference must be a positive corpus thread id")
	}
	return PortfolioSubject{kind: portfolioPullRequestSubject, ref: strconv.FormatInt(threadID, 10)}, nil
}

func (s PortfolioSubject) Kind() string {
	switch s.kind {
	case portfolioPullRequestSubject:
		return PortfolioSubjectPullRequest
	case portfolioOpportunitySubject:
		return PortfolioSubjectOpportunity
	case portfolioWorkspaceSubject:
		return PortfolioSubjectWorkspace
	default:
		return ""
	}
}

func (s PortfolioSubject) Ref() string { return s.ref }

func (s PortfolioSubject) valid() bool { return s.Kind() != "" && s.ref != "" }

func (s PortfolioSubject) MarshalJSON() ([]byte, error) {
	if !s.valid() {
		return nil, errors.New("invalid portfolio subject")
	}
	return json.Marshal(struct {
		Kind string `json:"kind"`
		Ref  string `json:"ref"`
	}{Kind: s.Kind(), Ref: s.Ref()})
}

func (s *PortfolioSubject) UnmarshalJSON(data []byte) error {
	var input struct {
		Kind string `json:"kind"`
		Ref  string `json:"ref"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return err
	}
	parsed, err := ParsePortfolioSubject(input.Kind, input.Ref)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
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

// PortfolioSignal is one normalized overlap input. Constructors seal the
// scalar path/issue variants apart from scored pull-request similarity.
type PortfolioSignal struct {
	kind   portfolioSignalKind
	value  string
	target PortfolioSubject
	score  float64
}

type portfolioSignalKind uint8

const (
	portfolioFilePathSignal portfolioSignalKind = iota + 1
	portfolioLinkedIssueSignal
	portfolioOpportunitySimilaritySignal
)

func NewPortfolioFilePathSignal(value string) (PortfolioSignal, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return PortfolioSignal{}, errors.New("portfolio signal value is required")
	}
	value = path.Clean(strings.ReplaceAll(value, `\`, "/"))
	return PortfolioSignal{kind: portfolioFilePathSignal, value: value}, nil
}

func NewPortfolioLinkedIssueSignal(value string) (PortfolioSignal, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return PortfolioSignal{}, errors.New("portfolio signal value is required")
	}
	return PortfolioSignal{kind: portfolioLinkedIssueSignal, value: value}, nil
}

func NewPortfolioOpportunitySimilaritySignal(target PortfolioSubject, score float64) (PortfolioSignal, error) {
	if target.Kind() != PortfolioSubjectPullRequest || !(score >= 0 && score <= 1) {
		return PortfolioSignal{}, errors.New("opportunity similarity requires a pull request target and score between zero and one")
	}
	return PortfolioSignal{kind: portfolioOpportunitySimilaritySignal, target: target, score: score}, nil
}

func (s PortfolioSignal) Kind() string {
	switch s.kind {
	case portfolioFilePathSignal:
		return PortfolioSignalFilePath
	case portfolioLinkedIssueSignal:
		return PortfolioSignalLinkedIssue
	case portfolioOpportunitySimilaritySignal:
		return PortfolioSignalOpportunitySimilarity
	default:
		return ""
	}
}

func (s PortfolioSignal) Value() string { return s.value }

func (s PortfolioSignal) Target() (PortfolioSubject, bool) {
	return s.target, s.kind == portfolioOpportunitySimilaritySignal
}

func (s PortfolioSignal) Score() float64 { return s.score }

func parsePortfolioSignal(kind, value, targetKind, targetRef string, score float64) (PortfolioSignal, error) {
	switch strings.TrimSpace(kind) {
	case PortfolioSignalFilePath:
		if strings.TrimSpace(targetKind) != "" || strings.TrimSpace(targetRef) != "" || score != 0 {
			return PortfolioSignal{}, errors.New("file path signal cannot carry a target or score")
		}
		return NewPortfolioFilePathSignal(value)
	case PortfolioSignalLinkedIssue:
		if strings.TrimSpace(targetKind) != "" || strings.TrimSpace(targetRef) != "" || score != 0 {
			return PortfolioSignal{}, errors.New("linked issue signal cannot carry a target or score")
		}
		return NewPortfolioLinkedIssueSignal(value)
	case PortfolioSignalOpportunitySimilarity:
		if strings.TrimSpace(value) != "" {
			return PortfolioSignal{}, errors.New("opportunity similarity signal cannot carry a scalar value")
		}
		target, err := ParsePortfolioSubject(targetKind, targetRef)
		if err != nil {
			return PortfolioSignal{}, err
		}
		return NewPortfolioOpportunitySimilaritySignal(target, score)
	default:
		return PortfolioSignal{}, errors.New("unknown portfolio signal kind")
	}
}

func (s PortfolioSignal) MarshalJSON() ([]byte, error) {
	if s.Kind() == "" {
		return nil, errors.New("invalid portfolio signal")
	}
	output := struct {
		Kind       string  `json:"kind"`
		Value      string  `json:"value"`
		TargetKind string  `json:"target_kind,omitempty"`
		TargetRef  string  `json:"target_ref,omitempty"`
		Score      float64 `json:"score,omitempty"`
	}{Kind: s.Kind(), Value: s.Value(), Score: s.Score()}
	if target, ok := s.Target(); ok {
		output.TargetKind, output.TargetRef = target.Kind(), target.Ref()
	}
	return json.Marshal(output)
}

func (s *PortfolioSignal) UnmarshalJSON(data []byte) error {
	var input struct {
		Kind       string  `json:"kind"`
		Value      string  `json:"value"`
		TargetKind string  `json:"target_kind"`
		TargetRef  string  `json:"target_ref"`
		Score      float64 `json:"score"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return err
	}
	parsed, err := parsePortfolioSignal(input.Kind, input.Value, input.TargetKind, input.TargetRef, input.Score)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
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

// PullRequestIssueLinks is the latest complete linked-issue projection for one
// stored pull request. Covered distinguishes an observed empty relationship
// set from a facet that has never completed.
type PullRequestIssueLinks struct {
	ThreadID              int64
	Number                int
	Covered               bool
	LinkedIssues          []string
	SourceUpdatedAt       time.Time
	SourceObservationRefs []ObservationRef
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
	Candidate PortfolioSubject
	status    portfolioOverlapStatus
	coverage  map[string]bool
	Matches   []PortfolioOverlapMatch
}

type portfolioOverlapStatus uint8

const (
	portfolioOverlapUnknown portfolioOverlapStatus = iota + 1
	portfolioOverlapFound
	portfolioNoOverlap
)

func (s portfolioOverlapStatus) String() string {
	switch s {
	case portfolioOverlapUnknown:
		return "unknown"
	case portfolioOverlapFound:
		return "overlap"
	case portfolioNoOverlap:
		return "no_overlap"
	default:
		return ""
	}
}

// Status returns the stable wire representation of the computed outcome.
func (r PortfolioOverlapResult) Status() string { return r.status.String() }

// Unknown reports whether incomplete facet coverage prevents a negative result.
func (r PortfolioOverlapResult) Unknown() bool { return r.status == portfolioOverlapUnknown }

// Coverage returns the stable wire representation of each required facet's
// observed completeness.
func (r PortfolioOverlapResult) Coverage() map[string]string {
	coverage := make(map[string]string, len(r.coverage))
	for facet, complete := range r.coverage {
		if complete {
			coverage[facet] = "complete"
		} else {
			coverage[facet] = "missing"
		}
	}
	return coverage
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
	if kind != string(domain.PullRequestKind) {
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
		FROM portfolio_links ORDER BY pull_request_thread_id, opportunity_id, workspace_id, id
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
	`, snapshot.Subject.Kind(), snapshot.Subject.Ref(), snapshot.Facet, encodeTime(snapshot.SourceUpdatedAt), snapshot.ObservationSequence, string(refs), encodeTime(snapshot.ObservedAt))
	if err != nil {
		return nil, fmt.Errorf("insert portfolio signal snapshot: %w", err)
	}
	snapshot.ID, err = result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("read portfolio signal snapshot id: %w", err)
	}
	snapshot.Signals = canonicalPortfolioSignals(snapshot.Signals)
	for position, signal := range snapshot.Signals {
		targetKind, targetRef := "", ""
		if target, ok := signal.Target(); ok {
			targetKind, targetRef = target.Kind(), target.Ref()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO portfolio_signals (snapshot_id, position, kind, value, target_kind, target_ref, score)
			VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)
		`, snapshot.ID, position, signal.Kind(), signal.Value(), targetKind, targetRef, signal.Score()); err != nil {
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
	`, snapshot.Subject.Kind(), snapshot.Subject.Ref(), snapshot.Facet, snapshot.ID, encodeTime(snapshot.SourceUpdatedAt), snapshot.ObservationSequence); err != nil {
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
		switch ref.Kind() {
		case "thread":
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM thread_observations WHERE id=?`, ref.ID()).Scan(&exists)
		case "facet":
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM facet_observations WHERE id=?`, ref.ID()).Scan(&exists)
		default:
			return fmt.Errorf("unsupported source observation kind %q", ref.Kind())
		}
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("source observation %s:%d does not exist", ref.Kind(), ref.ID())
		}
		if err != nil {
			return fmt.Errorf("validate source observation %s:%d: %w", ref.Kind(), ref.ID(), err)
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
		if !ref.valid() {
			return errors.New("invalid portfolio source observation reference")
		}
	}
	for _, signal := range snapshot.Signals {
		if signal.Kind() != wantKind {
			return fmt.Errorf("signal kind %q does not belong to facet %q", signal.Kind(), snapshot.Facet)
		}
	}
	return nil
}

func validatePortfolioSubject(subject PortfolioSubject) error {
	if !subject.valid() {
		return errors.New("unknown portfolio subject kind")
	}
	return nil
}

func canonicalPortfolioSignals(signals []PortfolioSignal) []PortfolioSignal {
	out := append([]PortfolioSignal(nil), signals...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Kind() != b.Kind() {
			return a.Kind() < b.Kind()
		}
		if a.Value() != b.Value() {
			return a.Value() < b.Value()
		}
		aTarget, _ := a.Target()
		bTarget, _ := b.Target()
		if aTarget.Kind() != bTarget.Kind() {
			return aTarget.Kind() < bTarget.Kind()
		}
		if aTarget.Ref() != bTarget.Ref() {
			return aTarget.Ref() < bTarget.Ref()
		}
		return a.Score() < b.Score()
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

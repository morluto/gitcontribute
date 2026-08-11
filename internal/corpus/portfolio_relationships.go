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

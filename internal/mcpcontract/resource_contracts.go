package mcpcontract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

// WorkspaceResource is the canonical host-path-free representation of a
// managed workspace.
type WorkspaceResource struct {
	SchemaVersion   string `json:"schema_version"`
	ID              string `json:"id"`
	InvestigationID string `json:"investigation_id,omitempty"`
	Owner           string `json:"owner"`
	Repo            string `json:"repo"`
	BaseSHA         string `json:"base_sha"`
	HeadSHA         string `json:"head_sha"`
	MergeBase       string `json:"merge_base,omitempty"`
	Ownership       string `json:"ownership"`
	Dirty           bool   `json:"dirty"`
	HasUntracked    bool   `json:"has_untracked"`
	CreatedAt       string `json:"created_at"`
}

// ResourceCoverage records the effective completeness of one stored resource.
type ResourceCoverage struct {
	Complete        bool   `json:"complete"`
	SourceUpdatedAt string `json:"source_updated_at"`
}

// ThreadFacetObservationResource preserves one immutable facet observation
// while leaving its facet-specific payload as validated JSON.
type ThreadFacetObservationResource struct {
	SourceUpdatedAt     string          `json:"source_updated_at"`
	ObservationSequence int64           `json:"observation_sequence"`
	Payload             json.RawMessage `json:"payload"`
}

// ThreadFacetResource is the canonical offline payload for one stored facet.
type ThreadFacetResource struct {
	SchemaVersion string                           `json:"schema_version"`
	Owner         string                           `json:"owner"`
	Repo          string                           `json:"repo"`
	Kind          string                           `json:"kind"`
	Number        int                              `json:"number"`
	Facet         string                           `json:"facet"`
	Observations  []ThreadFacetObservationResource `json:"observations"`
	Coverage      *ResourceCoverage                `json:"coverage,omitempty"`
}

// ActorFacetResource is one parsed stored actor facet. The facet-specific value
// remains JSON, but invalid durable payloads cannot be represented as a
// successful resource.
type ActorFacetResource struct {
	actorID            string
	facet              string
	complete           bool
	observedAt         string
	sourceUpdatedAt    string
	authorizationScope string
	value              json.RawMessage
}

func NewActorFacetResource(actorID, facet string, complete bool, observedAt, sourceUpdatedAt, authorizationScope string, value json.RawMessage) (ActorFacetResource, error) {
	if actorID == "" || facet == "" {
		return ActorFacetResource{}, errors.New("actor ID and facet are required")
	}
	if !json.Valid(value) {
		return ActorFacetResource{}, errors.New("actor facet value must be valid JSON")
	}
	return ActorFacetResource{
		actorID: actorID, facet: facet, complete: complete,
		observedAt: observedAt, sourceUpdatedAt: sourceUpdatedAt,
		authorizationScope: authorizationScope,
		value:              append(json.RawMessage(nil), value...),
	}, nil
}

func (r ActorFacetResource) MarshalJSON() ([]byte, error) {
	if r.actorID == "" || r.facet == "" || !json.Valid(r.value) {
		return nil, errors.New("actor facet resource is not parsed")
	}
	return json.Marshal(struct {
		SchemaVersion      string          `json:"schema_version"`
		ActorID            string          `json:"actor_id"`
		Facet              string          `json:"facet"`
		Complete           bool            `json:"complete"`
		ObservedAt         string          `json:"observed_at"`
		SourceUpdatedAt    string          `json:"source_updated_at"`
		AuthorizationScope string          `json:"authorization_scope"`
		Value              json.RawMessage `json:"value"`
	}{
		SchemaVersion: "gitcontribute.actor-facet.v1", ActorID: r.actorID,
		Facet: r.facet, Complete: r.complete, ObservedAt: r.observedAt,
		SourceUpdatedAt: r.sourceUpdatedAt, AuthorizationScope: r.authorizationScope,
		Value: r.value,
	})
}

// PullRequestFeedbackPullRequestResource identifies the parent pull request
// and preserves an unknown merge state as JSON null.
type PullRequestFeedbackPullRequestResource struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Author string `json:"author"`
	State  string `json:"state"`
	Merged *bool  `json:"merged"`
}

// PullRequestFeedbackItemResource is the exact normalized feedback record
// named by a search match.
type PullRequestFeedbackItemResource struct {
	SchemaVersion       string                                 `json:"schema_version"`
	Owner               string                                 `json:"owner"`
	Repo                string                                 `json:"repo"`
	Number              int                                    `json:"number"`
	Channel             string                                 `json:"channel"`
	FeedbackID          string                                 `json:"feedback_id"`
	FeedbackNodeID      string                                 `json:"feedback_node_id"`
	ThreadID            string                                 `json:"thread_id"`
	InReplyToID         string                                 `json:"in_reply_to_id"`
	FeedbackAuthor      string                                 `json:"feedback_author"`
	ReviewState         string                                 `json:"review_state"`
	Body                string                                 `json:"body"`
	Path                string                                 `json:"path"`
	Line                *int                                   `json:"line"`
	StartLine           *int                                   `json:"start_line"`
	Side                string                                 `json:"side"`
	StartSide           string                                 `json:"start_side"`
	CommitOID           string                                 `json:"commit_oid"`
	Outdated            bool                                   `json:"outdated"`
	Resolved            *bool                                  `json:"resolved"`
	ResolutionState     string                                 `json:"resolution_state"`
	ResolvedBy          string                                 `json:"resolved_by"`
	CreatedAt           string                                 `json:"created_at"`
	UpdatedAt           string                                 `json:"updated_at"`
	HeadSHA             string                                 `json:"head_sha"`
	SourceObservationID int64                                  `json:"source_observation_id"`
	PullRequest         PullRequestFeedbackPullRequestResource `json:"pull_request"`
	EffectiveCoverage   *ResourceCoverage                      `json:"effective_coverage,omitempty"`
}

// StoredFacetResource is one validated JSON object from durable facet storage
// plus effective corpus coverage. The payload remains facet-specific, while
// the reserved coverage key has one authoritative representation.
type StoredFacetResource struct {
	payload  json.RawMessage
	coverage *ResourceCoverage
}

func NewStoredFacetResource(payload json.RawMessage, coverage *ResourceCoverage) (StoredFacetResource, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil {
		return StoredFacetResource{}, err
	}
	encoded := json.RawMessage(compact.Bytes())
	if len(encoded) < 2 || encoded[0] != '{' || encoded[len(encoded)-1] != '}' {
		return StoredFacetResource{}, errors.New("stored facet payload must be a JSON object")
	}
	var reserved struct {
		EffectiveCoverage json.RawMessage `json:"effective_coverage"`
	}
	if err := json.Unmarshal(encoded, &reserved); err != nil {
		return StoredFacetResource{}, errors.New("stored facet payload must be a JSON object")
	}
	if reserved.EffectiveCoverage != nil {
		return StoredFacetResource{}, errors.New("stored facet payload owns reserved effective_coverage key")
	}
	return StoredFacetResource{payload: encoded, coverage: coverage}, nil
}

func (r StoredFacetResource) MarshalJSON() ([]byte, error) {
	if len(r.payload) < 2 || r.payload[0] != '{' || r.payload[len(r.payload)-1] != '}' {
		return nil, errors.New("stored facet resource is not parsed")
	}
	if r.coverage == nil {
		return append([]byte(nil), r.payload...), nil
	}
	coverage, err := json.Marshal(r.coverage)
	if err != nil {
		return nil, err
	}
	return mergeJSONObject(r.payload, jsonObjectField{name: "effective_coverage", value: coverage})
}

type jsonObjectField struct {
	name  string
	value json.RawMessage
}

func mergeJSONObject(payload json.RawMessage, fields ...jsonObjectField) ([]byte, error) {
	if len(payload) < 2 || payload[0] != '{' || payload[len(payload)-1] != '}' {
		return nil, errors.New("resource payload is not a parsed JSON object")
	}
	out := append([]byte(nil), payload[:len(payload)-1]...)
	hasFields := len(payload) > 2
	for _, field := range fields {
		if hasFields {
			out = append(out, ',')
		}
		name, err := json.Marshal(field.name)
		if err != nil {
			return nil, err
		}
		out = append(out, name...)
		out = append(out, ':')
		out = append(out, field.value...)
		hasFields = true
	}
	out = append(out, '}')
	return out, nil
}

// PullRequestFeedbackChannelsResource names the four closed feedback facets.
type PullRequestFeedbackChannelsResource struct {
	IssueComments    *StoredFacetResource `json:"issue_comments,omitempty"`
	SubmittedReviews *StoredFacetResource `json:"submitted_reviews,omitempty"`
	InlineComments   *StoredFacetResource `json:"inline_comments,omitempty"`
	ReviewThreads    *StoredFacetResource `json:"review_threads,omitempty"`
}

// PullRequestFeedbackResource is the canonical raw-facet view for one pull
// request. Missing channels are omitted rather than represented as empty data.
type PullRequestFeedbackResource struct {
	SchemaVersion string                              `json:"schema_version"`
	Owner         string                              `json:"owner"`
	Repo          string                              `json:"repo"`
	Number        int                                 `json:"number"`
	Channels      PullRequestFeedbackChannelsResource `json:"channels"`
}

// CIFailureResource augments the stored CI snapshot without decoding and
// re-encoding its provider-shaped body, preserving missing and nullable keys.
type CIFailureResource struct {
	payload       json.RawMessage
	schemaVersion string
	owner         string
	repo          string
	number        int
	coverage      *ResourceCoverage
}

func NewCIFailureResource(payload json.RawMessage, owner, repo string, number int, coverage *ResourceCoverage) (CIFailureResource, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil {
		return CIFailureResource{}, err
	}
	encoded := json.RawMessage(compact.Bytes())
	if len(encoded) < 2 || encoded[0] != '{' || encoded[len(encoded)-1] != '}' {
		return CIFailureResource{}, errors.New("stored CI payload must be a JSON object")
	}
	var reserved struct {
		SchemaVersion     json.RawMessage `json:"schema_version"`
		Owner             json.RawMessage `json:"owner"`
		Repo              json.RawMessage `json:"repo"`
		Number            json.RawMessage `json:"number"`
		EffectiveCoverage json.RawMessage `json:"effective_coverage"`
	}
	if err := json.Unmarshal(encoded, &reserved); err != nil {
		return CIFailureResource{}, errors.New("stored CI payload must be a JSON object")
	}
	if reserved.SchemaVersion != nil || reserved.Owner != nil || reserved.Repo != nil || reserved.Number != nil || reserved.EffectiveCoverage != nil {
		return CIFailureResource{}, errors.New("stored CI payload owns reserved resource envelope keys")
	}
	return CIFailureResource{payload: encoded, schemaVersion: "gitcontribute.ci-failure-report.v1", owner: owner, repo: repo, number: number, coverage: coverage}, nil
}

func (r CIFailureResource) MarshalJSON() ([]byte, error) {
	if r.schemaVersion == "" {
		return nil, errors.New("CI failure resource is not parsed")
	}
	schemaVersion, _ := json.Marshal(r.schemaVersion)
	owner, _ := json.Marshal(r.owner)
	repo, _ := json.Marshal(r.repo)
	number, _ := json.Marshal(r.number)
	fields := []jsonObjectField{
		{name: "schema_version", value: schemaVersion}, {name: "owner", value: owner},
		{name: "repo", value: repo}, {name: "number", value: number},
	}
	if r.coverage != nil {
		coverage, err := json.Marshal(r.coverage)
		if err != nil {
			return nil, err
		}
		fields = append(fields, jsonObjectField{name: "effective_coverage", value: coverage})
	}
	return mergeJSONObject(r.payload, fields...)
}

// CIJobLogResource is one parsed, exact stored workflow-job log.
type CIJobLogResource struct {
	jobID     int64
	body      string
	truncated bool
}

func NewCIJobLogResource(jobID int64, body string, truncated bool) (CIJobLogResource, error) {
	if jobID <= 0 {
		return CIJobLogResource{}, errors.New("CI job log ID must be positive")
	}
	return CIJobLogResource{jobID: jobID, body: body, truncated: truncated}, nil
}

func (r CIJobLogResource) MarshalJSON() ([]byte, error) {
	if r.jobID <= 0 {
		return nil, errors.New("CI job log resource is not parsed")
	}
	return json.Marshal(struct {
		SchemaVersion string `json:"schema_version"`
		JobID         int64  `json:"job_id"`
		Body          string `json:"body"`
		Truncated     bool   `json:"truncated"`
	}{
		SchemaVersion: "gitcontribute.ci-job-log.v1",
		JobID:         r.jobID,
		Body:          r.body,
		Truncated:     r.truncated,
	})
}

// Reader is the local, read-only application boundary exposed through MCP.
// Implementations must not perform network access.
type Reader interface {
	Search(context.Context, SearchInput) (SearchOutput, error)
	SearchRepositories(context.Context, SearchRepositoriesInput) (SearchRepositoriesOutput, error)
	Repository(context.Context, RepoInput) (RepositoryOutput, error)
	Thread(context.Context, ThreadInput) (ThreadOutput, error)
	ThreadByNumber(context.Context, ThreadByNumberInput) (ThreadOutput, error)
	Dossier(context.Context, RepoInput) (DossierOutput, error)
	SearchCode(context.Context, SearchCodeInput) (SearchCodeOutput, error)
	ExplainMatch(context.Context, ExplainMatchInput) (ExplainMatchOutput, error)
	GetJob(context.Context, GetJobInput) (GetJobOutput, error)
	Investigation(context.Context, InvestigationInput) (InvestigationOutput, error)
	ListOpportunities(context.Context, ListOpportunitiesInput) (ListOpportunitiesOutput, error)
	Opportunity(context.Context, OpportunityInput) (OpportunityOutput, error)
	Evidence(context.Context, EvidenceInput) (EvidenceOutput, error)
	Readiness(context.Context, ReadinessInput) (ReadinessOutput, error)
	FindClusters(context.Context, FindClustersInput) (FindClustersOutput, error)
	GetCoverage(context.Context, GetCoverageInput) (GetCoverageOutput, error)
	Lens(context.Context, LensInput) (LensOutput, error)
}

// RepoInput identifies a repository for an MCP operation.
type RepoInput struct {
	Owner string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo  string `json:"repo" jsonschema:"GitHub repository name"`
}

// ThreadInput identifies an issue or pull request for an MCP operation.
type ThreadInput struct {
	Owner         string `json:"owner" jsonschema:"GitHub repository owner"`
	Repo          string `json:"repo" jsonschema:"GitHub repository name"`
	Kind          string `json:"kind" jsonschema:"Thread kind: issue or pull_request"`
	Number        int    `json:"number" jsonschema:"GitHub issue or pull request number"`
	SnapshotToken string `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token"`
}

// ConcernInput identifies one persisted local concern.
type ConcernInput struct {
	ID string `json:"id" jsonschema:"Concern ID"`
}

// DraftInput identifies one immutable persisted contribution-draft revision.
type DraftInput struct {
	ID       string `json:"id" jsonschema:"Draft ID"`
	Revision int    `json:"revision" jsonschema:"Positive draft revision"`
}

// ManifestInput identifies one persisted contribution evidence manifest.
type ManifestInput struct {
	ID            string `json:"id" jsonschema:"Manifest ID"`
	SnapshotToken string `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token"`
}

// SearchInput describes an offline thread search page.
type SearchInput struct {
	Query         string   `json:"query" jsonschema:"Thread full-text query"`
	Owner         string   `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo          string   `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Kind          string   `json:"kind,omitempty" jsonschema:"Optional thread kind: issue or pull_request"`
	State         string   `json:"state,omitempty" jsonschema:"Optional open or closed state"`
	StateReason   string   `json:"state_reason,omitempty" jsonschema:"Optional GitHub completed or not_planned state reason"`
	Merged        *bool    `json:"merged,omitempty" jsonschema:"Optional pull request merged state"`
	Author        string   `json:"author,omitempty" jsonschema:"Optional author login"`
	Association   string   `json:"author_association,omitempty" jsonschema:"Optional GitHub author association"`
	Assignee      string   `json:"assignee,omitempty" jsonschema:"Optional assignee login"`
	Labels        []string `json:"labels,omitempty" jsonschema:"Labels that must all be present"`
	UpdatedAfter  string   `json:"updated_after,omitempty" jsonschema:"Optional RFC 3339 lower bound"`
	UpdatedBefore string   `json:"updated_before,omitempty" jsonschema:"Optional RFC 3339 upper bound"`
	Limit         int      `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor        string   `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	Sort          string   `json:"sort,omitempty" jsonschema:"Order: relevance or updated"`
	MatchMode     string   `json:"match_mode,omitempty" jsonschema:"Term matching: all requires every term; any requires at least one term"`
	View          string   `json:"view,omitempty" jsonschema:"compact omits full bodies and returns bounded excerpts; full includes stored bodies"`
	SnapshotToken string   `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token"`
}

// ThreadOutput is the stable MCP representation of an issue or pull request.
type ThreadOutput struct {
	Owner             string   `json:"owner"`
	Repo              string   `json:"repo"`
	Kind              string   `json:"kind"`
	Number            int      `json:"number"`
	State             string   `json:"state"`
	StateReason       string   `json:"state_reason,omitempty"`
	Title             string   `json:"title"`
	Body              string   `json:"body,omitempty"`
	Author            string   `json:"author,omitempty"`
	AuthorAssociation string   `json:"author_association,omitempty"`
	Labels            []string `json:"labels,omitempty"`
	Assignees         []string `json:"assignees,omitempty"`
	Draft             bool     `json:"draft,omitempty"`
	ClosedAt          string   `json:"closed_at,omitempty"`
	MergedAt          string   `json:"merged_at,omitempty"`
	Merged            *bool    `json:"merged,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
	MatchSource       string   `json:"match_source,omitempty"`
	MatchExcerpt      string   `json:"match_excerpt,omitempty"`
	MatchTruncated    bool     `json:"match_truncated,omitempty" jsonschema:"Whether the per-thread hydrated search document was bounded"`
	MatchUpdatedAt    string   `json:"match_updated_at,omitempty"`
	SnapshotToken     string   `json:"snapshot_token"`
}

// SearchOutput contains one page of offline thread matches.
type SearchOutput struct {
	Query               string               `json:"query"`
	QueryInterpretation string               `json:"query_interpretation"`
	MatchMode           string               `json:"match_mode"`
	View                string               `json:"view"`
	Matches             []ThreadOutput       `json:"matches"`
	Total               int                  `json:"total"`
	NextCursor          string               `json:"next_cursor,omitempty"`
	UnknownMergeCount   int                  `json:"unknown_merge_count,omitempty"`
	Suggestion          string               `json:"suggestion,omitempty"`
	Recovery            *RecoveryPlan        `json:"recovery,omitempty"`
	SnapshotToken       string               `json:"snapshot_token"`
	Provenance          CorpusReadProvenance `json:"provenance"`
}

// DossierOutput contains a persisted repository dossier snapshot.
type DossierOutput struct {
	Owner                string          `json:"owner"`
	Repo                 string          `json:"repo"`
	AsOf                 string          `json:"as_of,omitempty"`
	RecentItemsLimit     NonNegativeInt  `json:"recent_items_limit" jsonschema:"Maximum items retained in each recent-thread section"`
	RecentItemsTruncated bool            `json:"recent_items_truncated" jsonschema:"True when at least one recent-thread section is a bounded sample"`
	Sections             DossierSections `json:"sections"`
}

// DossierSections is the stable typed projection of persisted dossier data.
type DossierSections struct {
	Description                      string                `json:"description,omitempty"`
	Language                         string                `json:"language,omitempty"`
	Stars                            NonNegativeInt        `json:"stars"`
	OpenIssues                       NonNegativeInt        `json:"open_issues"`
	ClosedIssues                     NonNegativeInt        `json:"closed_issues"`
	OpenPullRequests                 NonNegativeInt        `json:"open_prs"`
	MergedPullRequests               NonNegativeInt        `json:"merged_prs"`
	ClosedUnmergedPullRequests       NonNegativeInt        `json:"closed_unmerged_prs"`
	ClosedUnknownMergePullRequests   NonNegativeInt        `json:"closed_unknown_merge_prs"`
	RecentMergedPullRequests         []DossierThreadOutput `json:"recent_merged_prs"`
	RecentOpenPullRequests           []DossierThreadOutput `json:"recent_open_prs"`
	RecentClosedUnmergedPullRequests []DossierThreadOutput `json:"recent_closed_unmerged_prs"`
	RecentClosedUnknownPullRequests  []DossierThreadOutput `json:"recent_closed_unknown_merge_prs"`
	RecentIssues                     []DossierThreadOutput `json:"recent_issues"`
	Guidance                         string                `json:"guidance,omitempty"`
	Coverage                         []string              `json:"coverage" jsonschema:"Observed dossier facets; omitted facets are unknown"`
}

// DossierThreadOutput is one bounded recent thread summary.
type DossierThreadOutput struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Author    string   `json:"author,omitempty"`
	State     string   `json:"state"`
	Draft     bool     `json:"draft,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	ClosedAt  string   `json:"closed_at,omitempty"`
	MergedAt  string   `json:"merged_at,omitempty"`
	Labels    []string `json:"labels,omitempty"`
}

// SourceRef records provenance for an MCP result or workflow artifact.
type SourceRef struct {
	Source     string `json:"source" jsonschema:"Source identifier"`
	URL        string `json:"url,omitempty" jsonschema:"Source URL"`
	CommitSHA  string `json:"commit_sha,omitempty" jsonschema:"Source commit SHA"`
	ObservedAt string `json:"observed_at,omitempty" jsonschema:"Observation timestamp"`
	AsOf       string `json:"as_of,omitempty" jsonschema:"As-of timestamp"`
}

// CorpusReadProvenance binds an offline result to the exact query and source
// observation it used. Transaction-bound identities are deliberately marked
// non-durable; callers that need cross-call reuse must request a persisted
// snapshot rather than treating a mutable projection as historical evidence.
type CorpusReadProvenance struct {
	SnapshotToken        string        `json:"snapshot_token"`
	Durable              bool          `json:"durable"`
	ObservationWatermark int64         `json:"observation_watermark"`
	QueryDigestSHA256    string        `json:"query_digest_sha256"`
	Limitations          []string      `json:"limitations,omitempty"`
	ExternalContext      []SourceRef   `json:"external_context,omitempty"`
	Recovery             *RecoveryPlan `json:"recovery,omitempty"`
	coverage             corpusReadCoverage
}

type corpusReadCoverage struct {
	known     bool
	truncated bool
	unknown   bool
}

// NewCorpusReadProvenance constructs one coverage-consistent read identity.
func NewCorpusReadProvenance(snapshotToken string, durable bool, observationWatermark int64, queryDigest string, truncated, unknownCoverage bool) CorpusReadProvenance {
	return CorpusReadProvenance{
		SnapshotToken: snapshotToken, Durable: durable,
		ObservationWatermark: observationWatermark, QueryDigestSHA256: queryDigest,
		coverage: corpusReadCoverage{known: true, truncated: truncated, unknown: unknownCoverage},
	}
}

func (p CorpusReadProvenance) Complete() bool {
	return p.coverage.known && !p.coverage.truncated && !p.coverage.unknown
}

func (p CorpusReadProvenance) Truncated() bool { return p.coverage.truncated }

func (p CorpusReadProvenance) UnknownCoverage() bool { return p.coverage.unknown }

type corpusReadProvenanceJSON struct {
	SnapshotToken        string        `json:"snapshot_token"`
	Durable              bool          `json:"durable"`
	ObservationWatermark int64         `json:"observation_watermark"`
	QueryDigestSHA256    string        `json:"query_digest_sha256"`
	Complete             bool          `json:"complete"`
	Truncated            bool          `json:"truncated"`
	UnknownCoverage      bool          `json:"unknown_coverage"`
	Limitations          []string      `json:"limitations,omitempty"`
	ExternalContext      []SourceRef   `json:"external_context,omitempty"`
	Recovery             *RecoveryPlan `json:"recovery,omitempty"`
}

func (p CorpusReadProvenance) MarshalJSON() ([]byte, error) {
	return json.Marshal(corpusReadProvenanceJSON{
		SnapshotToken: p.SnapshotToken, Durable: p.Durable,
		ObservationWatermark: p.ObservationWatermark, QueryDigestSHA256: p.QueryDigestSHA256,
		Complete: p.Complete(), Truncated: p.Truncated(), UnknownCoverage: p.UnknownCoverage(),
		Limitations: p.Limitations, ExternalContext: p.ExternalContext, Recovery: p.Recovery,
	})
}

func (p *CorpusReadProvenance) UnmarshalJSON(data []byte) error {
	var raw corpusReadProvenanceJSON
	if err := decodeStrictJSON(data, &raw); err != nil {
		return err
	}
	if raw.Complete && (raw.Truncated || raw.UnknownCoverage) {
		return errors.New("complete corpus read provenance cannot be truncated or have unknown coverage")
	}
	parsed := NewCorpusReadProvenance(raw.SnapshotToken, raw.Durable, raw.ObservationWatermark, raw.QueryDigestSHA256, raw.Truncated, raw.UnknownCoverage)
	if !raw.Complete && !raw.Truncated && !raw.UnknownCoverage {
		parsed.coverage.known = false
	}
	parsed.Limitations, parsed.ExternalContext, parsed.Recovery = raw.Limitations, raw.ExternalContext, raw.Recovery
	*p = parsed
	return nil
}

// SearchCodeInput describes an offline code search page.
type SearchCodeInput struct {
	Query         string `json:"query" jsonschema:"Code search query"`
	Owner         string `json:"owner,omitempty" jsonschema:"Optional repository owner"`
	Repo          string `json:"repo,omitempty" jsonschema:"Optional repository name"`
	Limit         int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
	Cursor        string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page"`
	SnapshotToken string `json:"snapshot_token,omitempty" jsonschema:"Optional immutable corpus snapshot token from a previous offline read"`
}

// CodeMatchOutput identifies one stored code match.
type CodeMatchOutput struct {
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	Commit   string `json:"commit"`
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Snippet  string `json:"snippet"`
	Bytes    int    `json:"bytes"`
}

// CodeIndexCoverageOutput reports one selected snapshot's indexing coverage.
type CodeIndexCoverageOutput struct {
	Repo           string        `json:"repo"`
	Status         string        `json:"status" jsonschema:"Index coverage state"`
	Commit         string        `json:"commit"`
	Truncated      bool          `json:"truncated" jsonschema:"Whether index limits omitted files"`
	IndexedFiles   int           `json:"indexed_files" jsonschema:"Files indexed in this snapshot"`
	TrackedEntries int           `json:"tracked_entries" jsonschema:"Tracked tree entries considered"`
	SkippedFiles   int           `json:"skipped_files" jsonschema:"Entries omitted by policy or limits"`
	SkippedPolicy  int           `json:"skipped_policy" jsonschema:"Invalid, excluded, or non-regular entries"`
	SkippedLimits  int           `json:"skipped_limits" jsonschema:"Entries omitted by file-size, total-size, or file-count bounds"`
	SkippedNonText int           `json:"skipped_non_text" jsonschema:"Entries omitted because content was binary or invalid UTF-8"`
	Recovery       *RecoveryPlan `json:"recovery,omitempty" jsonschema:"Typed action to acquire or re-index missing or incomplete code coverage"`
}

// SearchCodeOutput contains one page of offline code matches.
type SearchCodeOutput struct {
	Query         string                    `json:"query"`
	Total         int                       `json:"total"`
	Matches       []CodeMatchOutput         `json:"matches"`
	Coverage      []CodeIndexCoverageOutput `json:"coverage,omitempty"`
	NextCursor    string                    `json:"next_cursor,omitempty"`
	SnapshotToken string                    `json:"snapshot_token"`
	Recovery      *RecoveryPlan             `json:"recovery,omitempty"`
	Provenance    CorpusReadProvenance      `json:"provenance"`
}

// InvestigationInput selects an investigation and bounds nested hypotheses.
type InvestigationInput struct {
	ID              string `json:"id" jsonschema:"Investigation ID"`
	HypothesisLimit int    `json:"hypothesis_limit,omitempty" jsonschema:"Maximum hypotheses from 1 to 100"`
}

// HypothesisSummary is the compact hypothesis representation nested in an investigation.
type HypothesisSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

// InvestigationOutput is the stable MCP representation of an investigation.
type InvestigationOutput struct {
	ID              string              `json:"id"`
	Owner           string              `json:"owner"`
	Repo            string              `json:"repo"`
	CommitSHA       string              `json:"commit_sha,omitempty"`
	Lens            string              `json:"lens,omitempty"`
	Status          string              `json:"status"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at"`
	HypothesisTotal int                 `json:"hypothesis_total"`
	Hypotheses      []HypothesisSummary `json:"hypotheses,omitempty"`
}

// ListOpportunitiesInput selects and bounds opportunities for an investigation.
type ListOpportunitiesInput struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation ID"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum results from 1 to 100"`
}

// OpportunitySummary is the compact opportunity representation used in lists.
type OpportunitySummary struct {
	ID              string      `json:"id"`
	InvestigationID string      `json:"investigation_id"`
	Title           string      `json:"title"`
	Category        string      `json:"category"`
	Status          string      `json:"status"`
	Confidence      Probability `json:"confidence"`
	CollisionStatus string      `json:"collision_status"`
	CreatedAt       string      `json:"created_at"`
	UpdatedAt       string      `json:"updated_at"`
}

// ListOpportunitiesOutput contains bounded opportunities for an investigation.
type ListOpportunitiesOutput struct {
	Opportunities []OpportunitySummary `json:"opportunities"`
	Total         int                  `json:"total"`
}

// OpportunityInput selects an opportunity and bounds nested evidence.
type OpportunityInput struct {
	ID            string `json:"id" jsonschema:"Opportunity ID"`
	EvidenceLimit int    `json:"evidence_limit,omitempty" jsonschema:"Maximum evidence IDs from 1 to 100"`
}

// OpportunityOutput is the stable MCP representation of a contribution opportunity.
type OpportunityOutput struct {
	ID                  string      `json:"id"`
	InvestigationID     string      `json:"investigation_id"`
	HypothesisID        string      `json:"hypothesis_id,omitempty"`
	Title               string      `json:"title"`
	ProblemStatement    string      `json:"problem_statement"`
	Category            string      `json:"category"`
	Scope               string      `json:"scope"`
	Impact              string      `json:"impact"`
	Confidence          Probability `json:"confidence"`
	ExpectedEffort      string      `json:"expected_effort,omitempty"`
	Dependencies        []string    `json:"dependencies,omitempty"`
	CollisionStatus     string      `json:"collision_status"`
	MaintainerAlignment string      `json:"maintainer_alignment,omitempty"`
	SourceRefs          []SourceRef `json:"source_refs,omitempty"`
	EvidenceTotal       int         `json:"evidence_total"`
	EvidenceIDs         []string    `json:"evidence_ids,omitempty"`
	Status              string      `json:"status"`
	CreatedAt           string      `json:"created_at"`
	UpdatedAt           string      `json:"updated_at"`
}

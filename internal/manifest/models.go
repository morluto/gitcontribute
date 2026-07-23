// Package manifest owns the stable contribution evidence export contract.
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
	"github.com/morluto/gitcontribute/internal/evidence"
	"github.com/morluto/gitcontribute/internal/workspace"
)

// ErrNotFound means no persisted manifest matched the requested identity.
var (
	ErrNotFound         = errors.New("contribution manifest not found")
	ErrIdentityMismatch = errors.New("contribution manifest identity mismatch")
)

const (
	// StatementType identifies the standard in-toto statement envelope.
	StatementType = "https://in-toto.io/Statement/v1"
	// PredicateType identifies the GitContribute evidence predicate.
	PredicateType = "https://github.com/morluto/gitcontribute/attestation/contribution-evidence/v1"
	// SchemaVersion identifies the predicate's product-owned schema.
	SchemaVersion = "contribution-evidence.v1"
	// ManifestIDPrefix identifies the digest algorithm used by manifest IDs.
	ManifestIDPrefix = "sha256:"
)

// ResourceDescriptor follows the in-toto v1 subject identity shape.
type ResourceDescriptor struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Statement wraps the product predicate in an in-toto Statement v1 envelope.
type Statement struct {
	Type          string               `json:"_type"`
	Subject       []ResourceDescriptor `json:"subject"`
	PredicateType string               `json:"predicateType"`
	Predicate     Predicate            `json:"predicate"`
}

// Predicate is the self-contained contribution evidence manifest.
type Predicate struct {
	SchemaVersion string              `json:"schema_version"`
	ManifestID    string              `json:"manifest_id"`
	ContentSHA256 string              `json:"content_sha256"`
	GeneratedAt   time.Time           `json:"generated_at"`
	Repository    RepositoryIdentity  `json:"repository"`
	Opportunity   OpportunityRecord   `json:"opportunity"`
	Workspace     *workspace.Snapshot `json:"workspace,omitempty"`
	Validations   []ValidationRecord  `json:"validations"`
	Evidence      []EvidenceRecord    `json:"evidence"`
	Readiness     ReadinessRecord     `json:"readiness"`
	PullRequest   *PullRequestRecord  `json:"pull_request,omitempty"`
	Drafts        []DraftRecord       `json:"drafts"`
	Status        string              `json:"status"`
	Completeness  []CompletenessFacet `json:"completeness"`
	Gaps          []Gap               `json:"gaps"`
}

// RepositoryIdentity binds the manifest to the investigated source revision.
type RepositoryIdentity struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	CommitSHA string `json:"investigation_commit_sha"`
}

// OpportunityRecord captures the scoped contribution outcome.
type OpportunityRecord struct {
	ID               string             `json:"id"`
	InvestigationID  string             `json:"investigation_id"`
	HypothesisID     string             `json:"hypothesis_id,omitempty"`
	ProblemStatement string             `json:"problem_statement"`
	Scope            string             `json:"scope"`
	Impact           string             `json:"impact"`
	Status           string             `json:"status"`
	SourceRefs       []domain.SourceRef `json:"source_refs"`
}

// ValidationRecord binds a stored run to its command and candidate identity.
type ValidationRecord struct {
	DefinitionID            string                        `json:"definition_id"`
	RunID                   string                        `json:"run_id"`
	Kind                    string                        `json:"kind"`
	Command                 []string                      `json:"command"`
	CommandSHA256           string                        `json:"command_sha256"`
	ExecutionContractSHA256 string                        `json:"execution_contract_sha256"`
	EnvironmentAllowlist    []string                      `json:"environment_allowlist"`
	Timeout                 string                        `json:"timeout"`
	MaxOutputBytes          int64                         `json:"max_output_bytes"`
	Observation             *evidence.ObservationContract `json:"observation,omitempty"`
	Classification          string                        `json:"classification"`
	ObservationStatus       string                        `json:"observation_status"`
	Observations            []evidence.ObservationResult  `json:"observations"`
	StartedAt               time.Time                     `json:"started_at"`
	CompletedAt             time.Time                     `json:"completed_at"`
	WorkspaceSnapshotBefore string                        `json:"workspace_snapshot_before,omitempty"`
	WorkspaceSnapshotAfter  string                        `json:"workspace_snapshot_after,omitempty"`
	WorkspaceBindingStatus  string                        `json:"workspace_binding_status"`
	WorkspaceCompatibility  string                        `json:"workspace_compatibility"`
	CompatibilityReason     string                        `json:"compatibility_reason"`
	Selected                bool                          `json:"selected_for_completeness"`
}

// EvidenceRecord captures a stored evidence item and evaluated freshness.
type EvidenceRecord struct {
	ID               string                    `json:"id"`
	Type             string                    `json:"type"`
	Relation         string                    `json:"relation"`
	Description      string                    `json:"description"`
	ValidationRunID  string                    `json:"validation_run_id,omitempty"`
	SourceRefs       []domain.SourceRef        `json:"source_refs"`
	SourceProvenance []evidence.SourceRevision `json:"source_provenance"`
	Freshness        string                    `json:"freshness"`
	FreshnessReason  string                    `json:"freshness_reason"`
}

// ReadinessRecord captures the deterministic readiness rule evaluation.
type ReadinessRecord struct {
	RuleSetVersion string           `json:"rule_set_version"`
	Status         string           `json:"status"`
	EvaluatedAt    string           `json:"evaluated_at"`
	Checks         []ReadinessCheck `json:"checks"`
}

// ReadinessCheck captures one versioned readiness rule result.
type ReadinessCheck struct {
	RuleID       string   `json:"rule_id"`
	RuleVersion  string   `json:"rule_version"`
	Status       string   `json:"status"`
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs"`
}

// FacetStatus reports freshness and completeness for one GitHub projection.
type FacetStatus struct {
	Facet     string `json:"facet"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// PullRequestRecord captures explicitly selected, locally stored PR health.
type PullRequestRecord struct {
	Owner                   string        `json:"owner"`
	Repo                    string        `json:"repo"`
	Number                  int           `json:"number"`
	State                   string        `json:"state"`
	HeadSHA                 string        `json:"head_sha,omitempty"`
	BaseSHA                 string        `json:"base_sha,omitempty"`
	ChecksStatus            string        `json:"checks_status,omitempty"`
	ReviewDecision          string        `json:"review_decision,omitempty"`
	UnresolvedReviewThreads *int          `json:"unresolved_review_threads,omitempty"`
	MergeStateStatus        string        `json:"merge_state_status,omitempty"`
	MergeQueueState         string        `json:"merge_queue_state,omitempty"`
	Attention               string        `json:"attention"`
	SourceUpdatedAt         string        `json:"source_updated_at"`
	Facets                  []FacetStatus `json:"facets"`
}

// DraftRecord identifies a locally prepared contribution draft.
type DraftRecord struct {
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	RenderedAt time.Time `json:"rendered_at"`
	ManifestID string    `json:"manifest_id,omitempty"`
}

// CompletenessFacet reports whether one evidence area is usable.
type CompletenessFacet struct {
	Facet  string `json:"facet"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// Gap records evidence that is missing, stale, unknown, or incompatible.
type Gap struct {
	Code   string `json:"code"`
	Facet  string `json:"facet"`
	Reason string `json:"reason"`
}

// Finalize computes the deterministic content identity and in-toto subject.
func Finalize(predicate Predicate) (Statement, error) {
	predicate.SchemaVersion = SchemaVersion
	contentDigest, err := predicateIdentityDigest(predicate)
	if err != nil {
		return Statement{}, err
	}
	predicate.ContentSHA256 = contentDigest
	predicate.ManifestID = ManifestIDPrefix + predicate.ContentSHA256
	subjectDigest := predicate.ContentSHA256
	if predicate.Workspace != nil && predicate.Workspace.SHA256 != "" {
		subjectDigest = predicate.Workspace.SHA256
	}
	statement := Statement{
		Type:          StatementType,
		Subject:       []ResourceDescriptor{{Name: predicate.Repository.Owner + "/" + predicate.Repository.Repo, Digest: map[string]string{"sha256": subjectDigest}}},
		PredicateType: PredicateType,
		Predicate:     predicate,
	}
	if err := statement.Validate(); err != nil {
		return Statement{}, err
	}
	return statement, nil
}

// Validate checks the stable envelope and identity fields before persistence.
func (s Statement) Validate() error {
	if s.Type != StatementType || s.PredicateType != PredicateType || s.Predicate.SchemaVersion != SchemaVersion {
		return errors.New("manifest schema identity is invalid")
	}
	if len(s.Subject) != 1 || len(s.Subject[0].Digest["sha256"]) != sha256.Size*2 {
		return errors.New("manifest subject requires one SHA-256 digest")
	}
	if s.Predicate.ManifestID != ManifestIDPrefix+s.Predicate.ContentSHA256 || len(s.Predicate.ContentSHA256) != sha256.Size*2 {
		return errors.New("manifest content identity is invalid")
	}
	if s.Predicate.Repository.Owner == "" || s.Predicate.Repository.Repo == "" || s.Predicate.Opportunity.ID == "" {
		return errors.New("manifest repository and opportunity are required")
	}
	wantContent, err := predicateIdentityDigest(s.Predicate)
	if err != nil {
		return err
	}
	if s.Predicate.ContentSHA256 != wantContent {
		return errors.New("manifest content digest does not match predicate")
	}
	wantSubject := wantContent
	if s.Predicate.Workspace != nil && s.Predicate.Workspace.SHA256 != "" {
		wantSubject = s.Predicate.Workspace.SHA256
	}
	if s.Subject[0].Name != s.Predicate.Repository.Owner+"/"+s.Predicate.Repository.Repo || s.Subject[0].Digest["sha256"] != wantSubject {
		return errors.New("manifest subject does not match predicate identity")
	}
	return nil
}

func predicateIdentityDigest(predicate Predicate) (string, error) {
	identity := predicate
	identity.ManifestID, identity.ContentSHA256, identity.GeneratedAt = "", "", time.Time{}
	identity.Readiness.EvaluatedAt = ""
	payload, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode manifest identity: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

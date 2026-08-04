package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/morluto/gitcontribute/internal/domain"
)

const ExternalEvidenceManifestSchemaV1 = "gitcontribute.external-evidence.v1"

const maxExternalEvidenceManifestBytes = 2 << 20
const maxExternalEvidenceSharedMetadataBytes = 16 << 10

// ExternalEvidenceManifest is a bounded, producer-neutral handoff. It is
// imported as evidence only; no producer command, path, or reference is run.
type ExternalEvidenceManifest struct {
	SchemaVersion   string                  `json:"schema_version"`
	Producer        string                  `json:"producer"`
	InvestigationID string                  `json:"investigation_id"`
	HypothesisID    string                  `json:"hypothesis_id,omitempty"`
	OpportunityID   string                  `json:"opportunity_id,omitempty"`
	Repository      string                  `json:"repository"`
	Revision        string                  `json:"revision"`
	ArtifactSHA256  string                  `json:"artifact_sha256,omitempty"`
	ObservedAt      time.Time               `json:"observed_at"`
	Environment     map[string]string       `json:"environment,omitempty"`
	Completeness    string                  `json:"completeness"`
	Integrity       string                  `json:"integrity"`
	Limitations     []string                `json:"limitations,omitempty"`
	Claims          []ExternalEvidenceClaim `json:"claims"`
	ManifestSHA256  string                  `json:"manifest_sha256"`
}

type ExternalEvidenceClaim struct {
	ID           string         `json:"id"`
	Type         EvidenceType   `json:"type"`
	Relation     Relation       `json:"relation"`
	Description  string         `json:"description"`
	SourceRefs   []string       `json:"source_refs,omitempty"`
	Measurements map[string]any `json:"measurements,omitempty"`
}

type ImportedExternalEvidence struct {
	EvidenceID     string
	Producer       string
	ManifestSHA256 string
	ClaimCount     int
	Incomplete     bool
}

func DigestExternalEvidenceManifest(item ExternalEvidenceManifest) (string, error) {
	payload, err := canonicalExternalEvidenceManifest(item)
	if err != nil {
		return "", fmt.Errorf("encode external evidence manifest: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalExternalEvidenceManifest(item ExternalEvidenceManifest) ([]byte, error) {
	item.ManifestSHA256 = ""
	canonical := struct {
		SchemaVersion   string                  `json:"schema_version"`
		Producer        string                  `json:"producer"`
		InvestigationID string                  `json:"investigation_id"`
		HypothesisID    string                  `json:"hypothesis_id,omitempty"`
		OpportunityID   string                  `json:"opportunity_id,omitempty"`
		Repository      string                  `json:"repository"`
		Revision        string                  `json:"revision"`
		ArtifactSHA256  string                  `json:"artifact_sha256,omitempty"`
		ObservedAt      string                  `json:"observed_at"`
		Environment     map[string]string       `json:"environment,omitempty"`
		Completeness    string                  `json:"completeness"`
		Integrity       string                  `json:"integrity"`
		Limitations     []string                `json:"limitations,omitempty"`
		Claims          []ExternalEvidenceClaim `json:"claims"`
		ManifestSHA256  string                  `json:"manifest_sha256"`
	}{item.SchemaVersion, item.Producer, item.InvestigationID, item.HypothesisID, item.OpportunityID, item.Repository, item.Revision, item.ArtifactSHA256, item.ObservedAt.UTC().Format(time.RFC3339Nano), item.Environment, item.Completeness, item.Integrity, item.Limitations, item.Claims, ""}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(canonical); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(payload.Bytes(), []byte{'\n'}), nil
}

// ImportExternalEvidenceManifest validates and stores each typed claim as a
// separate evidence record. The manifest digest is the stable claim identity.
func (s *Service) ImportExternalEvidenceManifest(ctx context.Context, item ExternalEvidenceManifest) (*ImportedExternalEvidence, error) {
	if err := validateExternalEvidenceManifest(item); err != nil {
		return nil, err
	}
	digest, err := DigestExternalEvidenceManifest(item)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(item.ManifestSHA256, digest) {
		return nil, fmt.Errorf("external evidence manifest digest mismatch: got %q want %q", item.ManifestSHA256, digest)
	}
	firstEvidenceID := ""
	for _, claim := range item.Claims {
		claimDigest, err := DigestExternalEvidenceClaim(digest, claim)
		if err != nil {
			return nil, err
		}
		e := &Evidence{
			ID: "external-evidence-" + claimDigest, InvestigationID: item.InvestigationID,
			HypothesisID: item.HypothesisID, OpportunityID: item.OpportunityID,
			Type: claim.Type, Relation: claim.Relation, Description: claim.Description,
			CreatedAt: item.ObservedAt, Measurements: claim.Measurements, SourceRefs: externalClaimSourceRefs(claim.SourceRefs, item),
			External: &ExternalEvidenceProvenance{SchemaVersion: item.SchemaVersion, Producer: item.Producer, Repository: item.Repository, Revision: item.Revision, ArtifactSHA256: item.ArtifactSHA256, ObservedAt: item.ObservedAt, Environment: item.Environment, Completeness: item.Completeness, Integrity: item.Integrity, Limitations: item.Limitations, RawSHA256: digest},
		}
		if firstEvidenceID == "" {
			firstEvidenceID = e.ID
		}
		if err := s.repo.SaveEvidence(ctx, e); err != nil {
			return nil, fmt.Errorf("save imported external evidence %q: %w", claim.ID, err)
		}
	}
	return &ImportedExternalEvidence{EvidenceID: firstEvidenceID, Producer: item.Producer, ManifestSHA256: digest, ClaimCount: len(item.Claims), Incomplete: item.Completeness != "complete" || item.Integrity != "verified"}, nil
}

func externalClaimSourceRefs(refs []string, item ExternalEvidenceManifest) []domain.SourceRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]domain.SourceRef, 0, len(refs))
	for _, ref := range refs {
		source := domain.SourceRef{Source: ref, CommitSHA: item.Revision, ObservedAt: item.ObservedAt, AsOf: item.ObservedAt}
		if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
			source.URL = ref
		}
		out = append(out, source)
	}
	return out
}

func DigestExternalEvidenceClaim(manifestDigest string, claim ExternalEvidenceClaim) (string, error) {
	payload, err := json.Marshal(struct {
		Manifest string                `json:"manifest"`
		Claim    ExternalEvidenceClaim `json:"claim"`
	}{manifestDigest, claim})
	if err != nil {
		return "", fmt.Errorf("encode external evidence claim: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func validateExternalEvidenceManifest(item ExternalEvidenceManifest) error {
	if item.SchemaVersion != ExternalEvidenceManifestSchemaV1 || strings.TrimSpace(item.Producer) == "" || strings.TrimSpace(item.InvestigationID) == "" || strings.TrimSpace(item.Repository) == "" || strings.TrimSpace(item.Revision) == "" {
		return errors.New("external evidence manifest schema, producer, investigation_id, repository, and revision are required")
	}
	if item.ObservedAt.IsZero() || len(item.Claims) == 0 || len(item.Claims) > 1000 {
		return errors.New("external evidence manifest must contain 1 to 1000 claims and an observation time")
	}
	if item.Completeness != "complete" && item.Completeness != "incomplete" && item.Completeness != "unknown" {
		return fmt.Errorf("unsupported external evidence completeness %q", item.Completeness)
	}
	if item.Integrity != "verified" && item.Integrity != "unverified" && item.Integrity != "invalid" {
		return fmt.Errorf("unsupported external evidence integrity %q", item.Integrity)
	}
	if item.Integrity == "invalid" {
		return errors.New("external evidence manifest with invalid integrity cannot be imported")
	}
	if item.ArtifactSHA256 != "" {
		raw, err := hex.DecodeString(item.ArtifactSHA256)
		if err != nil || len(raw) != sha256.Size {
			return errors.New("artifact_sha256 must be a 64-character hexadecimal SHA-256")
		}
	}
	shared, err := json.Marshal(struct {
		Environment map[string]string `json:"environment,omitempty"`
		Limitations []string          `json:"limitations,omitempty"`
	}{item.Environment, item.Limitations})
	if err != nil || len(shared) > maxExternalEvidenceSharedMetadataBytes {
		return fmt.Errorf("external evidence environment and limitations exceed %d bytes", maxExternalEvidenceSharedMetadataBytes)
	}
	payload, err := json.Marshal(item)
	if err != nil || len(payload) > maxExternalEvidenceManifestBytes {
		return fmt.Errorf("external evidence manifest exceeds %d bytes", maxExternalEvidenceManifestBytes)
	}
	for _, claim := range item.Claims {
		if strings.TrimSpace(claim.ID) == "" || strings.TrimSpace(claim.Description) == "" {
			return errors.New("external evidence claims require id and description")
		}
		switch claim.Type {
		case EvidenceTypeBaseFailingRegression, EvidenceTypeCandidatePassingRegression, EvidenceTypeMinimalReproduction, EvidenceTypeBenchmark, EvidenceTypeProfiler, EvidenceTypeInvariantViolation, EvidenceTypeCompatibilityMatrix, EvidenceTypeStaticAnalysis, EvidenceTypeManualObservation, EvidenceTypeGitHubSource:
		default:
			return fmt.Errorf("unsupported external evidence type %q", claim.Type)
		}
		switch claim.Relation {
		case RelationSupporting, RelationContradicting, RelationInconclusive, RelationStale, RelationInvalid:
		default:
			return fmt.Errorf("unsupported external evidence relation %q", claim.Relation)
		}
	}
	return nil
}

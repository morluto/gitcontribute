package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const ExternalReceiptSchemaV1 = "gitcontribute.external-validation.v1"

const maxExternalReceiptBytes = 2 << 20

// ExternalReceipt is a producer-neutral, bounded execution record. Digest is
// the SHA-256 of the JSON encoding of this structure with ReceiptSHA256 empty.
type ExternalReceipt struct {
	SchemaVersion   string            `json:"schema_version"`
	Producer        string            `json:"producer"`
	ReceiptSHA256   string            `json:"receipt_sha256"`
	ValidationID    string            `json:"validation_id"`
	InvestigationID string            `json:"investigation_id"`
	OpportunityID   string            `json:"opportunity_id,omitempty"`
	Kind            RunKind           `json:"kind"`
	Repository      string            `json:"repository,omitempty"`
	Revision        string            `json:"revision,omitempty"`
	ArtifactSHA256  string            `json:"artifact_sha256,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	ExternalRunID   string            `json:"external_run_id,omitempty"`
	Command         []string          `json:"argv,omitempty"`
	WorkingDir      string            `json:"working_dir,omitempty"`
	Environment     map[string]string `json:"environment,omitempty"`
	Artifacts       map[string]string `json:"artifacts,omitempty"`
	StartedAt       time.Time         `json:"started_at"`
	CompletedAt     time.Time         `json:"completed_at"`
	ExitCode        int               `json:"exit_code"`
	Classification  RunClassification `json:"classification"`
	Stdout          string            `json:"stdout,omitempty"`
	Stderr          string            `json:"stderr,omitempty"`
	Truncated       bool              `json:"truncated,omitempty"`
	Limitations     []string          `json:"limitations,omitempty"`
	Incomplete      bool              `json:"incomplete,omitempty"`
}

// DigestExternalReceipt returns the stable receipt content identity.
func DigestExternalReceipt(receipt ExternalReceipt) (string, error) {
	receipt.ReceiptSHA256 = ""
	payload, err := json.Marshal(receipt)
	if err != nil {
		return "", fmt.Errorf("encode external validation receipt: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// AttachExternalReceipt validates and stores an external observation without
// executing any command or contacting its producer.
func (s *Service) AttachExternalReceipt(ctx context.Context, receipt ExternalReceipt) (*ValidationRun, error) {
	if receipt.SchemaVersion != ExternalReceiptSchemaV1 {
		return nil, fmt.Errorf("unsupported external receipt schema %q", receipt.SchemaVersion)
	}
	if strings.TrimSpace(receipt.Producer) == "" || strings.TrimSpace(receipt.InvestigationID) == "" || strings.TrimSpace(receipt.ValidationID) == "" {
		return nil, errors.New("external receipt producer, investigation_id, and validation_id are required")
	}
	if receipt.Kind != RunKindBase && receipt.Kind != RunKindCandidate {
		return nil, ErrMissingRunKind
	}
	switch receipt.Classification {
	case RunClassificationPassing, RunClassificationFailing, RunClassificationError, RunClassificationCancelled:
	default:
		return nil, fmt.Errorf("unsupported external run classification %q", receipt.Classification)
	}
	if receipt.StartedAt.IsZero() || receipt.CompletedAt.Before(receipt.StartedAt) {
		return nil, errors.New("external receipt timestamps are invalid")
	}
	if len(receipt.Stdout)+len(receipt.Stderr) > maxOutputBytes {
		return nil, ErrInvalidOutputLimit
	}
	if !receipt.Incomplete && (receipt.Repository == "" || receipt.Revision == "" || receipt.ArtifactSHA256 == "") {
		return nil, errors.New("complete external receipt requires repository, revision, and artifact_sha256")
	}
	if receipt.ArtifactSHA256 != "" {
		raw, err := hex.DecodeString(receipt.ArtifactSHA256)
		if err != nil || len(raw) != sha256.Size {
			return nil, errors.New("external receipt artifact_sha256 must be a 64-character hexadecimal SHA-256")
		}
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return nil, fmt.Errorf("encode external validation receipt: %w", err)
	}
	if len(payload) > maxExternalReceiptBytes {
		return nil, fmt.Errorf("external validation receipt exceeds %d bytes", maxExternalReceiptBytes)
	}
	digest, err := DigestExternalReceipt(receipt)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(receipt.ReceiptSHA256, digest) {
		return nil, fmt.Errorf("external receipt digest mismatch: got %q want %q", receipt.ReceiptSHA256, digest)
	}
	definitionKey := sha256.Sum256([]byte(receipt.Producer + "\x00" + receipt.InvestigationID + "\x00" + receipt.ValidationID))
	definitionID := "external-definition-" + hex.EncodeToString(definitionKey[:])
	definition := &ValidationDefinition{
		ID: definitionID, InvestigationID: receipt.InvestigationID, OpportunityID: receipt.OpportunityID,
		Name: receipt.Producer + " external validation", Kind: "external", Command: append([]string(nil), receipt.Command...),
		WorkingDir: receipt.WorkingDir, CreatedAt: receipt.StartedAt.UTC(),
	}
	existing, err := s.repo.GetValidationDefinition(ctx, definitionID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("read external validation definition: %w", err)
	}
	if existing != nil {
		if !slices.Equal(existing.Command, definition.Command) {
			return nil, errors.New("external receipt command differs from the existing validation_id")
		}
	} else if err := s.repo.SaveValidationDefinition(ctx, definition); err != nil {
		return nil, fmt.Errorf("save external validation definition: %w", err)
	}
	run := &ValidationRun{
		ID: "external-run-" + digest, DefinitionID: definitionID, InvestigationID: receipt.InvestigationID,
		OpportunityID: receipt.OpportunityID, Kind: receipt.Kind, StartedAt: receipt.StartedAt.UTC(),
		CompletedAt: receipt.CompletedAt.UTC(), ExitCode: receipt.ExitCode, Stdout: receipt.Stdout,
		Stderr: receipt.Stderr, Truncated: receipt.Truncated, Classification: receipt.Classification,
		ObservationStatus: ObservationNotEvaluated, ExecutionOrigin: "external",
		External: &ExternalReceiptProvenance{
			SchemaVersion: receipt.SchemaVersion, Producer: receipt.Producer, ValidationID: receipt.ValidationID, ReceiptSHA256: digest,
			Repository: receipt.Repository, Revision: receipt.Revision, ArtifactSHA256: receipt.ArtifactSHA256,
			Provider: receipt.Provider, ExternalRunID: receipt.ExternalRunID, Command: append([]string(nil), receipt.Command...),
			WorkingDir: receipt.WorkingDir, Environment: receipt.Environment, Artifacts: receipt.Artifacts,
			Limitations: append([]string(nil), receipt.Limitations...), Incomplete: receipt.Incomplete,
		},
	}
	if err := s.repo.SaveValidationRun(ctx, run); err != nil {
		return nil, fmt.Errorf("save external validation run: %w", err)
	}
	return run, nil
}

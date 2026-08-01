package corpus

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const ReadSnapshotContractVersion = "gitcontribute.corpus-snapshot.v1"

var (
	ErrSnapshotUnavailable = errors.New("snapshot_unavailable")
	ErrSnapshotExpired     = errors.New("snapshot_expired")
)

type SnapshotMaterialization struct {
	Kind            string
	Scope           any
	SourceManifest  any
	DerivedVersions any
	Completeness    any
	Provenance      any
	Payload         any
}

type ReadSnapshotArtifact struct {
	Token                string          `json:"snapshot_token"`
	ContractVersion      string          `json:"contract_version"`
	ObservationWatermark int64           `json:"observation_watermark"`
	Scope                json.RawMessage `json:"scope"`
	SourceManifestSHA256 string          `json:"source_manifest_sha256"`
	DerivedVersions      json.RawMessage `json:"derived_versions"`
	Completeness         json.RawMessage `json:"completeness"`
	Provenance           json.RawMessage `json:"provenance"`
	ArtifactKind         string          `json:"artifact_kind"`
	ArtifactDigest       string          `json:"artifact_digest"`
	Payload              json.RawMessage `json:"payload"`
	CreatedAt            time.Time       `json:"created_at"`
}

func (c *Corpus) MaterializeReadSnapshot(ctx context.Context, in SnapshotMaterialization) (ReadSnapshotArtifact, error) {
	if in.Kind == "" {
		return ReadSnapshotArtifact{}, errors.New("snapshot artifact kind is required")
	}
	marshal := func(name string, value any) ([]byte, error) {
		out, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode snapshot %s: %w", name, err)
		}
		return out, nil
	}
	scope, err := marshal("scope", in.Scope)
	if err != nil {
		return ReadSnapshotArtifact{}, err
	}
	source, err := marshal("source manifest", in.SourceManifest)
	if err != nil {
		return ReadSnapshotArtifact{}, err
	}
	derived, err := marshal("derived versions", in.DerivedVersions)
	if err != nil {
		return ReadSnapshotArtifact{}, err
	}
	complete, err := marshal("completeness", in.Completeness)
	if err != nil {
		return ReadSnapshotArtifact{}, err
	}
	provenance, err := marshal("provenance", in.Provenance)
	if err != nil {
		return ReadSnapshotArtifact{}, err
	}
	payload, err := marshal("payload", in.Payload)
	if err != nil {
		return ReadSnapshotArtifact{}, err
	}
	sourceHash, artifactHash := sha256.Sum256(source), sha256.Sum256(append([]byte(in.Kind+"\x00"), payload...))
	sourceDigest, artifactDigest := hex.EncodeToString(sourceHash[:]), hex.EncodeToString(artifactHash[:])
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return ReadSnapshotArtifact{}, err
	}
	defer func() { _ = tx.Rollback() }()
	watermark, err := transactionCorpusRevision(ctx, tx)
	if err != nil {
		return ReadSnapshotArtifact{}, err
	}
	tokenBody, err := json.Marshal([]any{ReadSnapshotContractVersion, watermark, json.RawMessage(scope), sourceDigest, json.RawMessage(derived), json.RawMessage(complete), artifactDigest})
	if err != nil {
		return ReadSnapshotArtifact{}, fmt.Errorf("encode snapshot token: %w", err)
	}
	tokenHash := sha256.Sum256(tokenBody)
	token := hex.EncodeToString(tokenHash[:])
	created := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO corpus_read_artifacts (digest, kind, payload_json, created_at) VALUES (?, ?, ?, ?)`, artifactDigest, in.Kind, string(payload), encodeTime(created)); err != nil {
		return ReadSnapshotArtifact{}, fmt.Errorf("store read artifact: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO corpus_snapshot_tokens (token, contract_version, observation_watermark, scope_json, source_manifest_sha256, derived_versions_json, completeness_json, provenance_json, artifact_kind, artifact_digest, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, token, ReadSnapshotContractVersion, watermark, string(scope), sourceDigest, string(derived), string(complete), string(provenance), in.Kind, artifactDigest, encodeTime(created)); err != nil {
		return ReadSnapshotArtifact{}, fmt.Errorf("store snapshot token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ReadSnapshotArtifact{}, err
	}
	return c.ResolveReadSnapshot(ctx, token)
}

func (c *Corpus) ResolveReadSnapshot(ctx context.Context, token string) (ReadSnapshotArtifact, error) {
	var out ReadSnapshotArtifact
	var scope, derived, complete, provenance string
	var payload sql.NullString
	var created int64
	err := c.db.QueryRowContext(ctx, `
		SELECT t.contract_version, t.observation_watermark, t.scope_json,
		       t.source_manifest_sha256, t.derived_versions_json,
		       t.completeness_json, t.provenance_json, t.artifact_kind,
		       t.artifact_digest, t.created_at, a.payload_json
		FROM corpus_snapshot_tokens t
		LEFT JOIN corpus_read_artifacts a
		  ON a.digest = t.artifact_digest AND a.kind = t.artifact_kind
		WHERE t.token = ?
	`, token).Scan(
		&out.ContractVersion, &out.ObservationWatermark, &scope,
		&out.SourceManifestSHA256, &derived, &complete, &provenance,
		&out.ArtifactKind, &out.ArtifactDigest, &created, &payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ReadSnapshotArtifact{}, fmt.Errorf("%w: %s", ErrSnapshotUnavailable, token)
	}
	if err != nil {
		return ReadSnapshotArtifact{}, fmt.Errorf("resolve snapshot: %w", err)
	}
	if !payload.Valid {
		return ReadSnapshotArtifact{}, fmt.Errorf("%w: artifact is missing", ErrSnapshotUnavailable)
	}
	out.Token = token
	out.Scope = json.RawMessage(scope)
	out.DerivedVersions = json.RawMessage(derived)
	out.Completeness = json.RawMessage(complete)
	out.Provenance = json.RawMessage(provenance)
	out.Payload = json.RawMessage(payload.String)
	out.CreatedAt = scanTime(created)
	if out.ContractVersion != ReadSnapshotContractVersion {
		return ReadSnapshotArtifact{}, fmt.Errorf("%w: unsupported contract %q", ErrSnapshotExpired, out.ContractVersion)
	}
	artifactHash := sha256.Sum256(append([]byte(out.ArtifactKind+"\x00"), out.Payload...))
	if hex.EncodeToString(artifactHash[:]) != out.ArtifactDigest {
		return ReadSnapshotArtifact{}, fmt.Errorf("%w: artifact digest mismatch", ErrSnapshotUnavailable)
	}
	return out, nil
}

// ResolveReadArtifact reads one immutable digest-bound artifact without
// consulting a mutable projection. It is the resource-plane counterpart to
// MaterializeReadSnapshot for artifacts whose producer returns a digest URI.
func (c *Corpus) ResolveReadArtifact(ctx context.Context, kind, digest string) (ReadSnapshotArtifact, error) {
	if kind == "" {
		return ReadSnapshotArtifact{}, fmt.Errorf("%w: artifact kind is required", ErrSnapshotUnavailable)
	}
	if len(digest) != sha256.Size*2 {
		return ReadSnapshotArtifact{}, fmt.Errorf("%w: artifact digest is not a SHA-256 hex digest", ErrSnapshotUnavailable)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return ReadSnapshotArtifact{}, fmt.Errorf("%w: artifact digest is not hexadecimal", ErrSnapshotUnavailable)
	}
	var payload string
	var created int64
	if err := c.db.QueryRowContext(ctx, `
		SELECT payload_json, created_at
		FROM corpus_read_artifacts
		WHERE kind = ? AND digest = ?
	`, kind, digest).Scan(&payload, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReadSnapshotArtifact{}, fmt.Errorf("%w: %s/%s", ErrSnapshotUnavailable, kind, digest)
		}
		return ReadSnapshotArtifact{}, fmt.Errorf("resolve read artifact: %w", err)
	}
	artifactHash := sha256.Sum256(append([]byte(kind+"\x00"), []byte(payload)...))
	if hex.EncodeToString(artifactHash[:]) != digest {
		return ReadSnapshotArtifact{}, fmt.Errorf("%w: artifact digest mismatch", ErrSnapshotUnavailable)
	}

	out := ReadSnapshotArtifact{ArtifactKind: kind, ArtifactDigest: digest, Payload: json.RawMessage(payload), CreatedAt: scanTime(created)}
	var token, contract, scope, sourceDigest, derived, complete, provenance string
	var watermark, tokenCreated int64
	if err := c.db.QueryRowContext(ctx, `
		SELECT token, contract_version, observation_watermark, scope_json,
		       source_manifest_sha256, derived_versions_json, completeness_json,
		       provenance_json, created_at
		FROM corpus_snapshot_tokens
		WHERE artifact_kind = ? AND artifact_digest = ?
		ORDER BY created_at DESC, token DESC
		LIMIT 1
	`, kind, digest).Scan(&token, &contract, &watermark, &scope, &sourceDigest, &derived, &complete, &provenance, &tokenCreated); err == nil {
		out.Token, out.ContractVersion, out.ObservationWatermark = token, contract, watermark
		out.Scope, out.SourceManifestSHA256 = json.RawMessage(scope), sourceDigest
		out.DerivedVersions, out.Completeness, out.Provenance = json.RawMessage(derived), json.RawMessage(complete), json.RawMessage(provenance)
		if tokenCreated > 0 {
			out.CreatedAt = scanTime(tokenCreated)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ReadSnapshotArtifact{}, fmt.Errorf("resolve read artifact metadata: %w", err)
	}
	return out, nil
}

-- +goose Up
-- +goose StatementBegin
CREATE TABLE code_index_artifacts (
    digest TEXT PRIMARY KEY,
    snapshot_id INTEGER NOT NULL,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    manifest_sha256 TEXT NOT NULL,
    manifest_json TEXT NOT NULL,
    snapshot_token TEXT NOT NULL,
    corpus_revision INTEGER NOT NULL,
    coverage_known INTEGER NOT NULL,
    indexed_files INTEGER NOT NULL,
    tracked_entries INTEGER NOT NULL,
    truncated INTEGER NOT NULL,
    schema_version TEXT NOT NULL,
    provenance TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (snapshot_id) REFERENCES code_snapshots (id) ON DELETE CASCADE
);
CREATE INDEX code_index_artifacts_commit_idx
ON code_index_artifacts (repo_owner, repo_name, commit_sha, created_at DESC, digest DESC);

CREATE TABLE corpus_snapshot_tokens (
    token TEXT PRIMARY KEY,
    contract_version TEXT NOT NULL,
    observation_watermark INTEGER NOT NULL,
    scope_json TEXT NOT NULL,
    source_manifest_sha256 TEXT NOT NULL,
    derived_versions_json TEXT NOT NULL,
    completeness_json TEXT NOT NULL,
    provenance_json TEXT NOT NULL,
    artifact_kind TEXT NOT NULL,
    artifact_digest TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE TABLE corpus_read_artifacts (
    digest TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS corpus_snapshot_tokens;
DROP TABLE IF EXISTS corpus_read_artifacts;
DROP INDEX IF EXISTS code_index_artifacts_commit_idx;
DROP TABLE IF EXISTS code_index_artifacts;
-- +goose StatementEnd

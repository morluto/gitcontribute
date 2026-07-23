-- +goose Up
-- +goose StatementBegin
CREATE TABLE contribution_manifests (
    id TEXT PRIMARY KEY,
    opportunity_id TEXT NOT NULL,
    workspace_id TEXT NOT NULL DEFAULT '',
    pull_request_ref TEXT NOT NULL DEFAULT '',
    content_sha256 TEXT NOT NULL,
    payload TEXT NOT NULL,
    generated_at INTEGER NOT NULL,
    FOREIGN KEY (opportunity_id) REFERENCES opportunities (id) ON DELETE CASCADE
);
CREATE INDEX idx_contribution_manifests_opportunity_generated
    ON contribution_manifests (opportunity_id, generated_at DESC, id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS contribution_manifests;
-- +goose StatementEnd

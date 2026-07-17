-- +goose Up
CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    investigation_id TEXT,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_workspaces_investigation ON workspaces (investigation_id);

-- +goose Down
DROP TABLE IF EXISTS workspaces;

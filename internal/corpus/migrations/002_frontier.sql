-- +goose Up
-- +goose StatementBegin

CREATE TABLE frontier_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    work_key TEXT NOT NULL UNIQUE,
    subject_kind TEXT NOT NULL,
    owner TEXT,
    repo TEXT,
    thread_kind TEXT,
    thread_number INTEGER,
    facet TEXT,
    priority INTEGER NOT NULL DEFAULT 0,
    reason TEXT,
    source TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    earliest_run_at INTEGER NOT NULL DEFAULT 0,
    budget_estimate INTEGER NOT NULL DEFAULT 1,
    state TEXT NOT NULL DEFAULT 'queued',
    lease_owner TEXT,
    lease_expires_at INTEGER,
    failure_kind TEXT,
    last_error TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX idx_frontier_ready
    ON frontier_items (state, earliest_run_at, priority DESC, id);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS frontier_items;

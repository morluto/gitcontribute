-- +goose Up
-- +goose StatementBegin
-- Frontier rows were scheduling hints for an executor retired before v3. They
-- are not source observations or projections, and no supported operation can
-- consume them. Retire the orphaned queue instead of reporting permanently
-- ready work.
DROP TRIGGER IF EXISTS corpus_revision_frontier_items_ai;
DROP TRIGGER IF EXISTS corpus_revision_frontier_items_au;
DROP TRIGGER IF EXISTS corpus_revision_frontier_items_ad;
DROP TABLE IF EXISTS frontier_items;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Schema rollback recreates an empty legacy queue. Retired scheduling hints
-- cannot be reconstructed; explicit corpus migration creates a verified backup
-- before this migration is applied.
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

CREATE TRIGGER corpus_revision_frontier_items_ai
AFTER INSERT ON frontier_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_frontier_items_au
AFTER UPDATE ON frontier_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_frontier_items_ad
AFTER DELETE ON frontier_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;
-- +goose StatementEnd

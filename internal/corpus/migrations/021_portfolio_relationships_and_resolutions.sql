-- +goose Up
-- +goose StatementBegin

-- Explicit local relationships are deliberately separate from GitHub state.
-- A row may connect a PR to an opportunity, a workspace, or both.
CREATE TABLE portfolio_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pull_request_thread_id INTEGER NOT NULL,
    opportunity_id TEXT,
    workspace_id TEXT,
    created_at INTEGER NOT NULL,
    CHECK (opportunity_id IS NOT NULL OR workspace_id IS NOT NULL),
    FOREIGN KEY (pull_request_thread_id) REFERENCES threads (id) ON DELETE CASCADE,
    FOREIGN KEY (opportunity_id) REFERENCES opportunities (id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_portfolio_links_identity
    ON portfolio_links (pull_request_thread_id, COALESCE(opportunity_id, ''), COALESCE(workspace_id, ''));
CREATE INDEX idx_portfolio_links_opportunity ON portfolio_links (opportunity_id, pull_request_thread_id);
CREATE INDEX idx_portfolio_links_workspace ON portfolio_links (workspace_id, pull_request_thread_id);

-- Each signal snapshot is immutable. The projection selects only the newest
-- complete snapshot, and its child rows therefore become visible atomically.
CREATE TABLE portfolio_signal_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subject_kind TEXT NOT NULL,
    subject_ref TEXT NOT NULL,
    facet TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    source_observation_refs TEXT NOT NULL,
    observed_at INTEGER NOT NULL
);

CREATE INDEX idx_portfolio_signal_snapshots_subject
    ON portfolio_signal_snapshots (subject_kind, subject_ref, facet, source_updated_at DESC, observation_sequence DESC);

CREATE TABLE portfolio_signals (
    snapshot_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    kind TEXT NOT NULL,
    value TEXT NOT NULL,
    target_kind TEXT,
    target_ref TEXT,
    score REAL,
    PRIMARY KEY (snapshot_id, position),
    FOREIGN KEY (snapshot_id) REFERENCES portfolio_signal_snapshots (id) ON DELETE CASCADE
);

CREATE INDEX idx_portfolio_signals_lookup ON portfolio_signals (kind, value);
CREATE INDEX idx_portfolio_signals_target ON portfolio_signals (target_kind, target_ref, kind);

CREATE TABLE portfolio_signal_projections (
    subject_kind TEXT NOT NULL,
    subject_ref TEXT NOT NULL,
    facet TEXT NOT NULL,
    snapshot_id INTEGER NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    PRIMARY KEY (subject_kind, subject_ref, facet),
    FOREIGN KEY (snapshot_id) REFERENCES portfolio_signal_snapshots (id) ON DELETE CASCADE
);

-- Resolution records are append-only derivations. The projection is advanced
-- by source order so stale derivations cannot replace newer conclusions.
CREATE TABLE resolution_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    summary TEXT NOT NULL,
    rule_version TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    source_observation_refs TEXT NOT NULL,
    derived_at INTEGER NOT NULL,
    FOREIGN KEY (thread_id) REFERENCES threads (id) ON DELETE CASCADE
);

CREATE INDEX idx_resolution_records_thread
    ON resolution_records (thread_id, source_updated_at DESC, observation_sequence DESC);

CREATE TABLE resolution_projections (
    thread_id INTEGER PRIMARY KEY,
    resolution_record_id INTEGER NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    FOREIGN KEY (thread_id) REFERENCES threads (id) ON DELETE CASCADE,
    FOREIGN KEY (resolution_record_id) REFERENCES resolution_records (id) ON DELETE CASCADE
);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS resolution_projections;
DROP TABLE IF EXISTS resolution_records;
DROP TABLE IF EXISTS portfolio_signal_projections;
DROP TABLE IF EXISTS portfolio_signals;
DROP TABLE IF EXISTS portfolio_signal_snapshots;
DROP TABLE IF EXISTS portfolio_links;

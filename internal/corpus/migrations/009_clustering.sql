-- +goose Up
-- +goose StatementBegin

CREATE TABLE cluster_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    source_revision TEXT NOT NULL,
    source_window_start INTEGER NOT NULL,
    source_window_end INTEGER NOT NULL,
    params_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    stats TEXT
);

CREATE INDEX idx_cluster_runs_repo ON cluster_runs (repo_owner, repo_name, completed_at DESC);

CREATE TABLE clusters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    stable_id TEXT NOT NULL,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    state TEXT NOT NULL,
    canonical_kind TEXT NOT NULL,
    canonical_owner TEXT NOT NULL,
    canonical_repo TEXT NOT NULL,
    canonical_number INTEGER NOT NULL,
    source_revision TEXT NOT NULL,
    source_window_start INTEGER NOT NULL,
    source_window_end INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (stable_id)
);

CREATE INDEX idx_clusters_repo_state ON clusters (repo_owner, repo_name, state);
CREATE INDEX idx_clusters_canonical ON clusters (canonical_kind, canonical_owner, canonical_repo, canonical_number);

CREATE TABLE cluster_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cluster_id INTEGER NOT NULL,
    thread_id INTEGER,
    kind TEXT NOT NULL,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    title TEXT NOT NULL,
    state TEXT NOT NULL,
    score REAL NOT NULL,
    reason TEXT NOT NULL,
    included INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (cluster_id) REFERENCES clusters (id) ON DELETE CASCADE,
    FOREIGN KEY (thread_id) REFERENCES threads (id),
    UNIQUE (cluster_id, kind, owner, repo, number)
);

CREATE INDEX idx_cluster_members_cluster ON cluster_members (cluster_id);
CREATE INDEX idx_cluster_members_ref ON cluster_members (kind, owner, repo, number);

CREATE TABLE cluster_overrides (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cluster_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    action TEXT NOT NULL,
    reason TEXT NOT NULL,
    target_cluster_id INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (cluster_id) REFERENCES clusters (id) ON DELETE CASCADE,
    FOREIGN KEY (target_cluster_id) REFERENCES clusters (id) ON DELETE CASCADE
);

CREATE INDEX idx_cluster_overrides_cluster ON cluster_overrides (cluster_id);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS cluster_overrides;
DROP TABLE IF EXISTS cluster_members;
DROP TABLE IF EXISTS clusters;
DROP TABLE IF EXISTS cluster_runs;

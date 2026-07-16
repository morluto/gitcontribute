-- +goose Up
-- +goose StatementBegin

CREATE TABLE observation_sequences (
    name TEXT PRIMARY KEY,
    next_value INTEGER NOT NULL
);

CREATE TABLE repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    external_id TEXT,
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (owner, name)
);

CREATE TABLE repository_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    payload TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories (id)
);

CREATE TABLE threads (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    number INTEGER NOT NULL,
    state TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    author TEXT,
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (repository_id, kind, number),
    FOREIGN KEY (repository_id) REFERENCES repositories (id)
);

CREATE TABLE thread_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id INTEGER NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    payload TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (thread_id) REFERENCES threads (id)
);

CREATE TABLE facet_coverage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    thread_id INTEGER,
    facet TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    complete INTEGER NOT NULL DEFAULT 0,
    run_id INTEGER,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories (id),
    FOREIGN KEY (thread_id) REFERENCES threads (id)
);

CREATE UNIQUE INDEX idx_facet_coverage_unq
    ON facet_coverage (repository_id, COALESCE(thread_id, -1), facet);

CREATE TABLE runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    stats TEXT,
    error TEXT
);

CREATE TABLE run_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    recorded_at INTEGER NOT NULL,
    FOREIGN KEY (run_id) REFERENCES runs (id)
);

CREATE VIRTUAL TABLE threads_fts USING fts5(
    title,
    body,
    content='threads',
    content_rowid='id'
);

CREATE TRIGGER threads_fts_insert AFTER INSERT ON threads
BEGIN
    INSERT INTO threads_fts (rowid, title, body)
        VALUES (new.id, new.title, COALESCE(new.body, ''));
END;

CREATE TRIGGER threads_fts_update AFTER UPDATE ON threads
BEGIN
    INSERT INTO threads_fts (threads_fts, rowid, title, body)
        VALUES ('delete', old.id, old.title, COALESCE(old.body, ''));
    INSERT INTO threads_fts (rowid, title, body)
        VALUES (new.id, new.title, COALESCE(new.body, ''));
END;

CREATE TRIGGER threads_fts_delete AFTER DELETE ON threads
BEGIN
    INSERT INTO threads_fts (threads_fts, rowid, title, body)
        VALUES ('delete', old.id, old.title, COALESCE(old.body, ''));
END;

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS run_events;
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS facet_coverage;
DROP TABLE IF EXISTS thread_observations;
DROP TABLE IF EXISTS threads_fts;
DROP TABLE IF EXISTS threads;
DROP TABLE IF EXISTS repository_observations;
DROP TABLE IF EXISTS repositories;
DROP TABLE IF EXISTS observation_sequences;

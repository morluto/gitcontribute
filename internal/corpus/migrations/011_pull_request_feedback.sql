-- +goose Up
-- +goose StatementBegin
CREATE TABLE pull_request_feedback_discovery (
    repository_id INTEGER PRIMARY KEY,
    state TEXT NOT NULL DEFAULT 'all',
    next_page INTEGER NOT NULL DEFAULT 1,
    complete INTEGER NOT NULL DEFAULT 0 CHECK (complete IN (0, 1)),
    truncated INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
    discovered_pull_requests INTEGER NOT NULL DEFAULT 0,
    requests INTEGER NOT NULL DEFAULT 0,
    channels_json TEXT NOT NULL DEFAULT '[]',
    thread_state TEXT NOT NULL DEFAULT 'all',
    last_error TEXT NOT NULL DEFAULT '',
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories (id) ON DELETE CASCADE
);

CREATE TABLE pull_request_feedback_projection (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    thread_id INTEGER NOT NULL,
    channel TEXT NOT NULL,
    feedback_id TEXT NOT NULL,
    thread_external_id TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    line INTEGER,
    start_line INTEGER,
    commit_oid TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL DEFAULT 0,
    resolved_known INTEGER NOT NULL DEFAULT 0 CHECK (resolved_known IN (0, 1)),
    resolved INTEGER NOT NULL DEFAULT 0 CHECK (resolved IN (0, 1)),
    outdated INTEGER NOT NULL DEFAULT 0 CHECK (outdated IN (0, 1)),
    head_sha TEXT NOT NULL DEFAULT '',
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    source_observation_sequence INTEGER NOT NULL DEFAULT 0,
    source_observation_id INTEGER NOT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories (id) ON DELETE CASCADE,
    FOREIGN KEY (thread_id) REFERENCES threads (id) ON DELETE CASCADE,
    UNIQUE (repository_id, thread_id, channel, feedback_id, author)
);
CREATE INDEX idx_pr_feedback_projection_repo ON pull_request_feedback_projection (repository_id, channel, id);
CREATE INDEX idx_pr_feedback_projection_author ON pull_request_feedback_projection (author, repository_id, channel, id);
CREATE INDEX idx_pr_feedback_projection_created ON pull_request_feedback_projection (created_at, id);
CREATE INDEX idx_pr_feedback_projection_updated ON pull_request_feedback_projection (updated_at, id);
CREATE INDEX idx_pr_feedback_projection_thread ON pull_request_feedback_projection (thread_id, channel, id);

CREATE VIRTUAL TABLE pull_request_feedback_fts USING fts5(
    author,
    body,
    path,
    content='pull_request_feedback_projection',
    content_rowid='id'
);
CREATE TRIGGER pull_request_feedback_fts_insert AFTER INSERT ON pull_request_feedback_projection BEGIN
    INSERT INTO pull_request_feedback_fts (rowid, author, body, path)
        VALUES (new.id, new.author, new.body, new.path);
END;
CREATE TRIGGER pull_request_feedback_fts_update AFTER UPDATE ON pull_request_feedback_projection BEGIN
    INSERT INTO pull_request_feedback_fts (pull_request_feedback_fts, rowid, author, body, path)
        VALUES ('delete', old.id, old.author, old.body, old.path);
    INSERT INTO pull_request_feedback_fts (rowid, author, body, path)
        VALUES (new.id, new.author, new.body, new.path);
END;
CREATE TRIGGER pull_request_feedback_fts_delete AFTER DELETE ON pull_request_feedback_projection BEGIN
    INSERT INTO pull_request_feedback_fts (pull_request_feedback_fts, rowid, author, body, path)
        VALUES ('delete', old.id, old.author, old.body, old.path);
END;
INSERT INTO pull_request_feedback_fts (pull_request_feedback_fts) VALUES ('rebuild');

INSERT INTO projection_states (name, version, status, refreshed_at, row_count, source_revision, content_hash)
VALUES ('pull_request_feedback_fts', 'pull-request-feedback-fts-v1', 'current',
        (strftime('%s','now') * 1000000000), 0, '', '');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM projection_states WHERE name = 'pull_request_feedback_fts';
DROP TRIGGER IF EXISTS pull_request_feedback_fts_insert;
DROP TRIGGER IF EXISTS pull_request_feedback_fts_update;
DROP TRIGGER IF EXISTS pull_request_feedback_fts_delete;
DROP TABLE IF EXISTS pull_request_feedback_fts;
DROP INDEX IF EXISTS idx_pr_feedback_projection_thread;
DROP INDEX IF EXISTS idx_pr_feedback_projection_updated;
DROP INDEX IF EXISTS idx_pr_feedback_projection_created;
DROP INDEX IF EXISTS idx_pr_feedback_projection_author;
DROP INDEX IF EXISTS idx_pr_feedback_projection_repo;
DROP TABLE IF EXISTS pull_request_feedback_projection;
DROP TABLE IF EXISTS pull_request_feedback_discovery;
-- +goose StatementEnd

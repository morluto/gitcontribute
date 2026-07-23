-- +goose Up
-- +goose StatementBegin
CREATE TABLE concerns (
    rowid INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    commit_sha TEXT NOT NULL DEFAULT '',
    workspace_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    problem_statement TEXT NOT NULL,
    suspected_owner TEXT NOT NULL DEFAULT '',
    unknowns TEXT NOT NULL DEFAULT '',
    success_criterion TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('untriaged', 'accepted', 'investigating', 'deferred', 'promoted', 'resolved')),
    confidence REAL NOT NULL CHECK (confidence >= 0.0 AND confidence <= 1.0),
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_concerns_repo_status_updated
    ON concerns (repo_owner, repo_name, status, updated_at DESC, id);

CREATE TABLE concern_links (
    concern_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('related', 'duplicate_candidate', 'hotspot', 'investigation', 'opportunity')),
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    PRIMARY KEY (concern_id, kind, target_type, target_id),
    FOREIGN KEY (concern_id) REFERENCES concerns (id) ON DELETE CASCADE
);
CREATE INDEX idx_concern_links_target ON concern_links (target_type, target_id, kind);

CREATE VIRTUAL TABLE concerns_fts USING fts5(
    title,
    problem_statement,
    suspected_owner,
    unknowns,
    success_criterion,
    content='concerns',
    content_rowid='rowid'
);
CREATE TRIGGER concerns_fts_insert AFTER INSERT ON concerns BEGIN
    INSERT INTO concerns_fts (rowid, title, problem_statement, suspected_owner, unknowns, success_criterion)
    VALUES (new.rowid, new.title, new.problem_statement, new.suspected_owner, new.unknowns, new.success_criterion);
END;
CREATE TRIGGER concerns_fts_update AFTER UPDATE ON concerns BEGIN
    INSERT INTO concerns_fts (concerns_fts, rowid, title, problem_statement, suspected_owner, unknowns, success_criterion)
    VALUES ('delete', old.rowid, old.title, old.problem_statement, old.suspected_owner, old.unknowns, old.success_criterion);
    INSERT INTO concerns_fts (rowid, title, problem_statement, suspected_owner, unknowns, success_criterion)
    VALUES (new.rowid, new.title, new.problem_statement, new.suspected_owner, new.unknowns, new.success_criterion);
END;
CREATE TRIGGER concerns_fts_delete AFTER DELETE ON concerns BEGIN
    INSERT INTO concerns_fts (concerns_fts, rowid, title, problem_statement, suspected_owner, unknowns, success_criterion)
    VALUES ('delete', old.rowid, old.title, old.problem_statement, old.suspected_owner, old.unknowns, old.success_criterion);
END;
INSERT INTO concerns_fts (concerns_fts) VALUES ('rebuild');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS concerns_fts_delete;
DROP TRIGGER IF EXISTS concerns_fts_update;
DROP TRIGGER IF EXISTS concerns_fts_insert;
DROP TABLE IF EXISTS concerns_fts;
DROP TABLE IF EXISTS concern_links;
DROP TABLE IF EXISTS concerns;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
CREATE TABLE code_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    repo_path TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    total_bytes INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (repo_owner, repo_name, commit_sha)
);

CREATE TABLE code_documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    content TEXT NOT NULL,
    bytes INTEGER NOT NULL,
    language TEXT,
    UNIQUE (snapshot_id, path),
    FOREIGN KEY (snapshot_id) REFERENCES code_snapshots (id) ON DELETE CASCADE
);

CREATE VIRTUAL TABLE code_documents_fts USING fts5(
    path,
    content,
    content='code_documents',
    content_rowid='id'
);

CREATE TRIGGER code_documents_fts_insert AFTER INSERT ON code_documents BEGIN
    INSERT INTO code_documents_fts (rowid, path, content) VALUES (new.id, new.path, new.content);
END;
CREATE TRIGGER code_documents_fts_delete AFTER DELETE ON code_documents BEGIN
    INSERT INTO code_documents_fts (code_documents_fts, rowid, path, content)
        VALUES ('delete', old.id, old.path, old.content);
END;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS code_documents_fts;
DROP TABLE IF EXISTS code_documents;
DROP TABLE IF EXISTS code_snapshots;

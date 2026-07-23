-- +goose Up
-- +goose StatementBegin
CREATE VIRTUAL TABLE repositories_fts USING fts5(
    owner,
    name,
    topics,
    description,
    content='repositories',
    content_rowid='id'
);
CREATE TRIGGER repositories_fts_insert AFTER INSERT ON repositories
BEGIN
    INSERT INTO repositories_fts (rowid, owner, name, topics, description)
        VALUES (new.id, new.owner, new.name, COALESCE(new.topics, ''), COALESCE(new.description, ''));
END;
CREATE TRIGGER repositories_fts_update AFTER UPDATE ON repositories
BEGIN
    INSERT INTO repositories_fts (repositories_fts, rowid, owner, name, topics, description)
        VALUES ('delete', old.id, old.owner, old.name, COALESCE(old.topics, ''), COALESCE(old.description, ''));
    INSERT INTO repositories_fts (rowid, owner, name, topics, description)
        VALUES (new.id, new.owner, new.name, COALESCE(new.topics, ''), COALESCE(new.description, ''));
END;
CREATE TRIGGER repositories_fts_delete AFTER DELETE ON repositories
BEGIN
    INSERT INTO repositories_fts (repositories_fts, rowid, owner, name, topics, description)
        VALUES ('delete', old.id, old.owner, old.name, COALESCE(old.topics, ''), COALESCE(old.description, ''));
END;
INSERT INTO repositories_fts (repositories_fts) VALUES ('rebuild');
INSERT INTO projection_states (name, version, status, refreshed_at, row_count, source_revision, content_hash)
VALUES ('repositories_fts', 'repositories-fts-v1', 'current', (strftime('%s','now') * 1000000000),
        (SELECT COUNT(*) FROM repositories), '', '');
CREATE TRIGGER projection_repositories_insert AFTER INSERT ON repositories BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000),
        row_count = row_count + 1, source_revision = '', content_hash = ''
    WHERE name = 'repositories_fts';
END;
CREATE TRIGGER projection_repositories_update AFTER UPDATE OF owner, name, topics, description ON repositories BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000),
        source_revision = '', content_hash = ''
    WHERE name = 'repositories_fts';
END;
CREATE TRIGGER projection_repositories_delete AFTER DELETE ON repositories BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000),
        row_count = MAX(row_count - 1, 0), source_revision = '', content_hash = ''
    WHERE name = 'repositories_fts';
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS repositories_fts_insert;
DROP TRIGGER IF EXISTS repositories_fts_update;
DROP TRIGGER IF EXISTS repositories_fts_delete;
DROP TRIGGER IF EXISTS projection_repositories_insert;
DROP TRIGGER IF EXISTS projection_repositories_update;
DROP TRIGGER IF EXISTS projection_repositories_delete;
DELETE FROM projection_states WHERE name = 'repositories_fts';
DROP TABLE IF EXISTS repositories_fts;
-- +goose StatementEnd

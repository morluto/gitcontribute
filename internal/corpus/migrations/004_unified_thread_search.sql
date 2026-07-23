-- +goose Up
-- +goose StatementBegin
DROP TRIGGER IF EXISTS threads_fts_insert;
DROP TRIGGER IF EXISTS threads_fts_update;
DROP TRIGGER IF EXISTS threads_fts_delete;
DROP TABLE IF EXISTS threads_fts;
DROP TRIGGER IF EXISTS projection_threads_insert;
DROP TRIGGER IF EXISTS projection_threads_update;
DROP TRIGGER IF EXISTS projection_threads_delete;
DROP TRIGGER IF EXISTS projection_threads_revision_insert;
DROP TRIGGER IF EXISTS projection_threads_revision_update;
DROP TRIGGER IF EXISTS projection_threads_revision_delete;

CREATE TABLE thread_search_documents (
    thread_id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    facets TEXT NOT NULL DEFAULT '',
    facets_updated_at INTEGER NOT NULL DEFAULT 0,
    facets_truncated INTEGER NOT NULL DEFAULT 0 CHECK (facets_truncated IN (0, 1)),
    FOREIGN KEY (thread_id) REFERENCES threads (id) ON DELETE CASCADE
);
INSERT INTO thread_search_documents (thread_id, title, labels, body, facets, facets_updated_at, facets_truncated)
SELECT t.id, t.title, COALESCE(t.labels, ''), COALESCE(t.body, ''),
       substr(COALESCE((SELECT group_concat(search_text, char(10)) FROM (SELECT search_text FROM facet_observations fo WHERE fo.thread_id = t.id ORDER BY fo.observation_sequence)), ''), 1, 262144),
       COALESCE((SELECT MAX(fo.source_updated_at) FROM facet_observations fo WHERE fo.thread_id = t.id), 0),
       length(COALESCE((SELECT group_concat(search_text, char(10)) FROM (SELECT search_text FROM facet_observations fo WHERE fo.thread_id = t.id ORDER BY fo.observation_sequence)), '')) > 262144
FROM threads t;

CREATE VIRTUAL TABLE threads_fts USING fts5(
    title,
    labels,
    body,
    facets,
    content='thread_search_documents',
    content_rowid='thread_id'
);
INSERT INTO threads_fts (threads_fts) VALUES ('rebuild');

CREATE TRIGGER thread_search_documents_fts_insert AFTER INSERT ON thread_search_documents BEGIN
    INSERT INTO threads_fts (rowid, title, labels, body, facets)
        VALUES (new.thread_id, new.title, new.labels, new.body, new.facets);
END;
CREATE TRIGGER thread_search_documents_fts_update AFTER UPDATE ON thread_search_documents BEGIN
    INSERT INTO threads_fts (threads_fts, rowid, title, labels, body, facets)
        VALUES ('delete', old.thread_id, old.title, old.labels, old.body, old.facets);
    INSERT INTO threads_fts (rowid, title, labels, body, facets)
        VALUES (new.thread_id, new.title, new.labels, new.body, new.facets);
END;
CREATE TRIGGER thread_search_documents_fts_delete AFTER DELETE ON thread_search_documents BEGIN
    INSERT INTO threads_fts (threads_fts, rowid, title, labels, body, facets)
        VALUES ('delete', old.thread_id, old.title, old.labels, old.body, old.facets);
END;
CREATE TRIGGER projection_thread_search_document_insert AFTER INSERT ON thread_search_documents BEGIN
    UPDATE projection_states
    SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = row_count + 1,
        source_revision = '', content_hash = ''
    WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_thread_search_document_update AFTER UPDATE ON thread_search_documents BEGIN
    UPDATE projection_states
    SET refreshed_at = (strftime('%s','now') * 1000000000), source_revision = '', content_hash = ''
    WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_thread_search_document_delete AFTER DELETE ON thread_search_documents BEGIN
    UPDATE projection_states
    SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = MAX(row_count - 1, 0),
        source_revision = '', content_hash = ''
    WHERE name = 'threads_fts';
END;

CREATE TRIGGER threads_search_document_insert AFTER INSERT ON threads BEGIN
    INSERT INTO thread_search_documents (thread_id, title, labels, body)
        VALUES (new.id, new.title, COALESCE(new.labels, ''), COALESCE(new.body, ''));
END;
CREATE TRIGGER threads_search_document_update AFTER UPDATE OF title, labels, body ON threads BEGIN
    UPDATE thread_search_documents
    SET title = new.title, labels = COALESCE(new.labels, ''), body = COALESCE(new.body, '')
    WHERE thread_id = new.id;
END;
CREATE TRIGGER threads_search_document_delete AFTER DELETE ON threads BEGIN
    DELETE FROM thread_search_documents WHERE thread_id = old.id;
END;

UPDATE projection_states
SET version = 'threads-fts-v3', status = 'current', refreshed_at = (strftime('%s','now') * 1000000000),
    row_count = (SELECT COUNT(*) FROM thread_search_documents), source_revision = '', content_hash = ''
WHERE name = 'threads_fts';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS facet_search_document_update;
DROP TRIGGER IF EXISTS facet_search_document_delete;
DROP TRIGGER IF EXISTS facet_search_document_insert;
DROP TRIGGER IF EXISTS threads_search_document_delete;
DROP TRIGGER IF EXISTS threads_search_document_update;
DROP TRIGGER IF EXISTS threads_search_document_insert;
DROP TRIGGER IF EXISTS thread_search_documents_fts_delete;
DROP TRIGGER IF EXISTS thread_search_documents_fts_update;
DROP TRIGGER IF EXISTS thread_search_documents_fts_insert;
DROP TRIGGER IF EXISTS projection_thread_search_document_delete;
DROP TRIGGER IF EXISTS projection_thread_search_document_update;
DROP TRIGGER IF EXISTS projection_thread_search_document_insert;
DROP TABLE IF EXISTS threads_fts;
DROP TABLE IF EXISTS thread_search_documents;

CREATE VIRTUAL TABLE threads_fts USING fts5(
    title,
    body,
    content='threads',
    content_rowid='id'
);
CREATE TRIGGER threads_fts_insert AFTER INSERT ON threads BEGIN
    INSERT INTO threads_fts (rowid, title, body)
        VALUES (new.id, new.title, COALESCE(new.body, ''));
END;
CREATE TRIGGER threads_fts_update AFTER UPDATE ON threads BEGIN
    INSERT INTO threads_fts (threads_fts, rowid, title, body)
        VALUES ('delete', old.id, old.title, COALESCE(old.body, ''));
    INSERT INTO threads_fts (rowid, title, body)
        VALUES (new.id, new.title, COALESCE(new.body, ''));
END;
CREATE TRIGGER threads_fts_delete AFTER DELETE ON threads BEGIN
    INSERT INTO threads_fts (threads_fts, rowid, title, body)
        VALUES ('delete', old.id, old.title, COALESCE(old.body, ''));
END;
INSERT INTO threads_fts (threads_fts) VALUES ('rebuild');
CREATE TRIGGER projection_threads_insert AFTER INSERT ON threads BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = row_count + 1 WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_update AFTER UPDATE OF title, body ON threads BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000) WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_delete AFTER DELETE ON threads BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = MAX(row_count - 1, 0) WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_revision_insert AFTER INSERT ON threads BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_revision_update AFTER UPDATE OF title, body ON threads BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_revision_delete AFTER DELETE ON threads BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'threads_fts';
END;
UPDATE projection_states SET version = 'threads-fts-v1', source_revision = '', content_hash = '' WHERE name = 'threads_fts';
-- +goose StatementEnd

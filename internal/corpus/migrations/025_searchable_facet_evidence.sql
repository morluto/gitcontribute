-- +goose Up
-- +goose StatementBegin

ALTER TABLE facet_observations ADD COLUMN search_text TEXT NOT NULL DEFAULT '';

-- Recover searchable text for existing snapshots without indexing serialized
-- field names, URLs, or unrelated adapter metadata. Hydration writes the same
-- product-selected fields directly for new observations.
UPDATE facet_observations
SET search_text = CASE facet
    WHEN 'issue_comments' THEN COALESCE((
        SELECT group_concat(trim(
            COALESCE(json_extract(value, '$.Author'), '') || ' ' ||
            COALESCE(json_extract(value, '$.Body'), '')
        ), char(10))
        FROM json_each(facet_observations.payload)
    ), '')
    WHEN 'pr_reviews' THEN COALESCE((
        SELECT group_concat(trim(
            COALESCE(json_extract(value, '$.Author'), '') || ' ' ||
            COALESCE(json_extract(value, '$.State'), '') || ' ' ||
            COALESCE(json_extract(value, '$.Body'), '')
        ), char(10))
        FROM json_each(facet_observations.payload)
    ), '')
    WHEN 'pr_review_comments' THEN COALESCE((
        SELECT group_concat(trim(
            COALESCE(json_extract(value, '$.Author'), '') || ' ' ||
            COALESCE(json_extract(value, '$.Path'), '') || ' ' ||
            COALESCE(json_extract(value, '$.Body'), '')
        ), char(10))
        FROM json_each(facet_observations.payload)
    ), '')
    WHEN 'issue_timeline' THEN COALESCE((
        SELECT group_concat(trim(
            COALESCE(json_extract(value, '$.Event'), '') || ' ' ||
            COALESCE(json_extract(value, '$.Actor'), '') || ' ' ||
            COALESCE(json_extract(value, '$.CommitID'), '') || ' ' ||
            COALESCE(json_extract(value, '$.SourceOwner'), '') || ' ' ||
            COALESCE(json_extract(value, '$.SourceRepository'), '') || ' ' ||
            CASE
                WHEN COALESCE(json_extract(value, '$.SourceNumber'), 0) > 0
                THEN CAST(json_extract(value, '$.SourceNumber') AS TEXT) || ' ' ||
                     CASE WHEN json_extract(value, '$.SourceIsPullRequest') THEN 'pull request' ELSE 'issue' END
                ELSE ''
            END
        ), char(10))
        FROM json_each(facet_observations.payload)
    ), '')
    ELSE ''
END
WHERE json_valid(payload) AND json_type(payload) = 'array';

-- Pagination is a transport detail, so one complete facet snapshot is one
-- search document. Store its aggregate on the first observation and leave the
-- remaining page rows empty while retaining their immutable source payloads.
UPDATE facet_observations AS target
SET search_text = COALESCE((
    SELECT group_concat(source.search_text, char(10))
    FROM facet_observations AS source
    WHERE source.repository_id = target.repository_id
      AND COALESCE(source.thread_id, -1) = COALESCE(target.thread_id, -1)
      AND source.facet = target.facet
), '')
WHERE target.facet IN ('issue_comments', 'pr_reviews', 'pr_review_comments', 'issue_timeline')
  AND target.id = (
      SELECT MIN(first.id)
      FROM facet_observations AS first
      WHERE first.repository_id = target.repository_id
        AND COALESCE(first.thread_id, -1) = COALESCE(target.thread_id, -1)
        AND first.facet = target.facet
  );

UPDATE facet_observations AS target
SET search_text = ''
WHERE target.facet IN ('issue_comments', 'pr_reviews', 'pr_review_comments', 'issue_timeline')
  AND target.id <> (
      SELECT MIN(first.id)
      FROM facet_observations AS first
      WHERE first.repository_id = target.repository_id
        AND COALESCE(first.thread_id, -1) = COALESCE(target.thread_id, -1)
        AND first.facet = target.facet
  );

CREATE VIRTUAL TABLE facet_observations_fts USING fts5(
    search_text,
    content='facet_observations',
    content_rowid='id'
);

INSERT INTO facet_observations_fts (rowid, search_text)
SELECT id, search_text FROM facet_observations;

CREATE TRIGGER facet_observations_fts_insert AFTER INSERT ON facet_observations
BEGIN
    INSERT INTO facet_observations_fts (rowid, search_text)
        VALUES (new.id, new.search_text);
END;

CREATE TRIGGER facet_observations_fts_update AFTER UPDATE OF search_text ON facet_observations
BEGIN
    INSERT INTO facet_observations_fts (facet_observations_fts, rowid, search_text)
        VALUES ('delete', old.id, old.search_text);
    INSERT INTO facet_observations_fts (rowid, search_text)
        VALUES (new.id, new.search_text);
END;

CREATE TRIGGER facet_observations_fts_delete AFTER DELETE ON facet_observations
BEGIN
    INSERT INTO facet_observations_fts (facet_observations_fts, rowid, search_text)
        VALUES ('delete', old.id, old.search_text);
END;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS facet_observations_fts_delete;
DROP TRIGGER IF EXISTS facet_observations_fts_update;
DROP TRIGGER IF EXISTS facet_observations_fts_insert;
DROP TABLE IF EXISTS facet_observations_fts;
ALTER TABLE facet_observations DROP COLUMN search_text;

-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin

ALTER TABLE threads ADD COLUMN merged_known INTEGER NOT NULL DEFAULT 0;

-- A positive projection is intrinsically known. For older corpora, also
-- recover explicit false values from complete pr_details snapshots or from
-- the PR-detail payloads written by the legacy eager sync path. Issue-header
-- payloads do not contain a Merged field and therefore remain unknown.
UPDATE threads
SET merged = COALESCE(
        (
            SELECT CASE WHEN json_extract(fo.payload, '$.Merged') THEN 1 ELSE 0 END
            FROM facet_observations fo
            JOIN facet_coverage fc
              ON fc.repository_id = fo.repository_id
             AND fc.thread_id = fo.thread_id
             AND fc.facet = fo.facet
             AND fc.complete = 1
            WHERE fo.thread_id = threads.id
              AND fo.facet = 'pr_details'
              AND json_type(fo.payload, '$.Merged') IN ('true', 'false', 'integer')
            ORDER BY fo.observation_sequence DESC
            LIMIT 1
        ),
        (
            SELECT CASE WHEN json_extract(o.payload, '$.Merged') THEN 1 ELSE 0 END
            FROM thread_observations o
            WHERE o.thread_id = threads.id
              AND json_type(o.payload, '$.Merged') IN ('true', 'false', 'integer')
            ORDER BY o.observation_sequence DESC
            LIMIT 1
        ),
        merged
    ),
    merged_known = CASE
        WHEN merged = 1 OR merged_at IS NOT NULL THEN 1
        WHEN EXISTS (
            SELECT 1
            FROM facet_observations fo
            JOIN facet_coverage fc
              ON fc.repository_id = fo.repository_id
             AND fc.thread_id = fo.thread_id
             AND fc.facet = fo.facet
             AND fc.complete = 1
            WHERE fo.thread_id = threads.id
              AND fo.facet = 'pr_details'
              AND json_type(fo.payload, '$.Merged') IN ('true', 'false', 'integer')
        ) THEN 1
        WHEN EXISTS (
            SELECT 1
            FROM thread_observations o
            WHERE o.thread_id = threads.id
              AND json_type(o.payload, '$.Merged') IN ('true', 'false', 'integer')
        ) THEN 1
        ELSE 0
    END
WHERE kind = 'pull_request';

-- +goose StatementEnd

-- +goose Down
ALTER TABLE threads DROP COLUMN merged_known;

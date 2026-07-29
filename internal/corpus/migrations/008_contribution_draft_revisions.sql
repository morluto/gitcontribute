-- +goose Up
-- +goose StatementBegin
CREATE TABLE contribution_draft_revisions (
    draft_id TEXT NOT NULL,
    opportunity_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('issue', 'pull_request')),
    revision INTEGER NOT NULL CHECK (revision > 0),
    title_sha256 TEXT NOT NULL,
    body_sha256 TEXT NOT NULL,
    payload TEXT NOT NULL,
    rendered_at INTEGER NOT NULL,
    PRIMARY KEY (draft_id, revision),
    UNIQUE (opportunity_id, kind, revision)
);
CREATE INDEX idx_contribution_draft_revisions_latest
    ON contribution_draft_revisions (opportunity_id, kind, revision DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS contribution_draft_revisions;
-- +goose StatementEnd

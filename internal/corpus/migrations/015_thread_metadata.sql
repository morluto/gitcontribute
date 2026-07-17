-- +goose Up
ALTER TABLE threads ADD COLUMN author_association TEXT;
ALTER TABLE threads ADD COLUMN assignees TEXT;
ALTER TABLE threads ADD COLUMN draft INTEGER NOT NULL DEFAULT 0;
ALTER TABLE threads ADD COLUMN locked INTEGER NOT NULL DEFAULT 0;
ALTER TABLE threads ADD COLUMN state_reason TEXT;
ALTER TABLE threads ADD COLUMN milestone TEXT;

-- +goose Down
-- SQLite column removal is intentionally omitted for this additive migration.

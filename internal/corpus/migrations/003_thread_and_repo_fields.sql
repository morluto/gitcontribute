-- +goose Up
ALTER TABLE repositories ADD COLUMN description TEXT;
ALTER TABLE repositories ADD COLUMN default_branch TEXT;
ALTER TABLE repositories ADD COLUMN language TEXT;
ALTER TABLE repositories ADD COLUMN license TEXT;
ALTER TABLE repositories ADD COLUMN topics TEXT;
ALTER TABLE repositories ADD COLUMN stars INTEGER NOT NULL DEFAULT 0;
ALTER TABLE repositories ADD COLUMN watchers INTEGER NOT NULL DEFAULT 0;
ALTER TABLE repositories ADD COLUMN forks INTEGER NOT NULL DEFAULT 0;
ALTER TABLE repositories ADD COLUMN open_issues INTEGER NOT NULL DEFAULT 0;
ALTER TABLE repositories ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;
ALTER TABLE repositories ADD COLUMN fork INTEGER NOT NULL DEFAULT 0;

ALTER TABLE threads ADD COLUMN closed_at INTEGER;
ALTER TABLE threads ADD COLUMN merged_at INTEGER;
ALTER TABLE threads ADD COLUMN merged INTEGER NOT NULL DEFAULT 0;
ALTER TABLE threads ADD COLUMN labels TEXT;

-- +goose Down
-- SQLite column removal is intentionally omitted for this additive migration.

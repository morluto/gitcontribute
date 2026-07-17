-- +goose Up
ALTER TABLE evidence ADD COLUMN source_provenance TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE evidence DROP COLUMN source_provenance;

-- +goose Up
ALTER TABLE source_partitions ADD COLUMN pages INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE source_partitions DROP COLUMN pages;

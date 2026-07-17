-- +goose Up
ALTER TABLE investigations
    ADD COLUMN origin_key TEXT;

CREATE UNIQUE INDEX idx_investigations_open_origin
    ON investigations (origin_key)
    WHERE origin_key IS NOT NULL AND status = 'open';

-- +goose Down
DROP INDEX IF EXISTS idx_investigations_open_origin;
ALTER TABLE investigations DROP COLUMN origin_key;

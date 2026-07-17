-- +goose Up
-- +goose StatementBegin

ALTER TABLE triage_events ADD COLUMN lens TEXT;
CREATE INDEX idx_triage_lens ON triage_events (lens);

-- +goose StatementEnd

-- +goose Down
DROP INDEX IF EXISTS idx_triage_lens;
ALTER TABLE triage_events DROP COLUMN lens;

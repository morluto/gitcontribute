-- +goose Up
-- +goose StatementBegin
ALTER TABLE code_snapshots ADD COLUMN manifest_json TEXT NOT NULL DEFAULT '{}';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE code_snapshots DROP COLUMN manifest_json;
-- +goose StatementEnd

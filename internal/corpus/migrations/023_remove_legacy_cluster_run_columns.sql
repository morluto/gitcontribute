-- +goose Up
-- +goose StatementBegin

ALTER TABLE cluster_runs DROP COLUMN stats;
ALTER TABLE cluster_runs DROP COLUMN params_hash;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE cluster_runs ADD COLUMN params_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE cluster_runs ADD COLUMN stats TEXT;

-- +goose StatementEnd

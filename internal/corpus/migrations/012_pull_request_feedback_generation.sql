-- +goose Up
-- +goose StatementBegin
ALTER TABLE pull_request_feedback_discovery
    ADD COLUMN generation INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pull_request_feedback_discovery
    DROP COLUMN generation;
-- +goose StatementEnd

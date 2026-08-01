-- +goose Up
-- +goose StatementBegin
ALTER TABLE pull_request_feedback_projection ADD COLUMN feedback_node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE pull_request_feedback_projection ADD COLUMN in_reply_to_id TEXT NOT NULL DEFAULT '';
ALTER TABLE pull_request_feedback_projection ADD COLUMN side TEXT NOT NULL DEFAULT '';
ALTER TABLE pull_request_feedback_projection ADD COLUMN start_side TEXT NOT NULL DEFAULT '';
ALTER TABLE pull_request_feedback_projection ADD COLUMN review_state TEXT NOT NULL DEFAULT '';
ALTER TABLE pull_request_feedback_projection ADD COLUMN resolved_by TEXT NOT NULL DEFAULT '';
UPDATE projection_states
SET status = 'stale', version = ''
WHERE name = 'pull_request_feedback_fts';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pull_request_feedback_projection DROP COLUMN resolved_by;
ALTER TABLE pull_request_feedback_projection DROP COLUMN review_state;
ALTER TABLE pull_request_feedback_projection DROP COLUMN start_side;
ALTER TABLE pull_request_feedback_projection DROP COLUMN side;
ALTER TABLE pull_request_feedback_projection DROP COLUMN in_reply_to_id;
ALTER TABLE pull_request_feedback_projection DROP COLUMN feedback_node_id;
UPDATE projection_states
SET status = 'current', version = 'pull-request-feedback-fts-v1'
WHERE name = 'pull_request_feedback_fts';
-- +goose StatementEnd

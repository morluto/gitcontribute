-- +goose Up
ALTER TABLE threads ADD COLUMN author_association TEXT;
ALTER TABLE threads ADD COLUMN assignees TEXT;
ALTER TABLE threads ADD COLUMN draft INTEGER NOT NULL DEFAULT 0;
ALTER TABLE threads ADD COLUMN locked INTEGER NOT NULL DEFAULT 0;
ALTER TABLE threads ADD COLUMN state_reason TEXT;
ALTER TABLE threads ADD COLUMN milestone TEXT;

-- +goose Down
-- +goose StatementBegin
ALTER TABLE threads DROP COLUMN milestone;
ALTER TABLE threads DROP COLUMN state_reason;
ALTER TABLE threads DROP COLUMN locked;
ALTER TABLE threads DROP COLUMN draft;
ALTER TABLE threads DROP COLUMN assignees;
ALTER TABLE threads DROP COLUMN author_association;
-- +goose StatementEnd

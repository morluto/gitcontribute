-- +goose Up
-- +goose StatementBegin
CREATE TABLE validation_run_groups (
    id TEXT PRIMARY KEY,
    definition_id TEXT NOT NULL,
    investigation_id TEXT NOT NULL,
    opportunity_id TEXT NOT NULL DEFAULT '',
    classification TEXT NOT NULL CHECK (classification IN ('stable_pass', 'stable_fail', 'flaky', 'inconclusive', 'cancelled')),
    requested_runs INTEGER NOT NULL CHECK (requested_runs > 0 AND requested_runs <= 200),
    completed_runs INTEGER NOT NULL CHECK (completed_runs >= 0 AND completed_runs <= requested_runs),
    payload TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    completed_at INTEGER NOT NULL,
    FOREIGN KEY (definition_id) REFERENCES validation_definitions (id) ON DELETE CASCADE,
    FOREIGN KEY (investigation_id) REFERENCES investigations (id) ON DELETE CASCADE
);
CREATE INDEX idx_validation_run_groups_definition_started
    ON validation_run_groups (definition_id, started_at DESC, id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS validation_run_groups;
-- +goose StatementEnd

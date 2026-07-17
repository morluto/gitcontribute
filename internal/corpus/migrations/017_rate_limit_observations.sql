-- +goose Up
CREATE TABLE rate_limit_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    attempt INTEGER NOT NULL,
    status_code INTEGER NOT NULL DEFAULT 0,
    resource TEXT,
    limit_value INTEGER NOT NULL DEFAULT 0,
    remaining INTEGER NOT NULL DEFAULT 0,
    used INTEGER NOT NULL DEFAULT 0,
    reset_at INTEGER NOT NULL DEFAULT 0,
    delay_ms INTEGER NOT NULL DEFAULT 0,
    api_version TEXT,
    source_url TEXT,
    observed_at INTEGER NOT NULL
);

CREATE INDEX idx_rate_limit_observed_at ON rate_limit_observations (observed_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS rate_limit_observations;

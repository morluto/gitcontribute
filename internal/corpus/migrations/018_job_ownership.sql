-- +goose Up
CREATE TABLE job_owners (
    owner_id TEXT PRIMARY KEY,
    process_id INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL,
    heartbeat_at INTEGER NOT NULL
);

CREATE INDEX idx_job_owners_heartbeat
    ON job_owners (heartbeat_at);

ALTER TABLE jobs
    ADD COLUMN owner_id TEXT REFERENCES job_owners(owner_id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE jobs DROP COLUMN owner_id;
DROP TABLE IF EXISTS job_owners;

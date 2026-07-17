-- +goose Up
CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    request TEXT NOT NULL,
    result TEXT,
    error TEXT,
    progress TEXT,
    statistics TEXT,
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    completed_at INTEGER,
    updated_at INTEGER NOT NULL,
    cancelled_at INTEGER
);

CREATE INDEX idx_jobs_status_created
    ON jobs (status, created_at);

CREATE INDEX idx_jobs_kind_created
    ON jobs (kind, created_at);

CREATE TABLE job_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    recorded_at INTEGER NOT NULL,
    FOREIGN KEY (job_id) REFERENCES jobs (id)
);

CREATE INDEX idx_job_events_job_id
    ON job_events (job_id, recorded_at);

-- +goose Down
DROP TABLE IF EXISTS job_events;
DROP TABLE IF EXISTS jobs;

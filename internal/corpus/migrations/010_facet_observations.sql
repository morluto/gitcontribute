-- +goose Up
CREATE TABLE facet_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    thread_id INTEGER,
    facet TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    payload TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories (id),
    FOREIGN KEY (thread_id) REFERENCES threads (id)
);

CREATE INDEX idx_facet_observations_lookup
    ON facet_observations (repository_id, COALESCE(thread_id, -1), facet, observation_sequence);

-- +goose Down
DROP TABLE IF EXISTS facet_observations;

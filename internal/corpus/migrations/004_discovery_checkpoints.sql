-- +goose Up
CREATE TABLE discovery_checkpoints (
    key TEXT PRIMARY KEY,
    checkpoint_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE archive_imports (
    hour_key TEXT PRIMARY KEY,
    imported_at INTEGER NOT NULL
);

CREATE TABLE discovery_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL,
    definition TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE source_partitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL,
    partition_key TEXT NOT NULL,
    query TEXT NOT NULL,
    qualifier TEXT,
    start_at INTEGER,
    end_at INTEGER,
    total_count INTEGER NOT NULL DEFAULT 0,
    incomplete_results INTEGER NOT NULL DEFAULT 0,
    unsplittable INTEGER NOT NULL DEFAULT 0,
    retries INTEGER NOT NULL DEFAULT 0,
    observed_at INTEGER NOT NULL,
    UNIQUE (source_id, partition_key),
    FOREIGN KEY (source_id) REFERENCES discovery_sources (id)
);

-- +goose Down
DROP TABLE IF EXISTS source_partitions;
DROP TABLE IF EXISTS discovery_sources;
DROP TABLE IF EXISTS archive_imports;
DROP TABLE IF EXISTS discovery_checkpoints;

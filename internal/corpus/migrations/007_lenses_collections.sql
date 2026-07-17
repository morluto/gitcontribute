-- +goose Up
CREATE TABLE lenses (
    name TEXT PRIMARY KEY,
    definition TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE collections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE collection_members (
    collection_id INTEGER NOT NULL,
    ref TEXT NOT NULL,
    kind TEXT NOT NULL,
    added_at INTEGER NOT NULL,
    PRIMARY KEY (collection_id, kind, ref),
    FOREIGN KEY (collection_id) REFERENCES collections (id) ON DELETE CASCADE
);

CREATE INDEX idx_collection_members_kind ON collection_members (kind, ref);

-- +goose Down
DROP TABLE IF EXISTS collection_members;
DROP TABLE IF EXISTS collections;
DROP TABLE IF EXISTS lenses;

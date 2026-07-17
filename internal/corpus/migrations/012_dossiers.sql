-- +goose Up
-- +goose StatementBegin

CREATE TABLE dossiers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    commit_sha TEXT,
    as_of INTEGER NOT NULL,
    section_metadata TEXT NOT NULL,
    snapshot TEXT NOT NULL,
    generated_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories (id) ON DELETE CASCADE
);

CREATE INDEX idx_dossiers_repository ON dossiers (repository_id, generated_at DESC);
CREATE INDEX idx_dossiers_repo_lookup ON dossiers (repo_owner, repo_name, generated_at DESC);

CREATE TABLE dossier_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dossier_id INTEGER NOT NULL,
    source TEXT NOT NULL,
    url TEXT NOT NULL,
    commit_sha TEXT,
    observed_at INTEGER NOT NULL,
    as_of INTEGER NOT NULL,
    FOREIGN KEY (dossier_id) REFERENCES dossiers (id) ON DELETE CASCADE
);

CREATE INDEX idx_dossier_sources_dossier ON dossier_sources (dossier_id);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS dossier_sources;
DROP TABLE IF EXISTS dossiers;

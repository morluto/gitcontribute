-- +goose Up
CREATE TABLE investigations (
    id TEXT PRIMARY KEY,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    status TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE hypotheses (
    id TEXT PRIMARY KEY,
    investigation_id TEXT NOT NULL,
    category TEXT NOT NULL,
    status TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (investigation_id) REFERENCES investigations (id)
);

CREATE TABLE opportunities (
    id TEXT PRIMARY KEY,
    investigation_id TEXT NOT NULL,
    hypothesis_id TEXT NOT NULL,
    category TEXT NOT NULL,
    status TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (investigation_id) REFERENCES investigations (id),
    FOREIGN KEY (hypothesis_id) REFERENCES hypotheses (id)
);

CREATE TABLE validation_definitions (
    id TEXT PRIMARY KEY,
    investigation_id TEXT,
    hypothesis_id TEXT,
    opportunity_id TEXT,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE validation_runs (
    id TEXT PRIMARY KEY,
    definition_id TEXT NOT NULL,
    investigation_id TEXT,
    hypothesis_id TEXT,
    opportunity_id TEXT,
    kind TEXT NOT NULL,
    classification TEXT NOT NULL,
    payload TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    completed_at INTEGER NOT NULL,
    FOREIGN KEY (definition_id) REFERENCES validation_definitions (id)
);

CREATE TABLE evidence (
    id TEXT PRIMARY KEY,
    investigation_id TEXT,
    hypothesis_id TEXT,
    opportunity_id TEXT,
    relation TEXT NOT NULL,
    evidence_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_evidence_scope ON evidence (investigation_id, hypothesis_id, opportunity_id, relation);

CREATE TABLE contribution_drafts (
    opportunity_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload TEXT NOT NULL,
    rendered_at INTEGER NOT NULL,
    PRIMARY KEY (opportunity_id, kind)
);

-- +goose Down
DROP TABLE IF EXISTS contribution_drafts;
DROP TABLE IF EXISTS evidence;
DROP TABLE IF EXISTS validation_runs;
DROP TABLE IF EXISTS validation_definitions;
DROP TABLE IF EXISTS opportunities;
DROP TABLE IF EXISTS hypotheses;
DROP TABLE IF EXISTS investigations;

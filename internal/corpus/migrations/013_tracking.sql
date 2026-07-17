-- +goose Up
-- +goose StatementBegin

CREATE TABLE triage_events (
    id TEXT PRIMARY KEY,
    target_kind TEXT NOT NULL,
    target_ref TEXT NOT NULL,
    outcome TEXT NOT NULL,
    reason TEXT,
    source_event_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    repository_id INTEGER,
    thread_id INTEGER,
    investigation_id TEXT,
    opportunity_id TEXT,
    FOREIGN KEY (repository_id) REFERENCES repositories (id) ON DELETE SET NULL,
    FOREIGN KEY (thread_id) REFERENCES threads (id) ON DELETE SET NULL,
    FOREIGN KEY (investigation_id) REFERENCES investigations (id) ON DELETE SET NULL,
    FOREIGN KEY (opportunity_id) REFERENCES opportunities (id) ON DELETE SET NULL
);

CREATE INDEX idx_triage_lookup ON triage_events (target_kind, target_ref, outcome);
CREATE INDEX idx_triage_event_at ON triage_events (source_event_at, created_at);

CREATE TABLE contributions (
    id TEXT PRIMARY KEY,
    opportunity_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    reference TEXT,
    reference_url TEXT,
    prepared_at INTEGER NOT NULL,
    submitted_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    payload TEXT NOT NULL,
    FOREIGN KEY (opportunity_id) REFERENCES opportunities (id) ON DELETE CASCADE
);

CREATE INDEX idx_contributions_opportunity ON contributions (opportunity_id);

CREATE TABLE contribution_outcomes (
    id TEXT PRIMARY KEY,
    contribution_id TEXT NOT NULL,
    outcome TEXT NOT NULL,
    reason TEXT,
    source_event_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (contribution_id) REFERENCES contributions (id) ON DELETE CASCADE
);

CREATE INDEX idx_contribution_outcomes_contribution ON contribution_outcomes (contribution_id, source_event_at);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS contribution_outcomes;
DROP TABLE IF EXISTS contributions;
DROP TABLE IF EXISTS triage_events;

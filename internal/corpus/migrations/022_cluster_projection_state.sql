-- +goose Up
-- +goose StatementBegin

ALTER TABLE cluster_runs ADD COLUMN governance_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cluster_runs ADD COLUMN rule_version TEXT NOT NULL DEFAULT 'duplicate-v1';
ALTER TABLE cluster_runs ADD COLUMN statistics_json TEXT NOT NULL DEFAULT '{}';

CREATE TABLE cluster_projection_state (
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    current_run_id INTEGER REFERENCES cluster_runs(id),
    source_revision TEXT,
    governance_revision INTEGER NOT NULL DEFAULT 0 CHECK (governance_revision >= 0),
    rule_version TEXT,
    refreshed_at INTEGER,
    PRIMARY KEY (repo_owner, repo_name)
) WITHOUT ROWID;

INSERT INTO cluster_projection_state (
    repo_owner, repo_name, current_run_id, source_revision,
    governance_revision, rule_version, refreshed_at
)
SELECT runs.repo_owner, runs.repo_name, runs.id, runs.source_revision,
       runs.governance_revision, runs.rule_version, runs.completed_at
FROM cluster_runs AS runs
JOIN (
    SELECT repo_owner, repo_name, MAX(id) AS id
    FROM cluster_runs
    WHERE status = 'completed'
    GROUP BY repo_owner, repo_name
) AS latest ON latest.id = runs.id;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS cluster_projection_state;
ALTER TABLE cluster_runs DROP COLUMN statistics_json;
ALTER TABLE cluster_runs DROP COLUMN rule_version;
ALTER TABLE cluster_runs DROP COLUMN governance_revision;

-- +goose StatementEnd

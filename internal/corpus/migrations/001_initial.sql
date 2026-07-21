-- +goose Up
-- +goose StatementBegin
CREATE TABLE observation_sequences (
    name TEXT PRIMARY KEY,
    next_value INTEGER NOT NULL
);
CREATE TABLE repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    external_id TEXT,
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL, description TEXT, default_branch TEXT, language TEXT, license TEXT, topics TEXT, stars INTEGER NOT NULL DEFAULT 0, watchers INTEGER NOT NULL DEFAULT 0, forks INTEGER NOT NULL DEFAULT 0, open_issues INTEGER NOT NULL DEFAULT 0, archived INTEGER NOT NULL DEFAULT 0, fork INTEGER NOT NULL DEFAULT 0, source_created_at INTEGER NOT NULL DEFAULT 0,
    UNIQUE (owner, name)
);
CREATE TABLE repository_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    payload TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories (id)
);
CREATE TABLE threads (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    number INTEGER NOT NULL,
    state TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    author TEXT,
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL, closed_at INTEGER, merged_at INTEGER, merged INTEGER NOT NULL DEFAULT 0, labels TEXT, source_created_at INTEGER NOT NULL DEFAULT 0, author_association TEXT, assignees TEXT, draft INTEGER NOT NULL DEFAULT 0, locked INTEGER NOT NULL DEFAULT 0, state_reason TEXT, milestone TEXT, merged_known INTEGER NOT NULL DEFAULT 0,
    UNIQUE (repository_id, kind, number),
    FOREIGN KEY (repository_id) REFERENCES repositories (id)
);
CREATE TABLE thread_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id INTEGER NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    payload TEXT NOT NULL,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (thread_id) REFERENCES threads (id)
);
CREATE TABLE facet_coverage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    thread_id INTEGER,
    facet TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    complete INTEGER NOT NULL DEFAULT 0,
    run_id INTEGER,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories (id),
    FOREIGN KEY (thread_id) REFERENCES threads (id)
);
CREATE UNIQUE INDEX idx_facet_coverage_unq
    ON facet_coverage (repository_id, COALESCE(thread_id, -1), facet);
CREATE TABLE runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    stats TEXT,
    error TEXT
);
CREATE TABLE run_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    recorded_at INTEGER NOT NULL,
    FOREIGN KEY (run_id) REFERENCES runs (id)
);
CREATE VIRTUAL TABLE threads_fts USING fts5(
    title,
    body,
    content='threads',
    content_rowid='id'
)
/* threads_fts(title,body) */;
CREATE TRIGGER threads_fts_insert AFTER INSERT ON threads
BEGIN
    INSERT INTO threads_fts (rowid, title, body)
        VALUES (new.id, new.title, COALESCE(new.body, ''));
END;
CREATE TRIGGER threads_fts_update AFTER UPDATE ON threads
BEGIN
    INSERT INTO threads_fts (threads_fts, rowid, title, body)
        VALUES ('delete', old.id, old.title, COALESCE(old.body, ''));
    INSERT INTO threads_fts (rowid, title, body)
        VALUES (new.id, new.title, COALESCE(new.body, ''));
END;
CREATE TRIGGER threads_fts_delete AFTER DELETE ON threads
BEGIN
    INSERT INTO threads_fts (threads_fts, rowid, title, body)
        VALUES ('delete', old.id, old.title, COALESCE(old.body, ''));
END;
CREATE TABLE frontier_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    work_key TEXT NOT NULL UNIQUE,
    subject_kind TEXT NOT NULL,
    owner TEXT,
    repo TEXT,
    thread_kind TEXT,
    thread_number INTEGER,
    facet TEXT,
    priority INTEGER NOT NULL DEFAULT 0,
    reason TEXT,
    source TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    earliest_run_at INTEGER NOT NULL DEFAULT 0,
    budget_estimate INTEGER NOT NULL DEFAULT 1,
    state TEXT NOT NULL DEFAULT 'queued',
    lease_owner TEXT,
    lease_expires_at INTEGER,
    failure_kind TEXT,
    last_error TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX idx_frontier_ready
    ON frontier_items (state, earliest_run_at, priority DESC, id);
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
    observed_at INTEGER NOT NULL, pages INTEGER NOT NULL DEFAULT 0,
    UNIQUE (source_id, partition_key),
    FOREIGN KEY (source_id) REFERENCES discovery_sources (id)
);
CREATE TABLE investigations (
    id TEXT PRIMARY KEY,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    status TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
, origin_key TEXT);
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
, source_provenance TEXT NOT NULL DEFAULT '[]');
CREATE INDEX idx_evidence_scope ON evidence (investigation_id, hypothesis_id, opportunity_id, relation);
CREATE TABLE contribution_drafts (
    opportunity_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload TEXT NOT NULL,
    rendered_at INTEGER NOT NULL,
    PRIMARY KEY (opportunity_id, kind)
);
CREATE TABLE code_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    repo_path TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    total_bytes INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (repo_owner, repo_name, commit_sha)
);
CREATE TABLE code_documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    content TEXT NOT NULL,
    bytes INTEGER NOT NULL,
    language TEXT,
    UNIQUE (snapshot_id, path),
    FOREIGN KEY (snapshot_id) REFERENCES code_snapshots (id) ON DELETE CASCADE
);
CREATE VIRTUAL TABLE code_documents_fts USING fts5(
    path,
    content,
    content='code_documents',
    content_rowid='id'
)
/* code_documents_fts(path,content) */;
CREATE TRIGGER code_documents_fts_insert AFTER INSERT ON code_documents BEGIN
    INSERT INTO code_documents_fts (rowid, path, content) VALUES (new.id, new.path, new.content);
END;
CREATE TRIGGER code_documents_fts_delete AFTER DELETE ON code_documents BEGIN
    INSERT INTO code_documents_fts (code_documents_fts, rowid, path, content)
        VALUES ('delete', old.id, old.path, old.content);
END;
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
CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    investigation_id TEXT,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_workspaces_investigation ON workspaces (investigation_id);
CREATE TABLE cluster_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    source_revision TEXT NOT NULL,
    source_window_start INTEGER NOT NULL,
    source_window_end INTEGER NOT NULL,
    status TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    governance_revision INTEGER NOT NULL DEFAULT 0, rule_version TEXT NOT NULL DEFAULT 'duplicate-v1', statistics_json TEXT NOT NULL DEFAULT '{}');
CREATE INDEX idx_cluster_runs_repo ON cluster_runs (repo_owner, repo_name, completed_at DESC);
CREATE TABLE clusters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    stable_id TEXT NOT NULL,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    state TEXT NOT NULL,
    canonical_kind TEXT NOT NULL,
    canonical_owner TEXT NOT NULL,
    canonical_repo TEXT NOT NULL,
    canonical_number INTEGER NOT NULL,
    source_revision TEXT NOT NULL,
    source_window_start INTEGER NOT NULL,
    source_window_end INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (stable_id)
);
CREATE INDEX idx_clusters_repo_state ON clusters (repo_owner, repo_name, state);
CREATE INDEX idx_clusters_canonical ON clusters (canonical_kind, canonical_owner, canonical_repo, canonical_number);
CREATE TABLE cluster_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cluster_id INTEGER NOT NULL,
    thread_id INTEGER,
    kind TEXT NOT NULL,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    title TEXT NOT NULL,
    state TEXT NOT NULL,
    score REAL NOT NULL,
    reason TEXT NOT NULL,
    included INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (cluster_id) REFERENCES clusters (id) ON DELETE CASCADE,
    FOREIGN KEY (thread_id) REFERENCES threads (id),
    UNIQUE (cluster_id, kind, owner, repo, number)
);
CREATE INDEX idx_cluster_members_cluster ON cluster_members (cluster_id);
CREATE INDEX idx_cluster_members_ref ON cluster_members (kind, owner, repo, number);
CREATE TABLE cluster_overrides (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cluster_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    action TEXT NOT NULL,
    reason TEXT NOT NULL,
    target_cluster_id INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (cluster_id) REFERENCES clusters (id) ON DELETE CASCADE,
    FOREIGN KEY (target_cluster_id) REFERENCES clusters (id) ON DELETE CASCADE
);
CREATE INDEX idx_cluster_overrides_cluster ON cluster_overrides (cluster_id);
CREATE TABLE facet_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL,
    thread_id INTEGER,
    facet TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    payload TEXT NOT NULL,
    observed_at INTEGER NOT NULL, search_text TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (repository_id) REFERENCES repositories (id),
    FOREIGN KEY (thread_id) REFERENCES threads (id)
);
CREATE INDEX idx_facet_observations_lookup
    ON facet_observations (repository_id, COALESCE(thread_id, -1), facet, observation_sequence);
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
, owner_id TEXT REFERENCES job_owners(owner_id) ON DELETE SET NULL);
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
    opportunity_id TEXT, lens TEXT,
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
CREATE INDEX idx_triage_lens ON triage_events (lens);
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
CREATE TABLE job_owners (
    owner_id TEXT PRIMARY KEY,
    process_id INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL,
    heartbeat_at INTEGER NOT NULL
);
CREATE INDEX idx_job_owners_heartbeat
    ON job_owners (heartbeat_at);
CREATE UNIQUE INDEX idx_investigations_open_origin
    ON investigations (origin_key)
    WHERE origin_key IS NOT NULL AND status = 'open';
CREATE TABLE portfolio_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pull_request_thread_id INTEGER NOT NULL,
    opportunity_id TEXT,
    workspace_id TEXT,
    created_at INTEGER NOT NULL,
    CHECK (opportunity_id IS NOT NULL OR workspace_id IS NOT NULL),
    FOREIGN KEY (pull_request_thread_id) REFERENCES threads (id) ON DELETE CASCADE,
    FOREIGN KEY (opportunity_id) REFERENCES opportunities (id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_portfolio_links_identity
    ON portfolio_links (pull_request_thread_id, COALESCE(opportunity_id, ''), COALESCE(workspace_id, ''));
CREATE INDEX idx_portfolio_links_opportunity ON portfolio_links (opportunity_id, pull_request_thread_id);
CREATE INDEX idx_portfolio_links_workspace ON portfolio_links (workspace_id, pull_request_thread_id);
CREATE TABLE portfolio_signal_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subject_kind TEXT NOT NULL,
    subject_ref TEXT NOT NULL,
    facet TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    source_observation_refs TEXT NOT NULL,
    observed_at INTEGER NOT NULL
);
CREATE INDEX idx_portfolio_signal_snapshots_subject
    ON portfolio_signal_snapshots (subject_kind, subject_ref, facet, source_updated_at DESC, observation_sequence DESC);
CREATE TABLE portfolio_signals (
    snapshot_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    kind TEXT NOT NULL,
    value TEXT NOT NULL,
    target_kind TEXT,
    target_ref TEXT,
    score REAL,
    PRIMARY KEY (snapshot_id, position),
    FOREIGN KEY (snapshot_id) REFERENCES portfolio_signal_snapshots (id) ON DELETE CASCADE
);
CREATE INDEX idx_portfolio_signals_lookup ON portfolio_signals (kind, value);
CREATE INDEX idx_portfolio_signals_target ON portfolio_signals (target_kind, target_ref, kind);
CREATE TABLE portfolio_signal_projections (
    subject_kind TEXT NOT NULL,
    subject_ref TEXT NOT NULL,
    facet TEXT NOT NULL,
    snapshot_id INTEGER NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    PRIMARY KEY (subject_kind, subject_ref, facet),
    FOREIGN KEY (snapshot_id) REFERENCES portfolio_signal_snapshots (id) ON DELETE CASCADE
);
CREATE TABLE resolution_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    summary TEXT NOT NULL,
    rule_version TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    source_observation_refs TEXT NOT NULL,
    derived_at INTEGER NOT NULL,
    FOREIGN KEY (thread_id) REFERENCES threads (id) ON DELETE CASCADE
);
CREATE INDEX idx_resolution_records_thread
    ON resolution_records (thread_id, source_updated_at DESC, observation_sequence DESC);
CREATE TABLE resolution_projections (
    thread_id INTEGER PRIMARY KEY,
    resolution_record_id INTEGER NOT NULL,
    source_updated_at INTEGER NOT NULL,
    observation_sequence INTEGER NOT NULL,
    FOREIGN KEY (thread_id) REFERENCES threads (id) ON DELETE CASCADE,
    FOREIGN KEY (resolution_record_id) REFERENCES resolution_records (id) ON DELETE CASCADE
);
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
CREATE VIRTUAL TABLE facet_observations_fts USING fts5(
    search_text,
    content='facet_observations',
    content_rowid='id'
)
/* facet_observations_fts(search_text) */;
CREATE TRIGGER facet_observations_fts_insert AFTER INSERT ON facet_observations
BEGIN
    INSERT INTO facet_observations_fts (rowid, search_text)
        VALUES (new.id, new.search_text);
END;
CREATE TRIGGER facet_observations_fts_update AFTER UPDATE OF search_text ON facet_observations
BEGIN
    INSERT INTO facet_observations_fts (facet_observations_fts, rowid, search_text)
        VALUES ('delete', old.id, old.search_text);
    INSERT INTO facet_observations_fts (rowid, search_text)
        VALUES (new.id, new.search_text);
END;
CREATE TRIGGER facet_observations_fts_delete AFTER DELETE ON facet_observations
BEGIN
    INSERT INTO facet_observations_fts (facet_observations_fts, rowid, search_text)
        VALUES ('delete', old.id, old.search_text);
END;
CREATE INDEX idx_thread_observations_thread_sequence
    ON thread_observations (thread_id, observation_sequence DESC);
CREATE INDEX idx_facet_observations_thread_facet_sequence
    ON facet_observations (thread_id, facet, observation_sequence DESC, repository_id);
CREATE INDEX idx_facet_coverage_thread_facet
    ON facet_coverage (thread_id, facet, repository_id, complete);
CREATE TABLE projection_states (
    name TEXT PRIMARY KEY,
    version TEXT NOT NULL,
    status TEXT NOT NULL,
    refreshed_at INTEGER,
    row_count INTEGER NOT NULL DEFAULT 0
, source_revision TEXT NOT NULL DEFAULT '', content_hash TEXT NOT NULL DEFAULT '', attempt_status TEXT NOT NULL DEFAULT '', attempt_started_at INTEGER, attempt_finished_at INTEGER, attempt_error TEXT NOT NULL DEFAULT '') WITHOUT ROWID;
CREATE TRIGGER projection_threads_insert AFTER INSERT ON threads BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = row_count + 1 WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_update AFTER UPDATE OF title, body ON threads BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000) WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_delete AFTER DELETE ON threads BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = MAX(row_count - 1, 0) WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_facets_insert AFTER INSERT ON facet_observations BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = row_count + 1 WHERE name = 'facet_observations_fts';
END;
CREATE TRIGGER projection_facets_update AFTER UPDATE OF search_text ON facet_observations BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000) WHERE name = 'facet_observations_fts';
END;
CREATE TRIGGER projection_facets_delete AFTER DELETE ON facet_observations BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = MAX(row_count - 1, 0) WHERE name = 'facet_observations_fts';
END;
CREATE TRIGGER projection_code_insert AFTER INSERT ON code_documents BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = row_count + 1 WHERE name = 'code_documents_fts';
END;
CREATE TRIGGER projection_code_delete AFTER DELETE ON code_documents BEGIN
    UPDATE projection_states SET refreshed_at = (strftime('%s','now') * 1000000000), row_count = MAX(row_count - 1, 0) WHERE name = 'code_documents_fts';
END;
CREATE TRIGGER projection_threads_revision_insert AFTER INSERT ON threads BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_revision_update AFTER UPDATE OF title, body ON threads BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_threads_revision_delete AFTER DELETE ON threads BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'threads_fts';
END;
CREATE TRIGGER projection_facets_revision_insert AFTER INSERT ON facet_observations BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'facet_observations_fts';
END;
CREATE TRIGGER projection_facets_revision_update AFTER UPDATE OF search_text ON facet_observations BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'facet_observations_fts';
END;
CREATE TRIGGER projection_facets_revision_delete AFTER DELETE ON facet_observations BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'facet_observations_fts';
END;
CREATE TRIGGER projection_code_revision_insert AFTER INSERT ON code_documents BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'code_documents_fts';
END;
CREATE TRIGGER projection_code_revision_delete AFTER DELETE ON code_documents BEGIN
    UPDATE projection_states SET source_revision = '', content_hash = '' WHERE name = 'code_documents_fts';
END;

INSERT INTO projection_states (name, version, status, refreshed_at, row_count) VALUES
('code_documents_fts','code-documents-fts-v1','current',(strftime('%s','now')*1000000000),0),
('facet_observations_fts','facet-observations-fts-v1','current',(strftime('%s','now')*1000000000),0),
('threads_fts','threads-fts-v1','current',(strftime('%s','now')*1000000000),0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS projection_states;
DROP TABLE IF EXISTS facet_observations_fts;
DROP TABLE IF EXISTS cluster_projection_state;
DROP TABLE IF EXISTS resolution_projections;
DROP TABLE IF EXISTS resolution_records;
DROP TABLE IF EXISTS portfolio_signal_projections;
DROP TABLE IF EXISTS portfolio_signals;
DROP TABLE IF EXISTS portfolio_signal_snapshots;
DROP TABLE IF EXISTS portfolio_links;
DROP TABLE IF EXISTS job_owners;
DROP TABLE IF EXISTS rate_limit_observations;
DROP TABLE IF EXISTS contribution_outcomes;
DROP TABLE IF EXISTS contributions;
DROP TABLE IF EXISTS triage_events;
DROP TABLE IF EXISTS dossier_sources;
DROP TABLE IF EXISTS dossiers;
DROP TABLE IF EXISTS job_events;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS facet_observations;
DROP TABLE IF EXISTS cluster_overrides;
DROP TABLE IF EXISTS cluster_members;
DROP TABLE IF EXISTS clusters;
DROP TABLE IF EXISTS cluster_runs;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS collection_members;
DROP TABLE IF EXISTS collections;
DROP TABLE IF EXISTS lenses;
DROP TABLE IF EXISTS code_documents_fts;
DROP TABLE IF EXISTS code_documents;
DROP TABLE IF EXISTS code_snapshots;
DROP TABLE IF EXISTS contribution_drafts;
DROP TABLE IF EXISTS evidence;
DROP TABLE IF EXISTS validation_runs;
DROP TABLE IF EXISTS validation_definitions;
DROP TABLE IF EXISTS opportunities;
DROP TABLE IF EXISTS hypotheses;
DROP TABLE IF EXISTS investigations;
DROP TABLE IF EXISTS source_partitions;
DROP TABLE IF EXISTS discovery_sources;
DROP TABLE IF EXISTS archive_imports;
DROP TABLE IF EXISTS discovery_checkpoints;
DROP TABLE IF EXISTS frontier_items;
DROP TABLE IF EXISTS threads_fts;
DROP TABLE IF EXISTS run_events;
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS facet_coverage;
DROP TABLE IF EXISTS thread_observations;
DROP TABLE IF EXISTS threads;
DROP TABLE IF EXISTS repository_observations;
DROP TABLE IF EXISTS repositories;
DROP TABLE IF EXISTS observation_sequences;
-- +goose StatementEnd

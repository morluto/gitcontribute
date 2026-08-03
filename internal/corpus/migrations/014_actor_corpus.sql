-- +goose Up
-- +goose StatementBegin
CREATE TABLE actors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_key TEXT NOT NULL UNIQUE,
    provider TEXT NOT NULL,
    node_id TEXT,
    database_id INTEGER,
    kind TEXT NOT NULL DEFAULT 'unknown',
    current_login TEXT NOT NULL DEFAULT '',
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX idx_actors_provider_node
    ON actors (provider, node_id) WHERE node_id IS NOT NULL AND node_id <> '';
CREATE INDEX idx_actors_login ON actors (provider, current_login COLLATE NOCASE);

CREATE TABLE actor_aliases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id INTEGER NOT NULL,
    login TEXT NOT NULL,
    normalized_login TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    first_observed_at INTEGER NOT NULL,
    last_observed_at INTEGER NOT NULL,
    FOREIGN KEY (actor_id) REFERENCES actors (id) ON DELETE CASCADE,
    UNIQUE (actor_id, normalized_login)
);
CREATE INDEX idx_actor_aliases_lookup
    ON actor_aliases (normalized_login, active, last_observed_at DESC);

CREATE TABLE actor_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id INTEGER NOT NULL,
    facet TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL,
    observed_at INTEGER NOT NULL,
    complete INTEGER NOT NULL DEFAULT 0 CHECK (complete IN (0, 1)),
    authorization_scope TEXT NOT NULL DEFAULT 'public',
    payload TEXT NOT NULL,
    FOREIGN KEY (actor_id) REFERENCES actors (id) ON DELETE CASCADE
);
CREATE INDEX idx_actor_observations_lookup
    ON actor_observations (actor_id, facet, source_updated_at DESC, observation_sequence DESC);

CREATE TABLE actor_profiles (
    actor_id INTEGER PRIMARY KEY,
    name TEXT,
    avatar_url TEXT,
    bio TEXT,
    company TEXT,
    location TEXT,
    website_url TEXT,
    public_email TEXT,
    twitter_username TEXT,
    hireable INTEGER,
    followers INTEGER,
    following INTEGER,
    public_repositories INTEGER,
    public_gists INTEGER,
    provider_created_at INTEGER NOT NULL DEFAULT 0,
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    observed_at INTEGER NOT NULL,
    authorization_scope TEXT NOT NULL DEFAULT 'public',
    FOREIGN KEY (actor_id) REFERENCES actors (id) ON DELETE CASCADE
);

CREATE TABLE actor_social_accounts (
    actor_id INTEGER NOT NULL,
    provider_name TEXT NOT NULL,
    url TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (actor_id) REFERENCES actors (id) ON DELETE CASCADE,
    PRIMARY KEY (actor_id, provider_name, url)
);

CREATE TABLE actor_organization_memberships (
    actor_id INTEGER NOT NULL,
    organization_node_id TEXT NOT NULL,
    organization_login TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (actor_id) REFERENCES actors (id) ON DELETE CASCADE,
    PRIMARY KEY (actor_id, organization_node_id)
);

CREATE TABLE actor_pinned_items (
    actor_id INTEGER NOT NULL,
    rank INTEGER NOT NULL,
    item_kind TEXT NOT NULL,
    target_node_id TEXT NOT NULL,
    target_name TEXT NOT NULL DEFAULT '',
    repository_owner TEXT NOT NULL DEFAULT '',
    showcase_kind TEXT NOT NULL DEFAULT 'pinned',
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (actor_id) REFERENCES actors (id) ON DELETE CASCADE,
    PRIMARY KEY (actor_id, rank)
);

CREATE TABLE actor_repository_affiliations (
    actor_id INTEGER NOT NULL,
    repository_id INTEGER NOT NULL,
    relationship TEXT NOT NULL,
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (actor_id) REFERENCES actors (id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id) REFERENCES repositories (id) ON DELETE CASCADE,
    PRIMARY KEY (actor_id, repository_id, relationship)
);
CREATE INDEX idx_actor_repo_affiliations_repository
    ON actor_repository_affiliations (repository_id, relationship, actor_id);

CREATE TABLE actor_contribution_periods (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id INTEGER NOT NULL,
    period_start INTEGER NOT NULL,
    period_end INTEGER NOT NULL,
    organization_node_id TEXT NOT NULL DEFAULT '',
    authorization_scope TEXT NOT NULL DEFAULT 'public',
    total_commits INTEGER,
    total_issues INTEGER,
    total_pull_requests INTEGER,
    total_pull_request_reviews INTEGER,
    total_repositories INTEGER,
    restricted_contributions INTEGER,
    complete INTEGER NOT NULL DEFAULT 0 CHECK (complete IN (0, 1)),
    source_updated_at INTEGER NOT NULL DEFAULT 0,
    observation_sequence INTEGER NOT NULL DEFAULT 0,
    observed_at INTEGER NOT NULL,
    FOREIGN KEY (actor_id) REFERENCES actors (id) ON DELETE CASCADE,
    UNIQUE (actor_id, period_start, period_end, organization_node_id, authorization_scope)
);

CREATE TABLE actor_contribution_days (
    period_id INTEGER NOT NULL,
    contribution_date TEXT NOT NULL,
    contribution_count INTEGER NOT NULL,
    contribution_level TEXT NOT NULL,
    FOREIGN KEY (period_id) REFERENCES actor_contribution_periods (id) ON DELETE CASCADE,
    PRIMARY KEY (period_id, contribution_date)
);

CREATE TABLE actor_contribution_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    period_id INTEGER NOT NULL,
    contribution_kind TEXT NOT NULL,
    occurred_at INTEGER NOT NULL,
    repository_id INTEGER,
    target_node_id TEXT NOT NULL DEFAULT '',
    target_url TEXT NOT NULL DEFAULT '',
    restricted INTEGER NOT NULL DEFAULT 0 CHECK (restricted IN (0, 1)),
    count INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (period_id) REFERENCES actor_contribution_periods (id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id) REFERENCES repositories (id) ON DELETE SET NULL
);
CREATE INDEX idx_actor_contribution_items_actor_time
    ON actor_contribution_items (period_id, occurred_at DESC, id DESC);
CREATE INDEX idx_actor_contribution_items_repository
    ON actor_contribution_items (repository_id, contribution_kind, occurred_at DESC);

CREATE TABLE actor_repository_contribution_totals (
    period_id INTEGER NOT NULL,
    repository_id INTEGER NOT NULL,
    contribution_kind TEXT NOT NULL,
    contribution_count INTEGER NOT NULL,
    FOREIGN KEY (period_id) REFERENCES actor_contribution_periods (id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id) REFERENCES repositories (id) ON DELETE CASCADE,
    PRIMARY KEY (period_id, repository_id, contribution_kind)
);

CREATE VIRTUAL TABLE actors_fts USING fts5(
    actor_id UNINDEXED,
    login,
    name,
    bio,
    company,
    location
);

INSERT INTO actors (actor_key, provider, kind, current_login, created_at, updated_at)
SELECT 'github:login:' || lower(login), 'github', 'unknown', login,
       (strftime('%s','now') * 1000000000), (strftime('%s','now') * 1000000000)
FROM (
    SELECT MIN(login) AS login FROM (
        SELECT owner AS login FROM repositories WHERE trim(owner) <> ''
        UNION ALL
        SELECT author AS login FROM threads WHERE trim(author) <> ''
        UNION ALL
        SELECT author AS login FROM pull_request_feedback_projection WHERE trim(author) <> ''
    ) observed_logins
    GROUP BY lower(login)
)
ORDER BY lower(login);

INSERT INTO actor_aliases (actor_id, login, normalized_login, active, first_observed_at, last_observed_at)
SELECT id, current_login, lower(current_login), 1, created_at, updated_at
FROM actors WHERE current_login <> '';

INSERT INTO actors_fts (actor_id, login, name, bio, company, location)
SELECT id, current_login, '', '', '', '' FROM actors;

INSERT INTO projection_states (name, version, status, refreshed_at, row_count, source_revision, content_hash)
SELECT 'actors_fts', 'actors-fts-v1', 'current',
       (strftime('%s','now') * 1000000000), COUNT(*), '', ''
FROM actors;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM projection_states WHERE name = 'actors_fts';
DROP TABLE IF EXISTS actors_fts;
DROP TABLE IF EXISTS actor_repository_contribution_totals;
DROP INDEX IF EXISTS idx_actor_contribution_items_repository;
DROP INDEX IF EXISTS idx_actor_contribution_items_actor_time;
DROP TABLE IF EXISTS actor_contribution_items;
DROP TABLE IF EXISTS actor_contribution_days;
DROP TABLE IF EXISTS actor_contribution_periods;
DROP INDEX IF EXISTS idx_actor_repo_affiliations_repository;
DROP TABLE IF EXISTS actor_repository_affiliations;
DROP TABLE IF EXISTS actor_pinned_items;
DROP TABLE IF EXISTS actor_organization_memberships;
DROP TABLE IF EXISTS actor_social_accounts;
DROP TABLE IF EXISTS actor_profiles;
DROP INDEX IF EXISTS idx_actor_observations_lookup;
DROP TABLE IF EXISTS actor_observations;
DROP INDEX IF EXISTS idx_actor_aliases_lookup;
DROP TABLE IF EXISTS actor_aliases;
DROP INDEX IF EXISTS idx_actors_login;
DROP INDEX IF EXISTS idx_actors_provider_node;
DROP TABLE IF EXISTS actors;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
-- Product-owned logical identity for every durable corpus state.
-- Triggers cover ordinary source and derived-corpus writes made by corpus
-- adapters and direct test fixtures alike. FTS virtual tables are projections
-- and are not revision sources themselves. Operational workflow state is
-- removed from this hook set below so job progress cannot stale a corpus read.
CREATE TABLE corpus_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision INTEGER NOT NULL CHECK (revision >= 0)
);

INSERT INTO corpus_state (id, revision) VALUES (1, 0);

CREATE TRIGGER corpus_revision_repositories_ai
AFTER INSERT ON repositories
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_repositories_au
AFTER UPDATE ON repositories
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_repositories_ad
AFTER DELETE ON repositories
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_repository_observations_ai
AFTER INSERT ON repository_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_repository_observations_au
AFTER UPDATE ON repository_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_repository_observations_ad
AFTER DELETE ON repository_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_threads_ai
AFTER INSERT ON threads
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_threads_au
AFTER UPDATE ON threads
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_threads_ad
AFTER DELETE ON threads
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_thread_observations_ai
AFTER INSERT ON thread_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_thread_observations_au
AFTER UPDATE ON thread_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_thread_observations_ad
AFTER DELETE ON thread_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_facet_coverage_ai
AFTER INSERT ON facet_coverage
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_facet_coverage_au
AFTER UPDATE ON facet_coverage
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_facet_coverage_ad
AFTER DELETE ON facet_coverage
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_runs_ai
AFTER INSERT ON runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_runs_au
AFTER UPDATE ON runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_runs_ad
AFTER DELETE ON runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_run_events_ai
AFTER INSERT ON run_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_run_events_au
AFTER UPDATE ON run_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_run_events_ad
AFTER DELETE ON run_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_frontier_items_ai
AFTER INSERT ON frontier_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_frontier_items_au
AFTER UPDATE ON frontier_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_frontier_items_ad
AFTER DELETE ON frontier_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_discovery_checkpoints_ai
AFTER INSERT ON discovery_checkpoints
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_discovery_checkpoints_au
AFTER UPDATE ON discovery_checkpoints
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_discovery_checkpoints_ad
AFTER DELETE ON discovery_checkpoints
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_archive_imports_ai
AFTER INSERT ON archive_imports
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_archive_imports_au
AFTER UPDATE ON archive_imports
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_archive_imports_ad
AFTER DELETE ON archive_imports
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_discovery_sources_ai
AFTER INSERT ON discovery_sources
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_discovery_sources_au
AFTER UPDATE ON discovery_sources
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_discovery_sources_ad
AFTER DELETE ON discovery_sources
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_source_partitions_ai
AFTER INSERT ON source_partitions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_source_partitions_au
AFTER UPDATE ON source_partitions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_source_partitions_ad
AFTER DELETE ON source_partitions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_investigations_ai
AFTER INSERT ON investigations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_investigations_au
AFTER UPDATE ON investigations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_investigations_ad
AFTER DELETE ON investigations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_hypotheses_ai
AFTER INSERT ON hypotheses
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_hypotheses_au
AFTER UPDATE ON hypotheses
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_hypotheses_ad
AFTER DELETE ON hypotheses
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_opportunities_ai
AFTER INSERT ON opportunities
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_opportunities_au
AFTER UPDATE ON opportunities
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_opportunities_ad
AFTER DELETE ON opportunities
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_definitions_ai
AFTER INSERT ON validation_definitions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_definitions_au
AFTER UPDATE ON validation_definitions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_definitions_ad
AFTER DELETE ON validation_definitions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_runs_ai
AFTER INSERT ON validation_runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_runs_au
AFTER UPDATE ON validation_runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_runs_ad
AFTER DELETE ON validation_runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_evidence_ai
AFTER INSERT ON evidence
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_evidence_au
AFTER UPDATE ON evidence
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_evidence_ad
AFTER DELETE ON evidence
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_drafts_ai
AFTER INSERT ON contribution_drafts
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_drafts_au
AFTER UPDATE ON contribution_drafts
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_drafts_ad
AFTER DELETE ON contribution_drafts
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_code_snapshots_ai
AFTER INSERT ON code_snapshots
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_code_snapshots_au
AFTER UPDATE ON code_snapshots
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_code_snapshots_ad
AFTER DELETE ON code_snapshots
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_code_documents_ai
AFTER INSERT ON code_documents
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_code_documents_au
AFTER UPDATE ON code_documents
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_code_documents_ad
AFTER DELETE ON code_documents
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_lenses_ai
AFTER INSERT ON lenses
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_lenses_au
AFTER UPDATE ON lenses
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_lenses_ad
AFTER DELETE ON lenses
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_collections_ai
AFTER INSERT ON collections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_collections_au
AFTER UPDATE ON collections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_collections_ad
AFTER DELETE ON collections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_collection_members_ai
AFTER INSERT ON collection_members
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_collection_members_au
AFTER UPDATE ON collection_members
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_collection_members_ad
AFTER DELETE ON collection_members
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_workspaces_ai
AFTER INSERT ON workspaces
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_workspaces_au
AFTER UPDATE ON workspaces
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_workspaces_ad
AFTER DELETE ON workspaces
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_runs_ai
AFTER INSERT ON cluster_runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_runs_au
AFTER UPDATE ON cluster_runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_runs_ad
AFTER DELETE ON cluster_runs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_clusters_ai
AFTER INSERT ON clusters
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_clusters_au
AFTER UPDATE ON clusters
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_clusters_ad
AFTER DELETE ON clusters
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_members_ai
AFTER INSERT ON cluster_members
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_members_au
AFTER UPDATE ON cluster_members
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_members_ad
AFTER DELETE ON cluster_members
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_overrides_ai
AFTER INSERT ON cluster_overrides
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_overrides_au
AFTER UPDATE ON cluster_overrides
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_overrides_ad
AFTER DELETE ON cluster_overrides
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_facet_observations_ai
AFTER INSERT ON facet_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_facet_observations_au
AFTER UPDATE ON facet_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_facet_observations_ad
AFTER DELETE ON facet_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_jobs_ai
AFTER INSERT ON jobs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_jobs_au
AFTER UPDATE ON jobs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_jobs_ad
AFTER DELETE ON jobs
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_job_events_ai
AFTER INSERT ON job_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_job_events_au
AFTER UPDATE ON job_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_job_events_ad
AFTER DELETE ON job_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_dossiers_ai
AFTER INSERT ON dossiers
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_dossiers_au
AFTER UPDATE ON dossiers
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_dossiers_ad
AFTER DELETE ON dossiers
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_dossier_sources_ai
AFTER INSERT ON dossier_sources
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_dossier_sources_au
AFTER UPDATE ON dossier_sources
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_dossier_sources_ad
AFTER DELETE ON dossier_sources
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_triage_events_ai
AFTER INSERT ON triage_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_triage_events_au
AFTER UPDATE ON triage_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_triage_events_ad
AFTER DELETE ON triage_events
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contributions_ai
AFTER INSERT ON contributions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contributions_au
AFTER UPDATE ON contributions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contributions_ad
AFTER DELETE ON contributions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_outcomes_ai
AFTER INSERT ON contribution_outcomes
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_outcomes_au
AFTER UPDATE ON contribution_outcomes
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_outcomes_ad
AFTER DELETE ON contribution_outcomes
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_rate_limit_observations_ai
AFTER INSERT ON rate_limit_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_rate_limit_observations_au
AFTER UPDATE ON rate_limit_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_rate_limit_observations_ad
AFTER DELETE ON rate_limit_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_job_owners_ai
AFTER INSERT ON job_owners
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_job_owners_au
AFTER UPDATE ON job_owners
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_job_owners_ad
AFTER DELETE ON job_owners
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_links_ai
AFTER INSERT ON portfolio_links
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_links_au
AFTER UPDATE ON portfolio_links
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_links_ad
AFTER DELETE ON portfolio_links
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signal_snapshots_ai
AFTER INSERT ON portfolio_signal_snapshots
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signal_snapshots_au
AFTER UPDATE ON portfolio_signal_snapshots
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signal_snapshots_ad
AFTER DELETE ON portfolio_signal_snapshots
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signals_ai
AFTER INSERT ON portfolio_signals
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signals_au
AFTER UPDATE ON portfolio_signals
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signals_ad
AFTER DELETE ON portfolio_signals
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signal_projections_ai
AFTER INSERT ON portfolio_signal_projections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signal_projections_au
AFTER UPDATE ON portfolio_signal_projections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_portfolio_signal_projections_ad
AFTER DELETE ON portfolio_signal_projections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_resolution_records_ai
AFTER INSERT ON resolution_records
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_resolution_records_au
AFTER UPDATE ON resolution_records
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_resolution_records_ad
AFTER DELETE ON resolution_records
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_resolution_projections_ai
AFTER INSERT ON resolution_projections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_resolution_projections_au
AFTER UPDATE ON resolution_projections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_resolution_projections_ad
AFTER DELETE ON resolution_projections
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_projection_state_ai
AFTER INSERT ON cluster_projection_state
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_projection_state_au
AFTER UPDATE ON cluster_projection_state
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_cluster_projection_state_ad
AFTER DELETE ON cluster_projection_state
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_projection_states_ai
AFTER INSERT ON projection_states
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_projection_states_au
AFTER UPDATE ON projection_states
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_projection_states_ad
AFTER DELETE ON projection_states
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_thread_search_documents_ai
AFTER INSERT ON thread_search_documents
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_thread_search_documents_au
AFTER UPDATE ON thread_search_documents
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_thread_search_documents_ad
AFTER DELETE ON thread_search_documents
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_concerns_ai
AFTER INSERT ON concerns
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_concerns_au
AFTER UPDATE ON concerns
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_concerns_ad
AFTER DELETE ON concerns
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_concern_links_ai
AFTER INSERT ON concern_links
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_concern_links_au
AFTER UPDATE ON concern_links
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_concern_links_ad
AFTER DELETE ON concern_links
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_run_groups_ai
AFTER INSERT ON validation_run_groups
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_run_groups_au
AFTER UPDATE ON validation_run_groups
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_validation_run_groups_ad
AFTER DELETE ON validation_run_groups
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_manifests_ai
AFTER INSERT ON contribution_manifests
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_manifests_au
AFTER UPDATE ON contribution_manifests
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_manifests_ad
AFTER DELETE ON contribution_manifests
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_draft_revisions_ai
AFTER INSERT ON contribution_draft_revisions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_draft_revisions_au
AFTER UPDATE ON contribution_draft_revisions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_contribution_draft_revisions_ad
AFTER DELETE ON contribution_draft_revisions
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

-- Operational state and local workflow artifacts are deliberately not part of
-- the corpus identity. Job heartbeats, progress events, validation records,
-- drafts, and evidence publication must not make an otherwise stable offline
-- corpus read stale while a workflow is running.
DROP TRIGGER corpus_revision_archive_imports_ai;
DROP TRIGGER corpus_revision_archive_imports_au;
DROP TRIGGER corpus_revision_archive_imports_ad;
DROP TRIGGER corpus_revision_cluster_runs_ai;
DROP TRIGGER corpus_revision_cluster_runs_au;
DROP TRIGGER corpus_revision_cluster_runs_ad;
DROP TRIGGER corpus_revision_collection_members_ai;
DROP TRIGGER corpus_revision_collection_members_au;
DROP TRIGGER corpus_revision_collection_members_ad;
DROP TRIGGER corpus_revision_collections_ai;
DROP TRIGGER corpus_revision_collections_au;
DROP TRIGGER corpus_revision_collections_ad;
DROP TRIGGER corpus_revision_concern_links_ai;
DROP TRIGGER corpus_revision_concern_links_au;
DROP TRIGGER corpus_revision_concern_links_ad;
DROP TRIGGER corpus_revision_concerns_ai;
DROP TRIGGER corpus_revision_concerns_au;
DROP TRIGGER corpus_revision_concerns_ad;
DROP TRIGGER corpus_revision_contribution_draft_revisions_ai;
DROP TRIGGER corpus_revision_contribution_draft_revisions_au;
DROP TRIGGER corpus_revision_contribution_draft_revisions_ad;
DROP TRIGGER corpus_revision_contribution_drafts_ai;
DROP TRIGGER corpus_revision_contribution_drafts_au;
DROP TRIGGER corpus_revision_contribution_drafts_ad;
DROP TRIGGER corpus_revision_contribution_manifests_ai;
DROP TRIGGER corpus_revision_contribution_manifests_au;
DROP TRIGGER corpus_revision_contribution_manifests_ad;
DROP TRIGGER corpus_revision_contribution_outcomes_ai;
DROP TRIGGER corpus_revision_contribution_outcomes_au;
DROP TRIGGER corpus_revision_contribution_outcomes_ad;
DROP TRIGGER corpus_revision_contributions_ai;
DROP TRIGGER corpus_revision_contributions_au;
DROP TRIGGER corpus_revision_contributions_ad;
DROP TRIGGER corpus_revision_discovery_checkpoints_ai;
DROP TRIGGER corpus_revision_discovery_checkpoints_au;
DROP TRIGGER corpus_revision_discovery_checkpoints_ad;
DROP TRIGGER corpus_revision_discovery_sources_ai;
DROP TRIGGER corpus_revision_discovery_sources_au;
DROP TRIGGER corpus_revision_discovery_sources_ad;
DROP TRIGGER corpus_revision_evidence_ai;
DROP TRIGGER corpus_revision_evidence_au;
DROP TRIGGER corpus_revision_evidence_ad;
DROP TRIGGER corpus_revision_frontier_items_ai;
DROP TRIGGER corpus_revision_frontier_items_au;
DROP TRIGGER corpus_revision_frontier_items_ad;
DROP TRIGGER corpus_revision_hypotheses_ai;
DROP TRIGGER corpus_revision_hypotheses_au;
DROP TRIGGER corpus_revision_hypotheses_ad;
DROP TRIGGER corpus_revision_investigations_ai;
DROP TRIGGER corpus_revision_investigations_au;
DROP TRIGGER corpus_revision_investigations_ad;
DROP TRIGGER corpus_revision_job_events_ai;
DROP TRIGGER corpus_revision_job_events_au;
DROP TRIGGER corpus_revision_job_events_ad;
DROP TRIGGER corpus_revision_job_owners_ai;
DROP TRIGGER corpus_revision_job_owners_au;
DROP TRIGGER corpus_revision_job_owners_ad;
DROP TRIGGER corpus_revision_jobs_ai;
DROP TRIGGER corpus_revision_jobs_au;
DROP TRIGGER corpus_revision_jobs_ad;
DROP TRIGGER corpus_revision_lenses_ai;
DROP TRIGGER corpus_revision_lenses_au;
DROP TRIGGER corpus_revision_lenses_ad;
DROP TRIGGER corpus_revision_opportunities_ai;
DROP TRIGGER corpus_revision_opportunities_au;
DROP TRIGGER corpus_revision_opportunities_ad;
DROP TRIGGER corpus_revision_projection_states_ai;
DROP TRIGGER corpus_revision_projection_states_au;
DROP TRIGGER corpus_revision_projection_states_ad;
DROP TRIGGER corpus_revision_rate_limit_observations_ai;
DROP TRIGGER corpus_revision_rate_limit_observations_au;
DROP TRIGGER corpus_revision_rate_limit_observations_ad;
DROP TRIGGER corpus_revision_run_events_ai;
DROP TRIGGER corpus_revision_run_events_au;
DROP TRIGGER corpus_revision_run_events_ad;
DROP TRIGGER corpus_revision_runs_ai;
DROP TRIGGER corpus_revision_runs_au;
DROP TRIGGER corpus_revision_runs_ad;
DROP TRIGGER corpus_revision_source_partitions_ai;
DROP TRIGGER corpus_revision_source_partitions_au;
DROP TRIGGER corpus_revision_source_partitions_ad;
DROP TRIGGER corpus_revision_triage_events_ai;
DROP TRIGGER corpus_revision_triage_events_au;
DROP TRIGGER corpus_revision_triage_events_ad;
DROP TRIGGER corpus_revision_validation_definitions_ai;
DROP TRIGGER corpus_revision_validation_definitions_au;
DROP TRIGGER corpus_revision_validation_definitions_ad;
DROP TRIGGER corpus_revision_validation_run_groups_ai;
DROP TRIGGER corpus_revision_validation_run_groups_au;
DROP TRIGGER corpus_revision_validation_run_groups_ad;
DROP TRIGGER corpus_revision_validation_runs_ai;
DROP TRIGGER corpus_revision_validation_runs_au;
DROP TRIGGER corpus_revision_validation_runs_ad;
DROP TRIGGER corpus_revision_workspaces_ai;
DROP TRIGGER corpus_revision_workspaces_au;
DROP TRIGGER corpus_revision_workspaces_ad;


-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Revisions are derived state; remove their mutation hooks with the table.
DROP TRIGGER IF EXISTS corpus_revision_repositories_ai;
DROP TRIGGER IF EXISTS corpus_revision_repositories_au;
DROP TRIGGER IF EXISTS corpus_revision_repositories_ad;
DROP TRIGGER IF EXISTS corpus_revision_repository_observations_ai;
DROP TRIGGER IF EXISTS corpus_revision_repository_observations_au;
DROP TRIGGER IF EXISTS corpus_revision_repository_observations_ad;
DROP TRIGGER IF EXISTS corpus_revision_threads_ai;
DROP TRIGGER IF EXISTS corpus_revision_threads_au;
DROP TRIGGER IF EXISTS corpus_revision_threads_ad;
DROP TRIGGER IF EXISTS corpus_revision_thread_observations_ai;
DROP TRIGGER IF EXISTS corpus_revision_thread_observations_au;
DROP TRIGGER IF EXISTS corpus_revision_thread_observations_ad;
DROP TRIGGER IF EXISTS corpus_revision_facet_coverage_ai;
DROP TRIGGER IF EXISTS corpus_revision_facet_coverage_au;
DROP TRIGGER IF EXISTS corpus_revision_facet_coverage_ad;
DROP TRIGGER IF EXISTS corpus_revision_runs_ai;
DROP TRIGGER IF EXISTS corpus_revision_runs_au;
DROP TRIGGER IF EXISTS corpus_revision_runs_ad;
DROP TRIGGER IF EXISTS corpus_revision_run_events_ai;
DROP TRIGGER IF EXISTS corpus_revision_run_events_au;
DROP TRIGGER IF EXISTS corpus_revision_run_events_ad;
DROP TRIGGER IF EXISTS corpus_revision_frontier_items_ai;
DROP TRIGGER IF EXISTS corpus_revision_frontier_items_au;
DROP TRIGGER IF EXISTS corpus_revision_frontier_items_ad;
DROP TRIGGER IF EXISTS corpus_revision_discovery_checkpoints_ai;
DROP TRIGGER IF EXISTS corpus_revision_discovery_checkpoints_au;
DROP TRIGGER IF EXISTS corpus_revision_discovery_checkpoints_ad;
DROP TRIGGER IF EXISTS corpus_revision_archive_imports_ai;
DROP TRIGGER IF EXISTS corpus_revision_archive_imports_au;
DROP TRIGGER IF EXISTS corpus_revision_archive_imports_ad;
DROP TRIGGER IF EXISTS corpus_revision_discovery_sources_ai;
DROP TRIGGER IF EXISTS corpus_revision_discovery_sources_au;
DROP TRIGGER IF EXISTS corpus_revision_discovery_sources_ad;
DROP TRIGGER IF EXISTS corpus_revision_source_partitions_ai;
DROP TRIGGER IF EXISTS corpus_revision_source_partitions_au;
DROP TRIGGER IF EXISTS corpus_revision_source_partitions_ad;
DROP TRIGGER IF EXISTS corpus_revision_investigations_ai;
DROP TRIGGER IF EXISTS corpus_revision_investigations_au;
DROP TRIGGER IF EXISTS corpus_revision_investigations_ad;
DROP TRIGGER IF EXISTS corpus_revision_hypotheses_ai;
DROP TRIGGER IF EXISTS corpus_revision_hypotheses_au;
DROP TRIGGER IF EXISTS corpus_revision_hypotheses_ad;
DROP TRIGGER IF EXISTS corpus_revision_opportunities_ai;
DROP TRIGGER IF EXISTS corpus_revision_opportunities_au;
DROP TRIGGER IF EXISTS corpus_revision_opportunities_ad;
DROP TRIGGER IF EXISTS corpus_revision_validation_definitions_ai;
DROP TRIGGER IF EXISTS corpus_revision_validation_definitions_au;
DROP TRIGGER IF EXISTS corpus_revision_validation_definitions_ad;
DROP TRIGGER IF EXISTS corpus_revision_validation_runs_ai;
DROP TRIGGER IF EXISTS corpus_revision_validation_runs_au;
DROP TRIGGER IF EXISTS corpus_revision_validation_runs_ad;
DROP TRIGGER IF EXISTS corpus_revision_evidence_ai;
DROP TRIGGER IF EXISTS corpus_revision_evidence_au;
DROP TRIGGER IF EXISTS corpus_revision_evidence_ad;
DROP TRIGGER IF EXISTS corpus_revision_contribution_drafts_ai;
DROP TRIGGER IF EXISTS corpus_revision_contribution_drafts_au;
DROP TRIGGER IF EXISTS corpus_revision_contribution_drafts_ad;
DROP TRIGGER IF EXISTS corpus_revision_code_snapshots_ai;
DROP TRIGGER IF EXISTS corpus_revision_code_snapshots_au;
DROP TRIGGER IF EXISTS corpus_revision_code_snapshots_ad;
DROP TRIGGER IF EXISTS corpus_revision_code_documents_ai;
DROP TRIGGER IF EXISTS corpus_revision_code_documents_au;
DROP TRIGGER IF EXISTS corpus_revision_code_documents_ad;
DROP TRIGGER IF EXISTS corpus_revision_lenses_ai;
DROP TRIGGER IF EXISTS corpus_revision_lenses_au;
DROP TRIGGER IF EXISTS corpus_revision_lenses_ad;
DROP TRIGGER IF EXISTS corpus_revision_collections_ai;
DROP TRIGGER IF EXISTS corpus_revision_collections_au;
DROP TRIGGER IF EXISTS corpus_revision_collections_ad;
DROP TRIGGER IF EXISTS corpus_revision_collection_members_ai;
DROP TRIGGER IF EXISTS corpus_revision_collection_members_au;
DROP TRIGGER IF EXISTS corpus_revision_collection_members_ad;
DROP TRIGGER IF EXISTS corpus_revision_workspaces_ai;
DROP TRIGGER IF EXISTS corpus_revision_workspaces_au;
DROP TRIGGER IF EXISTS corpus_revision_workspaces_ad;
DROP TRIGGER IF EXISTS corpus_revision_cluster_runs_ai;
DROP TRIGGER IF EXISTS corpus_revision_cluster_runs_au;
DROP TRIGGER IF EXISTS corpus_revision_cluster_runs_ad;
DROP TRIGGER IF EXISTS corpus_revision_clusters_ai;
DROP TRIGGER IF EXISTS corpus_revision_clusters_au;
DROP TRIGGER IF EXISTS corpus_revision_clusters_ad;
DROP TRIGGER IF EXISTS corpus_revision_cluster_members_ai;
DROP TRIGGER IF EXISTS corpus_revision_cluster_members_au;
DROP TRIGGER IF EXISTS corpus_revision_cluster_members_ad;
DROP TRIGGER IF EXISTS corpus_revision_cluster_overrides_ai;
DROP TRIGGER IF EXISTS corpus_revision_cluster_overrides_au;
DROP TRIGGER IF EXISTS corpus_revision_cluster_overrides_ad;
DROP TRIGGER IF EXISTS corpus_revision_facet_observations_ai;
DROP TRIGGER IF EXISTS corpus_revision_facet_observations_au;
DROP TRIGGER IF EXISTS corpus_revision_facet_observations_ad;
DROP TRIGGER IF EXISTS corpus_revision_jobs_ai;
DROP TRIGGER IF EXISTS corpus_revision_jobs_au;
DROP TRIGGER IF EXISTS corpus_revision_jobs_ad;
DROP TRIGGER IF EXISTS corpus_revision_job_events_ai;
DROP TRIGGER IF EXISTS corpus_revision_job_events_au;
DROP TRIGGER IF EXISTS corpus_revision_job_events_ad;
DROP TRIGGER IF EXISTS corpus_revision_dossiers_ai;
DROP TRIGGER IF EXISTS corpus_revision_dossiers_au;
DROP TRIGGER IF EXISTS corpus_revision_dossiers_ad;
DROP TRIGGER IF EXISTS corpus_revision_dossier_sources_ai;
DROP TRIGGER IF EXISTS corpus_revision_dossier_sources_au;
DROP TRIGGER IF EXISTS corpus_revision_dossier_sources_ad;
DROP TRIGGER IF EXISTS corpus_revision_triage_events_ai;
DROP TRIGGER IF EXISTS corpus_revision_triage_events_au;
DROP TRIGGER IF EXISTS corpus_revision_triage_events_ad;
DROP TRIGGER IF EXISTS corpus_revision_contributions_ai;
DROP TRIGGER IF EXISTS corpus_revision_contributions_au;
DROP TRIGGER IF EXISTS corpus_revision_contributions_ad;
DROP TRIGGER IF EXISTS corpus_revision_contribution_outcomes_ai;
DROP TRIGGER IF EXISTS corpus_revision_contribution_outcomes_au;
DROP TRIGGER IF EXISTS corpus_revision_contribution_outcomes_ad;
DROP TRIGGER IF EXISTS corpus_revision_rate_limit_observations_ai;
DROP TRIGGER IF EXISTS corpus_revision_rate_limit_observations_au;
DROP TRIGGER IF EXISTS corpus_revision_rate_limit_observations_ad;
DROP TRIGGER IF EXISTS corpus_revision_job_owners_ai;
DROP TRIGGER IF EXISTS corpus_revision_job_owners_au;
DROP TRIGGER IF EXISTS corpus_revision_job_owners_ad;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_links_ai;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_links_au;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_links_ad;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signal_snapshots_ai;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signal_snapshots_au;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signal_snapshots_ad;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signals_ai;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signals_au;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signals_ad;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signal_projections_ai;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signal_projections_au;
DROP TRIGGER IF EXISTS corpus_revision_portfolio_signal_projections_ad;
DROP TRIGGER IF EXISTS corpus_revision_resolution_records_ai;
DROP TRIGGER IF EXISTS corpus_revision_resolution_records_au;
DROP TRIGGER IF EXISTS corpus_revision_resolution_records_ad;
DROP TRIGGER IF EXISTS corpus_revision_resolution_projections_ai;
DROP TRIGGER IF EXISTS corpus_revision_resolution_projections_au;
DROP TRIGGER IF EXISTS corpus_revision_resolution_projections_ad;
DROP TRIGGER IF EXISTS corpus_revision_cluster_projection_state_ai;
DROP TRIGGER IF EXISTS corpus_revision_cluster_projection_state_au;
DROP TRIGGER IF EXISTS corpus_revision_cluster_projection_state_ad;
DROP TRIGGER IF EXISTS corpus_revision_projection_states_ai;
DROP TRIGGER IF EXISTS corpus_revision_projection_states_au;
DROP TRIGGER IF EXISTS corpus_revision_projection_states_ad;
DROP TRIGGER IF EXISTS corpus_revision_thread_search_documents_ai;
DROP TRIGGER IF EXISTS corpus_revision_thread_search_documents_au;
DROP TRIGGER IF EXISTS corpus_revision_thread_search_documents_ad;
DROP TRIGGER IF EXISTS corpus_revision_concerns_ai;
DROP TRIGGER IF EXISTS corpus_revision_concerns_au;
DROP TRIGGER IF EXISTS corpus_revision_concerns_ad;
DROP TRIGGER IF EXISTS corpus_revision_concern_links_ai;
DROP TRIGGER IF EXISTS corpus_revision_concern_links_au;
DROP TRIGGER IF EXISTS corpus_revision_concern_links_ad;
DROP TRIGGER IF EXISTS corpus_revision_validation_run_groups_ai;
DROP TRIGGER IF EXISTS corpus_revision_validation_run_groups_au;
DROP TRIGGER IF EXISTS corpus_revision_validation_run_groups_ad;
DROP TRIGGER IF EXISTS corpus_revision_contribution_manifests_ai;
DROP TRIGGER IF EXISTS corpus_revision_contribution_manifests_au;
DROP TRIGGER IF EXISTS corpus_revision_contribution_manifests_ad;
DROP TRIGGER IF EXISTS corpus_revision_contribution_draft_revisions_ai;
DROP TRIGGER IF EXISTS corpus_revision_contribution_draft_revisions_au;
DROP TRIGGER IF EXISTS corpus_revision_contribution_draft_revisions_ad;
DROP TABLE IF EXISTS corpus_state;
-- +goose StatementEnd

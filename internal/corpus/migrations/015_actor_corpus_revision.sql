-- +goose Up
-- +goose StatementBegin
CREATE TRIGGER corpus_revision_actors_ai
AFTER INSERT ON actors
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actors_au
AFTER UPDATE ON actors
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actors_ad
AFTER DELETE ON actors
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_aliases_ai
AFTER INSERT ON actor_aliases
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_aliases_au
AFTER UPDATE ON actor_aliases
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_aliases_ad
AFTER DELETE ON actor_aliases
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_observations_ai
AFTER INSERT ON actor_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_observations_au
AFTER UPDATE ON actor_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_observations_ad
AFTER DELETE ON actor_observations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_profiles_ai
AFTER INSERT ON actor_profiles
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_profiles_au
AFTER UPDATE ON actor_profiles
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_profiles_ad
AFTER DELETE ON actor_profiles
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_social_accounts_ai
AFTER INSERT ON actor_social_accounts
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_social_accounts_au
AFTER UPDATE ON actor_social_accounts
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_social_accounts_ad
AFTER DELETE ON actor_social_accounts
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_organization_memberships_ai
AFTER INSERT ON actor_organization_memberships
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_organization_memberships_au
AFTER UPDATE ON actor_organization_memberships
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_organization_memberships_ad
AFTER DELETE ON actor_organization_memberships
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_pinned_items_ai
AFTER INSERT ON actor_pinned_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_pinned_items_au
AFTER UPDATE ON actor_pinned_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_pinned_items_ad
AFTER DELETE ON actor_pinned_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_repository_affiliations_ai
AFTER INSERT ON actor_repository_affiliations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_repository_affiliations_au
AFTER UPDATE ON actor_repository_affiliations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_repository_affiliations_ad
AFTER DELETE ON actor_repository_affiliations
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_periods_ai
AFTER INSERT ON actor_contribution_periods
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_periods_au
AFTER UPDATE ON actor_contribution_periods
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_periods_ad
AFTER DELETE ON actor_contribution_periods
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_days_ai
AFTER INSERT ON actor_contribution_days
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_days_au
AFTER UPDATE ON actor_contribution_days
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_days_ad
AFTER DELETE ON actor_contribution_days
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_items_ai
AFTER INSERT ON actor_contribution_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_items_au
AFTER UPDATE ON actor_contribution_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_contribution_items_ad
AFTER DELETE ON actor_contribution_items
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_repository_contribution_totals_ai
AFTER INSERT ON actor_repository_contribution_totals
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_repository_contribution_totals_au
AFTER UPDATE ON actor_repository_contribution_totals
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

CREATE TRIGGER corpus_revision_actor_repository_contribution_totals_ad
AFTER DELETE ON actor_repository_contribution_totals
BEGIN
    UPDATE corpus_state SET revision = revision + 1 WHERE id = 1;
END;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS corpus_revision_actors_ai;
DROP TRIGGER IF EXISTS corpus_revision_actors_au;
DROP TRIGGER IF EXISTS corpus_revision_actors_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_aliases_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_aliases_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_aliases_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_observations_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_observations_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_observations_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_profiles_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_profiles_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_profiles_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_social_accounts_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_social_accounts_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_social_accounts_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_organization_memberships_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_organization_memberships_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_organization_memberships_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_pinned_items_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_pinned_items_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_pinned_items_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_repository_affiliations_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_repository_affiliations_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_repository_affiliations_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_periods_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_periods_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_periods_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_days_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_days_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_days_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_items_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_items_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_contribution_items_ad;
DROP TRIGGER IF EXISTS corpus_revision_actor_repository_contribution_totals_ai;
DROP TRIGGER IF EXISTS corpus_revision_actor_repository_contribution_totals_au;
DROP TRIGGER IF EXISTS corpus_revision_actor_repository_contribution_totals_ad;
-- +goose StatementEnd

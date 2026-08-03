# Actor corpus

GitContribute treats a GitHub account as an actor with independently acquired
facts. This avoids the common failure mode where a user-search result is
mistaken for a hydrated profile or a missing page is mistaken for an empty
relationship.

## Identity and profile

`actors` stores the current provider identity. Its product key is
`github:node:<node_id>` when GitHub supplies a node ID, otherwise the fallback
is `github:login:<normalized_login>`. `actor_aliases` records every observed
login and points it at the same actor, allowing exact reads by current or old
login. Search results write identity observations only.

`actor_profiles` is the current complete profile projection. GitHub fields
such as name, bio, company, location, website, public email, hireability,
followers, following, public repository count, and public gist count are
nullable. Null means “not present in this observation”; it is not converted to
an empty string, zero, or false. `actor_observations` retains the raw profile or
facet payload with acquisition provenance.

## Independently refreshable facets

| Facet | Projection | Acquisition primitive | Important bounds |
| --- | --- | --- | --- |
| profile | `actor_profiles` | `github.sync_users` | 100 exact users |
| social accounts | `actor_social_accounts` | `github.sync_user_social_accounts` | pages, items/user, total requests |
| organizations | `actor_organization_memberships` | `github.sync_user_organizations` | cursor pages, items/user, total requests |
| pinned items | `actor_pinned_items` | `github.sync_user_pinned_items` | 1–6 items/user |
| repositories | `actor_repository_affiliations` | `github.sync_user_repositories` | explicit owned, affiliated, or contributed relationship |
| contributions | period/day/item/total tables | `github.sync_user_contributions` | explicit RFC 3339 interval of at most one year, optionally scoped by organization node ID |

Repository affiliation is not collapsed into a single boolean. The stored
relationship explains why a repository is associated with an actor. A
contribution item records its source kind, occurrence time, optional
repository, provider target identity and URL, restricted flag, and count.
Restricted activity that GitHub cannot disclose remains an aggregate rather
than a fabricated item. Contribution observations use `viewer` authorization
scope because token-visible restricted and private activity is not a public fact.

## Freshness and replacement

Every facet observation carries `observed_at`, `source_updated_at`,
`authorization_scope`, and `complete`. Current projections use
`(source_updated_at, observation_sequence)` ordering. Complete child snapshots
replace atomically. Paginated or truncated reads are stored as observations
but do not replace a previous complete child set.

Corpus reads never contact GitHub. They return coverage alongside facts so an
agent can decide whether the observation is fresh enough for its task. The
server does not encode a universal freshness threshold: profile discovery,
review cleanup, and historical research have different tolerances.

## Query composition

`corpus.search_actors` filters and sorts profile facts. Exact identities use
`corpus.get_actors`; larger child payloads use actor facet resource URIs.
`corpus.search_contributions` composes actor, repository, kind, source, time,
organization scope, sort, and pagination filters. An empty organization scope
selects global contribution periods; an exact node ID selects that organization's
periods. Cursors bind those filters, so they cannot be
reused against a different query. All query pages bind to a corpus snapshot
token and missing coverage remains unknown.

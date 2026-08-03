# Composing MCP tools at scale

GitContribute exposes bounded primitives that agents can compose without
building scalar N+1 loops. Each tool owns one side-effect boundary: offline
corpus read, explicit GitHub acquisition, local state write, Git process, or
authorized validation process.

## Ground rules

- Corpus reads never contact GitHub or refresh data implicitly.
- GitHub reads are explicit, bounded, rate-limited, and may write only local
  observations and projections.
- Missing, stale, paginated, or truncated coverage is unknown, not evidence of
  absence.
- Durable jobs must reach a terminal state through `jobs.get` before their
  observations are treated as current.
- Resource URIs are opaque. Read the exact returned URI with MCP
  `resources/read` rather than reconstructing it.
- No MCP tool mutates GitHub or executes repository-controlled code.

## Repository and thread research

Discovery and hydration stay separate:

```text
github.search_repositories -> corpus.get_repositories
github.search_threads -> resources/read (immutable search artifact)
github.sync_repository_context -> jobs.get -> corpus.get_repositories
github.sync_threads -> jobs.get -> corpus.get_threads
github.sync_thread_facets -> jobs.get -> corpus.get_thread_facets
corpus.find_clusters | corpus.find_neighbors | corpus.find_precedents
```

`github.search_repositories` and `github.search_threads` return one bounded
live page. Search pages do not prove repository-wide absence. Repository and
thread synchronization records coverage separately from stored row counts.
Thread headers do not contain every pull-request fact; comments, reviews,
merge details, checks, files, and other children require explicit facets.

`github.read_source_files` resolves a ref once and reads up to 20 ordered
repository-relative files with per-file and total-byte limits. Its immutable
source-bundle resource records the resolved commit and blob provenance.

`corpus.search_code` accepts up to 20 queries over one repository or snapshot
scope. Every query uses the same offline corpus revision. It never falls back
to live GitHub code search.

## Actor and contributor research

User search deliberately stores identity stubs instead of hydrating every
result:

```text
github.search_users
  -> corpus.search_actors
  -> github.sync_users (selected identities only)
  -> github.sync_user_* (selected facets only)
  -> corpus.get_actors | corpus.get_actor_facets
```

Available facet acquisitions are social accounts, organizations, pinned or
showcase items, repositories, and contribution periods. Each has independent
request, page, or item bounds and independent coverage. Repository facts retain
the explicit `owned`, `affiliated`, or `contributed` relationship.

Contribution research composes an explicit acquisition period with an offline
query:

```text
github.sync_user_contributions -> jobs.get
  -> corpus.search_contributions
```

The offline query filters by actor, repository, contribution kind, source, organization scope, and
time. Its cursor binds those filters and cannot be reused for a different
query. Restricted GitHub activity remains an aggregate unless GitHub discloses
an item. See [Actor corpus](actor-corpus.md) for the SQLite mapping and
freshness semantics.

## Pull-request feedback and CI

Repository-wide feedback indexing is the route for questions such as “find
every comment by this reviewer”:

```text
github.index_pull_request_feedback(state=all) -> jobs.get
  -> corpus.search_pull_request_feedback(feedback_author=exact_login)
  -> resources/read
```

The index records discovery coverage and then acquires issue comments,
submitted reviews, inline comments, and review-thread topology. Body text and
author are separate filters. An empty result is unknown until both discovery
and requested feedback-channel coverage are complete.

Exact PR refresh uses `github.sync_pull_request_feedback`. CI uses
`github.sync_pull_request_ci`; checks and statuses are bound to the observed
head SHA. Offline authored-PR reads use `corpus.search_pull_requests`, and
overlap analysis uses `corpus.find_pull_request_overlaps`.

## Jobs, partial results, and recovery

A durable job may succeed with a partial result. Job success means the bounded
operation ran to completion and recorded every item outcome; it does not turn
retryable or unavailable items into complete facts. Poll several IDs together
with `jobs.get`, then retry only the affected inputs.

Per-item results distinguish:

- `complete`: the requested bounded observation was stored;
- `retryable`: rate limiting or a transient provider failure permits retry;
- `unavailable`: the identity, authorization, or provider capability is not
  available;
- `failed`: a non-retryable request or persistence error occurred.

Coverage carries observation time, source time, authorization scope, and
completeness. The caller chooses a task-appropriate freshness threshold; the
server does not claim that one age limit suits discovery, review cleanup, and
historical research equally.

## Canonical resources

Detailed persisted payloads use the `gitcontribute://` resource namespace.
Examples include thread facets, pull-request feedback, immutable source and
code-index artifacts, actors, investigations, opportunities, evidence,
manifests, drafts, concerns, and workspaces. The catalog does not advertise
dossier or fix-pattern workflow resources.

## Breaking name changes

| Previous name | Canonical name |
| --- | --- |
| `corpus.search_code_batch` | `corpus.search_code` |
| `corpus.list_pull_requests` | `corpus.search_pull_requests` |
| `github.sync_ci_failures` | `github.sync_pull_request_ci` |

Ranking, issue-set preparation, dossier, fix-pattern, preflight, related-work,
scalar code-search, and DeepWiki workflow tools are not advertised. Agents
should select the underlying acquisition and corpus facts that match the task.

## Side-effect matrix

| Tool family | Network | Local write | Process |
| --- | ---: | ---: | ---: |
| `corpus.*` | no | no | no |
| `github.search_*`, `github.sync_*`, source reads | yes | observations only | no |
| `jobs.get`, `jobs.cancel` | no | cancel only | no |
| local workflow state | no | yes | no |
| `code.index_repositories` | remote-dependent | yes | Git only |
| `workspace.*` Git operations | remote-dependent | yes | Git only |
| `validation.run` | no by default | yes | explicit command |

## Verification

The stdio integration tests exercise a real MCP subprocess, catalog discovery,
durable job polling, offline rereads, portfolio synchronization, and resource
handoffs:

```sh
go test ./internal/app -run '^TestMCPStdio(ScalableResearch|PullRequestPortfolio)Flow$' -count=1
```

Run `make verify` before a pull request for uncached tests, changed-code lint,
module tidiness, generated-output verification, and documentation validation.

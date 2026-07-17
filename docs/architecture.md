# Architecture

GitContribute is a local-first research workbench. GitHub is an explicit input
source; SQLite is the durable system of record. Commands that inspect the
corpus must not silently fetch data, execute repository code, or mutate GitHub.

## Component map

```text
                    explicit network reads
                            |
                            v
CLI ---------+         GitHub adapter ------ GitHub API
MCP ---------+----> application service
TUI ---------+              |
                            +----> acquisition/workspace adapters ----> git
                            |
                            +----> validation runner -----------------> process
                            |
                            v
                      SQLite corpus
                  observations + projections
                            |
                            v
              offline search, radar, health, dossiers, thread briefs,
             investigations, evidence, readiness, and drafts
```

The dependency direction is toward product-owned contracts:

- `internal/app` owns use cases and side-effect decisions.
- `internal/corpus` owns persistence, transactions, migrations, and local
  query behavior.
- `internal/github`, `internal/acquire`, `internal/workspace`, and the evidence
  runner are adapters for external capabilities.
- `internal/cli`, `internal/mcpserver`, and `internal/tui` translate user or
  protocol input into application calls. They do not own product rules.

Third-party SDK and database types terminate inside their adapters. The
application and domain packages expose product-owned values and interfaces.

## Capability boundaries

| Capability | Examples | Network | Local write | Process execution | GitHub mutation |
| --- | --- | ---: | ---: | ---: | ---: |
| Corpus read | search, health, dossier show, research brief, readiness, MCP resources | no | no | no | no |
| Corpus write | investigations, start-thread, evidence, lenses, tracking | no | yes | no | no |
| GitHub read | sync, crawl, hydrate | yes | yes | no | no |
| Git acquisition | acquire, workspace create | remote-dependent | yes | `git` only | no |
| Validation | validation run with explicit execution | no by default | yes | yes | no |

Version 1 has no GitHub mutation path. Adding one requires a separate
application capability and protocol annotation; it must not be hidden behind a
read operation.

## Corpus model

The corpus separates source history from convenient current state:

- **Observations** are append-only records of source payloads and provenance.
- **Projections** are normalized repository and thread rows used by local
  queries.
- **Facet observations** store paginated child data such as issue comments,
  reviews, and review comments.
- **Facet coverage** records whether a facet fetch completed and the source
  revision it represents.

Repository and thread projections use this ordering:

```text
(source_updated_at, observation_sequence)
```

A newer source timestamp wins. If timestamps are equal or unavailable, the
local observation sequence breaks the tie. This prevents late or retried work
from replacing a newer projection.

### Complete facet replacement

Hydration buffers every page before writing. `ApplyFacetObservationSet` then
replaces the previous facet and advances coverage in one transaction:

```text
fetch page 1..N -> validate complete set -> begin transaction
                 -> compare source ordering
                 -> replace observations
                 -> advance coverage
                 -> commit
```

Cancellation, a page error, or a stale source revision leaves the previous
complete set visible. An empty complete set is meaningful: it replaces old
children and records complete coverage with zero items.

## Durable jobs

Long-running application operations return durable job IDs. Job state and
events live in SQLite; goroutines only perform the active work.

Each `JobExecutor` registers an opaque owner ID and periodically updates its
lease. A new executor reconciles only running jobs whose owner is absent or
stale. It must never fail jobs owned by another live process.

```text
queued -> running -> succeeded
                  -> failed
                  -> cancelled
```

Terminal states do not transition again. Cancellation is first persisted, then
delivered to an in-process worker directly or observed by its polling loop from
another process. Reconciliation uses an immediate SQLite transaction so a
heartbeat cannot interleave between the liveness read and stale-owner update.

## GitHub transport

The GitHub adapter wraps `go-github` behind narrow read interfaces. For each
logical request, the retry transport invokes the rate-limited base transport:

```text
go-github -> bounded retry loop -> rate limiter -> HTTP transport
```

Putting the limiter inside the retry loop means every actual attempt consumes
rate capacity. Only replayable reads are retried. Backoff honors GitHub rate
headers, is bounded, observes context cancellation, and redacts URL userinfo
before retry metadata is persisted.

## Acquisition and workspaces

Acquisition and workspace packages invoke `git` directly with prompts, hooks,
global configuration, optional locks, and repository filesystem monitors
disabled where applicable. Crawling and indexing never run repository code.

Managed mirrors are keyed by repository identity and remote, and acquisition
uses both in-process and filesystem locks. Remote validation rejects embedded
credentials before metadata is written. Transient worktrees are checked for
cleanliness and removed after indexing.

Validation is a different capability. It executes only after the caller passes
the explicit execution flag and records the command, working directory,
environment allowlist, timeout, output bound, and result.

## Search and analysis

Search uses the local SQLite corpus and FTS indexes. Cursors encode their query
and scope so they cannot be reused for a different search. Ordering always has
a deterministic tie-breaker.

Scores are explanations, not opaque relevance claims. They are derived from
stored matches, freshness, coverage, and optional lens weights. Lens ranking
uses a bounded population and therefore does not support cursor pagination.
Contribution Radar similarly ranks a bounded open-issue population, separates
eligibility from score, and reports positive signals, risks, blockers, and
unknown evidence. Missing coverage is never silently converted into a negative
signal. Health metrics, dossier generation, and thread research briefs also
operate only on stored facts and report partial or missing coverage when
required facets are incomplete. A research-brief section must carry a source
reference or an explicit unknown reason; untrusted thread text remains data and
cannot grant an adapter additional authority.

Starting an investigation from a thread is an explicit corpus-write capability.
The investigation and seed hypothesis are committed in one transaction and
carry the exact thread observation ID, source timestamp, and observation
sequence used as their baseline. A partial unique origin key returns the
existing open pair on repeated or concurrent requests; later thread projections
do not rewrite that baseline, and a closed investigation releases the origin
for a deliberate new start.

Evidence freshness is a read-time assessment over stored corpus revisions, not
another persisted evidence relation. Source-backed evidence can record the
repository, thread, facet, or guidance revision it used; readers compare those
recorded revisions with the current winning local projections and return
`fresh`, `stale`, `unknown`, or `not_applicable`. Freshness reads must not
perform network access, execute processes, delete evidence, or silently treat
stale evidence as invalid. Tracking exports carry evidence provenance in schema
version 2 while accepting older unversioned bundles that do not contain
evidence records.

Contribution readiness is also a pure corpus-read capability. It re-evaluates a
versioned rule set for one opportunity and returns deterministic checks with
`pass`, `warn`, `block`, or `unknown` status, evidence references, and
remediation text. Only objectively unsubmitable local states, such as an
archived repository, closed target thread, failing candidate validation, or
unresolved contradicting evidence, should block. Missing guidance, missing
coverage, stale evidence, or incomplete validation usually remain `warn` or
`unknown`. Readiness must not fetch GitHub, execute validation, mutate
opportunities, or inspect repository-controlled code while evaluating a gate.

## Schema changes

Migrations are embedded from `internal/corpus/migrations` and applied by Goose
when the corpus opens. Add migrations in numeric order; never edit an applied
migration to change an existing database. Every migration needs a working Down
section unless SQLite cannot represent the reversal safely, in which case the
reason belongs in the migration and an ADR.

Storage changes should include tests for upgrade behavior, rollback when
supported, stale-write rejection, transaction atomicity, and deterministic
query ordering.

## Further decisions

- [ADR 0001: Independent implementation](adr/0001-independent-implementation.md)
- [ADR 0002: Application and corpus boundaries](adr/0002-application-and-corpus-boundaries.md)
- [ADR 0003: Explicit execution boundaries](adr/0003-execution-safety.md)

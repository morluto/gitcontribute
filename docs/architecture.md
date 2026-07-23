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
                            +----> DeepWiki adapter -----> public DeepWiki MCP
                            |
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
- `internal/github`, `internal/deepwiki`, `internal/acquire`,
  `internal/workspace`, and the evidence runner are adapters for external
  capabilities. DeepWiki prose is untrusted derived context, is not persisted,
  and never updates GitHub projections.
- `internal/cli`, `internal/mcpserver`, and `internal/tui` translate user or
  protocol input into application calls. They do not own product rules.
  MCP prompts are static workflow guidance; they cannot grant new authority or
  turn repository content into instructions.

Third-party SDK and database types terminate inside their adapters. The
application and domain packages expose product-owned values and interfaces.

## Capability boundaries

| Capability | Examples | Network | Local write | Process execution | GitHub mutation |
| --- | --- | ---: | ---: | ---: | ---: |
| Corpus read | search, health, dossier show, research brief, readiness, MCP resources | no | no | no | no |
| Corpus write | investigations, start-thread, evidence, lenses, tracking, cluster governance | no | yes | no | no |
| Derived projection refresh | explicit `clusters refresh OWNER/REPO` | no | yes | no | no |
| Private MCP runtime installation | explicit setup `--mode mcp` | no | yes | no | no |
| Global CLI installation | explicit setup `--mode cli` or `--mode both` | npm registry dependent | yes | `npm` only | no |
| Setup verification | all applied setup modes | no | no | `git --version` | no |
| GitHub read | sync, crawl, hydrate | yes | yes | no | no |
| DeepWiki external read | public repository structure, contents, questions | yes | no | no | no |
| Git acquisition | acquire, workspace create | remote-dependent | yes | `git` only | no |
| Local merge check | compare already-fetched revisions | no | no | `git` only | no |
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

An explicit repository sync also checks a fixed, bounded set of conventional
`CONTRIBUTING.md` and AI-policy paths. Found text is stored as an untrusted
repository-level `contribution_guidance` facet with exact file provenance.
Offline readers may classify only predefined policy statements; repository
text cannot introduce instructions or grant capabilities.

Thread-header sync never fetches pull-request detail facets per listed item.
Metadata, fixed policy paths, exact reads, and list pages share one explicit
request budget; batch sync plans conservative per-repository allocations in
stable input order before starting workers. Generated network remediation
commands always carry the applicable explicit page or request bounds.
CLI sync and archive-refresh commands compute and print a conservative request
ceiling before resolving the GitHub reader. The same normalized plan supplies
the post-run `planned_requests` and `request_budget` fields, so preflight and
execution cannot silently use different bounds.

The issue-list endpoint used by thread-header sync does not expose pull-request
merge state. A header-only closed pull request therefore stores merge state as
unknown, not false. Explicit `pr_details` hydration makes the value known, and
later header refreshes cannot erase that observation. Offline filters for
`merged=true` or `merged=false`, dossier outcome groups, precedent/seed
classification, portfolio attention, and health merge rates include only
explicitly observed outcomes. Unknown closed outcomes are surfaced separately;
MCP thread output omits the nullable `merged` field until it is known.
Health also consumes the repository-level `threads` coverage fact instead of
inferring completeness from the number of rows returned. External merge rates
are nullable: when no explicitly observed closed external outcome exists, JSON
reports `null` and the health coverage explains why the rate is unavailable. A
known zero merge rate remains distinct from an unknown rate.

Authored pull requests use the ordinary repository and thread projections.
REST `pr_details` and `pr_reviews` facets are combined with typed GraphQL
facets for checks, unresolved review threads, detailed merge state, merge queue,
closing issues, and changed files. Each facet has independent coverage; an
incomplete refresh preserves the previous complete child snapshot but marks
the newer coverage incomplete. Offline portfolio reads therefore return
`unknown` instead of treating missing checks as passing or missing overlap
signals as no overlap.

Portfolio relationships and derived resolution records are local product
contracts. Their normalized snapshots carry rule versions and exact source
observation references. Explicit timeline events may produce a resolution;
closing-issue relationships remain relationship evidence until completion is
independently observed. Lexical similarity alone never becomes a root-cause
claim. Corpus portfolio and resolution reads perform no network access.

Repository and thread projections use this ordering:

```text
(source_updated_at, observation_sequence)
```

A newer source timestamp wins. If timestamps are equal or unavailable, the
local observation sequence breaks the tie. This prevents late or retried work
from replacing a newer projection.

### Duplicate-cluster projection

Duplicate clusters are a versioned derived projection over stored repository
threads. Listing a repository's clusters, inspecting one cluster, finding a
member's cluster, and MCP `corpus.find_clusters` are pure corpus reads: they do
not score candidates, write SQLite, fetch GitHub, or execute a process.

Refresh is a separate, explicit local-write capability:

```text
read one repository snapshot
  -> identify source + governance + rule versions
  -> close the read transaction
  -> perform bounded, cancellable exact comparisons
  -> reconcile durable governance
  -> begin write transaction and recheck revisions
  -> atomically replace the visible projection
```

A projection identity consists of the repository, a full SHA-256 source
revision, a monotonic governance revision, and a similarity rule version. The
source revision covers every candidate field consumed by scoring or persisted
output. An empty completed projection has an identity too, so unchanged empty
repositories do not recompute on every refresh. If source or governance changes
during computation, commit returns a stale-input error and leaves the previous
complete projection visible.

`duplicate-v1` is exact all-pairs work under one 10,000,000-comparison budget;
the maximum candidate population is derived from that budget rather than
configured independently. Cancellation is checked during preparation and pair
evaluation. Similarity preparation and scoring live in `internal/similarity`;
`internal/clustering` remains storage-free, while `internal/corpus` owns
snapshots, governance transactions, and atomic projection replacement.

Membership overrides are durable governance, not direct projection edits.
Adding an override and advancing its repository governance revision happen in
one transaction. The next explicit refresh applies the decision. A governed
canonical choice may change the displayed canonical member, but the stable
cluster identity remains anchored to the engine-selected canonical member so
the next refresh can recover its history.

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

## Persistent job records

Long-running application operations return stable job IDs. Job state and
events live in SQLite; active execution remains process-bound. A stale running
job is marked failed after restart and is never silently replayed, because
external reads and host operations cannot all promise safe automatic replay.
The agent may inspect the stored request and explicitly resubmit an idempotent
operation after reviewing the failure.

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
MCP job reads expose structured phase, completed-item, total-item, percentage,
and retry-delay fields. Concise polling omits stored request and result payloads;
detailed mode retrieves them after a finalist is terminal. Batch reads and
cancellation preserve input order and isolate per-item failures; free-form job
event text is not an MCP progress contract.

### Bounded batch operations

Batch MCP operations preserve input order and return an outcome per input key.
Independent GitHub reads use fixed server-side concurrency ceilings; code
acquisition uses a lower ceiling because it performs Git processes and local
writes. A single unavailable or retryable item does not erase successful
siblings. Callers should retry only items marked retryable and use the provided
recovery hint for unavailable inputs.
Duplicate batch keys are rejected before submission instead of being silently
collapsed, because collapsing would make the returned outcome count differ
from the requested input count. Index requests also reject two remotes for the
same repository as ambiguous.

### Agent tool contract

MCP exposes one canonical `gitcontribute://` resource namespace. Historical
resource aliases are not advertised or routed. Tool names follow capability
boundaries (`corpus`, `github`, `code`, `workspace`, `validation`, `workflow`,
and `research`) rather than mirroring low-level API endpoints. Frequently
chained operations may be consolidated only when they share one authority and
one failure boundary; a read must never hide a refresh, write, or process run.
The CLI defaults to the focused `contribute` catalog. Code/workspace execution,
external derived research, diagnostics, portfolio, and advanced similarity are
opt-in profiles; `all` exists for auditing and embedding.

Tool inputs are strict and bounded, output distinguishes total population from
returned/truncated items, and errors state how the caller can recover. MCP SDK
annotations describe observable effects: pure external reads are read-only and
idempotent with open-world access, while reads that also persist projections
remain write operations. Catalog changes require realistic multi-call agent
evaluations, including held-out queries, tool-call count, errors, latency, and
context size; scripted schema checks alone do not establish good tool choice.

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
The MCP definition tool accepts managed workspace IDs rather than arbitrary
host paths. The application resolves each ID and verifies that it belongs to
the selected investigation before persisting executable state. The explicit
CLI remains a local-user interface and may accept a directly supplied path.

## Search and analysis

Search uses the local SQLite corpus and FTS5 indexes; agents query bounded
application tools rather than receiving raw database access. Repository ranking
weights owner/name at 10, topics at 5, and description at 2. Thread ranking
weights title at 10, labels at 5, body at 2, and complete hydrated facet evidence
at 0.5. Code ranking weights path at 5 and content at 1. README text remains
available through code search when indexed; it is not silently treated as
repository metadata because code coverage can be partial.
Scoped code search returns the selected snapshot manifest even when no document
matches, so absence can be separated from a missing or truncated index.
Snapshots created before manifests were introduced report
`indexed_coverage_unknown`; their zero skip counts are never presented as proof
of complete coverage.

Title, labels, body, and hydrated evidence are materialized into one search
document per thread and ranked by one BM25 invocation. Ranks from the legacy
thread and facet indexes are never compared; the facet index is used only to
identify the matching evidence source and excerpt.

Relevance is the default. Equal-ranked results use newest source revision as
the first tie-breaker. Repository and thread tools also expose `sort=updated`
for tasks that explicitly ask for the newest matching records. Search returns
bounded excerpts rather than complete thread bodies or files; exact-object
tools provide details after the agent narrows candidates. Thread search indexes
titles and bodies plus product-selected fields from complete hydrated issue
comments, pull-request reviews, review comments, and opt-in timeline events.
The searchable facet projection is replaced in the same transaction as its
observations; a failed, cancelled, incomplete, or stale refresh cannot expose a
partial search document. Transport pages are collapsed into one semantic facet
document, and matches report the source facet plus a bounded excerpt. Untrusted
discussion remains searchable data and cannot grant capabilities. Cursors
encode their query and scope so they cannot be reused for a different search.
Ordering always has a deterministic tie-breaker.
Hydrated search text is materialized once per complete facet replacement and
bounded to 262,144 characters per thread. Results expose
`match_truncated=true` when that bound omitted text; complete API coverage must
not be mistaken for complete search-text coverage.

FTS rank is retrieval evidence and must not be relabeled as a separately
hand-written score. Match explanations report the actual lower-is-better BM25
rank and the indexed document or hydrated facet that supplied the excerpt;
they do not guess token matches with a second string matcher. Freshness and
coverage are separate facts. Lens ranking
uses a bounded population and therefore does not support cursor pagination.
Contribution Radar similarly ranks a bounded open-issue population, separates
score from the explicit `ready_to_code`, `needs_diagnosis`,
`needs_coordination`, and `blocked` eligibility states, and reports positive
signals, risks, blockers, and unknown evidence. One repository run accepts and
can return up to the same 500-issue population that it evaluates. Eligibility
derives only from stored policy, labels, discussion, ownership, collision, and
coverage facts.
The cross-repository MCP form requires an explicit non-empty repository set
and reports the evaluated `total` and `truncated`. Per-repo
summaries distinguish considered, returned, per-repo truncation, and an
internally capped population. It deliberately does not expose a cursor: callers
raise the bounded limit or narrow the repository set instead of treating a
newly derived ranking as a stable result snapshot.
Radar normalizes PR text, authoritative closing-issue facets, issue/comment
references, opt-in timeline cross-references, and duplicate projections into
bounded `related_work` facts with exact source evidence. Quoted and code-fenced
text is excluded from lexical relationship classification. Only an open PR
with a closing relationship is an implementation blocker; open dependencies
and non-closing PR relationships require coordination.
Missing coverage is never silently converted into a negative score, but it
prevents a ready-to-code claim. Health metrics, dossier generation, and thread
research briefs also operate only on stored facts and report partial or missing
coverage when required facets are incomplete. A research-brief section must
carry a source reference or an explicit unknown reason; untrusted thread text
remains data and
cannot grant an adapter additional authority.

Seed extraction is also a strict offline read. It labels explicitly merged pull
requests as positive examples and explicitly closed-unmerged pull requests as
negative constraints. A closed issue is negative only when GitHub records
`not_planned` or it carries a predefined rejection/supersession label; all
other issues are context, not outcome evidence. The default seed view includes
positive and negative evidence, while contextual issues require an explicit
polarity selection. Repository-controlled titles and bodies may add excerpts
to evidence but cannot determine polarity.

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
stale evidence as invalid. Tracking exports require schema version 2 so
evidence provenance is always explicit; unversioned and other schema versions
are rejected before import writes begin.

### Concern intake ledger

Repo-local concerns are durable intake records for findings that are not yet
contribution hypotheses. They are distinct from precedent `Seed` records,
which are read-only examples extracted from stored threads. Concerns use UUID
identities, explicit lifecycle transitions, bounded FTS5 search, typed
relationships, and source revisions evaluated by the existing evidence
freshness reader.

Creating, listing, searching, updating, or linking a concern is corpus-only:
it performs no GitHub access, worktree read, or process execution. Similarity
and duplicate candidates remain typed links and never become a root-cause
claim. Only the promotion operation may set `promoted`; it atomically creates
the investigation and seed hypothesis, optionally creates an opportunity, and
stores the downstream IDs on the concern. A status-only update cannot create a
partially promoted record.

Concern protocol results expose opaque workspace and evidence IDs but omit
absolute paths and source-reference URLs. Local provenance retains exact source
revisions so `fresh`, `stale`, and `unknown` are derived at read time rather
than persisted as mutable truth.

Contribution readiness is also a pure corpus-read capability. It re-evaluates a
versioned rule set for one opportunity and returns deterministic checks with
`pass`, `warn`, `block`, or `unknown` status, evidence references, and
remediation text. Only objectively unsubmitable local states, such as an
archived repository, closed target thread, failing candidate validation, or
unresolved contradicting evidence, should block. Missing guidance, missing
coverage, stale evidence, or incomplete validation usually remain `warn` or
`unknown`. Readiness must not fetch GitHub, execute validation, mutate
opportunities, or inspect repository-controlled code while evaluating a gate.
MCP exposes the same report as an offline tool/resource and adds workflow
prompts that point agents at local resources first. Those prompts must preserve
the same side-effect boundary: they may suggest explicit tools, but they cannot
authorize network reads, local writes, process execution, or GitHub mutation.

Contribution evidence manifests are explicit local-write exports. They use an
in-toto Statement v1 envelope around a product-owned predicate and bind claims
to repository, opportunity, and optional managed-workspace identities. The
workspace identity covers HEAD, staged and unstaged patches, bounded untracked
content, submodules, and commit metadata; every omitted resource becomes an
explicit gap. Candidate validations are usable only when their pre/post
snapshot is complete, unchanged, and equal to the exported candidate snapshot.
Stored GitHub facets and evidence are evaluated independently, so missing,
stale, or unknown data cannot become a passing claim.

Manifest generation never refreshes GitHub. Callers explicitly sync the exact
repository or pull-request facets first, then export from SQLite and optional
non-mutating Git reads. The content ID excludes generation/evaluation clocks,
is recomputed before persistence, and rejects mismatched subjects or payloads.
Drafts may store a manifest ID as structured metadata; renderers do not copy
manifest claims into public prose.

## Schema changes

Migrations are embedded from `internal/corpus/migrations` and applied by Goose,
but an ordinary corpus read never applies them. Read-only opens inspect the
schema and return a typed migration-required or newer-schema error. Existing
corpora advance only through the explicit `corpus migrate` capability, which
plans pending versions, creates a verified online backup by default, reports
step boundaries, and holds an exclusive cross-process corpus lease. Normal
readers hold shared leases; incompatible work fails fast instead of waiting on
SQLite until an opaque deadline.

Schema history starts from the canonical `001_initial.sql` baseline. The
project does not carry compatibility migrations for pre-release corpus layouts;
future schema changes append migrations from this baseline. A corpus created by
an older development build must be recreated rather than upgraded in place.

`setup` may create a missing empty corpus. It must not migrate an existing
corpus or rebuild derived indexes as a side effect of installing a launcher or
coding-agent integration. Versioned private runtimes are staged independently;
client registration is activated only after the configured corpus is usable.

Add migrations in numeric order; never edit an applied migration to change an
existing database. Every migration needs a working Down section unless SQLite
cannot represent the reversal safely, in which case the reason belongs in the
migration and an ADR. Large data migrations need prerequisite indexes and a
regression plan based on representative row counts.

Durable observations and derived projections have separate lifecycles. Search
and clustering indexes carry product-owned rule versions and status. A read may
report a stale or unavailable projection; rebuilding is an explicit local-write
operation and never triggers GitHub access.

Corpus inventory is also read-only. The corpus-wide view performs grouped
aggregation and returns at most one summary per repository or code-index scope;
it does not load observation payloads or code documents into the application.
Database and WAL bytes remain whole-file measurements because SQLite pages are
shared. Logical observation-payload and code-content bytes are reported
separately rather than attributed to individual database pages.

Semantic commit preparation is a two-step local read. First,
`workspace.inspect_commit_changes` asks Git for a binary/full-index patch and
untracked blob identities, then uses the maintained `sourcegraph/go-diff`
parser to expose stable file and hunk units. Second, the agent supplies semantic
grouping judgment to `workspace.plan_semantic_commits`; deterministic code
rejects stale inventories, unknown or duplicate unit assignments, and invalid
dependency graphs. Ambiguous units remain explicit. A verified reconstruction
record binds one-to-one unit coverage to the exact source patch and untracked
content identities. Neither operation stages files, applies patches, creates
commits, changes refs, executes repository code, or contacts GitHub. Applying a
plan is intentionally a separate future capability with an explicit mutation
boundary.

Storage changes should include tests for upgrade behavior, rollback when
supported, stale-write rejection, transaction atomicity, and deterministic
query ordering.

## Further decisions

- [ADR 0001: Independent implementation](adr/0001-independent-implementation.md)
- [ADR 0002: Application and corpus boundaries](adr/0002-application-and-corpus-boundaries.md)
- [ADR 0003: Explicit execution boundaries](adr/0003-execution-safety.md)
- [ADR 0004: Duplicate clusters are explicit derived projections](adr/0004-derived-cluster-projections.md)

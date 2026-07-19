# ADR 0004: Duplicate clusters are explicit derived projections

- Status: Accepted
- Date: 2026-07-19

## Context

Duplicate clustering is exact all-pairs work and may update durable cluster
state. Performing that work from list or detail operations makes read-shaped
commands unexpectedly expensive and mutating. Computing from several scalar
corpus reads also risks mixing source and governance states, while keeping a
transaction open during pair evaluation blocks unrelated SQLite work.

An empty successful result must be distinguishable from “never refreshed.” A
refresh computed from old thread data or old governance must not replace a
newer completed result.

## Decision

Duplicate clusters are a versioned derived projection with a separate explicit
refresh use case.

- List, detail, member lookup, and MCP cluster operations read only the current
  stored projection.
- Refresh reads candidates, existing clusters, and overrides from one SQLite
  snapshot, closes that transaction, and performs exact computation in memory.
- Exact work uses one pair budget. The candidate ceiling is derived from the
  budget, and cancellation is observed before and during pair evaluation.
- A projection is identified by repository, full SHA-256 candidate source
  revision, monotonic governance revision, and similarity rule version. Empty
  completed projections receive the same identity as non-empty projections.
- Commit obtains the SQLite writer, reloads the bounded source population and
  governance revision, and advances visibility only if both still match the
  computation inputs. A stale or failed commit leaves the previous projection
  visible.
- Membership overrides and governance-revision advancement are one corpus
  transaction. Overrides take effect on the next explicit refresh.
- `internal/similarity` owns versioned scoring rules,
  `internal/clustering` owns pure exact computation, `internal/clusterprojection`
  owns dependency-neutral refresh contracts, and `internal/corpus` owns SQLite
  snapshots and atomic persistence.

No MCP mutation is introduced by this decision.

## Consequences

Cluster reads have predictable offline read semantics and never hide full
population computation. Unchanged and empty repositories can skip work using a
durable identity. Pair evaluation does not lengthen SQLite transactions, and a
concurrent source or governance update cannot be overwritten by stale results.

Callers must deliberately refresh before expecting newly synchronized threads
or governance decisions to appear. Exact all-pairs semantics impose a hard
population ceiling; exceeding it returns a capacity error instead of silently
switching to approximate retrieval.

Changing any output-affecting preparation or scoring behavior requires a new
rule version. Changing fields consumed by clustering requires updating the
source-revision input and its regression tests.

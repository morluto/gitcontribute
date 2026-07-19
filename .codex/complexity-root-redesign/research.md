# Complexity Root-Cause Redesign Research

Goal: [goal.md](goal.md)
Plan: [plan.md](plan.md)

## Evidence policy

- Repository source and `docs/architecture.md` define current behavior and constraints.
- Official Go and SQLite documentation and original papers support technical claims.
- Context7 and DeepWiki are required discovery aids, not authorities. Their output
  is verified against primary material before it influences a design decision.
- No provider-derived prose may alter stored GitHub facts or authorize side effects.

## Current capability map

### CLI cluster command: refresh plus projection write plus response

```text
CLI `clusters OWNER/REPO`
  -> cli.CLI.runClusters(ctx, cmd)
  -> cli.ClusteringService.Clusters(ctx, repo, limit)
  -> app.Service.Clusters(ctx, repo, limit)
  -> corpus.Corpus.Clustering().ComputeForRepo(ctx, repo, DefaultConfig)
  -> clustering.Store.loadCandidates(ctx, repo, MaxCandidates)
  -> clustering.Clusterer.Cluster(candidates, nil)       // no context
  -> clustering.Store.compute: existing clusters + scalar override reads
  -> clustering.Store.persist: run + clusters + members // SQLite write
  -> app projection to cli.ClusterListResult
  -> CLI render
```

Evidence:

- `internal/cli/cli.go:1931-1948` explicitly prints "computing clusters" and
  calls the ambiguous `Clusters` method.
- `internal/cli/interfaces.go:579-584` exposes only `Clusters` and `Cluster`;
  refresh and list capabilities are not distinct.
- `internal/app/clustering.go:14-46` computes and persists before applying the
  response limit.
- `internal/clustering/store.go:25-160` combines candidate loading, CPU
  derivation, governance reconciliation, and persistence.

Caller-visible consequence: a list-shaped CLI surface has write authority and
latency proportional to the entire repository population, not the requested limit.

### MCP cluster command: pure stored projection read

```text
MCP corpus.find_clusters
  -> mcpserver.Server.findClusters(ctx, input)
  -> app.MCPReader.FindClusters(ctx, input)
  -> clustering.Store.ListClusters(ctx, repo, open, limit)
  -> one cluster query + one member query per cluster
  -> MCP output projection
```

Evidence:

- `internal/mcpserver/server.go:644-657` validates a read input bounded to 100.
- `internal/app/mcp.go:546-576` explicitly documents no recomputation.
- `internal/app/surfaces_test.go:244-254` must call the CLI application method
  first so that the MCP read can return clusters.

Caller-visible consequence: CLI and MCP expose different lifecycle semantics
for the same product concept, and the only refresh path is coupled to CLI output.

### Neighbor command: pure live derivation over stored threads

```text
CLI/MCP neighbors
  -> app.Service.Neighbors(ctx, repo, kind, number, limit)
  -> corpus repository/thread/list reads (up to 10,000 candidates)
  -> candidate projections
  -> clustering.Neighbors(query, candidates, DefaultConfig, limit)
  -> score every candidate + full sort
  -> caller projection
```

Properties:

- No network or corpus write occurs.
- Query and candidate feature extraction are not shared with persisted clustering.
- The public operation accepts context, but the CPU scoring helper does not.
- Full population sorting is implementation detail; output requires only stable top-k.

### Precedent command: bounded batch over scalar corpus operations

```text
MCP corpus.find_precedents (1..20 source threads)
  -> app.MCPReader.FindPrecedents(ctx, input)
  -> for each input in order:
       GetRepository
       GetThreadByNumber
       ListThreadsFiltered(closed, limit=2000)
       application-local wordSet/Jaccard for every candidate
       full stable sort + limit
  -> ordered item-level outcomes
```

Properties:

- It correctly remains an offline corpus read and preserves input order.
- Repository and closed populations are re-read for sibling inputs.
- Candidate token sets are recomputed for sibling inputs.
- `wordSet` and `jaccard` duplicate a second definition of similarity outside
  `internal/clustering`, without shared rule/version attribution.

## Root-cause inventory

| Issue | Immediate pattern | Root cause | Violated or missing invariant |
| --- | --- | --- | --- |
| Refresh on list | `Service.Clusters` calls `ComputeForRepo` | Capability and lifecycle ownership are collapsed | Corpus reads and local writes should be explicit separate capabilities |
| Repeated unchanged work | source revision is calculated only after loading and is always persisted | Derived projection has no current/stale refresh decision contract | Idempotent derived refresh keyed by source and rule revision |
| Impossible defaults | 5,000 candidates, 10,000,000 pairs | Capacity is represented by independent knobs rather than one exact-work contract | Every accepted request must have a coherent executable bound |
| Missing CPU cancellation | `Clusterer.Cluster` has no context | Long-running domain operation contract omits cancellation | Context passes through long-running operations |
| Duplicate similarity | clustering signals versus precedent word-set Jaccard | Normalization and scoring ownership are feature-local | Scores are explainable, deterministic, and version attributable |
| N+1 cluster reads | member query per cluster | Store exposes scalar child access where callers need a bounded snapshot | Corpus boundary should express repository-scoped bulk reads |
| N+1 refresh reads | override/member reads per existing cluster | Reconciliation is composed over scalar storage helpers | Atomic reconciliation consumes one consistent bounded snapshot |
| Rebuilt indexes | candidate map created inside per-cluster helpers | Derived indexes have no computation-scope owner | Build immutable derived data once per operation |
| Full-sort top-k | sort all neighbors/precedents | Selection and presentation ordering are not separated | Bounded selection must preserve the canonical total order |
| Reprepared writes | identical `ExecContext` in member loops | Transaction owns atomicity but not statement lifetime | Resource owner prepares and closes repeated statements |

## Persistence lifecycle and schema

- Migration `009_clustering.sql` defines `cluster_runs`, `clusters`, members,
  and overrides. Runs carry source revision and params hash, but there is no
  unique identity for `(repository, source_revision, params_hash)` and no
  current-projection pointer/state contract.
- Cluster `stable_id` is globally unique and derived from the canonical member.
  Governance operations can preserve or alter shape across recomputation.
- Existing indexes cover repository/state cluster lookup, member cluster/ref
  lookup, and override cluster lookup.
- `ListClusters` nevertheless issues one child query per returned parent.
- `persist` computes completely before opening its transaction, which is a good
  boundary: CPU work does not hold a write transaction. It then performs atomic
  run/cluster/member replacement and retirement.
- The redesign must preserve governance records and must not let cancellation
  or stale source state replace the last complete projection.

## Capacity facts

For an all-pairs exact algorithm, comparisons are `n*(n-1)/2`.

- 4,472 candidates require 9,997,156 comparisons and remain below 10 million.
- 4,473 candidates require 10,001,628 comparisons and fail the current pair budget.
- Therefore `MaxCandidates=5000` is not executable with the default pair budget.

The final spec should avoid exposing two independent knobs unless one is
derived and validated against the other. Candidate preparation can reduce work
per comparison but does not change the exact worst-case pair count.

## Research already collected

### Context7

Queries performed:

1. Resolve current Go standard-library documentation for `container/heap`,
   `database/sql`, context cancellation, and allocation benchmarks.
2. Resolve current SQLite documentation for bulk reads, indexes, transactions,
   prepared statements, parameter limits, and FTS5.
3. Query Go docs for `testing.B.ReportAllocs`, context-aware database APIs, and
   heap/transaction facilities.
4. Query SQLite docs for transaction-scoped prepared inserts, query planning,
   and FTS5 behavior.
5. Resolve `modernc.org/sqlite` and query its current `database/sql`, context,
   connection-limit, and transaction facilities.
6. Query SQLite documentation for atomic projection replacement, uniqueness,
   UPSERT, explicit transactions, and bounded parameter handling.

Useful corroboration:

- `testing.B.ReportAllocs` is the standard allocation benchmark facility.
- Go exposes context-aware database operations and transaction-scoped prepared statements.
- SQLite supports repeated prepared execution inside one explicit transaction.
- The repository's existing `modernc.org/sqlite` driver exposes ordinary
  `database/sql` behavior; no driver-specific batching layer is required.
- Driver `Limit` is connection-scoped, so it is not a substitute for an
  application-owned exact-work budget or explicit SQL chunk size.

Limitation: one Context7 Go query returned unrelated `stdchat` material. That
output is rejected and must not influence the design.

### DeepWiki

Repositories queried: `golang/go`, `sqlite/sqlite`, `blevesearch/bleve`.

Question covered deterministic top-k, allocation benchmarks, bulk parent-child
loads, repeated transactional inserts, cancellation, and resource lifetimes.

Useful corroboration:

- Go's own database tests use allocation reporting and context cancellation.
- Transaction-scoped statements end with transaction lifetime.
- CPU loops require explicit periodic context observation.

Follow-up questions covered input/content identities for derived artifacts,
Bleve's exact `TopNCollector`, canonical tie breaking and final ordering,
transaction-scoped statements, and periodic cancellation checks.

Useful leads, subsequently verified against repository source:

- Go build artifacts distinguish a hash of action inputs from a hash of action
  output. The useful design lesson is narrower than adopting the Go build ID:
  a derived projection identity must include every semantic input and rule version.
- Bleve's collector retains only bounded results with `container/heap`, uses one
  comparator throughout selection, and performs a final ordered extraction.
- Bleve checks context at a fixed cadence of 1,024 visited documents, supporting
  a fixed implementation cadence rather than another caller-facing tuning knob.

Limitations:

- Go's build ID is evidence for input-attributed derived work, not a reusable
  cluster lifecycle API.
- Bleve's collector owns Bleve search types and scoring, so importing it would
  leak an unrelated search engine and is rejected. The applicable maintained
  facility is the standard-library heap.
- DeepWiki's generated suggestions remain leads only.

### Primary sources

- Go prepared statements: https://go.dev/doc/database/prepared-statements
- Go database/context guidance: https://go.dev/doc/database/
- Go heap package: https://pkg.go.dev/container/heap
- Go diagnostics: https://go.dev/doc/diagnostics
- SQLite query planner: https://www.sqlite.org/queryplanner.html
- SQLite query-plan inspection: https://www.sqlite.org/eqp.html
- SQLite transactions: https://www.sqlite.org/lang_transaction.html
- SQLite limits: https://www.sqlite.org/limits.html
- SQLite FTS5/BM25: https://www.sqlite.org/fts5.html
- PPJoin exact similarity filtering: https://archives.iw3c2.org/www2008/papers/pdf/p131-xiaoA.pdf
- Go build input/content identities: https://github.com/golang/go/blob/master/src/cmd/go/internal/work/buildid.go
- Bleve exact top-N collector and cancellation: https://github.com/blevesearch/bleve/blob/master/search/collector/topn.go
- Bleve heap retention/final extraction: https://github.com/blevesearch/bleve/blob/master/search/collector/heap.go
- SQLite isolation and read snapshots: https://www.sqlite.org/isolation.html
- SQLite UPSERT and conditional update: https://www.sqlite.org/lang_upsert.html

Verified conclusions:

- Precompute exact representations before attempting safe pair pruning.
- PPJoin-style filtering is not directly reusable for the current composite
  score without proving safe bounds for every weighted signal.
- FTS5/BM25 is not semantically equivalent to current Jaccard/composite scoring.
- Use standard-library `container/heap` for bounded selection, followed by a
  final canonical sort of retained results.
- Bulk parent/child loading should preserve explicit ordering and be checked
  with `EXPLAIN QUERY PLAN`; its unstable textual output should not be snapshotted.
- Prefer transaction-scoped prepared statements before dynamic multi-row SQL.
- A read transaction gives one historic SQLite snapshot, but CPU clustering must
  not hold it open. Load a complete bounded input snapshot, close the read
  transaction, compute, then re-read and compare the exact input revision inside
  the short write transaction before replacing the projection.
- Source freshness alone is insufficient: membership overrides and scoring-rule
  version are also semantic inputs. Projection identity is therefore
  `(repository, source revision, governance revision, rule version)`.
- A successful empty cluster result still needs durable freshness metadata;
  cluster rows cannot serve as the current-projection marker. Use one explicit
  current projection row per repository.
- Reusing Bleve itself would introduce unrelated third-party types and indexing
  behavior. A small product-owned bounded selector backed by `container/heap`
  is an adapter around the maintained heap, not a hand-rolled priority queue.

## Phase 3 design decisions

### Refresh topology

Choose an explicit synchronous refresh capability and a separate pure list
capability. It preserves the current local, bounded execution model while making
the write and latency visible. A durable job remains a future application adapter:
it may call the same refresh use case if measured latency later requires
resumability or asynchronous UX. Refresh-on-read is rejected because it violates
the side-effect boundary; durable-job-first is rejected because it adds queue,
recovery, and status semantics without current evidence they are required.

### Similarity ownership

Choose shared exact preparation primitives with distinct, versioned policies.
Clustering and neighbors use the existing weighted duplicate policy; precedents
retain their existing lexical Jaccard policy. A single universal policy would
silently change outputs, while feature-local tokenizers leave determinism and
version attribution duplicated. Third-party search/approximate scorers are rejected.

### Persistence boundary

Choose repository-scoped product snapshots and an atomic projection commit port.
Scalar ports preserve N+1 composition. SQL-shaped application contracts leak
adapter details. The SQLite adapter may use bounded `IN` chunks, prepared
transaction statements, and query-plan inspection internally.

### Top-k

Choose exact bounded heap selection with one canonical total-order comparator,
then final-sort the retained `k` results. Full sort is the correctness oracle but
does unnecessary `O(n log n)` work. Storage-ranked top-k cannot implement the
current composite in-memory score without changing semantics.

### Exact candidate evaluation

Choose prepared all-pairs evaluation under one exact pair budget. PPJoin-style
filtering is a future optimization only after a proof that it cannot prune a pair
that meets the versioned composite threshold. Approximate blocking, FTS/BM25,
embeddings, and LSH remain out of scope.

## Resolved Phase 3 questions

1. Refresh is an explicit synchronous command/use case in the first redesign.
2. Features share exact normalization/preparation; score policies remain distinct
   and versioned to preserve existing behavior.
3. Add an explicit one-row-per-repository current projection relation because
   zero-cluster results need identity and freshness.
4. Reject work before pair evaluation when `n*(n-1)/2` exceeds the one exact
   pair budget. A future exact filter must earn a larger bound with equivalence proof.

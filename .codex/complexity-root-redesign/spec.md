# Complexity Root-Cause Redesign

Status: implemented after explicit user approval on 2026-07-19; verification is recorded in [evidence.md](evidence.md).
Goal: [goal.md](goal.md)
Plan: [plan.md](plan.md)
Research: [research.md](research.md)

## 1. Outcome

Treat clusters as a versioned derived projection. Reading that projection is a
pure corpus capability. Refreshing it is a separate, explicit local-write
capability that loads one consistent input snapshot, skips unchanged work,
performs bounded and cancellable exact computation outside a database
transaction, and commits only if the source and governance inputs are still the
ones that were computed.

The same redesign gives exact similarity one owner, derives capacity from one
pair budget, replaces scalar database composition with bounded repository
snapshots, and changes top-k implementation from full sort to a standard-library
heap without changing the canonical result order.

No network access, process execution, GitHub mutation, approximate retrieval,
or third-party type enters any proposed domain or application contract.

## 2. Design invariants

1. `ListClusters` and `GetCluster` perform SQLite reads only.
2. Only `RefreshClusters` may compute and write the cluster projection.
3. A complete projection is identified by repository, candidate source revision,
   governance revision, and duplicate-rule version.
4. An empty successful projection has the same durable identity as a non-empty one.
5. A refresh never holds a read or write transaction during pair evaluation.
6. A stale or cancelled refresh cannot replace the last complete projection.
7. Every accepted all-pairs refresh can finish within its declared exact pair budget.
8. Duplicate and precedent scores remain exact, deterministic, explainable, and
   attributed to a concrete rule version.
9. A top-k optimization returns the same ordered prefix as a canonical full sort.
10. Batch inputs preserve caller order and item-level outcomes even when storage
    work is grouped by repository.

## 3. Decisions and alternatives

### D1. Refresh ownership

| Alternative | Consequence | Decision |
| --- | --- | --- |
| Refresh from `Clusters` list | Hides writes and full-population latency behind a read-shaped method | Reject |
| Explicit synchronous refresh | Makes authority, cancellation, and cost visible; reuses current bounded local execution | Choose |
| Durable refresh job | Adds queue, recovery, and polling semantics; useful only if measured synchronous latency later requires it | Defer behind the same use case |

CLI shape after an approved implementation:

- `gitcontribute clusters OWNER/REPO` is a pure stored-projection list.
- `gitcontribute clusters refresh OWNER/REPO` is an explicit local mutation.
- MCP `corpus.find_clusters` remains a pure read. No MCP mutation is added by
  this redesign.

### D2. Derived-projection identity

| Alternative | Consequence | Decision |
| --- | --- | --- |
| Infer freshness from cluster rows | Cannot represent a successful zero-cluster projection | Reject |
| Unique run key only | Records history but does not identify the current visible projection and may conflict with repeated legacy runs | Reject |
| One current-projection row plus immutable completed runs | Represents empty results, skip state, and atomic visibility | Choose |

The action identity is:

```text
ClusterProjectionKey = repository
                     + candidate_source_revision
                     + governance_revision
                     + duplicate_rule_version
```

`candidate_source_revision` hashes every candidate field consumed by scoring or
projection. `governance_revision` is a monotonic repository-scoped integer
incremented only in the same transaction as an override or cluster-governance
mutation. `duplicate_rule_version` changes whenever preparation, signal weights,
threshold semantics, or output-affecting rules change.

### D3. Similarity ownership

| Alternative | Consequence | Decision |
| --- | --- | --- |
| Feature-local helpers | Duplicates tokenization and loses rule attribution | Reject |
| One universal score | Changes precedent behavior by replacing lexical Jaccard with the duplicate composite | Reject |
| Shared preparation ownership with distinct versioned policies | Consolidates exact primitives while preserving outputs | Choose |

`internal/similarity` owns both policies. It does not expose a generic config bag
or boolean modes. Concrete constructors produce valid immutable rules.

### D4. Exact evaluation and capacity

| Alternative | Consequence | Decision |
| --- | --- | --- |
| Independent candidate and pair limits | Permits configurations that cannot execute | Reject |
| Exact all-pairs under one pair budget | Coherent, simple, and preserves behavior | Choose |
| PPJoin-style exact filtering | Potential future gain, but no safe bound is proved for the weighted composite | Defer |
| FTS/BM25, LSH, embeddings, approximate blocking | Changes result semantics | Reject |

For `n` candidates, required pairs are `n*(n-1)/2`, computed with overflow-safe
integer arithmetic before allocation or pair evaluation. The maximum accepted
candidate count is derived from the pair budget; it is not caller-configurable
independently. With the current 10,000,000-pair default, 4,472 is accepted and
4,473 is rejected before CPU work starts.

### D5. Storage contracts

| Alternative | Consequence | Decision |
| --- | --- | --- |
| Scalar repository, cluster, member, and override reads | Produces N+1 call graphs and inconsistent multi-read snapshots | Reject |
| SQL-shaped application port | Leaks SQLite bind limits and rows into application ownership | Reject |
| Product-owned bounded repository snapshots | Expresses caller intent and lets SQLite batch internally | Choose |

### D6. Top-k

| Alternative | Consequence | Decision |
| --- | --- | --- |
| Full sort of all scored candidates | Exact but `O(n log n)` time and `O(n)` retained results | Correctness oracle only |
| Bounded heap, then canonical final sort | Exact `O(n log k)` selection and `O(k)` retention | Choose |
| Storage-ranked limit | Cannot express the current in-memory composite score exactly | Reject |

The selector is a small product adapter around `container/heap`; the heap
algorithm itself is not reimplemented. One strict total-order comparator is used
for admission, eviction, and final sort. NaN scores are invalid.

### D7. Persistence

Use `database/sql`, the existing `modernc.org/sqlite` driver, explicit
transactions, `Tx.PrepareContext`, and `Stmt.ExecContext`. Do not build dynamic
multi-row SQL, a transaction manager, retry scheduler, or custom priority queue.
The commit transaction first obtains the SQLite writer by ensuring/conditionally
touching the projection-state row, then verifies governance and source revisions,
then writes a complete run, clusters, and members, and finally advances the
current projection row.

## 4. Product-owned typed contracts

The following is Go pseudocode. Names and ownership are normative; adapter SQL
details are not part of these contracts.

### 4.1 Exact similarity

```go
package similarity

type RuleVersion string

const (
    DuplicateV1 RuleVersion = "duplicate-v1"
    PrecedentV1 RuleVersion = "precedent-v1"
)

type Score float64 // constructor rejects NaN, infinity, and values outside [0,1]

type DuplicateRule struct { /* private validated fields */ }
func DefaultDuplicateRule() DuplicateRule
func (r DuplicateRule) Version() RuleVersion
func (r DuplicateRule) Prepare(c ThreadText) PreparedDuplicate
func (r DuplicateRule) Compare(a, b PreparedDuplicate) DuplicateScore

type PrecedentRule struct { /* private validated fields */ }
func DefaultPrecedentRule() PrecedentRule
func (r PrecedentRule) Version() RuleVersion
func (r PrecedentRule) Prepare(c ThreadText) PreparedLexical
func (r PrecedentRule) Compare(a, b PreparedLexical) Score

type ThreadText struct {
    Ref    ThreadRef
    Title  string
    Body   string
    Labels []string
    Author string
}

type ThreadRef struct {
    Repo   domain.RepoRef
    Kind   domain.ThreadKind
    Number int
}

type PreparedDuplicate struct { /* immutable normalized sets and refs */ }
type PreparedLexical struct { /* immutable precedent-v1 token set */ }

type DuplicateScore struct {
    Value   Score
    Signals DuplicateSignals
}

type DuplicateSignals struct {
    ExplicitReference bool
    TitleJaccard       Score
    BodyJaccard        Score
    LabelJaccard       Score
    SameAuthor         bool
}
```

`DuplicateV1` reproduces the existing weighted signals and body-token limit.
`PrecedentV1` reproduces the existing lower-cased alphanumeric tokens of length
at least three, threshold `0.08`, and Jaccard score. Extraction moves code; it
does not unify the two policies or change golden outputs.

### 4.2 Work capacity and cancellation

```go
package clustering

type ExactPairBudget uint64

func DefaultExactPairBudget() ExactPairBudget // 10_000_000
func (b ExactPairBudget) Required(candidateCount int) (uint64, error)
func (b ExactPairBudget) MaxCandidates() int

type CapacityError struct {
    CandidateCount int
    RequiredPairs  uint64
    AllowedPairs   uint64
}

type Engine struct {
    rule   similarity.DuplicateRule
    budget ExactPairBudget
}

func NewEngine(rule similarity.DuplicateRule, budget ExactPairBudget) (Engine, error)

func (e Engine) Cluster(
    ctx context.Context,
    candidates []Candidate,
) (Computation, error)

type Computation struct {
    Clusters      []Cluster
    CandidateCount int
    RequiredPairs uint64
    ComparedPairs uint64
    RuleVersion   similarity.RuleVersion
}
```

Cancellation is checked, in order:

1. on entry, before validation or allocation;
2. once per prepared candidate;
3. every fixed 1,024 pair comparisons;
4. before grouping and governance reconciliation;
5. immediately before requesting a commit.

The cadence is an implementation constant, not another mode/configuration knob.
On cancellation the engine returns `ctx.Err()` and no partial computation. The
application checks cancellation before reporting capacity or stale-input errors.

### 4.3 Cluster projection values and ports

Projection values live in a dependency-neutral product package so the corpus
adapter can implement application ports without importing `internal/app` and
creating a Go cycle.

```go
package clusterprojection

type ListQuery struct {
    Repo  domain.RepoRef
    State clustering.ClusterState
    Limit int // validated 1..1000 by application
}

type List struct {
    Repo       domain.RepoRef
    Projection *Identity // nil means never refreshed
    Clusters   []clustering.Cluster
}

type Identity struct {
    SourceRevision     string
    GovernanceRevision uint64
    RuleVersion        similarity.RuleVersion
    RunID              int64
}

type RefreshSnapshot struct {
    Repo               domain.RepoRef
    Candidates         []clustering.Candidate
    ExistingClusters   []clustering.Cluster
    OverridesByCluster map[string][]clustering.MembershipOverride
    SourceRevision     string
    GovernanceRevision uint64
    CurrentProjection  *Identity
    SourceWindow       clustering.SourceWindow
    ReadStatements     int
}

type Commit struct {
    Repo               domain.RepoRef
    ExpectedSource     string
    ExpectedGovernance uint64
    RuleVersion        similarity.RuleVersion
    SourceWindow       clustering.SourceWindow
    Clusters           []clustering.Cluster
    Stats              RefreshStats
}

type RefreshStats struct {
    CandidateCount  int
    RequiredPairs   uint64
    ComparedPairs   uint64
    ClusterCount    int
    SnapshotQueries int
    CommitQueries   int
}

type CommitDisposition string
const (
    Committed      CommitDisposition = "committed"
    AlreadyCurrent CommitDisposition = "already_current"
)

type CommitResult struct {
    Disposition    CommitDisposition
    Projection     Identity
    WriteStatements int
}

type StaleInputError struct {
    ExpectedSource, ActualSource string
    ExpectedGovernance, ActualGovernance uint64
    CurrentCandidateCount int
}
```

```go
package app

type ClusterProjectionReader interface {
    ListClusters(ctx context.Context, q clusterprojection.ListQuery) (clusterprojection.List, error)
    GetCluster(ctx context.Context, stableID string) (*clustering.Cluster, error)
}

type ClusterProjectionWriter interface {
    LoadClusterRefreshSnapshot(
        ctx context.Context,
        repo domain.RepoRef,
        maxCandidates int,
    ) (clusterprojection.RefreshSnapshot, error)

    CommitClusterProjection(
        ctx context.Context,
        commit clusterprojection.Commit,
    ) (clusterprojection.CommitResult, error)
}
```

The snapshot loader uses one read transaction and returns only product types. It
loads candidates, projection state, existing clusters, members, and overrides in
bounded set queries. It returns `CapacityError` if a `maxCandidates+1` sentinel
row exists. It closes all rows and the transaction before returning.

`CommitClusterProjection` is a compare-and-swap operation. Its adapter reloads
at most `maxCandidates+1` current candidate rows and recomputes the exact revision
under the write transaction; it does not trust a timestamp or the previously
loaded snapshot. A new sentinel row is reported as stale input with the current
candidate count. The adapter returns `AlreadyCurrent` when another refresh
already committed the same identity.

### 4.4 Refresh use case

```go
package app

type ClusterService struct {
    reader ClusterProjectionReader
    writer ClusterProjectionWriter
    engine clustering.Engine
}

type RefreshDisposition string
const (
    RefreshCommitted RefreshDisposition = "committed"
    RefreshUnchanged RefreshDisposition = "unchanged"
)

type ClusterRefreshResult struct {
    Repo        domain.RepoRef
    Disposition RefreshDisposition
    Projection  clusterprojection.Identity
    Stats       clusterprojection.RefreshStats
}

func (s *ClusterService) ListClusters(
    ctx context.Context,
    q clusterprojection.ListQuery,
) (clusterprojection.List, error)

func (s *ClusterService) RefreshClusters(
    ctx context.Context,
    repo domain.RepoRef,
) (ClusterRefreshResult, error)
```

`RefreshClusters` loads a snapshot. If current identity equals snapshot source,
governance, and engine rule version, it returns `RefreshUnchanged` with zero
pair comparisons and no write call. Otherwise it computes, reconciles existing
governance from the same snapshot, checks context, and calls the commit port.

### 4.5 Bulk precedent and cluster reads

```go
package precedent

type RepositorySnapshot struct {
    Repo           domain.RepoRef
    SourcesByNumber map[int]Thread
    Closed         []Thread
    SourceRevision string
}

type Thread struct {
    ID          int64
    Text        similarity.ThreadText
    Kind        string
    Number      int
    State       string
    StateReason string
    Merged      bool
    Labels      []string
    ClosedAt    time.Time
    MergedAt    time.Time
}
```

```go
package app

type PrecedentSnapshotReader interface {
    LoadPrecedentRepositories(
        ctx context.Context,
        refs []similarity.ThreadRef, // 1..20
        closedLimit int,         // current behavior: 2000 per unique repository
    ) ([]precedent.RepositorySnapshot, error)
}
```

The application groups input indices by repository, calls the bulk port once,
prepares each repository's closed population once, and then writes results back
to their original indices. Repository-not-indexed and thread-not-indexed remain
item-level outcomes. A storage failure remains an operation-level error.

Cluster list uses one bounded header query plus member queries chunked only when
the returned cluster IDs exceed the verified SQLite bind limit. For the public
limit of 1,000 and current driver defaults, the intended plan is two statements.
The adapter must preserve the existing cluster and member canonical order.

### 4.6 Exact top-k

```go
package ranking

// better must define a strict total order: true means a ranks before b.
func TopK[T any](values []T, k int, better func(a, b T) bool) []T
```

Implementation requirements:

- `k <= 0` returns an empty non-nil slice.
- `k >= len(values)` returns a copied canonical full sort.
- the heap root is the worst retained value;
- a candidate replaces the root only when `better(candidate, root)`;
- retained results receive a final `sort.Slice` with the same comparator;
- inputs are not mutated;
- property tests compare the result with the first `k` values of a full sort.

Neighbor and precedent total order is score descending, then canonical thread
reference ascending. Any additional existing feature-specific tie breaker is
placed before the final reference tie breaker and captured in golden tests.

## 5. Current and proposed call stacks

### F1. Cluster list

```text
CURRENT CLI clusters
  cli.runClusters
  -> app.Service.Clusters
  -> clustering.Store.ComputeForRepo
  -> candidate reads + O(n^2) CPU + projection transaction
  -> limit response

PROPOSED CLI clusters
  cli.runClusters
  -> app.ClusterService.ListClusters
  -> corpus cluster projection reader
  -> projection-state query + bounded cluster/member queries
  -> render
```

No proposed list frame has a writer, network client, or process runner.

### F2. Cluster refresh

```text
PROPOSED CLI clusters refresh
  cli.runClusterRefresh
  -> app.ClusterService.RefreshClusters
  -> corpus.LoadClusterRefreshSnapshot [one read transaction]
  -> unchanged identity? return skipped
  -> clustering.Engine.Cluster(ctx) [no transaction]
  -> clustering.Reconcile(snapshot governance) [pure]
  -> corpus.CommitClusterProjection [short compare-and-swap write transaction]
  -> render disposition + identity + stats
```

### F3. MCP clusters

```text
CURRENT/PROPOSED MCP corpus.find_clusters
  mcpserver.findClusters
  -> app.MCPReader.FindClusters
  -> app.ClusterService.ListClusters
  -> corpus projection reader only
```

### F4. Neighbors

```text
CURRENT
  app.Service.Neighbors
  -> corpus scalar/list reads
  -> clustering.Neighbors [repeated preparation, full sort]

PROPOSED
  app.Service.Neighbors
  -> corpus.LoadNeighborSnapshot
  -> similarity.DuplicateRule.Prepare once per thread
  -> score with context checks
  -> ranking.TopK with canonical comparator
  -> surface projection including rule_version
```

### F5. Precedents

```text
CURRENT
  MCPReader.FindPrecedents
  -> for each input: repository + source + closed list queries
  -> application-local wordSet/Jaccard
  -> full sort

PROPOSED
  MCPReader.FindPrecedents
  -> corpus.LoadPrecedentRepositories for all refs
  -> similarity.PrecedentRule prepares each closed thread once per repository
  -> score each ordered source with context checks
  -> ranking.TopK per source
  -> restore ordered item-level outcomes including rule_version
```

## 6. Transaction and schema design

### 6.1 Migration `022_cluster_projection_state.sql`

Normative schema changes:

```sql
ALTER TABLE cluster_runs ADD COLUMN governance_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cluster_runs ADD COLUMN rule_version TEXT NOT NULL DEFAULT 'duplicate-v1';
ALTER TABLE cluster_runs ADD COLUMN statistics_json TEXT NOT NULL DEFAULT '{}';

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
```

No uniqueness constraint is added to legacy `cluster_runs`: existing databases
may contain repeated historical runs for the same source and parameters. The
single current-state row and SQLite's one-writer transaction provide the
idempotency boundary.

The up migration backfills one state row from the greatest completed run ID for
each repository. Repositories without a completed run remain absent and read as
"never refreshed, governance revision zero." The down migration drops the state
table, then removes the added columns using the SQLite version
already supplied by the pinned driver. Migration tests cover a populated v21
database, an empty database, and round-trip schema version.

Every governance mutation ensures the state row exists and increments
`governance_revision` in its existing transaction. Derived refresh writes never
increment it.

### 6.2 Commit algorithm

```text
begin database/sql transaction
  ensure cluster_projection_state row exists (also obtains writer)
  read governance_revision; mismatch -> rollback, clusterprojection.StaleInputError
  reload bounded candidate source fields and hash exact revision
  source mismatch -> rollback, clusterprojection.StaleInputError
  if current identity equals desired identity -> rollback, AlreadyCurrent
  insert completed cluster_run with identity and statistics
  prepare cluster upsert/update, member delete, and member insert statements
  upsert clusters; replace all members; retire missing ungoverned shapes
  update cluster_projection_state.current_run_id and identity
commit
```

Cluster computation is already complete before `begin`. All repeated member
inserts execute through one transaction-scoped prepared statement. An error,
busy timeout, cancellation, or failed revision check rolls back every change.
The last complete projection remains visible until commit.

Do not retry stale input inside the adapter. The explicit caller receives a
typed stale error and may invoke refresh again. Database busy handling uses the
existing driver/context policy; this design adds no retry loop.

## 7. Failure and idempotency contract

| Condition | Result | Side effect |
| --- | --- | --- |
| Context cancelled before/during load | `context.Canceled`/`DeadlineExceeded` | None |
| Candidate count exceeds derived max | `CapacityError` with counts | None |
| Context cancelled during CPU work | context error, no partial clusters | None |
| Snapshot identity already current | `RefreshUnchanged`, zero comparisons if detected before engine | None |
| Source/governance changes after snapshot | `clusterprojection.StaleInputError` | Transaction rollback |
| Concurrent identical refresh wins | `RefreshUnchanged` from `AlreadyCurrent` | No replacement by loser |
| Member/run/state write fails | wrapped storage error | Transaction rollback |
| Successful empty computation | `RefreshCommitted`, current state points at completed zero-cluster run | Atomic projection update |
| List before first refresh | complete empty list with `Projection=nil` | None |

Validation and error precedence is: cancellation, malformed identity/input,
capacity, storage, stale input. Panics are not converted to ordinary errors.

## 8. Observability and performance evidence

Production result/run statistics are stable counts, not timing promises:

- candidate count;
- required and compared pairs;
- output cluster count;
- snapshot and commit SQL statement counts;
- committed versus unchanged disposition;
- source, governance, and rule identity.

`statistics_json` stores the successful run counts for attribution. Cancelled
and stale attempts do not create run rows; their errors carry enough identity
for the CLI/application logger. No metrics SDK is introduced.

Benchmark-only evidence uses `testing.B`, `ReportAllocs`, real temporary SQLite
corpora, and deterministic generated thread text:

1. `BenchmarkClusterEngine/{100,1000,4472}` reports ns/op, B/op, allocs/op, and
   exact compared pairs.
2. `BenchmarkClusterCancellation/4472` cancels at a controlled comparison
   checkpoint and reports observed return latency; the verifier requires it to
   return by the next fixed 1,024-comparison check, not an invented wall-clock SLA.
3. `BenchmarkClusterList/{10,100,1000}` records statements and allocations; the
   statement count must remain constant until bind-limit chunking is necessary.
4. `BenchmarkPrecedents/{1,20}-same-repo` proves storage work scales with unique
   repositories rather than source count and reports allocations.
5. `BenchmarkTopK` compares canonical full sort and heap selection for current
   limits; adoption requires identical ordered results and lower asymptotic
   retained memory, not a fabricated percentage threshold.

Use `EXPLAIN QUERY PLAN` in focused store tests to confirm repository/state and
cluster/member indexes are selected. Assert planner structure or index presence;
do not snapshot unstable human-readable plan text. Add a new index only if the
measured plan demonstrates one is needed.

## 9. File and ownership map

| File | Change |
| --- | --- |
| `internal/similarity/types.go` | New product score, rule version, text, and prepared types |
| `internal/similarity/duplicate.go` | Move duplicate-v1 preparation/scoring without behavior change |
| `internal/similarity/precedent.go` | Move precedent-v1 preparation/Jaccard without behavior change |
| `internal/ranking/topk.go` | New generic adapter backed by `container/heap` and final `sort.Slice` |
| `internal/clusterprojection/contracts.go` | New dependency-neutral projection identities, snapshots, commits, results, stats, and errors |
| `internal/precedent/models.go` | New dependency-neutral repository snapshot and precedent thread values |
| `internal/clustering/cluster.go` | Replace config bag with engine/rule/pair budget; accept context and prepared scores |
| `internal/clustering/similarity.go` | Delete after behavior-preserving extraction |
| `internal/clustering/neighbors.go` | Use shared duplicate rule, context, and exact top-k |
| `internal/clustering/models.go` | Add typed computation/projection attribution fields as needed |
| `internal/clustering/store.go` | Delete after moving SQLite concerns to corpus adapter |
| `internal/corpus/cluster_projection.go` | New bulk reader/writer adapter and compare-and-swap transaction |
| `internal/corpus/precedent_snapshot.go` | New repository-grouped precedent snapshot adapter |
| `internal/corpus/migrations/022_cluster_projection_state.sql` | Projection state, run identity, rule/governance/stats columns |
| `internal/app/clustering.go` | Split pure list/get from explicit refresh orchestration |
| `internal/app/neighbors.go` | Use context-aware prepared duplicate scoring and top-k |
| `internal/app/mcp_scalable_reads.go` | Remove local tokenizer/Jaccard and scalar precedent loop |
| `internal/cli/interfaces.go` | Replace ambiguous `Clusters` with `ListClusters` and `RefreshClusters` |
| `internal/cli/cli.go` | Make current list pure; add explicit `clusters refresh` route |
| `internal/mcpserver/server.go` | Add rule-version attribution to cluster/neighbor outputs; retain read-only tool |
| `internal/mcpserver/scalable.go` | Add precedent rule-version attribution, no lifecycle change |

Tests follow the production owner: pure score/heap tests under their new
packages; real SQLite projection tests under `internal/corpus`; application and
surface behavior tests under `internal/app`, `internal/cli`, and
`internal/mcpserver`.

## 10. Vertical red-green-refactor delivery plan

Each slice is independently reviewable. Production implementation requires the
approval gate in `goal.md`.

### RGR-1: Freeze exact behavior and consolidate score ownership

- Red: add table/golden tests through public neighbor and precedent application
  methods covering punctuation, short tokens, labels, authors, explicit refs,
  empty sets, thresholds, and tie order.
- Green: introduce `internal/similarity` and route all three features through
  `DuplicateV1` or `PrecedentV1` without output changes.
- Refactor: remove local `wordSet`, local Jaccard, and duplicated preparation.
- Proof: existing plus new behavior tests pass byte-for-byte; rule version is
  present in typed results.

### RGR-2: Coherent capacity and CPU cancellation

- Red: public refresh test with 4,473 candidates expects typed pre-work
  `CapacityError`; cancellation test expects context error and no store commit.
- Green: add `ExactPairBudget` and `Engine.Cluster(ctx, ...)` checks.
- Refactor: remove `MaxCandidates`, `MaxPairs`, normalization fallbacks, and mode
  booleans from the old config.
- Proof: 4,472/4,473 boundary, overflow cases, comparison count, and cancellation
  cadence tests pass.

### RGR-3: Exact bounded top-k

- Red: property tests compare public neighbor/precedent results with canonical
  full-sort oracles across duplicates and ties.
- Green: add the `container/heap` adapter and final canonical sort.
- Refactor: share only the selection mechanism; keep feature comparators explicit.
- Proof: ordered outputs are identical; benchmarks report `O(k)` retained results.

### RGR-4: Repository-scoped precedent snapshot

- Red: real SQLite MCP test with 20 same-repository inputs asserts input order,
  item-level missing-thread outcomes, and one prepared closed population.
- Green: add `LoadPrecedentRepositories` and group application work by repository.
- Refactor: remove scalar corpus calls from the per-input loop.
- Proof: same outputs; storage statement count scales with unique repositories.

### RGR-5: Pure cluster reads and explicit refresh surface

- Red: CLI/application test lists clusters in a read-only SQLite transaction and
  asserts no run/projection row changes; separate refresh test expects a write.
- Green: split interfaces and add the explicit route.
- Refactor: route MCP and CLI list through the same reader.
- Proof: list has no hidden writes/network/process effects; refresh remains the
  only application capability with `ClusterProjectionWriter`.

### RGR-6: Projection identity, unchanged skip, and stale rejection

- Red: migration and real SQLite tests cover never-refreshed, empty projection,
  unchanged source, rule change, governance change, concurrent source change,
  and concurrent identical refresh.
- Green: add migration 022, bulk snapshot loading, projection-state identity,
  and compare-and-swap commit.
- Refactor: move SQLite store code from clustering to corpus and isolate pure
  governance reconciliation.
- Proof: unchanged refresh performs zero comparisons/writes; stale/cancelled
  attempts preserve the prior complete projection.

### RGR-7: Bulk cluster children and prepared writes

- Red: real SQLite list test with multiple clusters detects canonical parent and
  child order; atomicity test injects a member constraint failure.
- Green: bulk member load and transaction-scoped prepared member insert/delete.
- Refactor: remove scalar `listMembersForCluster`, per-cluster override reads,
  and repeated unprepared SQL.
- Proof: list uses bounded constant statements; injected failure exposes none of
  the new run, clusters, members, or projection-state pointer.

### RGR-8: Evidence and rollout gate

- Red: benchmark verifier fails when exact counts, allocations, cancellation,
  unchanged disposition, or query statements are not reported.
- Green: add benchmarks and successful-run statistics.
- Refactor: document measured baselines and remove instrumentation not needed in
  production contracts.
- Proof: `go test ./...`, focused benchmarks, `go vet ./...` if already part of
  repository practice, `gofmt`, `git diff --check`, and migration tests pass.

## 11. Traceability matrix

References: decisions `D1`-`D7`, flows `F1`-`F5`, and test slices `RGR-1`-`RGR-8`.

| Baseline issue and evidence | Root cause / invariant | Alternatives and chosen contract | Call stack / failure and transaction | Files / RGR proof / measurable criterion | Residual risk |
| --- | --- | --- | --- | --- | --- |
| Refresh on list: `internal/app/clustering.go:14-46`, `internal/clustering/store.go:28-151` | Capability ownership collapsed; reads must not write | Refresh-on-read vs explicit sync vs job; choose `ListClusters` + `RefreshClusters` (`D1`) | Current/proposed `F1`,`F2`; failure/cancel returns no partial result; list is idempotent/pure with no transaction; refresh uses the CAS transaction | `internal/app/clustering.go`, `internal/cli/cli.go`, `internal/cli/interfaces.go`; `RGR-5`; list creates zero run/state changes | Public CLI change needs approval and release note |
| Repeated unchanged work: source revision always followed by compute/persist in `store.go:49-151` | No current derived-action identity; refresh must be idempotent and stale-safe | Cluster-row inference vs unique runs vs current state; choose projection identity/CAS (`D2`) | Current/proposed `F1`,`F2`; cancellation exits before commit; unchanged is idempotent; stale/failure rolls back the CAS transaction | `internal/clusterprojection/contracts.go`, `internal/corpus/cluster_projection.go`, migration 022; `RGR-6`; unchanged has 0 comparisons and durable writes | Source rehash in commit is O(n), intentionally below O(n²) |
| Impossible defaults: `cluster.go:14-29,69-116`; 4,473 exceeds 10m pairs | Independent knobs violate executable-bound invariant | independent limits vs one exact budget vs filtered/approximate; choose `ExactPairBudget` (`D4`) | Current/proposed `F1`,`F2`; cancellation precedes typed capacity failure; rejection is idempotent and occurs before any transaction/writer | `internal/clustering/cluster.go`; `RGR-2`; exact 4,472/4,473 and overflow tests | Maximum remains bounded until a safe exact filter is proved |
| Missing CPU cancellation: `Cluster` lacks context at `cluster.go:69` | Long-running operation omits cancellation | no checks vs per-pair vs fixed periodic checks; choose context at entry/preparation/every 1,024 pairs | Current/proposed `F1`,`F2`,`F4`,`F5`; context is the failure, pure work is repeatable, and cancellation prevents the commit transaction | `internal/clustering/cluster.go`, `internal/clustering/neighbors.go`, `internal/similarity/*`; `RGR-2`; return by next check and no write | Wall-clock latency varies by hardware; comparison cadence is deterministic |
| Duplicate similarity: weighted `signalsBetween` vs app-local `wordSet/jaccard` at `mcp_scalable_reads.go:638-664` | Feature-local semantic ownership; scores lack rule identity | local helpers vs universal policy vs distinct versioned policies; choose `internal/similarity` (`D3`) | Current/proposed `F4`,`F5`; invalid score/context returns no partial list; scoring is deterministic/idempotent and has no transaction | `internal/similarity/duplicate.go`, `internal/similarity/precedent.go`, `internal/app/mcp_scalable_reads.go`; `RGR-1`; golden outputs unchanged and versions present | A future rule change requires a new version and explicit product decision |
| N+1 cluster reads: `ListClusters` calls member read per parent at `store.go:482-502` | Scalar child port where caller needs bounded aggregate | scalar vs SQL leakage vs bulk snapshot; choose bounded product reader (`D5`) | Current/proposed `F1`,`F3`; context/storage failure returns no partial list; repeated reads are idempotent in one read transaction | `internal/corpus/cluster_projection.go`, `internal/clustering/store.go` (deleted); `RGR-7`; two statements for public <=1000 limit unless verified bind chunking | Planner choice may change; verify indexes, not textual plan snapshots |
| N+1 refresh/governance reads and rebuilt candidate maps: `store.go:58-126`, `cluster.go:249-340` | Reconciliation lacks one operation-scoped snapshot/index owner | scalar reads vs bulk snapshot vs denormalized SQL engine; choose immutable snapshot plus once-built maps (`D5`) | Current/proposed `F1`,`F2`; cancellation/failure closes the read snapshot; repeat is identity-idempotent; CAS rejects changes in the write transaction | `internal/corpus/cluster_projection.go`, `internal/clustering/cluster.go`, `internal/clusterprojection/contracts.go`; `RGR-6`,`RGR-7`; reads bounded independent of cluster count | Governance generation must be incremented by every mutation path |
| Full-sort top-k: neighbors and precedents score all then sort | Selection and presentation order conflated | full sort vs heap vs storage rank; choose heap + final sort (`D6`) | Current/proposed `F4`,`F5`; context/invalid comparator failure returns no partial output; selection is pure/idempotent with no transaction | `internal/ranking/topk.go`, `internal/clustering/neighbors.go`, `internal/app/mcp_scalable_reads.go`; `RGR-3`; property equality to full-sort prefix and O(k) retention | For very small k/n full sort can be faster; correctness holds either way |
| Reprepared member writes: `replaceMembersTx` executes identical insert in loop at `store.go:464-479` | Transaction does not own repeated statement lifetime | raw repeated exec vs dynamic multi-row SQL vs Tx-prepared statement; choose `Tx.PrepareContext` (`D7`) | Current/proposed `F1`,`F2`; context/row failure rolls back; projection identity makes retry idempotent; one short atomic write transaction | `internal/corpus/cluster_projection.go`, `internal/clustering/store.go` (deleted); `RGR-7`; one prepared insert per commit and atomic failure test | SQLite remains single-writer; context/busy behavior must be surfaced |

## 12. Dependency decision

| Facility | Status | Reason |
| --- | --- | --- |
| `context` | Use | Standard cancellation contract |
| `container/heap` + `sort` | Use | Maintained exact bounded selection plus final order |
| `database/sql` + existing `modernc.org/sqlite` | Use | Existing adapter, context, transactions, prepared statements |
| `testing.B.ReportAllocs` | Use | Standard allocation evidence |
| Bleve collector | Do not import | Useful verified pattern, but unrelated search types and index semantics |
| PPJoin implementation | Do not add now | No proved safe bound for the weighted composite |
| SQLite FTS5/BM25 | Do not use for scoring | Not Jaccard/composite equivalent |
| Custom job runner/transaction manager/heap | Do not build | Existing repository jobs and standard facilities cover future/runtime needs |

## 13. Approval and next action

The next action, if explicitly approved, is `RGR-1`: freeze public behavior and
extract exact similarity ownership without changing results. Public CLI/MCP
fields, migration 022, production code, and dependency changes remain gated.

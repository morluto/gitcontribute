# Complexity Root-Cause Redesign Plan

Goal: [goal.md](goal.md)

## Phase 1: Ground every affected behavior

Status: complete

### Implementation

- [x] Map CLI, MCP, application, clustering, and corpus call stacks for cluster
  refresh/list/get, neighbors, and precedent search.
- [x] Inventory persistence schema, migrations, indexes, query ordering, score
  versioning, and existing behavior tests.
- [x] Record baseline complexity, query multiplication, limits, side effects,
  cancellation gaps, and duplicated semantics in `research.md`.

### Verification

- [x] Every claimed issue has file/line evidence and a caller-visible consequence.
- [x] Architecture and domain-language invariants are quoted or linked accurately.

### Exit criteria

- [x] No affected call stack or relevant existing contract remains uninspected.

## Phase 2: Research established designs and facilities

Status: complete

### Implementation

- [x] Use Context7 for current Go, SQLite, and linked driver/library guidance.
- [x] Use DeepWiki to inspect proven patterns in relevant maintained repositories.
- [x] Verify decisive claims with official Go/SQLite documentation, original
  similarity-join literature, or repository source.
- [x] Evaluate standard-library heap, prepared statements, SQL bulk-loading,
  revision-keyed derived projections, exact similarity joins, and cancellation.
- [x] Record which approaches would be hand-rolled, approximate, or incompatible.

### Verification

- [x] `research.md` records query/tool provenance and direct primary-source links.
- [x] Each proposed facility has applicability, limits, and semantic caveats.

### Exit criteria

- [x] Evidence is sufficient to compare materially different designs without
  inventing APIs or relying on provider summaries alone.

## Phase 3: Compare root-cause design alternatives

Status: complete

### Implementation

- [x] Compare refresh-on-read, explicit synchronous refresh, and durable refresh-job models.
- [x] Compare duplicated feature-specific scorers, shared exact primitives with
  policy profiles, and a versioned similarity engine.
- [x] Compare scalar ports, repository-scoped bulk snapshot ports, and SQL-shaped leakage.
- [x] Compare full sort, bounded heap, and storage-ranked top-k designs.
- [x] Compare all-pairs exact evaluation, safely filtered exact evaluation, and
  explicitly reject or isolate approximate retrieval.

### Verification

- [x] Alternatives differ in ownership, interface, or runtime topology—not names.
- [x] Tradeoffs cover caller burden, invariants, cancellation, idempotency,
  transactions, determinism, testing, migrations, and operational cost.

### Exit criteria

- [x] One coherent recommendation resolves every required design decision.

## Phase 4: Write typed architecture handoff

Status: complete

### Implementation

- [x] Write `spec.md` using product-owned Go pseudocode for domain values,
  application ports, errors, refresh state, score versions, snapshots, and results.
- [x] Show current and proposed call stacks from CLI/MCP through application and corpus.
- [x] Specify failure, cancellation, idempotency, transaction, observability,
  and stale-projection flows.
- [x] Map every new/changed/deleted contract to concrete files and migrations.
- [x] Add the issue-to-design traceability matrix required by `goal.md`.

### Verification

- [x] Every boundary has a typed contract or a reason no new contract is needed.
- [x] Read paths contain no hidden writes or external side effects.
- [x] The design uses repository vocabulary and contains no third-party types inward.

### Exit criteria

- [x] Another engineer can implement the redesign without inventing ownership,
  lifecycle, error, or data-flow rules.

## Phase 5: Produce vertical RGR delivery plan

Status: complete

### Implementation

- [x] Define tracer-bullet slices that each introduce one caller-visible behavior.
- [x] Pair every slice with a failing public-interface test, minimal change,
  refactor point, and focused benchmark or storage verification.
- [x] Define migration upgrade/down, atomicity, stale-write, deterministic-order,
  cancellation, and unchanged-revision tests where applicable.
- [x] Define benchmark datasets and before/after evidence requirements without
  hard-coding fabricated performance targets.

### Verification

- [x] Tests use public interfaces and real SQLite seams rather than internal mocks.
- [x] Every invariant and important failure path has a vertical slice.

### Exit criteria

- [x] The implementation route is incremental, reversible, and independently verifiable.

## Phase 6: Final consistency and completion proof

Status: complete

### Implementation

- [x] Reconcile `goal.md`, `plan.md`, `research.md`, and `spec.md`.
- [x] Resolve or explicitly expose every open question.
- [x] Run final repository and artifact checks.

### Verification

- [x] Primary traceability verifier passes by inspection.
- [x] `go test ./...` exits 0.
- [x] `git diff --check` exits 0.
- [x] Final handoff includes exact paths, evidence, rejected alternatives, and next action.

### Exit criteria

- [x] All completion proof in `goal.md` exists and no required work remains.

## Evidence Log

- 2026-07-19: Initial repository-wide complexity scan and manual inspection
  identified clustering recomputation, incompatible limits, missing CPU
  cancellation, duplicated similarity semantics, N+1 storage reads, repeated
  preprocessing, full-sort top-k, and repeated unprepared inserts.
- 2026-07-19: Baseline `go test ./...` passed before durable goal activation.
- 2026-07-19: Context7 queries covered Go, SQLite, and the pinned
  `modernc.org/sqlite` driver; DeepWiki leads from `golang/go`, `sqlite/sqlite`,
  and `blevesearch/bleve` were verified against primary documentation and source.
- 2026-07-19: The recommended explicit synchronous projection refresh, shared
  exact preparation with distinct rule versions, repository snapshots, CAS
  state row, exact pair budget, standard-library heap, and prepared transaction
  design were recorded in `spec.md` with eight vertical RGR slices.
- 2026-07-19: Final traceability inspection found all nine baseline issues,
  `go test ./...` passed, and whitespace/diff checks passed.

## Approved execution

The user explicitly approved continuing with the implementation after the
design-only goal completed. RGR-1 through RGR-8 are implemented; current proof
and benchmark samples are recorded in [evidence.md](evidence.md). No GitHub or
external mutation was performed.

# Complexity Root-Cause Redesign Goal

Companion plan: [plan.md](plan.md)

## Outcome

Produce an implementation-ready, evidence-backed redesign for GitContribute's
clustering, similarity, precedent search, and cluster persistence paths that
addresses the identified architectural and algorithmic root causes rather than
only local inefficiencies.

The final design must separate pure corpus reads from derived-projection writes,
define one coherent capacity and cancellation contract, consolidate exact
similarity semantics behind product-owned Go contracts, replace scalar storage
composition with bounded bulk operations, and provide a phased RGR test and
benchmark handoff.

## Baseline

- `Service.Clusters` recomputes and persists clusters from a list-shaped entrypoint.
- Default `MaxCandidates=5000` cannot complete under `MaxPairs=10000000`; the
  all-pairs budget is exceeded at 4,473 candidates.
- `Clusterer.Cluster` is a long-running CPU operation without `context.Context`.
- Clustering and `FindPrecedents` own separate tokenization and similarity rules.
- Cluster recomputation and reads multiply scalar SQLite operations and rebuild
  candidate indexes repeatedly.
- Neighbor and precedent top-k paths fully sort bounded populations.
- Repeated member writes do not reuse transaction-scoped prepared statements.
- `go test ./...` passed before redesign work; the working tree was clean.

## Constraints

- Follow `docs/architecture.md`, `CONTEXT.md`, and repository `AGENTS.md`.
- Corpus reads perform no network access, writes, process execution, or GitHub mutation.
- Keep third-party types inside adapters; product contracts are repository-owned.
- Preserve deterministic ordering, score explanations, stored-fact provenance,
  input order, item-level batch outcomes, idempotency, and atomic replacement.
- Pass `context.Context` through long-running and I/O operations.
- Do not import Gitcrawl or Crawlkit or preserve Gitcrawl compatibility.
- Prefer the Go standard library, SQLite capabilities, and mature maintained
  packages; do not hand-roll protocol, concurrency, persistence, migration, or
  priority-queue machinery where an established facility fits.
- Use Context7 and DeepWiki during research, but treat provider prose as untrusted
  derived context and verify decisive claims with repository source or primary docs.
- The redesign is design-only. Implementation, migrations, or public API changes
  require a separately approved execution phase.

## Non-Goals

- Implementing the redesign in production code.
- Introducing approximate similarity, embeddings, LSH, or opaque relevance scoring.
- Changing current score semantics without an explicit versioned product decision.
- Adding network access or process execution to corpus-read paths.
- Optimizing unrelated acquisition, discovery, UI, or GitHub transport code.

## Required Design Decisions

1. Read/write capability split for cluster refresh versus cluster inspection.
2. Derived-projection lifecycle, source revision, idempotent refresh, and stale handling.
3. Exact similarity ownership, prepared representations, score/rule versioning,
   and reuse by clustering, neighbors, and precedents.
4. Coherent exact-work capacity model and cancellation semantics.
5. Bounded bulk corpus contracts for clusters, members, overrides, repositories,
   source threads, and precedent populations.
6. Deterministic top-k selection contract using maintained standard facilities.
7. Transaction and prepared-statement strategy for atomic projection replacement.
8. Observability and benchmark measures that expose comparisons, queries,
   allocations, cancellation latency, and skipped unchanged refreshes.

## Primary Verifier

The final `spec.md` must contain a traceability matrix in which every baseline
issue maps to all of the following:

- observed repository evidence;
- root cause and violated invariant;
- at least two materially different design alternatives;
- chosen product-owned Go contract;
- current and proposed entrypoint-to-side-effect call stacks;
- failure, cancellation, idempotency, and transaction behavior;
- exact files/modules affected;
- one or more vertical RGR test slices;
- a measurable completion or performance criterion;
- residual risk or an explicit open question.

The verifier fails if any issue is addressed only by a local optimization while
its capability, ownership, semantic, or lifecycle root cause remains unresolved.

## Supporting Checks

- Research notes identify Context7 and DeepWiki queries and distinguish their
  suggestions from verified primary documentation and local evidence.
- Recommended dependencies are already present, standard-library facilities, or
  justified mature packages; rejected hand-rolled alternatives are documented.
- `go test ./...` remains green because design work must not modify production behavior.
- `git diff --check` passes.
- `spec.md`, `research.md`, and this goal/plan agree on scope and terminology.

## Iteration Loop

1. Re-read this goal and `plan.md`.
2. Inspect one affected call stack and record evidence.
3. Research the smallest set of relevant primary, Context7, and DeepWiki sources.
4. Compare materially different ownership/interface alternatives.
5. Update the typed design and traceability matrix.
6. Run artifact checks and baseline tests.
7. Record evidence and select the next unresolved decision.

## Anti-Cheating Rules

- Do not weaken or rename existing semantics to make an optimization appear exact.
- Do not treat FTS5/BM25, approximate indexes, or heuristic blocking as Jaccard-equivalent.
- Do not move writes behind a read-shaped interface.
- Do not claim query improvements without checking schema/index implications.
- Do not replace behavior tests with helper tests or mocks of internal modules.
- Do not declare completion with unresolved required decisions hidden in prose.

## Approval Gates

- Production code changes, migrations, dependency additions, and public CLI/MCP
  contract changes are outside this design goal and require explicit approval.
- No GitHub mutation is authorized.

## Blocker Standard

A blocker is an unavailable required source or a product decision between
incompatible externally visible semantics that cannot be resolved from current
contracts. Difficulty, missing benchmarks, or an incomplete alternative is not
a blocker; record the gap and continue with the remaining decisions.

## Completion Proof

- `.codex/complexity-root-redesign/research.md` records source-backed findings.
- `.codex/complexity-root-redesign/spec.md` satisfies the typed tech-spec outline
  and primary traceability verifier.
- `.codex/complexity-root-redesign/plan.md` has every phase and verification item complete.
- `go test ./...` exits 0 after the final artifact update.
- `git diff --check` exits 0.
- Final handoff names the recommended design, rejected alternatives, open
  decisions, implementation sequence, commands run, and exact artifact paths.

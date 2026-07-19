# Complexity Root-Cause Redesign Implementation Evidence

Date: 2026-07-19

## Delivered invariants

- Cluster list/get operations use corpus-owned read-only SQLite snapshots and do
  not compute or write projection state.
- `clusters refresh OWNER/REPO` is the explicit local-write surface.
- Projection identity includes source revision, governance revision, and
  `duplicate-v1`; unchanged refreshes perform zero comparisons and no writes.
- Commit rechecks source and governance inputs after obtaining the SQLite writer;
  stale attempts roll back and concurrent identical commits produce one current run.
- Exact work is governed by one 10,000,000-pair budget: 4,472 candidates fit and
  4,473 fail before pair evaluation.
- Similarity rules are centralized as `duplicate-v1` and `precedent-v1` without
  unifying their distinct semantics.
- Neighbor and precedent result selection uses `container/heap` and a final
  canonical sort, preserving the full-sort ordered prefix.
- Precedent inputs are grouped by repository, read from one SQLite snapshot, and
  each closed population is prepared once.
- Cluster members and overrides are bulk-loaded, and repeated member writes use
  transaction-scoped prepared statements.
- Cluster governance mutations are corpus-owned and advance the governance
  revision in the same transaction as the override.
- The legacy clustering SQL store, config/clusterer facade, duplicate similarity
  wrappers, and implementation-detail store tests have been removed.
- Migration 023 removes the obsolete `cluster_runs.params_hash` and `stats`
  columns after projection migration 022 has taken ownership of run identity.
- Corpus commit validation rejects incomplete identities and cluster payloads
  from a different source revision or repository before opening a transaction.
- Source revisions retain the full SHA-256 digest, and governance canonical
  validation now occurs in the same transaction as the override write.
- The lifecycle and ownership boundaries are recorded in architecture
  documentation and ADR 0004.

## Verification

- `go test ./...`: pass.
- `go vet ./...`: pass.
- `git diff --check`: pass.
- `go test -race ./internal/clustering ./internal/corpus ./internal/app`: pass.
- MCP serialized tool catalog: 131,010 bytes, below the 131,072-byte budget.
- Migration 022 and 023 up/backfill/down tests: pass.
- Focused stale-source, governance-revision, empty-projection, unchanged-refresh,
  canonical-order, cancellation, and exact-capacity tests: pass.

## One-shot benchmark sample

Host: Apple M5, Darwin arm64. These are diagnostic samples, not an SLA.

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| top-k heap, 10,000 to 100 | 160,375 | 7,904 | 106 |
| canonical full sort, 10,000 to 100 | 2,371,166 | 245,856 | 4 |
| exact cluster engine, 100 candidates / 4,950 pairs | 600,250 | 41,112 | 715 |

Reproduce with:

```sh
go test ./internal/ranking -run '^$' -bench '^BenchmarkTopK$' -benchtime=1x -benchmem
go test ./internal/clustering -run '^$' -bench '^BenchmarkClusterEngine/100$' -benchtime=1x -benchmem
```

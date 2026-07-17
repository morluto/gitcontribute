# Contributing

GitContribute is organized around explicit capability boundaries. Start with
[the architecture guide](docs/architecture.md) before changing storage,
network, process-execution, or protocol code.

## Development setup

Requirements:

- Go 1.26 or newer
- Git

Run the standard validation suite from the repository root:

```sh
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

Use focused race tests for changes involving SQLite transactions, goroutines,
filesystem locks, job ownership, or cancellation:

```sh
go test -race ./internal/app ./internal/corpus
```

The SQLite driver is pure Go. Keep CGO disabled compatibility when changing
storage or build dependencies.

## Where changes belong

- Put use-case decisions and capability composition in `internal/app`.
- Put SQL, transactions, migrations, and local query ordering in
  `internal/corpus`.
- Keep GitHub SDK mapping and HTTP policy in `internal/github`.
- Keep CLI, MCP, and TUI code as adapters over application contracts.
- Keep process execution in the acquisition, workspace, or evidence boundary;
  corpus reads must never execute commands.

Prefer a narrow product-owned interface over exposing a third-party type to
another package.

## Storage invariants

When changing corpus writes, preserve these rules:

1. Record source observations even when an older observation loses projection
   ordering.
2. Update projections only when `(source_updated_at,
   observation_sequence)` wins.
3. Buffer paginated child data and replace it atomically only after retrieval
   completes.
4. Treat an empty complete child set as a valid replacement.
5. Give paginated queries a stable final tie-breaker and bind cursors to their
   query scope.
6. Keep multi-record imports and state transitions transactional.

Add a new numbered Goose migration for schema changes. Test both upgrade and
rollback behavior when the migration is reversible.

## Side effects and security

- Local inspection commands must remain offline.
- GitHub reads must be explicit, bounded, rate-limited, and cancellation-aware.
- Do not add GitHub mutation without a separate reviewed capability.
- Never execute repository-controlled code during crawl, sync, indexing,
  search, health, or dossier operations.
- Reject credentials in persisted remote URLs and redact secrets and absolute
  local paths from exported metadata.
- Require explicit authorization for validation commands and keep their
  environment allowlisted.

## Tests

Tests should prove behavior at the owner boundary. Important regression cases
include:

- stale and equal-timestamp observations;
- interrupted pagination and complete empty replacement;
- stable cursor ordering and scope mismatch;
- retry cancellation and per-attempt rate limiting;
- cross-process mirror locking and job reconciliation;
- read-only operations performing no network or process work;
- transaction rollback after a late validation or persistence failure.

Use local HTTP servers, temporary repositories, and temporary databases. Do
not depend on live GitHub state in the test suite.

## Pull requests

Keep changes focused on one outcome. For storage, concurrency, protocol, or
execution changes, describe the invariant being protected and include the
focused regression command. For large changes, give reviewers an order that
starts with the owner boundary and highest-risk state transition.

# Contributing

GitContribute is organized around explicit capability boundaries. Start with
[the architecture guide](docs/architecture.md) before changing storage,
network, process-execution, or protocol code.

## Development setup

Requirements:

- the Go version declared in `go.mod`;
- Git;
- Make and curl for the documented convenience targets;
- golangci-lint v2.12.2 for local linting;
- pre-commit 4.6.1 if you want the repository-managed Git hooks.

The dev container provides the pinned tools and installs the hooks. For a local
checkout, install golangci-lint and the hooks after installing
[pre-commit](https://pre-commit.com/#installation):

```sh
go mod download
make install-tools
pre-commit install --install-hooks
```

The commit hook only formats staged Go files and performs inexpensive text and
configuration checks. Cached Go tests run at pre-push time. Skip a hook when
necessary with standard Git or pre-commit controls, then run the corresponding
check explicitly before opening a pull request.

## Development loop

Use cached tests during normal development:

```sh
make test
go test ./internal/app -run '^TestName$'
```

Run the fast local checks before pushing and the complete validation before a
pull request:

```sh
make check
make verify
```

`make verify` runs uncached tests, changed-code linting, module-tidiness checks,
generated-output verification, and documentation validation. It is the complete
local check, while CI adds platform, coverage, security, and focused race jobs.
`make lint-full` is available for auditing existing repository-wide lint debt.
Use `make test-uncached` when only a fresh test run is needed.

Use focused race tests for changes involving SQLite transactions, goroutines,
filesystem locks, job ownership, or cancellation:

```sh
make test-race
```

The SQLite driver is pure Go. Keep CGO-disabled compatibility when changing
storage or build dependencies.

## Generated output and documentation

Generated files are verified in pull-request CI, not at commit time. When a
change affects generator inputs, refresh and inspect the output explicitly:

```sh
make generate
make generate-check
```

Run `make docs` to perform the same repository-documentation validation used by
CI. Publishing-only documentation and release notes belong in release
automation rather than local hooks.

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
not depend on live external services in the test suite.

## Pull requests

Keep changes focused on one outcome. For storage, concurrency, protocol, or
execution changes, describe the invariant being protected and include the
focused regression command. For large changes, give reviewers an order that
starts with the owner boundary and highest-risk state transition.

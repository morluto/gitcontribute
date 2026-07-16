# ADR 0002: Application and corpus boundaries

Status: accepted

## Context

The CLI and MCP server need equivalent behavior. Crawling requires network
access, while search and dossier reads must remain predictably local.

## Decision

Product use cases live in `internal/app`. CLI and MCP packages are adapters and
do not implement domain decisions. GitHub types terminate at the GitHub adapter;
SQLite types terminate at the corpus adapter.

The corpus stores immutable source observations and normalized current-state
projections. A projection accepts a newer source timestamp, or a higher local
observation sequence when source timestamps are equal or unavailable. Hydration
facets maintain independent ordering and are replaced atomically only after a
complete fetch.

## Consequences

- CLI and MCP can be tested against the same application fake.
- Local reads cannot accidentally trigger network access.
- Adapter mapping is explicit but prevents vendor contracts from becoming the
  product's public model.
- Interrupted and out-of-order crawl work can be replayed safely.

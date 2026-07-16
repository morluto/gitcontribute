# ADR 0001: Independent implementation

Status: accepted

## Context

Gitcrawl demonstrates valuable local archive, observation-ordering, hydration,
and search behavior. Its packages, schema, CLI, and release lifecycle are built
for a different product, and most implementation packages are internal.

## Decision

GitContribute is a new repository and Go module. It does not import Gitcrawl or
Crawlkit, preserve their compatibility contracts, or require their releases.
Their source and tests may be used as behavioral references.

Small source units may be adapted directly when useful. Adapted code is
maintained here and does not create an upstream synchronization requirement.

## Consequences

- GitContribute owns its schema, domain model, interfaces, and migrations.
- More initial implementation is required.
- Product boundaries remain aligned with contribution research rather than
  inherited compatibility.
- Proven edge cases can still be recreated and attributed when appropriate.

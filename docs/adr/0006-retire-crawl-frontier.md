# ADR 0006: Retire the orphaned crawl frontier

- Status: Accepted
- Date: 2026-08-10

## Context

The durable crawl frontier originally fed a bounded worker. That executor and
its only application entry points were later removed, while repository
discovery continued to enqueue work and status continued to report it as
ready. No supported operation could lease or complete those rows. The queue
therefore represented planned work that the product could never perform.

Frontier rows contain scheduling hints derived from already stored repository
identities. They are neither source observations nor derived projections, and
they have no independent recovery value after the executor's removal.

## Decision

Repository discovery stores its observations and checkpoints directly and no
longer creates frontier rows. Migration 016 drops the unused queue and its
revision triggers. Its Down section recreates the legacy schema empty; the
discarded hints cannot be reconstructed, so the explicit migration workflow's
verified backup is the data rollback path.

The existing `frontier_ready` and `frontier_items` JSON fields remain as
zero-valued compatibility fields. Internal status, repository-removal, and
storage models no longer carry the retired concept.

## Consequences

- Discovery no longer creates permanently pending work or a misleading status
  warning.
- Corpus observations, checkpoints, projections, and explicit hydration
  capabilities are unchanged.
- Downgrading the schema recreates an empty legacy queue; restoring the
  pre-migration backup is required to inspect discarded scheduling hints.

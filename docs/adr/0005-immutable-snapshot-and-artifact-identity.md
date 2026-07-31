# ADR 0005: Immutable snapshot and artifact identity

## Status

Accepted

## Context

Global corpus revisions identify a changing database as a whole, but they do
not identify the repository, thread facets, derived inputs, completeness, or
payload used by a research read. Repository-and-commit code-index URIs had the
same problem: re-indexing could replace the payload behind an old URI.

SQLite read transactions provide consistency for one operation. Holding one
across network work, agent turns, or durable jobs is neither safe nor a durable
identity. Copying an entire SQLite database image for every local read would
also make ordinary research unnecessarily expensive and couple identities to
unrelated corpus state.

## Decision

Reusable reads are materialized as scoped immutable artifacts. A
`CorpusSnapshotToken` binds the contract version, observation watermark,
scope, source-manifest digest, derived-input versions, completeness,
provenance, and exact artifact digest. Resolution either returns that payload
after digest verification or a typed `snapshot_unavailable` or
`snapshot_expired` error. It never falls back to current projections.

Code-index artifacts use
`gitcontribute://artifact/code-index/<artifact-digest>`. Their persisted
manifest includes the repository, source commit, indexed document digests,
coverage counts, every skip category, truncation, schema version, and creation
provenance. Re-indexing the same commit creates a new record; old URIs remain
stable.

SQLite transactions remain the consistency mechanism inside one operation.
Whole-database SQLite snapshots are reserved for a future explicit
publication/export capability, not ordinary local read identity.

## Consequences

- Snapshot and artifact URIs are opaque, digest-bound handoffs.
- Missing or inconsistent artifacts fail closed.
- Corpus reads remain offline; materialization is an explicit local write.
- Global revisions may remain internal ordering/watermark data, but are not a
  sufficient reusable research identity.
- Migrations contain no network work and Gitcrawl wire compatibility is not a
  goal.

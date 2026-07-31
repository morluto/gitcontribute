# MCP composed-workflow decisions

GitContribute does not mirror every CLI command with one MCP tool. CLI commands
may compose application capabilities for a person, while the MCP catalog
exposes bounded operations that agents can combine without hiding coverage,
provenance, authorization, or partial failure.

## Decision criteria

A repeated CLI workflow warrants a new MCP operation only when controlled
evaluation shows at least one of these outcomes:

- the same multi-call sequence recurs across real contribution workflows;
- clients lose coverage, provenance, or explicit unknowns while joining
  results;
- a bounded operation materially reduces calls or payload without broadening
  authority;
- independent clients repeatedly make the same semantic composition mistake.

The evidence must include semantic task success, side-effect and uncertainty
correctness, operational calls, result payload, initial and subsequently loaded
tool context, invalid calls, and latency. Catalog size and overlap count
against a new operation. Feature-count parity with the CLI is not evidence.

Evaluation is explicit and opt-in. Run the public scenarios under
`internal/mcpserver/testdata/agent-eval/` with frozen model, sampling, catalog,
corpus, permissions, prompt, and token budget. Keep the oracle outside the
candidate workspace and retain the tool transcript and host request or
telemetry used for token claims. GitContribute sends no background evaluation
telemetry.

## Issue or pull-request research brief

For a complete source audit, the canonical route is:

```text
coverage -> ensure_coverage -> jobs.get -> snapshot-bound offline reread -> duplicate checks -> live verification -> receipt attachment -> evidence/draft handoff
```

Coverage reads are offline and missing coverage is unknown. Synchronization is
always an explicit bounded operation; after `jobs.get`, perform the offline
reread and reuse its returned `snapshot_token` for any composed duplicate
checks. Use exact resource URIs only through MCP `resources/read` before attaching
receipts or handing evidence to a draft workflow.

Canonical MCP composition:

1. Use `workflow.prepare_issue_set` for supplied exact issues. It returns
   stored facts, per-thread coverage gaps, related work, merged precedents, and
   linkage candidates without network access.
2. For an exact pull request or fields not covered by that aggregate, use
   `corpus.get_threads` with `response_format=detailed`.
3. Read repository guidance or a persisted dossier only when the task needs
   repository-wide context. Do not treat missing coverage as a negative result.
4. Perform a bounded GitHub sync only when current live state is required, wait
   for its durable job, and then repeat the relevant offline read.

Decision: **compose**. `workflow.prepare_issue_set` already removes the
error-prone issue fan-out while preserving coverage and provenance. A second
general “research brief” MCP operation would overlap it and the exact-thread
read without demonstrated semantic benefit.

## Base and candidate validation comparison

Canonical MCP composition:

1. Use `workspace.adopt` or `workspace.create` for the authorized base and
   candidate worktrees.
2. Use `validation.define` once with the exact command and both workspace IDs.
3. Use `validation.run` with `target: both`; set `run_count` above one when
   repeated evidence is required. Execution remains an explicit process
   capability.
4. Wait for the returned durable jobs and compare their typed validation
   records. Preserve the command, commit, environment policy, timing, exit
   status, bounded output, and observation contract in the evidence handoff.

Decision: **compose**. The MCP operations already share the application
validation contracts used by the CLI. Current evidence does not show repeated
loss of comparison semantics that would justify another process-capable tool.
Reconsider only if controlled traces show recurring client mistakes or a
material call/payload reduction that preserves authorization and proof.

## Unified catalog

The managed server advertises the unified `all` catalog. Current Codex and
Claude Code hosts can defer MCP definitions and discover them with native tool
search; GitContribute does not reproduce that host machinery. Focused catalog
projections exist only inside the evaluation harness and are not a user-facing
configuration surface.

The v5 evaluation compares unified eager and unified host-search conditions.
Serialized definition bytes are a deterministic catalog-cost proxy, not a
model-token or selection-accuracy claim. Token claims require the exact host
request or equivalent telemetry.

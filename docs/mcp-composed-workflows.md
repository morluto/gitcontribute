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
corpus.get_coverage -> typed exact/repository recovery -> jobs.get
  -> snapshot-bound offline reread -> duplicate checks
  -> explicit live verification -> jobs.get -> receipt attachment
  -> evidence/draft handoff
```

Coverage reads are offline and missing coverage is unknown. Synchronization is
always an explicit bounded operation. If `corpus.get_coverage`, an exact thread
read, or a facet read returns incomplete coverage, follow its item-level typed
recovery action while preserving exact-thread versus repository scope. Poll the
returned job, perform the offline reread, and reuse its returned
`snapshot_token` for composed duplicate checks. Use exact resource URIs only
through MCP `resources/read` before attaching receipts or handing evidence to a
draft workflow.

Canonical MCP composition:

1. Use `corpus.get_threads` for supplied exact issues or pull requests and
   `corpus.get_thread_facets` for selected child coverage.
2. Use `corpus.find_clusters`, `corpus.find_neighbors`, and
   `corpus.find_precedents` only for the duplicate or historical evidence the
   task needs.
3. Read repository guidance only when the task needs repository-wide context.
   Do not treat missing coverage as a negative result.
4. Perform a bounded GitHub sync only when current live state is required or
   when returned coverage recovery requests it, wait for its durable job, and
   then repeat the relevant offline read.

Decision: **compose**. The facts-first catalog removed the aggregate issue-set
operation. Exact reads and bounded duplicate primitives make coverage and
selection visible to the calling agent without granting another workflow tool
authority over the sequence.

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

## Contribution collision checks

```text
github.search_threads (bounded current work)
github.sync_pull_request_portfolio(selection=authored) -> jobs.get
corpus.search_pull_requests | corpus.find_pull_request_overlaps
workspace.check_merge_conflicts (only after explicit acquisition)
```

Decision: **compose**. The catalog no longer exposes a preflight workflow that
decides whether an agent should start work. Agents select the current-work,
portfolio, overlap, or Git comparison facts appropriate to the task. Unknown
discovery or facet coverage prevents a “no competing work” conclusion.

## Unified catalog

The managed server advertises the unified `all` catalog. Current Codex and
Claude Code hosts can defer MCP definitions and discover them with native tool
search; GitContribute does not reproduce that host machinery. Focused catalog
projections exist only inside the evaluation harness and are not a user-facing
configuration surface.

`workflow.get_catalog_contract` is the canonical parity check for a live
connection. It returns the running version, post-registration catalog mode,
tool count, deterministic catalog fingerprint, and explicit availability of
the repository-wide feedback index, exact-PR feedback sync, and offline
feedback search. A setup or upgrade changes the installed registration only;
the client must start a fresh MCP connection before it can observe the new
catalog. A read-only server reports that restricted mode explicitly instead of
making its missing network tools look like an installation failure.

The v5 evaluation compares unified eager and unified host-search conditions.
Serialized definition bytes are a deterministic catalog-cost proxy, not a
model-token or selection-accuracy claim. Token claims require the exact host
request or equivalent telemetry.

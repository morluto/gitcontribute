# MCP tool redesign

GitContribute targets `github.com/modelcontextprotocol/go-sdk` `v1.7.0` and
negotiates MCP `2026-07-28`. The server continues to
register generic SDK tools so the SDK owns input decoding and output-schema
validation at the protocol boundary.

## Contract ownership

Schema semantics live with their Go values. Probability, similarity, radar
score, progress, non-negative counts, batch status, and job status are reusable
typed schema values. A field named `score`, `confidence`, `status`, `kind`, or
`result` never receives semantics from its JSON name alone.

Multi-mode tools keep an object root and compose draft-2020-12 schema nodes
over SDK-inferred Go structs. `oneOf`, `required`, `not`,
`dependentRequired`, bounds, defaults, and constants express protocol shape.
Handlers retain checks for repository existence, authorization, lifecycle
legality, RFC 3339 values, and stored-state consistency. There is no parallel
JSON decoder or validator.

## Response and side-effect boundaries

`jobs.get` returns bounded status, progress, typed artifact references, and a
suggested follow-up; it does not expose stored request or result blobs.
Repository dossiers, repository projections, and manifest statements are
typed. DeepWiki defaults to 32 KiB and directs truncated reads toward structure
or a focused question before a larger response.

Tool results link durable concerns, dossiers, investigations, opportunities,
evidence, readiness reports, immutable draft revisions, contribution manifests,
and job artifacts with SDK-native resource links. Resources are the canonical
detailed read plane for durable artifacts; the catalog does not duplicate them
as scalar tools. Producers return compact typed references so clients follow
the exact resource URI instead of consuming duplicate structured output.
Recording a hypothesis returns its parent investigation reference because the
investigation resource is the canonical aggregate that contains hypotheses.
Bounded searches, rankings, and multi-object reads remain tools because they are
queries rather than durable-artifact representations.
Clients must support MCP `resources/read`; Codex exposes that operation as
`read_mcp_resource`. GitContribute does not provide a parallel generic read
tool or scalar artifact fallback.

Validation receipts remain ordinary tool results. They are small execution
classifications needed immediately by the caller, so replacing them with a
second read would add ceremony without reducing a material payload. Workspaces
also remain operational tool results: their lifecycle is coupled to host
filesystem and process capabilities, and their public representation
intentionally omits host paths. Neither is an independently browsable durable
artifact today; revisit that boundary only when a concrete cross-session read
workflow needs it.

The catalog preserves offline reads, network reads, local writes, process
execution, and external mutation as separate capabilities. The unified catalog
exposes the complete concern lifecycle rather than a partial subset. The
dossier build operation is named `workflow.build_repository_dossier` because
it writes local state.

## Consolidation decisions

Exact issue preparation remains the aggregate `workflow.prepare_issue_set`.
Durable submission and polling, validation definition and authorized
execution, and commit inspection and planning remain separate because each
boundary permits meaningful agent judgment or authorization.

Live repository search includes local dossier availability. The catalog offers
`github.sync_pull_request_portfolio`, a bounded durable job with authored and
explicit selection modes. Authenticated identity, authored discovery, and
exact status refresh are internal phases and are not advertised as recovery
primitives. Feedback and CI use the separate
`github.sync_pull_request_feedback` and `github.sync_ci_failures` jobs so their
coverage and retry behavior remain visible.

The catalog offers
`workflow.mine_repository_fix_patterns`. It consolidates the observed
search-select-hydrate-rescan loop while preserving the real durable-job,
network-read, and local-write boundaries. The triggering agent trace found 587
otherwise matching pull requests with unknown merge state, then required 26
exact hydrations to recover 21 confirmed merged examples; one persistence step
also encountered `SQLITE_BUSY`. The aggregate therefore hydrates only bounded
unknown-state finalists, reports unknowns before and after hydration, and
separates confirmed merged fixes from merely similar closed work.

DeepWiki retains one tool with three discriminated modes. Host-native tool
search is the capability-discovery mechanism; the server does not mutate its
catalog or add a custom discovery meta-tool.

## Evidence gate

Catalog byte measurements are regression proxies, not model evidence. The v5
fixture under `internal/mcpserver/testdata/agent-eval/v5` records the unified
catalog decision and compares eager loading with host-native tool search. Each
condition requires at least three trials with frozen model, sampling settings,
catalog, corpus revision, permissions, prompt, and token budget.

Semantic correctness and side-effect correctness are gates. Only afterward may
invalid calls, redundant calls, result tokens, latency, and recovery success be
compared. The fixture does not claim that model trials have run.

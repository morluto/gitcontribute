# MCP tool redesign

GitContribute targets `github.com/modelcontextprotocol/go-sdk`
`v1.7.0-pre.3` and negotiates MCP `2026-07-28`. The server continues to
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

Tool results link durable dossiers, investigations, opportunities, evidence,
readiness reports, and job artifacts with SDK-native resource links. Scalar
read tools remain available until the v4 client-compatibility evaluation shows
that supported clients reliably follow resources. Resources and scalar tools
must not both be read for one result.

The catalog preserves offline reads, network reads, local writes, process
execution, and external mutation as separate capabilities. The default
`contribute` profile contains no partial concern lifecycle; the specialized
`concerns` profile exposes the complete lifecycle. The dossier build operation
is named `workflow.build_repository_dossier` because it writes local state.

## Consolidation decisions

Exact issue preparation remains the aggregate `workflow.prepare_issue_set`.
Durable submission and polling, validation definition and authorized
execution, and commit inspection and planning remain separate because each
boundary permits meaningful agent judgment or authorization.

Live repository search includes local dossier availability. The specialized
portfolio profile offers `github.sync_portfolio`, a bounded durable job that
uses the existing authored-discovery and exact-status operations and chunks
status refreshes at 50 pull requests. The underlying primitives remain
available for recovery and partial workflows.

The `patterns` profile offers
`workflow.mine_repository_fix_patterns`. It consolidates the observed
search-select-hydrate-rescan loop while preserving the real durable-job,
network-read, and local-write boundaries. The triggering agent trace found 587
otherwise matching pull requests with unknown merge state, then required 26
exact hydrations to recover 21 confirmed merged examples; one persistence step
also encountered `SQLITE_BUSY`. The aggregate therefore hydrates only bounded
unknown-state finalists, reports unknowns before and after hydration, and
separates confirmed merged fixes from merely similar closed work. It remains
opt-in until held-out model trials justify default-profile membership.

DeepWiki retains one tool with three discriminated modes. Static profiles
remain the capability-discovery mechanism; the server does not mutate a global
catalog or add a custom discovery meta-tool.

## Evidence gate

Catalog byte measurements are regression proxies, not model evidence. The v4
fixtures under `internal/mcpserver/testdata/agent-eval/v4` freeze catalog
fingerprints and compare the ten ambiguous workflows called out in the design
review. Each condition requires at least three trials with frozen model,
sampling settings, catalog, corpus revision, permissions, prompt, and token
budget.

Semantic correctness and side-effect correctness are gates. Only afterward may
invalid calls, redundant calls, result tokens, latency, and recovery success be
compared. No further default-profile reduction ships until those trials show at
least 25% lower model-visible catalog context without a meaningful task-success
regression, an increased invalid-call rate, or lost side-effect disclosure.

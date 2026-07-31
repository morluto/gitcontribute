# Agent tool evaluation

GitContribute evaluates its MCP surface with realistic, scripted tool calls
through the same in-memory client/server boundary used by consumers. The
required suite is deterministic: it does not call a language model, access
GitHub, or execute repository code. Latency is recorded for comparison, but
never controls pass/fail.

## What the suite measures

Each scenario records the tool, arguments, result status, and compact metrics:

- task completion at the protocol-contract level;
- tool-call and tool-error counts;
- invalid-argument errors;
- retry count and elapsed latency for each multi-step workflow;
- exact opaque resource-read handoffs;
- structured response bytes as a context-pressure proxy;
- a deterministic context-token estimate derived from serialized responses;
- durable-job polling calls.

Schema checks inspect the serialized MCP catalog. They require an object input
schema, visible documented properties, and no root `allOf` intersection that a
client may render as an opaque or `unknown` type.

## Interpretation limits

Scripted calls can reveal protocol burden, response size, recovery sequencing,
and ambiguous validation. They cannot establish whether a model will select
the right tool, recover from an error, or benefit from a search preset. The
context-token estimate is a stable serialization proxy, not a claim about any
model tokenizer; local elapsed latency is likewise not a service SLO. Do not
call these metrics model success rates.

Changes that consolidate jobs or add opinionated presets should additionally
be supported by repeated model-backed or human-agent traces. Such evaluations
must remain optional and non-gating unless their model, prompts, credentials,
and sampling policy are made reproducible outside the unit-test suite.

## Optional model-in-the-loop suite

The v5 fixture under `internal/mcpserver/testdata/agent-eval` evaluates the
unified catalog under eager and host-native tool-search conditions. Give the
candidate only `v5/public.json` and the seeded MCP server. Keep the semantic
oracle outside its filesystem and context. A separate reviewer scores semantic
correctness and side-effect correctness before comparing tool calls, context
tokens, or latency.

The held-out `v5/heldout.json` fixture exercises bounded coverage recovery,
stale-token recovery, exact resource handoffs, and audit-only fix-pattern
preview. `TestAgentEvalHeldOutWorkflowMetrics` runs those scenarios through
the public MCP boundary and logs task success, calls, errors, retries, latency,
response bytes, and the context-token estimate. Its oracle is the test's
contract-level assertion; it is intentionally not presented as a model-backed
benchmark.

Use the same model, sampling settings, corpus fixture revision, catalog
condition, and read-only mode for baseline/candidate comparisons. Save
initialize, tools/list, tool calls, tool results, final answer, elapsed time,
and failures. At least three repeated runs per scenario are needed before
making tool-choice claims; deterministic Go tests validate contracts but never
count as model runs.

## Current decisions

The durable-job scenario requires one submission and one poll. The current
surface already polls multiple IDs through one `jobs.get` call, so current
evidence does not justify merging job submission and status reads. Job references now
carry a polling delay and a suggested `jobs.get` call; further consolidation is
deferred until agent traces show missed, redundant, or premature polling.

There is no current evidence that an opinionated repository-search preset
improves held-out task completion. The surface therefore has validated
structured filters but no `trending`, `active`, or `contribution_friendly`
preset. A preset should be introduced only with a disclosed definition and a
measurable improvement on repeated model-backed or human-agent traces.

## Extending the suite

Keep public scenarios small and representative. Add held-out cases with
different values rather than copying description examples. Useful held-out
contracts include:

- rejecting a cursor reused with different filters;
- rejecting simultaneous structured filters and `raw_query`;
- preserving semantic references across concise and detailed responses;
- returning stable, duplicate-free pagination;
- avoiding poll suggestions for terminal jobs.
- comparing `workflow.mine_repository_fix_patterns` with manual
  search/select/hydrate loops on a repository where closed PR headers have
  unknown merge state; score confirmed merged, closed-unmerged, superseded,
  open, and unknown outcomes separately.

Run the focused suite with:

```sh
go test ./internal/mcpserver -run AgentEval
```

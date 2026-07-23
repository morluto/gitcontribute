# Agent tool evaluation

GitContribute evaluates its MCP surface with realistic, scripted tool calls
through the same in-memory client/server boundary used by consumers. The
required suite is deterministic: it does not call a language model, access
GitHub, execute repository code, or depend on wall-clock timing.

## What the suite measures

Each scenario records the tool, arguments, result status, and compact metrics:

- task completion at the protocol-contract level;
- tool-call and tool-error counts;
- invalid-argument errors;
- structured response bytes as a context-pressure proxy;
- durable-job polling calls.

Schema checks inspect the serialized MCP catalog. They require an object input
schema, visible documented properties, and no root `allOf` intersection that a
client may render as an opaque or `unknown` type.

The committed baseline under
`internal/mcpserver/testdata/agent-eval/baseline.json` describes the contract
before the agent-tool ergonomics work. It is evidence for comparison, not an
expected output that new behavior must preserve.

## Interpretation limits

Scripted calls can reveal protocol burden, response size, ambiguous validation,
and polling sequences. They cannot establish whether a model will select the
right tool, recover from an error, or benefit from a search preset. Do not call
these metrics token counts or model success rates.

Changes that consolidate jobs or add opinionated presets should additionally
be supported by repeated model-backed or human-agent traces. Such evaluations
must remain optional and non-gating unless their model, prompts, credentials,
and sampling policy are made reproducible outside the unit-test suite.

## Optional model-in-the-loop suite

The paired v2 fixtures under `internal/mcpserver/testdata/agent-eval` preserve
three real failure modes: confusing relevance with newest order, treating
repository metadata as README coverage, and silently rebuilding a persisted
dossier. Give the candidate only `public-v2.json` and the seeded MCP server.
Keep `oracle-v2.json` outside its filesystem and context. A separate reviewer
scores semantic correctness, required evidence, the critical discriminator,
and uncertainty before comparing tool calls, response bytes, or latency.

Use the same model, sampling settings, corpus fixture revision, toolsets, and
read-only mode for baseline/candidate comparisons. Save initialize, tools/list,
tool calls, tool results, final answer, elapsed time, and failures. At least
three repeated runs per scenario are needed before making tool-choice claims;
the deterministic Go tests validate contracts but never count as model runs.

## Decisions from the initial baseline

The durable-job scenario requires one submission and one poll. The current
surface already polls multiple IDs through one `jobs.get` call, so the baseline
does not justify merging job submission and status reads. Job references now
carry a polling delay and a suggested `jobs.get` call; further consolidation is
deferred until agent traces show missed, redundant, or premature polling.

The baseline also contains no evidence that an opinionated repository-search
preset improves held-out task completion. This change therefore adds validated
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

Run the focused suite with:

```sh
go test ./internal/mcpserver -run AgentEval
```

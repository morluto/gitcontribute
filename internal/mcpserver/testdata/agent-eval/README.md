# Agent-tool evaluation fixtures

`baseline.json` records a small set of scripted MCP interactions from the
pre-ergonomics contract. It is retained for before/after comparison and is not
a golden output that the improved implementation must reproduce.

The public scenarios cover exact repository search, input correction, and a
durable job round trip. Future held-out cases should use different values and
test the same capabilities: cursor scope, response detail, structured/raw
search exclusivity, semantic references, and terminal job behavior.

Do not add credentials, live GitHub responses, wall-clock timings, or claims
about model success to these fixtures.

`public-v2.json` contains natural held-out prompts only. `oracle-v2.json` is
evaluator-only: do not place it in the candidate workspace or prompt context.
It scores conclusions and evidence, not one exact call order. Run the public
cases repeatedly through the same model and sampling settings with the focused
catalog, save the full MCP transcript, then have a separate reviewer apply the
oracle.

# Unified-catalog evaluation v5

This suite checks the decision to advertise one unified catalog and rely on
host-native tool search. GitContribute does not implement deferred loading;
core MCP only provides tool discovery.

Run every condition at least three times with the same model, sampling
settings, prompt, corpus, permissions, and token budget:

- `unified_eager`: advertise `all` and place the complete `tools/list` payload
  in model context;
- `unified_host_search`: advertise `all`, but use the target host's native
  tool search so definitions are loaded only after discovery.

Record the exact `initialize` and `tools/list` payloads, initial tool-definition
bytes or tokens actually placed in model context, search-result and expanded
definition bytes or tokens, tool-search calls, operational tool calls,
argument-validation failures, task success, latency, and native fallback.
Host telemetry or a captured request is required for context-token claims;
serialized bytes are only a deterministic proxy.

Give candidates only `public.json`. Mount the hash-committed oracle from an
out-of-band evaluator store and keep it outside the candidate workspace and
prompt context. Score task outcome and side-effect correctness before
efficiency. Deterministic catalog measurements are not model-selection rates.

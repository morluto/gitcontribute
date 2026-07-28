# Tool-selection evaluation v4

This suite compares competing MCP surfaces and input modes. Give candidates
only `public.json`; mount the hash-committed oracle from an out-of-band
evaluator store and keep it outside their filesystem and context. Run every
condition at least three times with the same model,
sampling settings, corpus revision, permissions, prompt, and token budget.

The catalog SHA-256 values cover the exact JSON serialization returned by the
in-memory SDK client after applying each catalog's disabled-tool ablation.
The Go contract test rejects drift. When a deliberate tool contract change
updates a fingerprint, review the serialized catalog and update the fixture in
the same change.

Apply semantic hard failures before comparing invalid calls, redundant calls,
result tokens, latency, or recovery. These fixtures define an evaluation; they
do not claim that model trials have run.

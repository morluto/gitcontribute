# Private evaluator input

The v4 semantic oracle is deliberately not stored in this repository. Mount it
as `oracle.json` only in the evaluator process, never in the candidate
workspace or model context. The runner must reject it unless its version and
SHA-256 match the commitment in `../public.json`.

The committed hash identifies the frozen oracle without publishing its
required behaviors or hard-failure traps. Rotating the oracle requires a new
fixture revision and catalog-controlled evaluation run.

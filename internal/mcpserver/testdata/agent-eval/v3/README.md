# Evidence-boundary agent evaluation

Version 3 contains three model-in-the-loop scenarios:

- contribution readiness with stale or unavailable checks;
- repository research with empty or unavailable evidence;
- version-aware critique of a completed tool workflow.

`public.json` and `artifacts/` are candidate-visible. The runner must mount only
the common artifact named by the selected scenario plus that condition's
artifact overlay. Other condition overlays must remain absent. `private/` is
evaluator-only and must never enter the candidate workspace, prompt, retrieval
index, or reviewer bundle.

Keep model, prompt, fixture revision, permissions, token budget, and sampling
settings fixed across conditions. Run at least the configured number of
independent trials. Save full MCP transcripts and declared evidence bundles.
Apply semantic gates before comparing calls, latency, tokens, retries, or
handoff size.

The calibration trajectories are frozen examples for checking the rubric.
They are not model measurements. A production run writes new transcripts
outside this directory and records model/client versions plus every
candidate-visible byte.

Before each run:

1. Verify every SHA-256 value in `manifest.json`.
2. Copy one public scenario, its common artifact, and only the selected
   condition overlay into a fresh workspace.
3. Confirm `private/` and prior run outputs are absent.
4. Record tool availability and exact binary/schema revisions.
5. Run the natural prompt without oracle labels or issue history.
6. Give a second reviewer only the public task and candidate evidence bundle.

<!--
PR title: type(optional-scope): imperative outcome
Use a Conventional Commit title; release automation derives notes from it.
-->

## Summary

<!-- What user, agent, or maintainer problem does this solve? Link the issue
with "Fixes #123" when applicable. Keep prior behavior, expected contract,
and new behavior understandable without private context. -->

## Problem and expected behavior

<!-- For fixes, state the smallest trigger and violated invariant. For features,
state the use case and observable outcome. -->

## Change and scope

<!-- Explain the approach, why it fits the package boundaries, and intentional
non-goals. Avoid a file-by-file walkthrough. -->

## Contract and boundary impact

<!-- Complete the applicable lines; use "none" or "not applicable" explicitly. -->

- Semantic owner and earliest changed stage:
- GitHub acquisition, network, or process side effects:
- Corpus, transaction, projection, pagination, or migration contract:
- CLI, TUI, MCP, resource, or serialized schema contract:
- Job, retry, cancellation, or concurrency impact:
- Security, privacy, credential, or no-execution impact:

## Evidence and regression coverage

<!-- State whether evidence is executed, source-derived, or proposed. For fixes,
explain how the regression would fail on the affected base revision when practical. -->

- Tests added or updated:
- Base reproduction or other evidence:
- User-visible CLI/MCP/resource output (if applicable):
- Remaining proof gaps:

## Validation performed

<!-- List only commands that actually ran, with observed results. Include the
focused command as well as broader checks when they materially support the change. -->

- `command` — result

## Compatibility and safety

<!-- Call out breaking changes, migration steps, platform effects, limitations,
and intentionally unchanged behavior. -->

- Breaking changes or migration steps:
- Storage and side-effect invariants:
- Generated output or release impact:
- Performance or resource-budget impact:

## Review checklist

- [ ] The PR has one focused outcome and the title follows `type(scope): outcome`.
- [ ] Related issue is linked, or the reason for not linking one is stated above.
- [ ] Tests cover changed observable behavior and meaningful failure paths.
- [ ] Documentation, ADRs, or generated contracts are updated where needed.
- [ ] Read-only operations do not gain implicit network or process side effects.
- [ ] I checked the final diff for secrets, unrelated cleanup, and unsupported claims.

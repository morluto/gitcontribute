# External tool audit — 2026-09-08

The standalone GitContribute MCP server received calls to all **73 advertised
tools** in an isolated, persistent audit workspace. Four confirmed defects were
fixed: invalid detailed index-job responses, incorrect source completeness,
false complete coverage in related-work checks, and missing validation-group
handoffs. Invocation coverage is not complete behavioral coverage or evidence
that no defects remain.

The [tool-by-tool coverage table](audits/2026-09-08/coverage.md) records 85 baseline
and eight candidate tool calls from the initial pass. That pass recorded 108
JSON-RPC exchanges. Temporary runtime artifacts and raw logs were removed after
the follow-up at the user's request; this report retains the findings and coverage
summary. See the [concurrency and ranking follow-up](concurrency-ranking-audit.md).

## Runtime and evidence identity

- Source base: `c1930cd3f5f8ee37ef993935ef716f22f24b1684`, with existing search and
  ownership changes already present. This is not a clean-HEAD comparison.
- Standalone runtime: version `dev`, Go 1.26.5, Linux amd64, Git 2.53.0;
  MCP protocol `2025-11-25`, stdio JSON-RPC, initialized before tool calls.
- Baseline catalog: 73 tools, fingerprint
  `747848aa0864e7bb3d188b2ae7975541cb40fdd057e4e9035521e5d4cb981dd5`.
- Candidate catalog: 73 tools, fingerprint
  `7f36215dd9a57942d7549589f218e79222ac77591a2ec08ee627f048164a355f`.
- The separately connected installed runtime reported version `1.0.0`, 62 tools,
  fingerprint `b8ae9dfde5bd81a139ab0095e5353320fa955c3739311c62c94b2cfaed8281ed`.
  Only its catalog contract was queried; source identity equivalence was not assumed.
- Candidate `--read-only` catalog advertised 33 tools. This filters read-only
  annotations and still permits explicit external reads; it is not offline mode.

Baseline and candidate labels identify separate executable sessions against the
same evolving audit corpus, not a controlled latency comparison. The starting
tracked diff did not capture pre-existing untracked files. Documentation does
not preserve a replayable runtime snapshot after artifact cleanup.

## Real workload

Public GitHub REST reads searched `spf13/cobra` for completion-related closed
pull requests, inspected PR #2467, acquired repository/thread/actor facets,
read commit-bound source, indexed code, and inspected feedback and CI coverage.
The main ref resolved to `adbc8813901bba65827259daa8e22ff94ec1f30e`;
PR #2467's head was `4a56f4211a97e7796c3f81022231d0c776510453`.
Live search returned three of 205 results with a next page and partial status.
Code acquisition indexed 65 text files out of 66 tracked entries, skipping one
nontext file without truncation. DeepWiki retrieval was explicitly external
and retained derived-external provenance.

Local reads exercised thread/code/actor/contribution/feedback search, facets,
coverage, ranking, clusters, neighbors, precedents, overlaps, and explanations.
Local workflows created an investigation, hypothesis, opportunity, concern,
draft, workspace, and adopted checkout; inspected Git changes, merge conflicts,
and semantic commit plans; and linked evidence and a pull request. Draft
preparation was not publication. No GitHub mutation occurred.

Validation executed only `git diff --check` in the task-owned checkout. Both
individual runs passed, but their group remained **inconclusive** because no
observation contract established a functional result. A separately recorded
external execution was attached as a digest-bound receipt with a JUnit summary;
a manifest import preserved its limited external provenance. These observations
do not prove a Cobra bug fixed. No repository-controlled code ran during crawl
or indexing.

Selected CLI smoke checks also succeeded: `metadata --json`, `status --json`,
`doctor --json`, and `search threads completion --repo spf13/cobra --limit 2 --json`.
This is not full CLI, installation, upgrade, or cross-platform coverage.

## Confirmed defects and fixes

| Defect | Reproduction and impact | Correction and validation |
| --- | --- | --- |
| Detailed index jobs fail output validation | A successful index job embedded a partially populated full artifact, including null provenance where the output schema required an object. One such item broke an entire detailed job batch. | Job summaries now own a narrow artifact reference; the exact resource owns the full manifest. A failing MCP-boundary regression became green. The candidate retrieved the same index job together with a validation job, then read the returned index resource successfully. |
| Source response loses completeness | Two successfully acquired files had accurate artifact metadata but zero requested/completed counts and an empty completeness status in the compact response. | Copy the persisted artifact's completeness into the response. Regression failed before the fix and passed after it. The same external request returned two requested/two complete/zero failed, and its exact source resource was readable. |
| Related-work coverage overclaims | Duplicate and competing-PR checks reported `coverage: complete` despite incomplete stored thread coverage. | Consult the repository's thread coverage; missing/incomplete coverage yields unknown coverage, partial status, and typed sync recovery. MCP regressions cover missing, incomplete, and complete cases. Both real checks now preserve unknown coverage. |
| Validation job has no usable group handoff | A completed validation job exposed a group ID without a resource or attempt identities, leaving the agent unable to follow that result directly. | Add a narrow validation-group resource and exact returned URI/follow-up. A failing boundary regression became green. External resource read exposed both attempt IDs and the inconclusive group classification without commands, environment, or host paths. |

Documentation also now describes `--read-only` accurately and states that
`make verify` runs full repository lint. Existing search/ownership work is
covered separately in [search audit](search-audit.md) and
[ownership audit](ownership-audit.md); it was excluded from this pass's independent
diff review.

## Coverage limits and correct negative behavior

Acquisition jobs were polled through `jobs.get`, rather than treating queued
acknowledgments as completion. Repository context, threads, exact thread facets,
profiles, social accounts, repository listings, exact PR feedback, coverage,
indexing, workspace creation, validation execution, and checks waiting reached
successful terminal execution. A successful job can still contain bounded or
incomplete facet coverage.

Organization, pinned-item, and contribution facets remained partial. Repository
list coverage was truncated. Repository-wide feedback discovery stopped at its
page cap; CI acquisition exhausted its three-request budget. PR portfolio
acquisition encountered a GraphQL deadline. These limitations were retained as
partial/unknown evidence. Authenticated identity returned an authentication
error; no authenticated success path was exercised.

Other deliberately or accidentally invalid inputs were rejected: incorrect
search/source/CI argument shapes, comparing a fork to itself, a manifest's
inconsistent head identity, and adopting an already-bound workspace path. The
calls were corrected where applicable; these input errors are not product bugs.
A stale durable snapshot returned `snapshot_expired`. In the initial pass cancellation was tested
only against a terminal job. The follow-up exercised cross-process running and
queued cancellation, graceful shutdown, and abrupt restart recovery. Published-draft verification returned unknown against an
unsynced target; a matching published draft was not tested.

Candidate search pages returned different thread identities, and reusing the
cursor with another query returned `invalid search cursor`. This confirms the
sampled pagination boundary, not relevance quality across repositories. Local
search retained unknown population coverage. No cold/warm benchmark, concurrency
stress test, controlled model evaluation, authenticated large-corpus workload,
or exhaustive resource-template matrix was performed.

## Remaining improvement opportunities

1. **Related-work signal quality (addressed in follow-up):** some candidates display `score 0.00: no
   strong signal`, yet competing-PR evidence uses a contradicting relation.
   Rounded display values do not prove the exact numeric score is zero. Build
   a judged query set and inspect score/relation calibration before changing
   thresholds. The follow-up removes exact zero-score findings, changes similarity evidence
   to inconclusive, and preserves candidate bounds. Wider relevance calibration
   remains unmeasured.
2. **Acquisition recovery:** repeat the partial actor/portfolio workload with
   authenticated access and bounded retries to separate permission, rate-limit,
   and timeout causes. Avoid increasing budgets without measuring useful yield.
3. **Result size and polling cost:** logs record response sizes indirectly and
   per-call timings. Measure large detailed job batches and facet payloads before
   changing response defaults; keep manifests in resources and summaries narrow.
   This audit establishes a correctness improvement, not a measured speedup.
4. **Evidence navigation:** evaluate whether manifest imports should expose
   evidence identities directly; current summaries may require another lookup.
5. **Lifecycle and trust boundaries:** the follow-up adds external running-cancel,
   graceful shutdown, and interrupted-process reconciliation workloads. Malformed
   resources, prolonged writer stalls, and larger contention workloads remain
   useful future cases.
6. **Deployment identity:** the connected 62-tool runtime differs from the
   tested 73-tool source build. Verify catalog fingerprints when deploying;
   this audit did not update the installed runtime.

## Research and validation

The protocol requires structured tool results to conform to their advertised
output schema. That makes the detailed job failure a contract defect even
though indexing itself succeeded. The artifact-reference fix keeps summary
fields truthful while providing the full resource separately.
[MCP tools specification](https://modelcontextprotocol.io/specification/2025-11-25/server/tools).
The probe follows the protocol initialization sequence.
[MCP lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle).

GitHub recommends authenticated access, bounded/queued requests, and respecting
retry/reset headers. Apply these to follow-up acquisition experiments; do not
interpret anonymous failures as absence of repository or actor data.
[GitHub REST best practices](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api),
[rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api),
[troubleshooting](https://docs.github.com/en/rest/using-the-rest-api/troubleshooting-the-rest-api).
MCP documentation was consulted through Context7; GitHub guidance came from
official documentation.

Focused regressions reproduced the four defects before fixes and passed after.
An independent review of this pass's exact production/test/documentation diff
reported no verified findings and passed application, MCP server, and contract
tests plus whitespace checks. It did not independently repeat the external
stdio workload. Final `make verify` passed formatting, vet, and every uncached short-test
package, then stopped on one test import-grouping lint finding. After that
formatting-only correction, `make fmt-check lint-full tidy-check generate-check
docs-check` passed with zero lint issues and current generated outputs. Thus
all verification gates passed; unchanged behavioral tests were not repeated.
`git diff --check` also passed.

# Scalable MCP workflows

GitContribute exposes bounded, vectorized primitives: each tool owns one
side-effect boundary but can process a collection. This keeps agents from
building slow N+1 loops while preserving explicit control over network access,
local writes, and process execution.

## Repository research

Use the cheapest authoritative source first, and hydrate only finalists:

```text
github.search_repositories -> corpus.get_repositories
github.sync_repository_context -> jobs.get -> corpus.get_repositories
research.query_deepwiki
github.sync_threads -> jobs.get -> corpus.rank_threads
github.hydrate_threads -> jobs.get -> corpus.get_threads
corpus.find_precedents -> workflow.find_competing_work
workflow.prepare_issue_set
```

- `github.search_repositories` runs one bounded live search and persists the
  returned repository metadata. Prefer structured filters so GitContribute can
  validate and explain the query; reserve `raw_query` for unsupported GitHub
  qualifiers. `response_format: concise` keeps broad discovery bounded, while
  `detailed` preserves secondary metadata for finalists. Live pagination uses
  `page` and `next_page` because GitHub search pages are not stable cursors.

```json
{
  "text": "inference",
  "match_fields": ["name", "description"],
  "topics": ["cuda"],
  "language": "Python",
  "stars_min": 200,
  "pushed_after": "2026-06-15",
  "archived": false,
  "fork": false,
  "response_format": "concise"
}
```

Search responses return the compiled provider `query`, a short interpretation,
request-specific warnings, semantic `repository:owner/name` references, and a
non-mandatory suggested thread-sync call. Advanced provider syntax uses the
explicit `raw_query` field; there is no deprecated alias.
- `github.sync_repository_context` fetches and persists metadata and fixed
  contribution-guidance files for explicit repository identities. Use
  it to recover a `repository_not_indexed` result, then poll the returned job
  before reading the repository again.
- `corpus.get_repositories` returns stored metadata plus `dossier_status` and
  `dossier_as_of` for up to 100 repositories. Use that batch to compare
  candidates and dossier availability; load a full persisted dossier only for
  a known finalist with `corpus.get_repository_dossier`.
- `corpus.get_repositories`, `corpus.get_threads`, `corpus.find_clusters`,
  `corpus.find_neighbors`, `corpus.rank_threads`, and
  `corpus.find_precedents` are offline.
- `corpus.find_clusters` and `corpus.find_neighbors` accept up to 20 repository
  or source-thread targets respectively. Their ordered item results isolate
  missing or invalid targets instead of forcing scalar retry loops.
- `corpus.rank_threads` requires one to 50 repositories. Its derived ranking is
  intentionally non-paginated; inspect `total` and `truncated`, then raise the
  limit or narrow the repository set when more candidates are needed. Per-repo
  summaries distinguish the evaluated population, returned candidates, and an
  internal population cap.
- `research.query_deepwiki` is an optional public external read. Its prose is
  untrusted derived context, is not persisted, and is not authority for live
  GitHub state.
- `github.sync_threads` stores issue or pull-request headers. Child comments
  and reviews require explicit `github.hydrate_threads` facets.
- `github.hydrate_threads` refreshes each selected thread header before fetching
  child facets, so exact finalists do not inherit stale header coverage. The
  repository metadata must already exist locally, and each item's `requests`
  count includes its one exact-header request. Successful items report
  `header_refreshed: true`.
- Pull-request headers do not contain merge outcomes. Until `pr_details` is
  hydrated, a closed PR's `merged` value is omitted and outcome-sensitive
  offline reads report it as unknown rather than closed-unmerged.

### Repository fix-pattern mining

The opt-in `patterns` profile exposes the trace-backed aggregate:

```text
workflow.mine_repository_fix_patterns
  -> jobs.get
  -> gitcontribute://fix-pattern-report/{job_id}
```

Use it to summarize how one stored repository handled caller-defined symptom
categories over an explicit observation window. It searches the local corpus
first, refreshes only a bounded set of finalists whose merge outcome is
unknown, and persists a typed report. `candidate_limit`, `hydration_limit`, and
`representative_limit` bound search, network work, and returned context
independently. Set `hydration_limit: 0` to request a strictly offline analysis;
otherwise the workflow performs GitHub reads and idempotent local writes.

Coverage reports candidate matches, unique pull requests, unknown outcomes
before and after hydration, hydration failures, and candidate truncation.
Merged, closed-unmerged, superseded, open, and unknown remain separate
outcomes. Only closed PRs with unknown merge state consume the hydration
budget. An example is marked `accepted_fix` only when refreshed state confirms
it was merged and stored pull-request text contains an explicit closing
relationship. A similar closed PR is never promoted to accepted-fix evidence.
Relationship and proof-style labels are bounded lexical projections, so the
report preserves their supporting phrase and states that similarity is not
causal proof.

## Exact issue-set preparation

Use `workflow.prepare_issue_set` when the contribution is already scoped by
known issue numbers and creating opportunities would add no useful state:

```json
{
  "owner": "acme",
  "repo": "rocket",
  "issue_numbers": [7, 11, 14],
  "precedent_limit": 3,
  "response_format": "concise"
}
```

The tool is an offline read. It composes exact issue facts, body and
comment/timeline coverage, related open and closed work, merge-confirmed
precedents, duplicate-cluster evidence, and precise sync or hydration recovery
calls. It does not render a draft, create an opportunity, inspect a workspace
diff, or claim that an implementation satisfies an issue. Linkage therefore
defaults to `related` and always requires caller confirmation before choosing
`Closes`, `Advances`, or `Related`.

Repository `threads` coverage qualifies the related-work population. When that
coverage is absent or incomplete, the result remains partial and suggests an
explicit all-state pull-request sync instead of treating the stored count as
exhaustive.

`concise` omits issue bodies and detailed relationship evidence and returns at
most five related-work records per issue. `related_work_total` distinguishes
that response shortening from missing corpus evidence. When an upstream bound
prevents an exhaustive count, `related_work_total_known` is false and the count
is a lower bound. `related_work_truncated` says explicitly that records or
evidence were omitted. Empty stored bodies are reported as unknown because the
current corpus projection cannot distinguish a known-empty body from a body
that was not captured.

## Pull-request portfolio

```text
github.get_authenticated_identity
-> github.sync_authored_pull_requests -> jobs.get
-> github.sync_pull_request_status -> jobs.get
-> corpus.list_pull_request_portfolio
-> corpus.find_portfolio_overlaps
```

The status adapter stores REST pull-request details and reviews plus typed,
independently covered GraphQL snapshots for checks, unresolved review threads,
detailed merge state, merge queue, closing issues, and changed files. The
offline portfolio derives deterministic attention states only from complete
facets. A null or still-computing mergeability value remains unknown.

`corpus.find_portfolio_overlaps` compares up to 50 stored candidates with
authored pull requests using complete normalized changed-path, linked-issue,
and stored opportunity-similarity evidence. It returns `unknown` unless every
required facet is complete; it never performs network access. Use
`workflow.link_pull_request` to record an explicit local PR association with an
opportunity or workspace. That local write does not mutate GitHub.

Issue timeline hydration is an explicit, opt-in `issue_timeline` facet. Complete
timeline observations may create versioned resolution records with exact source
observation references. Closing-issue observations remain relationship evidence
until completion is independently observed. Similar prose is not resolution
evidence.

`workspace.check_merge_conflicts` is different from GitHub mergeability. It runs
a non-mutating Git comparison between already-fetched object IDs in a managed
workspace. It never fetches refs or modifies an index or worktree.

## Partial results and recovery

Batch outputs preserve input order. Each item has one of these statuses:

- `complete`: use the value;
- `retryable`: retry that item after `retry_after_ms` when present;
- `unavailable`: follow `next_action` or acquire the missing facet explicitly;
- `failed`: fix the input or local failure before retrying.

A durable job can succeed while its result is `partial`: job success means the
bounded operation completed and recorded every item outcome. Poll concurrent
jobs together with vectorized `jobs.get`, then retry only retryable items. Never
interpret absent coverage as a zero, a passing check, or a lack of competing
work. New job references carry a semantic `job:<id>` reference,
`poll_after_ms`, and a machine-readable suggested `jobs.get` call.

Repository and dossier absence have different recovery paths:

- `repository_not_indexed` means no local repository projection exists. Call
  `github.sync_repository_context`, poll the job with `jobs.get`, and then
  retry the offline read.
- `dossier_not_persisted` means the repository exists locally but has no saved
  dossier. Use `corpus.get_repositories` for metadata and dossier availability;
  call `workflow.build_repository_dossier` only when creating that local artifact
  is actually required.

Retrying `corpus.get_repository_dossier` alone cannot resolve either state.

`corpus.get_coverage` accepts up to 100 ordered repository or exact-thread
targets. `jobs.cancel` accepts up to 100 IDs and returns isolated item outcomes;
repeating cancellation is safe. `jobs.get` exposes structured phase and item
counts rather than requiring clients to parse event prose.

The MCP catalog does not advertise scalar compatibility aliases. Use one-item
arrays with `corpus.get_repositories`, `corpus.get_threads`,
`github.sync_threads`, `github.hydrate_threads`, and `jobs.get` when only one
target is needed. Configured recurring-source crawls remain a CLI/TUI workflow,
not an MCP discovery primitive.

## Side-effect boundaries

| Tool family | Network | Corpus/local write | Process |
| --- | ---: | ---: | ---: |
| `corpus.get_*`, rank, precedents, portfolio | no | no | no |
| `workflow.link_pull_request` | no | yes | no |
| `github.search_*`, sync, hydrate | yes | yes | no |
| `research.query_deepwiki` | yes | no | no |
| `code.index_repositories` | remote-dependent | yes | Git only |
| `workspace.check_merge_conflicts` | no | no | Git only |

No tool in these workflows mutates GitHub or executes repository-controlled
code.

## End-to-end verification

Run the real stdio protocol tests with:

```sh
go test ./internal/app -run '^TestMCPStdio(ScalableResearch|PullRequestPortfolio)Flow$' -count=1
```

The tests launch the application as an MCP subprocess, use a real file-backed
SQLite corpus, and route the real GitHub HTTP adapter to a controlled test
server. They cover initialization and tool discovery, repository-context synchronization,
offline batch reads, ranking, precedents, authored-PR discovery, status
hydration, portfolio classification, vectorized durable-job polling, and a
protocol-visible invalid hydration request. They do not contact live GitHub or
DeepWiki and do not run repository code.

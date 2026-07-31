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
github.sync_threads -> jobs.get -> corpus.rank_contribution_candidates
github.sync_thread_facets -> jobs.get -> corpus.get_thread_facets
corpus.find_precedents
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
  a known finalist through `gitcontribute://dossier/{owner}/{repo}`.
- `corpus.get_repositories`, `corpus.get_threads`, `corpus.find_clusters`,
  `corpus.find_neighbors`, `corpus.rank_contribution_candidates`, and
  `corpus.find_precedents` are offline.
- `corpus.find_clusters` and `corpus.find_neighbors` accept up to 20 repository
  or source-thread targets respectively. Their ordered item results isolate
  missing or invalid targets instead of forcing scalar retry loops.
- Search, coverage, precedent, code-search, fix-pattern, and research-brief
  results carry a query digest, observation watermark, completeness,
  truncation, and unknown-coverage status. Read-only operations return an
  `ephemeral:` transaction-bound identity and state that it is not reusable.
  Call `corpus.ensure_coverage` when a composed workflow needs a durable
  `snapshot_token`; reading that token either returns its immutable payload or
  fails with a typed unavailable result. Reads never refresh the corpus or
  silently substitute current projections.
- `corpus.rank_contribution_candidates` requires one to 50 repositories. Its derived ranking is
  intentionally non-paginated; inspect `total` and `truncated`, then raise the
  limit or narrow the repository set when more candidates are needed. Per-repo
  summaries distinguish the evaluated population, returned candidates, and an
  internal population cap.
- `research.query_deepwiki` is an optional public external read. Its prose is
  untrusted derived context, is not persisted, and is not authority for live
  GitHub state.
- `github.sync_threads` stores issue or pull-request headers. Child comments
  and reviews require explicit `github.sync_thread_facets` facets.
- `github.sync_thread_facets` refreshes each selected thread header before fetching
  child facets, so exact finalists do not inherit stale header coverage. The
  repository metadata must already exist locally, and each item's `requests`
  count includes its one exact-header request. Successful items report
  `header_refreshed: true`.
- Pull-request headers do not contain merge outcomes. Until `pr_details` is
  hydrated, a closed PR's `merged` value is omitted and outcome-sensitive
  offline reads report it as unknown rather than closed-unmerged.

### Repository fix-pattern mining

The unified catalog exposes the trace-backed aggregate:

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
independently. The durable operation always creates a job and persists its
report. Use `corpus.preview_fix_patterns` when the analysis must be strictly
offline and must create no job, artifact, hydration, or local write.

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

For an analysis that must not create a job, artifact, hydration, or write, use
`corpus.preview_fix_patterns`. It returns `persisted: false`, zero hydration,
and the captured snapshot identity. The durable operation remains the path for
persisted reports.

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
github.sync_pull_request_portfolio(selection=authored) -> jobs.get
-> corpus.list_pull_requests
-> corpus.find_pull_request_overlaps
```

The status adapter stores REST pull-request details and reviews plus typed,
independently covered GraphQL snapshots for checks, unresolved review threads,
detailed merge state, merge queue, closing issues, and changed files. The
offline portfolio derives deterministic attention states only from complete
facets. A null or still-computing mergeability value remains unknown.

`corpus.find_pull_request_overlaps` compares up to 50 stored candidates with
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
- `unavailable`: follow the typed `recovery` plan or acquire the missing facet explicitly;
- `failed`: fix the input or local failure before retrying.

A durable job can succeed while its result is `partial`: job success means the
bounded operation completed and recorded every item outcome. Poll concurrent
jobs together with vectorized `jobs.get`, then retry only retryable items. Never
interpret absent coverage as a zero, a passing check, or a lack of competing
work. New job references carry a semantic `job:<id>` reference,
`poll_after_ms`, and a typed `jobs.get` follow-up with its job ID.

Facet synchronization completes on the same offline read plane: use
`corpus.get_thread_facets` for bounded coverage metadata and follow each
returned `resource_uri` through MCP `resources/read` for the persisted facet
observations. A missing repository, thread, or facet returns a versioned
`recovery` plan whose `then` calls are ordered and carry typed arguments.

Repository and dossier absence have different recovery paths:

- `repository_not_indexed` means no local repository projection exists. Call
  `github.sync_repository_context`, poll the job with `jobs.get`, and then
  retry the offline read.
- `dossier_not_persisted` means the repository exists locally but has no saved
  dossier. Use `corpus.get_repositories` for metadata and dossier availability;
  call `workflow.build_repository_dossier` only when creating that local artifact
  is actually required.

Reading the dossier resource again cannot resolve either state.

## Canonical source audit

Use this order when producing a source-backed audit or contribution handoff:

```text
corpus.get_coverage -> typed exact/repository recovery -> jobs.get
         -> snapshot-bound offline reread -> duplicate checks
         -> explicit live verification -> jobs.get -> receipt attachment
         -> evidence/draft handoff
```

Start with `corpus.get_coverage` and treat missing or incomplete coverage as
unknown. Follow the item-level typed recovery action it returns: use
`corpus.ensure_coverage` for repository bootstrap or broad target recovery,
`github.sync_threads` for exact or repository header recovery, and
`github.sync_thread_facets` for selected child facets. Poll the returned job
with `jobs.get`, then perform the offline reread. Use the reread's returned
`snapshot_token` for duplicate checks over that same state.
Perform live verification after local evidence selection, attach a producer-neutral
validation receipt, and hand the exact resource and any returned revision
references to the evidence or draft workflow. Resources that do not expose a
revision are point-in-time reads and should be reread after an explicit sync.
Larger persisted payloads are always read with MCP `resources/read` using the
exact opaque URI returned by the tool.

Completed code-index jobs return a typed artifact containing repository, commit
SHA, snapshot token, manifest identity and digest, file/truncation counts, and
an exact `gitcontribute://artifact/code-index/<artifact-digest>` resource. Consume that URI through
`resources/read`; do not infer an artifact identity from a repository name
alone.

`corpus.get_coverage` accepts up to 100 ordered repository or exact-thread
targets. `jobs.cancel` accepts up to 100 IDs and returns isolated item outcomes;
repeating cancellation is safe. `jobs.get` exposes structured phase and item
counts rather than requiring clients to parse event prose.

The MCP catalog does not advertise scalar compatibility aliases or duplicate
durable-artifact getters. Read dossiers, investigations, opportunities,
evidence, readiness reports, workflows, and lenses through their
`gitcontribute://` resources. Use one-item arrays with
`corpus.get_repositories`, `corpus.get_threads`,
`github.sync_threads`, `github.sync_thread_facets`, and `jobs.get` when only one
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

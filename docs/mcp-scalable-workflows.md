# Scalable MCP workflows

GitContribute exposes bounded, vectorized primitives: each tool owns one
side-effect boundary but can process a collection. This keeps agents from
building slow N+1 loops while preserving explicit control over network access,
local writes, and process execution.

## Repository research

Use the cheapest authoritative source first, and hydrate only finalists:

```text
github.search_repositories -> corpus.get_repositories
github.sync_repository_metadata -> jobs.get -> corpus.get_repositories
research.query_deepwiki
github.sync_threads -> jobs.get -> corpus.rank_threads
github.hydrate_threads -> jobs.get -> corpus.get_threads
corpus.find_precedents -> workflow.find_competing_work
```

- `github.search_repositories` runs one bounded live search and persists the
  returned repository metadata. It does not rank contribution candidates.
- `github.sync_repository_metadata` refreshes facts for known repositories only.
- `corpus.get_repositories`, `corpus.get_threads`,
  `corpus.rank_threads`, and `corpus.find_precedents` are offline.
- `research.query_deepwiki` is an optional public external read. Its prose is
  untrusted derived context, is not persisted, and is not authority for live
  GitHub state.
- `github.sync_threads` stores issue or pull-request headers. Child comments
  and reviews require explicit `github.hydrate_threads` facets.

## Pull-request portfolio

```text
github.get_authenticated_identity
-> github.sync_authored_pull_requests -> jobs.get
-> github.sync_pull_request_status -> jobs.get
-> corpus.list_pull_request_portfolio
```

The current status adapter stores REST pull-request details and reviews. The
portfolio can classify merged, closed-unmerged, conflicted, changes-requested,
approved, stale, awaiting-review, and unknown states from those facts.

The following facts are not currently fetched and remain explicit in coverage
and reason fields:

- check rollups and failing checks;
- unresolved review conversations;
- detailed merge state and merge queue position;
- closing issue links and cross-portfolio overlap.

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
work.

The MCP catalog does not advertise scalar compatibility aliases. Use one-item
arrays with `corpus.get_repositories`, `corpus.get_threads`,
`github.sync_threads`, `github.hydrate_threads`, and `jobs.get` when only one
target is needed. Configured recurring-source crawls remain a CLI/TUI workflow,
not an MCP discovery primitive.

## Side-effect boundaries

| Tool family | Network | Corpus/local write | Process |
| --- | ---: | ---: | ---: |
| `corpus.get_*`, rank, precedents, portfolio | no | no | no |
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
server. They cover initialization and tool discovery, metadata synchronization,
offline batch reads, ranking, precedents, authored-PR discovery, status
hydration, portfolio classification, vectorized durable-job polling, and a
protocol-visible invalid hydration request. They do not contact live GitHub or
DeepWiki and do not run repository code.

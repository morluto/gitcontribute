# GitContribute

GitContribute is a local-first GitHub contribution research workbench. It keeps a
durable SQLite corpus of repositories, issues, pull requests, reviews, code
snapshots, and your own research artifacts so you and your agents can discover,
investigate, validate, and prepare focused open-source contributions.

Network access is explicit and read-only: the CLI and MCP surface fetch data only
when you ask them to. Search and inspection commands read from the local corpus.

See [SPEC.md](SPEC.md) for the product contract and delivery plan.

## Install

GitContribute requires Go 1.26 or newer. A C toolchain is not required because
the SQLite driver is pure Go.

```sh
go install github.com/morluto/gitcontribute/cmd/gitcontribute@latest
```

Or build a checkout:

```sh
go build -o gitcontribute ./cmd/gitcontribute
```

You also need a `git` executable. Optional helpers that the CLI can use:

- `gh` for the `gh auth token` authentication source.
- `ripgrep` (`rg`) for an extra `doctor` check.

## Quick start

```sh
gitcontribute init
gitcontribute configure --token-source=env --token-source-key=GITHUB_TOKEN
gitcontribute sync owner/repo
gitcontribute search threads "connection timeout" --repo owner/repo --json
gitcontribute dossier build owner/repo --json
```

Run `gitcontribute --help` and `gitcontribute <command> --help` for full flag
reference.

## Initialization, configuration, and authentication

`gitcontribute init` creates the default corpus database and directories if they
do not exist. It does not fetch anything from GitHub.

`gitcontribute configure` edits `config.toml` atomically. Useful options include:

```sh
gitcontribute configure --database /path/to/corpus.db
gitcontribute configure --token-source env --token-source-key GITHUB_TOKEN
gitcontribute configure --token-source gh-cli
gitcontribute configure --output-format json
```

Supported `token-source` methods:

- `none` — no token (public reads, lower rate limits).
- `env` — read from the environment variable named by `token-source-key`
  (defaults to `GITHUB_TOKEN`).
- `gh-cli` — run `gh auth token`.
- `keyring` — read from the OS credential store; `token-source-key` is the
  keyring account name.

Tokens are resolved at runtime and are never written into the corpus or logs.
Runtime environment variables override config without persisting it:
`GITCONTRIBUTE_DATABASE`, `GITCONTRIBUTE_TOKEN_SOURCE_METHOD`,
`GITCONTRIBUTE_TOKEN_SOURCE_KEY`, `GITCONTRIBUTE_CRAWL_BUDGET`,
`GITCONTRIBUTE_CRAWL_CONCURRENCY`, `GITCONTRIBUTE_CRAWL_RETRY_LIMIT`,
`GITCONTRIBUTE_CRAWL_TIMEOUT`, `GITCONTRIBUTE_OUTPUT_FORMAT`, and
`GITCONTRIBUTE_OUTPUT_MAX_RESULTS`.

Use `gitcontribute status`, `gitcontribute metadata`, and `gitcontribute doctor`
to inspect the corpus and local environment. Status includes the latest observed
GitHub rate-limit state; `doctor` redacts credential details in its output.

## Discovery sources

Sources determine what you track. You can add three kinds of discovery sources
and then `crawl` or `tail` them.

- Explicit repos:
  ```sh
  gitcontribute source add repos --name my-go "golang/go" "cli/cli" --json
  ```
  A source can also be read from a file or stdin (`--file -`).

- GitHub repository search:
  ```sh
  gitcontribute source add search --name go-network \
      --query "language:go stars:>100" --json
  ```

- GH Archive:
  ```sh
  gitcontribute source add gharchive --name golang-events \
      --events "IssuesEvent,PullRequestEvent" --json
  ```
  `gharchive` supports `--events all` to allow all known event types. Each crawl
  or tail run is idempotent and skips hours that have already been imported.

Run a source once or continuously:

```sh
gitcontribute crawl golang-events --since 720h --budget 500 --json
gitcontribute tail golang-events --since 2h --budget 500 --interval 1h
```

`crawl` and `tail` respect the configured request budget. `tail` loops until you
interrupt it; use `--once` for a single iteration.

## Repository archive sync and hydration

### Sync

```sh
gitcontribute sync owner/repo                  # fetch all issues and PRs
gitcontribute archive sync owner/repo --since 168h --state open
gitcontribute archive sync owner/repo --numbers 42,108
gitcontribute archive refresh owner/repo        # full all-state archive refresh
```

`sync` fetches repository metadata and lists issues and pull requests using the
GitHub API. `archive sync` offers bounded incremental refreshes; `archive
refresh` performs a full all-state list refresh. Synced data is written as
immutable observations behind a durable SQLite corpus. Facet coverage is recorded
so later commands can tell how fresh the data is.

### Hydrate

```sh
gitcontribute archive hydrate owner/repo#42 --with issue_comments
gitcontribute archive hydrate owner/repo#108 --with pr_reviews,pr_review_comments
```

Available facets:

- `issue_comments`
- `pr_details`
- `pr_reviews`
- `pr_review_comments`

Hydration is explicit, paginated, and cancellation-aware. It stores each facet
as an immutable observation.

### Archive inspection

```sh
gitcontribute archive threads owner/repo --state open --limit 50
gitcontribute archive coverage owner/repo
gitcontribute coverage owner/repo
```

## Code indexing

`gitcontribute index` indexes a clean local checkout at its current commit:

```sh
gitcontribute index owner/repo /path/to/checkout --json
```

`gitcontribute acquire` clones or fetches a repository into a managed bare
mirror, creates a clean worktree at the default branch, indexes the tracked text
files, then removes the worktree:

```sh
gitcontribute acquire owner/repo --remote https://github.com/owner/repo.git --json
```

The indexer reads blobs directly from Git, skips binaries and non-UTF-8 content,
enforces size limits, and rejects dirty worktrees. Indexed code snapshots are
stored in the corpus and are searched with SQLite FTS5.

## Typed offline search

```sh
gitcontribute search repos "cli" --limit 20 --json
gitcontribute search issues "data race" --repo owner/repo --state open --json
gitcontribute search prs "flaky" --repo owner/repo --label "bug" --json
gitcontribute search threads "memory leak" --repo owner/repo
gitcontribute search code "context.WithTimeout" --repo owner/repo
gitcontribute search all "retry" --repo owner/repo
gitcontribute search issues "retry" --lens my-lens
```

Search is local-only SQLite FTS5 keyword search over the corpus. Thread searches
support `--repo`, `--state`, `--author`, `--association`, `--assignee`,
`--label`, `--updated-after`, `--limit`, and `--cursor`. Repository and code
searches ignore the thread metadata filters. `search all` combines threads,
repositories, and code but does not support cursor pagination.

Results include a transparent score with reasons: title/body term matches,
freshness, and coverage. Use `gitcontribute search ... --json` to consume results
programmatically.

Add `--lens NAME` to apply a saved filter and weighted ranking to a bounded
candidate population. Lens-ranked searches do not support `--cursor` because
normalization depends on the complete candidate population used for that run.

## Dossiers, health, and seeds

Build a repository dossier from the local corpus:

```sh
gitcontribute dossier build owner/repo
gitcontribute dossier show owner/repo
gitcontribute dossier export owner/repo --format markdown --output owner-repo-dossier.md
```

Compute offline health and community metrics:

```sh
gitcontribute health owner/repo --stale-after 336h --json
```

`health` returns repository, issue, PR, external contributor, congestion,
stale, response-time, and coverage metrics without network access.

Extract contribution seeds from merged PRs, closed-unmerged PRs, and issues:

```sh
gitcontribute seeds owner/repo --json
```

Seeds are derived from stored thread and PR metadata; they include signals such as
conventional-commit prefixes, issue linkages, validation keywords, approximate
scope, and problem labels.

## Investigations, opportunities, validation, and evidence

### Workflow

1. Start an investigation scoped to a repository:
   ```sh
   gitcontribute investigation start owner/repo --lens my-lens --json
   ```
2. Record a hypothesis:
   ```sh
   gitcontribute hypothesis add --title="Fix retry timeout" \
       --description="..." --category=bug <investigation-id>
   ```
3. Check for duplicates and collisions:
   ```sh
   gitcontribute duplicates check <hypothesis-id>
   gitcontribute collisions check <hypothesis-id>
   ```
4. Promote the hypothesis to an opportunity:
   ```sh
   gitcontribute opportunity promote --problem="..." --scope="small" \
       --impact="reduces flakes" --effort="1h" --confidence=0.8 <hypothesis-id>
   ```
5. Create a workspace (managed Git worktree):
   ```sh
   gitcontribute workspace create <investigation-id>
   ```
6. Record evidence:
   ```sh
   gitcontribute evidence add --type=note --relation=supporting \
       --description="..." --opportunity <opportunity-id>
   ```
7. Define and run a validation:
   ```sh
   gitcontribute validation define --kind=test --command="go test ./..." \
       --working-dir=/path/to/ws <investigation-id>
   gitcontribute validation run <validation-id> --kind=base --execute
   gitcontribute validation run <validation-id> --kind=candidate --execute
   gitcontribute validation compare <base-run-id> <candidate-run-id>
   ```
8. Prepare contribution drafts:
   ```sh
   gitcontribute prepare issue <opportunity-id>
   gitcontribute prepare pr --approach="..." --workspace <workspace-id> <opportunity-id>
   gitcontribute prepare review <opportunity-id>
   ```

Statuses are stored with a required rationale. Hypothesis statuses:
`proposed`, `promoted`, `rejected`, `deferred`, `superseded`. Opportunity
statuses include `hypothesis`, `reproduced`, `validated`, `maintainer_aligned`,
`implemented`, `submitted`, `merged`, `rejected`, `deferred`, `superseded`.
Collision statuses: `unknown`, `none`, `possible`, `confirmed`, `blocked`.

### Validation safety

`validation run` requires `--execute`. Without it, the command prints the parsed
command and working directory but does not run anything. Validation commands run
on the host in the directory you specify; only environment variables listed in
`--env` are passed through.

## Workspaces

A workspace clones the upstream repository into a managed bare mirror and adds a
transient worktree at the candidate commit. You can inspect it with:

```sh
gitcontribute workspace show <workspace-id>
gitcontribute diff <workspace-id>
```

`diff` returns the patch, changed files, and a suggested review order. `prepare
pr` can include a workspace diff automatically; untracked files must be staged or
replaced by an explicit `--changes` summary.

## Lenses and collections

Lenses are JSON-defined filters and weight sets stored in the corpus:

```sh
gitcontribute lens add my-lens --file lens.json
gitcontribute lens list
gitcontribute lens show my-lens
gitcontribute lens explain my-lens issue:owner/repo#42 --query "retry"
```

`lens explain` reports the candidate facts, population scope, normalized
signals, weighted contributions, final score, and missing signals. Pass the
same query used for search when explaining a text-relevance score.

Collections are named groups of typed references:

```sh
gitcontribute collection create interesting
gitcontribute collection add interesting repo:owner/repo issue:owner/repo#42
gitcontribute collection list
```

## Tracking

Record triage decisions and contribution outcomes locally:

```sh
gitcontribute triage record issue:owner/repo#42 viewed --reason "..."
gitcontribute triage list --outcome viewed
gitcontribute contribution record <opportunity-id> issue "Draft title" --body "..."
gitcontribute contribution outcome <contribution-id> submitted
```

Export and re-import local tracking metadata:

```sh
gitcontribute tracking export --output tracking.json
gitcontribute tracking import --file tracking.json
```

The export is redacted: it never includes tokens, credentials, or absolute local
paths.

## Jobs and runs

Durable operation runs are recorded for sync, crawl, hydrate, and other
network-touching work:

```sh
gitcontribute runs --limit 20
```

Background durable jobs (mostly used from the MCP surface) can be inspected and
cancelled:

```sh
gitcontribute jobs
gitcontribute job show <id>
gitcontribute job cancel <id>
```

## TUI

Browse the corpus interactively:

```sh
gitcontribute tui
gitcontribute tui owner/repo
```

The TUI loads the local corpus only. Add `--json` to emit a snapshot of
repositories, threads, clusters, investigations, and opportunities to stdout
instead of starting the interactive terminal.

## MCP server

Start the MCP server over stdio:

```sh
gitcontribute mcp serve --transport=stdio
```

`stdio` is the only supported transport in this release.

The server exposes:

- Read-only tools: `search`, `search_repositories`, `search_threads`,
  `search_code`, `get_repository`, `get_thread`, `get_dossier`,
  `get_repository_dossier`, `get_investigation`, `list_opportunities`,
  `get_opportunity`, `get_evidence`, `find_clusters`, `find_neighbors`,
  `get_coverage`, `get_lens`, `get_job`, and `explain_match`.
- Network-read tools: `sync_repository`, `hydrate_thread`, `start_crawl`,
  `hydrate_repository`, `create_workspace`.
- Local-write tools: `start_investigation`, `record_hypothesis`,
  `promote_opportunity`, `define_validation`, `prepare_contribution`,
  `cancel_job`.
- Execution tool: `run_validation` requires `execute: true`.

Resources are published under `gitcontribute://` and `github-index://` URI
schemes, including repository, thread, dossier, investigation, opportunity,
evidence, lens, and job resources. All return JSON.

## JSON, cursor, and output behavior

Most non-interactive commands support `--json`. JSON is written to stdout;
progress and status messages go to stderr. Search commands return an opaque
`next_cursor` for pagination, except `search all`. List commands accept
`--limit` and return a `total` count when the corpus can determine it.

`dossier export`, `export dossier`, `export evidence`, and `tracking export`
support `--output <file>` to write to a file instead of stdout.

## Storage and paths

GitContribute uses platform-native directories.

### Linux / other Unix

- Config: `$XDG_CONFIG_HOME/gitcontribute` or `~/.config/gitcontribute`
- Data (including the default corpus): `$XDG_DATA_HOME/gitcontribute` or
  `~/.local/share/gitcontribute`
- Cache (including code acquisition mirrors): `$XDG_CACHE_HOME/gitcontribute` or
  `~/.cache/gitcontribute`
- Logs/state: `$XDG_STATE_HOME/gitcontribute` or
  `~/.local/state/gitcontribute/logs`

### macOS

- Config/Data: `~/Library/Application Support/gitcontribute` (Data subfolder
  `Data`)
- Cache: `~/Library/Caches/gitcontribute`
- Logs: `~/Library/Logs/gitcontribute`

### Windows

- Config: `%APPDATA%\gitcontribute`
- Data: `%LOCALAPPDATA%\gitcontribute\Data`
- Cache: `%LOCALAPPDATA%\gitcontribute\Cache`
- Logs: `%LOCALAPPDATA%\gitcontribute\Logs`

The default corpus database is `gitcontribute.db` inside the data directory. The
config file is `config.toml` inside the config directory.

## End-to-end example

```sh
# 1. Initialize the corpus and authenticate
gitcontribute init
gitcontribute configure --token-source env --token-source-key GITHUB_TOKEN

# 2. Sync a small repository
gitcontribute sync golang/go

# 3. Search for open issues about a topic
gitcontribute search issues "data race" --repo golang/go --state open --json

# 4. Hydrate a specific issue
gitcontribute archive hydrate golang/go#12345 --with issue_comments --json

# 5. Build a dossier and run health metrics
gitcontribute dossier build golang/go
gitcontribute health golang/go --json

# 6. Extract seeds
gitcontribute seeds golang/go --json

# 7. Start an investigation
gitcontribute investigation start golang/go --json

# 8. Add a hypothesis
gitcontribute hypothesis add --title "Fix data race in net/http" \
    --description "Investigate the reported race." \
    --category bug <investigation-id>

# 9. Check for duplicates and collisions
gitcontribute duplicates check <hypothesis-id>
gitcontribute collisions check <hypothesis-id>

# 10. Promote to opportunity
gitcontribute opportunity promote --problem "data race in net/http" \
    --scope small --impact "improves reliability" --effort 2h \
    --confidence 0.7 <hypothesis-id>

# 11. Create a workspace and inspect it
gitcontribute workspace create <investigation-id>
gitcontribute diff <workspace-id>

# 12. Record supporting evidence
gitcontribute evidence add --type note --relation supporting \
    --description "Reproduced with -race on go1.26" \
    --opportunity <opportunity-id>

# 13. Prepare a draft PR (uses the workspace diff)
gitcontribute prepare pr --approach "Add mutex around shared counter" \
    --workspace <workspace-id> <opportunity-id>
```

Replace `golang/go` with a smaller repository for faster initial sync.

## Safety boundaries and non-goals

- **No GitHub writes.** GitContribute does not open issues, create pull requests,
  push commits, or otherwise mutate GitHub. `prepare` and `validation` run on
  your local machine and only emit drafts and local reports.
- **No remote service.** There is no hosted backend or telemetry. Everything
  runs on your machine and stores data in your local corpus.
- **No semantic search.** Search is keyword-based SQLite FTS5 over the local
  corpus. Ranking uses transparent signals such as title/body term matches,
  freshness, and coverage, not vector embeddings.
- **No container isolation.** Validation commands and code indexing run on the
  host with the privileges of the user. You choose the working directory and
  the commands; no repository-controlled code is executed automatically during
  crawling or indexing.
- **Explicit capabilities.** Network reads, local writes, code execution, and
  GitHub mutations are deliberately separated. `validation run` needs `--execute`;
  network sync is a distinct command from offline search.

## Development

```sh
go test ./...
go build -o gitcontribute ./cmd/gitcontribute
./gitcontribute --help
```

## Platform support

Linux and macOS are the primary development and test targets. Windows builds
are expected to work where Go, Git for Windows, and SQLite support are available,
and paths resolve to the standard Windows `%APPDATA%` and `%LOCALAPPDATA%`
locations. If you hit a Windows-specific issue, please report it with the
output of `gitcontribute doctor --json`.

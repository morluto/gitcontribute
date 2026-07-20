<div align="center">

# GitContribute

### Find better open-source contributions—with evidence, not guesswork.

GitContribute is a local-first research workbench for discovering, investigating,
validating, and preparing focused GitHub contributions.

[![CI](https://github.com/morluto/gitcontribute/actions/workflows/ci.yml/badge.svg)](https://github.com/morluto/gitcontribute/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/gitcontribute?logo=npm&color=CB3837)](https://www.npmjs.com/package/gitcontribute)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/github/license/morluto/gitcontribute)](LICENSE)
[![Platforms](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-5C6AC4)](#-platform-support)

[Quick start](#-quick-start) · [How it works](#-how-it-works) · [CLI guide](#-cli-guide) · [MCP](#-use-with-ai-agents) · [Safety](#-safety-by-design) · [Contributing](CONTRIBUTING.md)

</div>

---

> [!IMPORTANT]
> GitContribute never writes to GitHub. It syncs public or authenticated read-only
> data, keeps research on your machine, and prepares local drafts for you to review.

## Why GitContribute?

Finding an issue is easy. Finding a contribution that is useful, unclaimed,
appropriately scoped, and backed by evidence is the hard part.

GitContribute gives developers and coding agents a durable SQLite corpus of
repositories, issues, pull requests, reviews, code snapshots, and research
artifacts. Network access is always explicit. Once data is synced, search,
inspection, health analysis, dossiers, and investigations run entirely offline.

| | Capability | What it gives you |
| :---: | --- | --- |
| 🔎 | **Typed offline search** | Search repositories, issues, PRs, threads, and indexed code with transparent ranking. |
| 📡 | **Contribution Radar** | Rank promising open issues with explicit positives, risks, blockers, confidence, and missing-evidence warnings. |
| 🗂️ | **Durable research corpus** | Keep observations, coverage, investigations, evidence, and outcomes in local SQLite. |
| 🧭 | **Contribution workflow** | Move from hypothesis to opportunity, workspace, validation, readiness checks, and a prepared issue, PR, or review. |
| 🤖 | **Agent-ready MCP server** | Give Codex or Claude Code structured tools and resources with explicit capability boundaries. |
| 🛡️ | **Safe by default** | Separate network reads, local writes, process execution, and GitHub mutation. |

## ⚡ Quick start

Run the guided setup with Node.js 18 or newer:

```sh
npx gitcontribute setup
```

The wizard shows the resolved GitContribute version before any choice.

The interactive wizard offers three access modes:

- **MCP** — installs a private native runtime and configures selected coding agents;
- **CLI** — installs the persistent `gitcontribute` command and TUI;
- **Both** — installs the CLI and uses that verified executable for MCP.

The wizard displays detected coding agents without treating detection as
permission to modify them. Existing GitContribute registrations may be
preselected; new targets require an explicit selection. GitHub authentication
is chosen from described options rather than typed internal identifiers. A
dry-run plan shows every effect before final confirmation, which defaults to
apply.

`npx` only bootstraps the wizard. MCP configuration never records `npx`, an npm
cache path, or `@latest`: it points to an absolute native executable managed by
GitContribute.

If you chose MCP, restart the selected coding agent and use GitContribute
there. If you chose CLI or Both, launch the TUI, sync a repository, rank
contribution candidates, and search the local corpus:

```sh
gitcontribute tui
gitcontribute sync owner/repo
gitcontribute radar owner/repo --limit 10
gitcontribute search threads "connection timeout" \
  --repo owner/repo --json
gitcontribute dossier build owner/repo --json
```

`setup` initializes the corpus, helps select a GitHub authentication source,
can install the CLI or a private MCP runtime, and can register the MCP server
with Codex and Claude Code. Adding a repository during setup does **not**
contact GitHub or begin a sync.

For non-interactive setup, select the access mode explicitly:

```sh
# Both: CLI and Codex MCP
npx gitcontribute setup --mode both --codex --token-source none --yes

# CLI only
npx gitcontribute setup --mode cli --token-source none --yes

# MCP only; installs a private runtime without a global command
npx gitcontribute setup --mode mcp --codex --token-source none --yes
```

MCP-only setup is used through the configured coding agent. Run setup again and
choose CLI or Both if you also want the `gitcontribute` terminal command.

<details>
<summary><strong>Other installation options</strong></summary>

### Install a persistent command

```sh
npm install --global gitcontribute@latest
gitcontribute setup
```

To remove a global npm installation later, run
`npm uninstall --global gitcontribute`. The `gitcontribute remove` command only
removes selected MCP registrations. It does not delete versioned private
runtimes, uninstall the CLI, or remove application configuration or corpus data.

### Pin a project version

```sh
npm install --save-dev gitcontribute
npx gitcontribute setup --mode mcp --codex --token-source none --yes
```

The npm package has no install lifecycle and performs no download during
installation. Native binaries are included for macOS ARM64/x64, Linux ARM64/x64,
and Windows x64.

### Build from source

Developers with Go 1.26 or newer can install or build the CLI directly:

```sh
go install github.com/morluto/gitcontribute/cmd/gitcontribute@latest
go build -o gitcontribute ./cmd/gitcontribute
```

</details>

You need `git`. The `gh` CLI is optional and can provide authentication through
`gh auth token`; `ripgrep` is optional and adds an extra `doctor` check.

## 🧩 How it works

```text
 GitHub read APIs                  Local checkout
       │                                │
       │ explicit sync / hydrate        │ explicit index / acquire
       ▼                                ▼
  ┌──────────────────────────────────────────┐
  │          Local SQLite corpus             │
  │ observations · projections · coverage   │
  │ investigations · evidence · outcomes    │
  └────────────────────┬─────────────────────┘
                       │ offline reads
          ┌────────────┼─────────────┐
          ▼            ▼             ▼
       CLI / TUI    MCP agents    JSON exports
```

### 1. Discover

Track explicit repositories, GitHub search results, or GH Archive event streams.
Sync only when you ask, then search locally as often as you like.

```sh
gitcontribute source add repos --name my-go "golang/go" "cli/cli" --json
gitcontribute crawl my-go --since 720h --budget 500 --json
gitcontribute radar golang/go --limit 20 --json
gitcontribute search issues "data race" --repo golang/go --state open --json
gitcontribute search code "context.WithTimeout" --repo golang/go
```

### 2. Investigate

Build a dossier or per-thread research brief, inspect repository health, record
a hypothesis, and check for duplicate or competing work before committing time.

```sh
gitcontribute dossier build owner/repo
gitcontribute research brief issue:owner/repo#42 --format markdown
gitcontribute health owner/repo --json
gitcontribute investigation start-thread issue:owner/repo#42 --json
gitcontribute duplicates check <hypothesis-id>
gitcontribute collisions check <hypothesis-id>
```

### 3. Validate

Promote promising research into an opportunity, create an isolated worktree,
record evidence, and compare a baseline with your candidate change.

```sh
gitcontribute opportunity promote --problem="Retry can hang indefinitely" \
  --scope=small --impact="reduces flakes" --effort=1h \
  --confidence=0.8 <hypothesis-id>
gitcontribute workspace create <investigation-id>
gitcontribute validation define --kind=test --command="go test ./..." \
  --working-dir=/path/to/workspace <investigation-id>
gitcontribute validation run <validation-id> --kind=base --execute
gitcontribute validation run <validation-id> --kind=candidate --execute
gitcontribute validation compare <base-run-id> <candidate-run-id>
```

### 4. Check readiness

Run a deterministic local readiness gate before turning a candidate into a
public submission:

```sh
gitcontribute readiness opportunity <opportunity-id>
gitcontribute readiness explain <opportunity-id>:evidence_freshness
```

Readiness reports `pass`, `warn`, `block`, or `unknown` per rule. It only reads
stored corpus and draft metadata: it does not fetch GitHub, execute validation,
write state, or infer that missing coverage is a hard blocker.

### 5. Prepare

Create a local contribution draft from the evidence and workspace diff:

```sh
gitcontribute prepare issue <opportunity-id>
gitcontribute prepare pr --approach="Bound retries with context" \
  --workspace <workspace-id> <opportunity-id>
gitcontribute prepare review <opportunity-id>
```

Nothing is posted. You decide what leaves your machine.

## 🤖 Use with AI agents

The MCP server gives agents structured access to the same corpus and workflow:

```sh
gitcontribute setup --mode mcp --codex --token-source none --yes
gitcontribute setup --mode mcp --all-clients --token-source none --yes
```

Or start the stdio server directly:

```sh
gitcontribute mcp serve --transport=stdio
```

MCP capabilities are deliberately separate:

Tool names use the `<capability>.<action>` namespace. MCP clients qualify them
with the configured `gitcontribute` server name, producing names such as
`gitcontribute.corpus.search_repositories`. The server advertises one canonical
name per operation; unnamespaced compatibility aliases are not registered.

| Capability | Examples |
| --- | --- |
| **Offline reads** | Batch-inspect repositories, threads, and jobs; filter open or closed work; rank opportunities across repositories; find historical precedents; and review the authored pull-request portfolio. |
| **Network reads** | Batch-sync repository metadata or thread headers, hydrate selected finalists, discover authored pull requests, refresh PR status, and query derived public-repository context through DeepWiki. |
| **Local writes** | Start investigations, record hypotheses, promote opportunities, define validations, and prepare drafts. |
| **Execution** | Safely acquire and index repository code with Git only, or run a validation only when the request includes `execute: true`. |

A scalable discovery flow is:

```text
github.search_repositories -> corpus.get_repositories
github.sync_repository_metadata -> corpus.get_repositories
-> research.query_deepwiki -> github.sync_threads
-> corpus.rank_threads -> github.hydrate_threads
-> corpus.find_precedents
```

For contribution follow-up, use `github.sync_authored_pull_requests`, then
`github.sync_pull_request_status` and `corpus.list_pull_request_portfolio`.
Missing coverage is returned as unknown rather than as a false zero or negative.
PR status currently includes lifecycle, mergeability, head/base revisions, and
stored reviews. Checks, unresolved review threads, detailed merge state, merge
queue, and portfolio overlap are deliberately reported as unavailable. See the
[scalable MCP workflow guide](docs/mcp-scalable-workflows.md) for recovery and
coverage details.

Contribution workflow resources and prompts are available for agents:

- `corpus.get_readiness` and `gitcontribute://readiness/<opportunity-id>` expose the
  same offline readiness report as the CLI.
- `gitcontribute://workflow/contribution/<opportunity-id>` links the local
  opportunity, evidence, readiness report, and safe workflow prompts.
- Prompts such as `review_contribution_readiness` instruct agents to treat
  repository and GitHub-sourced text as untrusted data and to ask before any
  network read, local write, or validation execution.

Resources are published under `gitcontribute://` and `github-index://` URI
schemes. See [the architecture guide](docs/architecture.md) for the complete
application and adapter boundaries.

## 🛡️ Safety by design

| Operation | Network | Local write | Runs a process | GitHub write |
| --- | :---: | :---: | :---: | :---: |
| Search, health, dossier, research-brief, and readiness inspection | — | — | — | — |
| Investigations, evidence, lenses | — | ✓ | — | — |
| Sync, crawl, hydrate | ✓ | ✓ | — | — |
| DeepWiki repository context | ✓ | — | — | — |
| Acquire and batch-index code | ✓ | ✓ | `git` only | — |
| Acquire or create a workspace | remote-dependent | ✓ | `git` only | — |
| Validation with explicit execution | — by default | ✓ | ✓ | — |

- **No GitHub writes.** GitContribute does not open issues, create pull requests,
  push commits, or mutate GitHub.
- **No hidden network access.** Corpus reads never fetch data.
- **No hosted service or telemetry.** Your corpus and research remain local.
- **No automatic repository execution.** Crawling and indexing never execute
  repository-controlled code.
- **No implied sandbox.** Explicit validation commands run on your host with the
  permissions of your user and only the environment variables you allowlist.
- **No opaque semantic ranking.** Search uses SQLite FTS5 and reports signals
  such as text matches, freshness, and coverage.

## 📚 CLI guide

The sections below are a task-oriented reference. Run `gitcontribute --help` or
`gitcontribute <command> --help` for every flag.

<details>
<summary><strong>Setup, configuration, and authentication</strong></summary>

```sh
gitcontribute setup                              # interactive
gitcontribute setup --mode mcp --codex --token-source none --yes
gitcontribute setup --mode mcp --all-clients --token-source none --yes
gitcontribute setup --mode mcp --codex --token-source env \
  --token-source-key GITHUB_TOKEN --yes
gitcontribute setup --mode mcp --codex --token-source none --dry-run --json
gitcontribute remove --all-clients --yes         # remove MCP registrations only
gitcontribute upgrade --check
gitcontribute upgrade --yes
```

`gitcontribute init` creates the default database and directories without
contacting GitHub. `gitcontribute configure` updates `config.toml` atomically:

```sh
gitcontribute configure --database /path/to/corpus.db
gitcontribute configure --token-source env --token-source-key GITHUB_TOKEN
gitcontribute configure --token-source gh-cli
gitcontribute configure --output-format json
```

Authentication sources are `none`, `env`, `gh-cli`, and `keyring`. Tokens are
resolved at runtime and are never stored in the corpus or logs. Use
`gitcontribute status`, `metadata`, and `doctor` to inspect the local setup.
Use `gitcontribute doctor --strict` in automation when unhealthy required
checks should produce a non-zero exit status. Write contention is reported as
an optional availability warning rather than database corruption.

See [the onboarding design](docs/onboarding.md) for the full contract and
environment-variable reference.

</details>

<details>
<summary><strong>Sources, crawling, sync, and hydration</strong></summary>

Add a source:

```sh
gitcontribute source add repos --name my-go "golang/go" "cli/cli" --json
gitcontribute source add search --name go-network \
  --query "language:go stars:>100" --json
gitcontribute source add gharchive --name golang-events \
  --events "IssuesEvent,PullRequestEvent" --json
```

Run it once or continuously:

```sh
gitcontribute crawl golang-events --since 720h --budget 500 --json
gitcontribute tail golang-events --since 2h --budget 500 --interval 1h
```

Sync and selectively hydrate repository archives:

```sh
gitcontribute sync owner/repo
gitcontribute archive sync owner/repo --since 168h --state open
gitcontribute archive sync owner/repo --numbers 42,108
gitcontribute archive refresh owner/repo
gitcontribute archive hydrate owner/repo#42 --with issue_comments
gitcontribute archive hydrate owner/repo#108 \
  --with pr_reviews,pr_review_comments
gitcontribute archive coverage owner/repo
```

Hydration supports `issue_comments`, `pr_details`, `pr_reviews`, and
`pr_review_comments`. Fetches are paginated and cancellation-aware; a complete
facet replaces its previous snapshot atomically.

</details>

<details>
<summary><strong>Code indexing and acquisition</strong></summary>

Index a clean local checkout at its current commit:

```sh
gitcontribute index owner/repo /path/to/checkout --json
```

Or acquire a repository into a managed mirror, index a clean temporary
worktree, and remove that worktree afterward:

```sh
gitcontribute acquire owner/repo \
  --remote https://github.com/owner/repo.git --json
```

The indexer reads blobs directly from Git, skips binaries and non-UTF-8 content,
enforces size limits, and rejects dirty worktrees.

</details>

<details>
<summary><strong>Radar, research briefs, search, dossiers, health, seeds, and lenses</strong></summary>

```sh
gitcontribute radar owner/repo --limit 20
gitcontribute search repos "cli" --limit 20 --json
gitcontribute search issues "data race" --repo owner/repo --state open --json
gitcontribute search prs "flaky" --repo owner/repo --label bug --json
gitcontribute search threads "memory leak" --repo owner/repo
gitcontribute search code "context.WithTimeout" --repo owner/repo
gitcontribute search all "retry" --repo owner/repo

gitcontribute dossier build owner/repo
gitcontribute dossier export owner/repo --format markdown \
  --output owner-repo-dossier.md
gitcontribute research brief owner/repo#42
gitcontribute research brief pr:owner/repo#108 --json
gitcontribute health owner/repo --stale-after 336h --json
gitcontribute seeds owner/repo --json
```

`radar` is a strict offline corpus read. It ranks a bounded population of the
newest stored open issues and separates the numeric score from four explicit
eligibility states: `ready_to_code`, `needs_diagnosis`, `needs_coordination`,
and `blocked`. Every candidate reports positive signals, risks, blockers,
confidence, linked open PRs, evidence coverage, and unknowns. An explicit sync
ingests a fixed, bounded set of conventional CONTRIBUTING and AI-policy files;
hydrate comments when you want maintainer-direction and recent-claim evidence:

```sh
gitcontribute archive hydrate owner/repo#42 --with issue_comments
gitcontribute radar owner/repo --json
```

Radar scores carry a version (`radar.v2`) so saved JSON remains auditable when
ranking semantics evolve. It never syncs, hydrates, executes repository code,
or writes to GitHub.

`research brief` is also a strict offline read. Its versioned
`research-brief.v1` output has fixed sections for state, stored problem fields,
acceptance hints, participants, timeline, explicit/duplicate references,
linked PRs, indexed code, contribution guidance, health, coverage gaps, and
next commands. Every section carries source references or an explicit unknown
reason. Checkboxes and maintainer phrases remain source extracts—not fabricated
or complete acceptance criteria. Markdown output redacts common credentials and
quotes untrusted source text; JSON ordering is deterministic.

Missing child facets and code stay visible instead of triggering hidden work:

```sh
gitcontribute archive hydrate owner/repo#42 --with issue_comments
gitcontribute index owner/repo /path/to/clean-checkout
gitcontribute research brief issue:owner/repo#42 --json
```

Use a lens to apply saved filters and weighted ranking to a bounded population:

```sh
gitcontribute lens add my-lens --file lens.json
gitcontribute search issues "retry" --lens my-lens
gitcontribute lens explain my-lens issue:owner/repo#42 --query "retry"
```

Search results explain their scores. Most typed searches support opaque cursor
pagination; `search all` and lens-ranked searches do not.

</details>

<details>
<summary><strong>Investigations, evidence, tracking, and collections</strong></summary>

Start an investigation and its initial hypothesis from one exact stored thread
revision:

```sh
gitcontribute investigation start-thread issue:owner/repo#42 --json
```

This is a local-write operation with no network or process execution. The
investigation saves the immutable observation ID, source update time, sequence,
and source reference used for its title and bounded description. Repeating the
command returns the existing open pair with `"created": false`; it never updates
that baseline silently. The seed category is `other` because the command does
not infer scope or defect type from untrusted issue text.

The manual two-command path remains available when no stored thread should be
the baseline:

```sh
gitcontribute investigation start owner/repo --json
gitcontribute hypothesis add --title="Fix retry timeout" \
  --description="Reproduce and isolate the timeout." \
  --category=bug <investigation-id>
```

Record supporting or contradicting evidence:

```sh
gitcontribute evidence add --type=manual_observation --relation=supporting \
  --description="Reproduced on the current default branch." \
  --opportunity <opportunity-id>
```

Evidence shown through the CLI, exports, and MCP includes a derived freshness
status. `github_source` evidence recorded from a started thread carries the
exact corpus source revision it used, so later issue, pull request, facet, or
guidance updates can mark it `stale`. Missing source projections are
`unknown`; local/manual evidence is `not_applicable`. Freshness is evaluated
from local corpus reads only, and stale evidence is a prompt to re-check, not a
claim that the evidence is invalid.

Group typed references and record local decisions:

```sh
gitcontribute collection create interesting
gitcontribute collection add interesting \
  repo:owner/repo issue:owner/repo#42
gitcontribute triage record issue:owner/repo#42 viewed --reason "..."
gitcontribute contribution record <opportunity-id> issue \
  "Draft title" --body "..."
gitcontribute contribution outcome <contribution-id> submitted
```

Export and restore tracking metadata:

```sh
gitcontribute tracking export --output tracking.json
gitcontribute tracking import --file tracking.json
```

Tracking exports use schema version 2 for portable evidence provenance and
remain backward compatible with older unversioned tracking bundles. Exports are
redacted: they exclude credentials, tokens, and absolute local paths.

</details>

<details>
<summary><strong>Workspaces, jobs, runs, and TUI</strong></summary>

```sh
gitcontribute workspace show <workspace-id>
gitcontribute diff <workspace-id>
gitcontribute runs --limit 20
gitcontribute jobs
gitcontribute job show <id>
gitcontribute job cancel <id>
gitcontribute tui owner/repo
```

`diff` returns the patch, changed files, and suggested review order. The TUI is
local-only; add `--json` to emit a non-interactive snapshot.

</details>

<details>
<summary><strong>JSON and output behavior</strong></summary>

Most non-interactive commands accept `--json`. Machine-readable output goes to
stdout; progress and status messages go to stderr. List commands accept
`--limit`, and paginated searches return an opaque `next_cursor` where supported.

`dossier export`, `export dossier`, `export evidence`, and `tracking export`
accept `--output <file>`.

</details>

## 💾 Storage locations

| Platform | Config | Data | Cache | Logs |
| --- | --- | --- | --- | --- |
| **Linux / Unix** | `$XDG_CONFIG_HOME/gitcontribute` or `~/.config/gitcontribute` | `$XDG_DATA_HOME/gitcontribute` or `~/.local/share/gitcontribute` | `$XDG_CACHE_HOME/gitcontribute` or `~/.cache/gitcontribute` | `$XDG_STATE_HOME/gitcontribute` or `~/.local/state/gitcontribute/logs` |
| **macOS** | `~/Library/Application Support/gitcontribute` | `~/Library/Application Support/gitcontribute/Data` | `~/Library/Caches/gitcontribute` | `~/Library/Logs/gitcontribute` |
| **Windows** | `%APPDATA%\gitcontribute` | `%LOCALAPPDATA%\gitcontribute\Data` | `%LOCALAPPDATA%\gitcontribute\Cache` | `%LOCALAPPDATA%\gitcontribute\Logs` |

The default database is `gitcontribute.db` in the data directory. The
configuration file is `config.toml` in the config directory.

## 🖥️ Platform support

Linux and macOS are the primary development and test targets. Windows builds
are expected to work with Git for Windows and standard `%APPDATA%` and
`%LOCALAPPDATA%` paths. For platform-specific problems, open a bug report with
the output of `gitcontribute doctor --json`.

## 🛠️ Development

```sh
go test ./...
go build -o gitcontribute ./cmd/gitcontribute
./gitcontribute --help
```

Before changing package boundaries or side effects, read
[docs/architecture.md](docs/architecture.md). See [CONTRIBUTING.md](CONTRIBUTING.md)
for the complete development and testing workflow.

---

<div align="center">

Built for contributors who want to understand the problem before writing the patch.

[Architecture](docs/architecture.md) · [Onboarding](docs/onboarding.md) · [Runbooks](docs/runbooks.md) · [Security](SECURITY.md) · [Contributing](CONTRIBUTING.md) · [License](LICENSE)

</div>

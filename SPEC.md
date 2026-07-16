# GitContribute Specification

Status: Draft 0.2  
Last updated: 2026-07-16

## 1. Summary

GitContribute is a local-first GitHub contribution research workbench. It gives
people and agents the tools to discover repositories, retain GitHub history,
understand code and maintainer conventions, investigate contribution
opportunities, validate findings, and prepare focused upstream contributions.

The product is not only an issue finder or an autonomous PR generator. It is a
durable GitHub corpus and a set of composable research, validation, and
contribution tools. Saved lenses and workflows can make it personal, but the
engine itself remains general-purpose.

The initial implementation will be written in Go as an independent codebase.
It will apply lessons from OpenClaw's Gitcrawl and other mature systems without
depending on their modules, schemas, release process, or repository history.
Established libraries are preferred over custom infrastructure. Small,
well-bounded portions of permissively licensed source may be adapted when that
is safer and clearer than reimplementation, with provenance and attribution.

## 2. Problem

GitHub contains the information needed to identify valuable contributions, but
that information is fragmented across repository metadata, issues, pull
requests, review conversations, commits, code, tests, CI, release history, and
contribution documentation.

Contributors repeatedly pay the cost of:

- discovering repositories worth understanding;
- re-fetching the same issues, PRs, and conversations;
- learning project conventions from recent accepted and rejected work;
- checking whether an idea is already reported or being implemented;
- inspecting a codebase for unreported bugs and significant improvements;
- distinguishing plausible hypotheses from reproducible findings;
- proving impact with tests, benchmarks, or other evidence;
- preparing a contribution that fits maintainer expectations.

GitContribute makes that context durable and exposes the operations needed to
move from broad discovery to a well-supported contribution.

## 3. Product definition

> GitContribute is an all-in-one GitHub contribution kit built around a
> persistent local corpus.

Its principal flow is:

```text
discover
   |
   v
archive repository context
   |
   v
inspect code and history
   |
   v
investigate opportunities
   |
   v
validate the strongest finding
   |
   v
prepare and track a contribution
```

## 4. Goals

### 4.1 Primary goals

- Crawl GitHub broadly enough to discover active repositories and contribution
  signals without deeply fetching every repository.
- Maintain a durable, incrementally refreshed corpus of selected repositories,
  issues, PRs, comments, reviews, commits, checks, and code documents.
- Build an independently owned repository archive and search core informed by
  proven Gitcrawl behavior and tests.
- Build a repository dossier that captures code structure, active work,
  accepted contribution patterns, rejected approaches, maintainer behavior,
  and validation conventions.
- Represent contribution opportunities independently from GitHub issues so
  unreported code findings and abandoned work can be investigated.
- Preserve hypotheses, evidence, validation runs, decisions, and outcomes.
- Offer equivalent structured operations through a CLI and MCP server.
- Keep network access, local mutations, code execution, and GitHub mutations
  explicit and distinguishable.
- Support personal workflows through saved lenses, weights, collections, and
  feedback without making personalization mandatory.

### 4.2 Secondary goals

- Provide a terminal UI for browsing repositories, threads, clusters,
  opportunities, and evidence.
- Support portable local archives and exportable evidence bundles.
- Allow optional semantic search, related-thread discovery, and duplicate
  clustering.
- Allow future analyzer integrations such as Semgrep, SCIP, tree-sitter, and
  language-native linters.

### 4.3 Non-goals for the first release

- Automatically opening issues or PRs on GitHub.
- Automatically executing arbitrary code from untrusted repositories on the
  host.
- Mirroring all GitHub data or retaining the complete GH Archive firehose.
- Replacing Git, GitHub, a code editor, or a general-purpose coding agent.
- Producing a universal opaque score that claims to identify the objectively
  best issue or contributor.
- Supporting GitLab, Gitea, Forgejo, Bitbucket, or package registries.
- Building a hosted multi-user service, web application, or public analytics
  platform in v1.

## 5. Design principles

### 5.1 Local corpus first

Search and inspection should normally read from the local corpus. Network
operations happen only through explicit discovery, synchronization, refresh,
or hydration operations.

### 5.2 Broad shallow crawl, selective deep hydration

Broad ingestion records cheap repository and thread observations. Policies
promote a small subset for expensive comments, reviews, PR details, code,
embeddings, and validation.

### 5.3 Tools rather than predetermined judgment

The engine provides facts, search, measurements, workspaces, and memory.
Lenses, workflows, humans, and agents decide what is interesting and how to
rank it.

### 5.4 Evidence before implementation

An opportunity can begin as a hypothesis, but it should not be presented as a
confirmed bug or improvement until its evidence supports that conclusion.

### 5.5 Transparent scoring

Search and ranking results expose the contributing signals and freshness.
Users can see why a result matched and change the weights.

### 5.6 One application core, multiple interfaces

The CLI, MCP server, TUI, and future interfaces call the same application
services. They do not shell out to one another or reimplement domain behavior.

### 5.7 Safe boundaries

Read-only corpus queries, network reads, local writes, process execution, and
external GitHub writes are separate capabilities. No command should hide a
more powerful side effect behind an ordinary search operation.

### 5.8 Adopt before building

Use the standard library or a mature, maintained library for protocols,
database access, migrations, CLI parsing, authentication, rate limiting,
terminal behavior, and MCP integration when it satisfies the contract. Custom
code is justified only when existing options do not fit the required behavior
or would introduce a larger dependency than the problem warrants.

Evaluate dependencies for maintenance activity, API stability, license,
security history, transitive dependency cost, cross-platform behavior, and
testability. Wrap third-party packages at narrow adapter boundaries so domain
models and application services do not inherit vendor-specific types.

### 5.9 Independent ownership

GitContribute must build, test, migrate, and release without Gitcrawl or
Crawlkit. It does not preserve their CLI, database, package, or remote-service
contracts. Directly adapted permissive source becomes locally maintained code;
its origin and modifications remain documented, but no upstream synchronization
is required for the product to function.

## 6. Users and use cases

### 6.1 Open-source contributor

- Discover active projects in a domain.
- Study recent merged PRs and open work.
- Find explicit or latent contribution opportunities.
- Prove a bug or improvement before investing in a patch.
- Prepare a contribution that follows repository norms.

### 6.2 Maintainer

- Search and cluster local issue/PR history.
- Find duplicates, recurring reports, and stale work.
- Build contributor-facing dossiers and opportunity lists.
- Review evidence and contribution history.

### 6.3 Researcher

- Build a bounded GitHub corpus.
- Query repository, issue, PR, and event observations.
- Export normalized data with provenance and freshness.

### 6.4 Coding agent

- Retrieve structured repository and thread context through MCP.
- Start explicit crawl, dossier, investigation, and validation tasks.
- Record hypotheses and evidence durably.
- Prepare but not silently publish issue or PR material.

## 7. Terminology

- **Source**: A producer of observations, such as GitHub Search, GH Archive, or
  an explicit repository list.
- **Observation**: Immutable data received from an external source at a known
  time.
- **Current state**: The latest accepted normalized state derived from
  observations.
- **Frontier item**: A repository, thread, or facet waiting to be crawled or
  hydrated.
- **Facet**: A separately fetchable category such as comments, reviews, README,
  PR details, code, or repository health.
- **Coverage**: Which facets are present, complete, and fresh.
- **Lens**: Saved filters, scoring weights, budgets, and optional promotion
  rules.
- **Dossier**: A bounded repository context package assembled from GitHub
  history, code, documentation, activity, and conventions.
- **Investigation**: A durable workspace for exploring one or more hypotheses.
- **Hypothesis**: A possible bug or improvement that has not yet met a defined
  evidence threshold.
- **Opportunity**: A scoped potential contribution with sources, impact,
  confidence, collision status, and evidence.
- **Evidence**: Reproduction output, test results, benchmark measurements,
  code references, GitHub references, or other material supporting or
  contradicting a claim.
- **Contribution**: Prepared issue or PR material and its eventual outcome.

## 8. Feature inventory

### 8.1 Initialization and control plane

- Initialize a local SQLite corpus and configuration.
- Resolve platform-native config, data, cache, state, and log directories.
- Report machine-readable application metadata and capabilities.
- Report corpus status, counts, freshness, active jobs, and warnings.
- Diagnose database access, schema, locks, GitHub authentication, Git
  availability, and optional provider availability.
- Track every crawl, hydration, indexing, embedding, clustering, dossier,
  investigation, and validation run.
- Expose progress, cancellation, errors, retry state, rate-limit state, and
  actual completion statistics.
- Support JSON output for all non-interactive commands.
- Perform a cached, passive release update check that can be disabled.

### 8.2 GitHub authentication and API behavior

- Resolve a token from explicit configuration, environment, or `gh auth token`.
- Support public unauthenticated reads where appropriate.
- Observe GitHub core, search, GraphQL, and secondary rate limits.
- Persist ETag and Last-Modified validators where useful.
- Use conditional requests for unchanged canonical resources.
- Respect `Retry-After` and rate-limit reset headers.
- Bound concurrency and API budgets per run.
- Record the API version and source URL associated with observations.
- Treat GitHub as the canonical current state.

### 8.3 Discovery sources

#### GitHub Search

- Search repositories, issues, PRs, and optionally commits.
- Persist search definitions and incremental run state.
- Partition searches by time and other stable qualifiers when a query exceeds
  GitHub's 1,000-result limit.
- Use `created` windows for historical backfills and overlapping `updated`
  windows for refresh.
- Persist `total_count`, `incomplete_results`, pages, windows, and retries.
- Deduplicate results by GitHub node or database ID.
- Support repository, language, topic, label, state, assignee, archive, fork,
  visibility, activity, and time filters exposed by GitHub.

#### GH Archive

- Stream selected hourly GH Archive files.
- Retain only configured event types and normalized signals by default.
- Record imported archive filenames/hours so runs are idempotent.
- Support bounded backfills and continuous tailing.
- Initially recognize:
  - `IssuesEvent`;
  - `IssueCommentEvent`;
  - `PullRequestEvent`;
  - `PullRequestReviewEvent`;
  - `PullRequestReviewCommentEvent`;
  - `PushEvent`;
  - `ReleaseEvent`;
  - `WatchEvent`;
  - `ForkEvent`;
  - `DiscussionEvent` and `DiscussionCommentEvent` when useful.
- Use events as discovery and freshness signals, not as guaranteed canonical
  state.

#### Explicit repository sources

- Add repositories by `owner/repo`, URL, file, or stdin.
- Import a user's starred, watched, owned, forked, or previously contributed
  repositories when explicitly requested.
- Add repositories discovered by other tools through a structured import.

### 8.4 Persistent frontier

- Queue repositories, threads, and facets independently.
- Persist priority, reason, source, attempts, earliest run time, and budget
  estimate.
- Deduplicate equivalent work.
- Lease work so interrupted workers can safely resume.
- Retry transient failures with bounded exponential backoff.
- Distinguish terminal absence, authorization failure, deletion, archival, and
  transient errors.
- Prioritize by configurable signals such as novelty, freshness, activity,
  missing coverage, lens match, and expected API cost.
- Make crawl work idempotent and safe to replay.

### 8.5 Repository archive

- Register and synchronize selected repositories.
- Fetch repository metadata before thread synchronization.
- Fetch open, closed, or all issues and PRs.
- Incrementally sync threads updated since a timestamp.
- Sweep recently closed threads during incremental open synchronization.
- Refresh exact issue or PR numbers independently of list ordering.
- Store issue and PR titles, bodies, authors, associations, labels, assignees,
  state, draft status, and GitHub timestamps.
- Optionally hydrate issue comments.
- Optionally hydrate PR reviews and review comments.
- Optionally hydrate PR review-thread state.
- Optionally hydrate PR files, commits, head/base metadata, checks, and workflow
  runs.
- Preserve immutable thread revisions and deterministic fingerprints.
- Use observation ordering to prevent stale observations replacing newer
  evidence.
- Preserve raw source payloads where their storage value justifies the cost.
- Report facet coverage and missing enrichment.
- Track run statistics and failures.

### 8.6 Repository metadata and health

- README and preferred README metadata.
- CONTRIBUTING, SECURITY, SUPPORT, CODE_OF_CONDUCT, GOVERNANCE, and CODEOWNERS
  files where public and present.
- License, topics, languages, default branch, archive/fork status, stars,
  watchers, forks, and open issue counts.
- Releases, tags, recent commits, and change cadence.
- CI workflow inventory and recent public check outcomes.
- Issue and PR response-time distributions.
- External-contributor PR open, close, and merge outcomes.
- Open PR congestion and stale-review signals.
- Maintainer and ownership signals derived from public GitHub data.
- Every derived metric records its time window and source coverage.

### 8.7 Search and retrieval

- SQLite FTS5 keyword search across issue/PR documents.
- Search thread metadata by repository, kind, state, author, association,
  assignee, labels, and time.
- Code-document search across indexed tracked files.
- Combined thread and code search.
- Optional semantic search over selected documents.
- Hybrid keyword and semantic result modes.
- Related-thread and nearest-neighbor lookup.
- Duplicate-candidate clustering using semantic and explicit GitHub-reference
  evidence.
- Optional source-grounded thread and repository summaries that record their
  provider, model, source revision, and generation time.
- Search across one repository, a collection, a lens, or the entire local
  corpus.
- Opaque cursor pagination for stable machine consumption.
- Result freshness, coverage, source, and score explanation.
- Local search must not silently trigger a network refresh.

### 8.8 Code acquisition and indexing

- Clone or fetch a repository into a managed cache when explicitly requested.
- Record remote URL, default branch, commit SHA, and acquisition time.
- Use native Git for clone, fetch, worktree, log, blame, diff, and merge-base
  semantics.
- Enumerate tracked files through Git.
- Index bounded UTF-8 text files with file, byte, and total-size limits.
- Respect repository attributes and configurable binary/secret exclusions.
- Keep code snapshots identified by commit SHA.
- Search checkouts with `rg` where available.
- Associate code documents with repository and snapshot, not mutable paths.
- Allow future syntax/symbol index adapters without requiring them for v1.

### 8.9 Repository dossiers

- Build a bounded dossier at a known repository commit and GitHub observation
  time.
- Include configurable sections:
  - repository purpose and domain;
  - architecture and important modules;
  - contribution and validation instructions;
  - recent releases and active areas;
  - recent merged PRs, especially from non-maintainers;
  - closed unmerged PRs and stated rejection reasons;
  - open PRs and likely collision areas;
  - open and recently closed issues;
  - recurring issue/PR clusters;
  - CI and supported-platform signals;
  - repository responsiveness and external-contributor outcomes;
  - indexed code and test layout.
- Store the exact source set and time range used to create the dossier.
- Refresh only stale or explicitly requested dossier sections.
- Export dossiers as structured JSON and readable Markdown.

### 8.10 Seed extraction and pattern learning

- Select recent merged PRs as examples of accepted scope and evidence.
- Prefer like-for-like examples: fixes with fixes, features with features,
  documentation with documentation, and so on.
- Extract title conventions, description structure, issue linkage, validation,
  compatibility notes, review comments, and approximate change size.
- Examine closed unmerged PRs for explicit rejection or supersession context.
- Mine recent issues and recurring clusters for unresolved problem areas.
- Identify recently changing modules and repeated fix patterns as investigation
  seeds.
- Preserve extracted patterns as evidence, not universal project rules.

### 8.11 Investigations and hypotheses

- Create an investigation scoped to a repository, commit, and optional lens.
- Record one or more hypotheses with explicit source references.
- Classify hypotheses as bug, performance, architecture, testing,
  documentation, maintenance, compatibility, security, or other.
- Record affected components, expected behavior, observed behavior, potential
  impact, and open questions.
- Link hypotheses to issues, PRs, commits, files, symbols, tests, logs, and
  other hypotheses.
- Search for duplicate issues, duplicate PRs, related clusters, and competing
  open work.
- Record support, contradiction, rejection, deferral, and supersession.
- Preserve an audit trail of status changes and rationale.

### 8.12 Opportunities

- Promote a hypothesis to an opportunity without claiming it is proven.
- Allow opportunities originating from:
  - open issues;
  - closed or stale issues;
  - open, abandoned, or rejected PRs;
  - code inspection;
  - tests, benchmarks, CI, or static analysis;
  - repeated historical patterns;
  - maintainer requests or roadmaps.
- Store problem statement, type, scope, impact, confidence, expected effort,
  dependencies, collision status, and maintainer-alignment evidence.
- Track an opportunity lifecycle:
  - `hypothesis`;
  - `reproduced`;
  - `validated`;
  - `maintainer_aligned`;
  - `implemented`;
  - `submitted`;
  - `merged`;
  - `rejected`;
  - `deferred`;
  - `superseded`.
- Never infer a stronger status only from prose generated by a model.

### 8.13 Lenses and ranking

- Save reusable filters, named signals, weights, budgets, and hydration rules.
- Provide standard signals without fixing one universal formula:
  - domain or text relevance;
  - semantic similarity;
  - repository activity;
  - freshness;
  - maintainer responsiveness;
  - external-contributor merge rate;
  - issue clarity;
  - proof strength;
  - impact;
  - scope clarity;
  - expected effort;
  - novelty;
  - collision risk;
  - open PR congestion.
- Normalize signals within documented populations and time windows.
- Display individual signal values and the final weighted result.
- Support hard filters independently from scoring.
- Limit repeated results from one repository.
- Allow personal lenses to reference explicit interests, collections, prior
  outcomes, or example documents.
- Do not call inferred preferences objective facts.

### 8.14 Workspaces

- Create isolated Git worktrees at a recorded base SHA.
- Track workspace path, branch, base, head, dirty state, and investigation.
- Support refresh/rebase planning without silently rewriting user work.
- Compute diffs against the intended base.
- Keep user-owned checkouts separate from managed workspaces by default.
- Never delete a dirty workspace without explicit authorization.

### 8.15 Validation and evidence

- Define validation commands explicitly and associate them with an
  investigation or opportunity.
- Capture command, working directory, environment allowlist, start/end time,
  exit status, stdout/stderr references, and relevant artifacts.
- Support evidence types:
  - base-branch failing regression test;
  - fixed-branch passing regression test;
  - minimal reproduction;
  - benchmark and methodology;
  - profiler output;
  - invariant or property violation;
  - compatibility matrix;
  - static-analysis result;
  - manual observation with recorded steps;
  - GitHub source evidence.
- Compare base and candidate behavior.
- Mark evidence as supporting, contradicting, inconclusive, stale, or invalid.
- Preserve proof gaps and unrun checks when they materially affect confidence.
- Export an evidence packet suitable for issue or PR preparation.
- Require explicit authorization before executing repository-controlled code.
- Add containerized execution as a later capability, not a pretense of safety
  around unrestricted host execution.

### 8.16 Contribution preparation

- Read repository contribution guidance and templates from the dossier.
- Select relevant recent merged contributions as unwritten-convention
  references.
- Check for current duplicate issues, PRs, and competing work before preparing
  text.
- Prepare issue material with problem, context, evidence, reproduction,
  impact, and success criteria.
- Prepare PR material with motivation, concrete outcome, approach, focused
  changes, validation, compatibility, limitations, and issue linkage.
- Generate a suggested review order for larger diffs.
- Review the complete diff against its intended base.
- Keep generated claims synchronized with actual evidence.
- Export drafts to stdout, JSON, or files.
- Do not post, push, comment, or mutate GitHub without a separate explicit
  operation and authorization.

### 8.17 Tracking and feedback

- Save repositories, threads, opportunities, and investigations to named
  collections.
- Record viewed, ignored, saved, investigated, implemented, submitted, merged,
  rejected, and abandoned outcomes.
- Record reasons for skipping or rejecting an opportunity.
- Allow outcomes to be used as optional lens signals.
- Keep local triage state separate from GitHub state.
- Support import/export of local metadata without exposing tokens or secrets.

### 8.18 Clustering and local governance

- Generate duplicate-candidate clusters.
- Preserve durable cluster IDs and local overrides across recomputation.
- Set a canonical member.
- Include or exclude members with rationale.
- Close/reopen local threads and clusters without changing GitHub.
- Produce cluster reports.
- Use clusters as dossier and duplicate-search inputs.

### 8.19 Terminal UI

- Browse repositories, threads, clusters, opportunities, and investigations.
- Search and filter without network access.
- Inspect detail, source references, freshness, and coverage.
- Navigate between related threads and nearest neighbors.
- Trigger explicit refresh/hydration tasks with visible side effects.
- Use established terminal interaction patterns and libraries where a TUI
  remains valuable.

### 8.20 Portability and remote operation

Initial scope:

- Local SQLite database.
- JSON and Markdown exports.
- Portable evidence and dossier bundles.

Deferred scope:

- Git-backed portable stores.
- Sanitized snapshot publication.
- Remote read-only corpus service.
- OAuth and organization/team authorization.
- Cloud snapshot retention and deletion policy.

Remote publication must not be enabled until private-content handling,
authorization, retention, integrity, and deletion contracts are explicitly
designed and tested.

## 9. End-to-end workflows

### 9.1 Discover and evaluate a repository

1. Run configured Search or GH Archive sources.
2. Upsert shallow repository and thread observations.
3. Apply a lens to the local corpus.
4. Hydrate repository metadata for the top results.
5. Build dossiers for selected repositories.
6. Inspect score explanations and source freshness.
7. Save, skip, or investigate a repository.

### 9.2 Audit a repository for contribution opportunities

1. Sync repository issues and PRs.
2. Hydrate recent merged, open, and closed-unmerged PR conversations.
3. Clone/fetch the repository and index code at a known SHA.
4. Build a dossier and extract recent accepted/rejected patterns.
5. Search code, tests, issues, PRs, and clusters together.
6. Record candidate hypotheses.
7. Check duplicates and collisions.
8. Reproduce and validate the strongest candidate.
9. Promote it to an evidence-backed opportunity.
10. Prepare an issue or PR draft.

### 9.3 Reproduce a reported bug

1. Open an investigation from the GitHub issue.
2. Hydrate the full thread and linked PRs.
3. Create a worktree at the intended base.
4. Record reproduction steps.
5. Run the proof on the base branch.
6. Confirm failure for the intended reason.
7. Implement or import a candidate fix.
8. Run the same proof on the candidate branch.
9. Capture supporting and contradicting evidence.
10. Prepare the contribution using repository conventions.

### 9.4 Agent workflow through MCP

1. Search the local corpus.
2. Read repository and thread resources.
3. Request missing hydration explicitly.
4. Start a dossier or investigation task.
5. Record hypotheses with source references.
6. Request duplicate/collision checks.
7. Create a workspace only with explicit local-write intent.
8. Request validation only with explicit process-execution intent.
9. Read the evidence packet.
10. Prepare but do not publish contribution material.

## 10. Architecture

### 10.1 Language and runtime

- Go 1.26 or newer within the Go 1 compatibility promise.
- Single local binary where practical.
- Pure-Go SQLite driver to avoid CGO in standard builds.
- Native Git executable as an explicit runtime dependency for contribution
  semantics.

### 10.2 Package boundaries

```text
cmd/gitcontribute       binary entry point
internal/app            shared application services and use cases
internal/github         REST adapters, authentication, and rate limits
internal/corpus         SQLite schema, observations, projections, migrations
internal/crawl          sources, frontier, jobs, retries, and checkpoints
internal/search         FTS, semantic, hybrid, and result explanation
internal/dossier        repository context assembly
internal/investigation  hypotheses and opportunities
internal/workspace      clones, worktrees, and diff context
internal/evidence       validation runs and evidence bundles
internal/contribution   issue/PR preparation
internal/lens           filters, signals, scoring, and promotion policy
internal/cli            CLI adapter
internal/mcp            MCP adapter
internal/tui            terminal interface
```

The `internal/app` layer owns transaction boundaries and use-case contracts.
CLI, MCP, and TUI adapters must not own domain decisions.

### 10.3 Data flow

```text
GitHub Search ----+
GH Archive -------+--> frontier --> shallow observations
Explicit repos ---+                        |
                                           v
                                  normalized current state
                                           |
                                    lens/promotion
                                           |
                                           v
                               selective deep hydration
                                           |
                                           v
                            dossier/investigation/evidence
```

### 10.4 Service contracts

Representative application services:

```text
DiscoveryService
  AddSource
  RunSource
  TailSource
  GetRun
  CancelRun

CorpusService
  SyncRepository
  SyncThreads
  HydrateRepository
  HydrateThread
  GetCoverage

SearchService
  SearchRepositories
  SearchThreads
  SearchCode
  FindNeighbors
  ExplainResult

DossierService
  BuildDossier
  GetDossier
  ExtractSeeds

InvestigationService
  StartInvestigation
  RecordHypothesis
  CheckDuplicates
  CheckCollisions
  PromoteOpportunity

WorkspaceService
  CreateWorkspace
  GetWorkspace
  ComputeDiff

EvidenceService
  DefineValidation
  RunValidation
  CompareValidation
  ExportEvidence

ContributionService
  PrepareIssue
  PreparePullRequest
  ReviewContribution
```

## 11. Storage model

The exact schema will evolve through migrations. The initial conceptual tables
are:

### 11.1 Corpus and observations

- `repositories`
- `repository_observations`
- `threads`
- `thread_observations`
- `thread_revisions`
- `thread_fingerprints`
- `comments`
- `pull_request_details`
- `pull_request_files`
- `pull_request_commits`
- `pull_request_checks`
- `workflow_runs`
- `review_threads`
- `events`
- `documents`
- `code_snapshots`
- `code_documents`
- `vectors`
- `clusters`
- `cluster_members`
- `cluster_overrides`

### 11.2 Crawl control

- `sources`
- `source_partitions`
- `frontier_items`
- `facet_coverage`
- `http_validators`
- `runs`
- `run_events`
- `rate_limit_observations`

### 11.3 Contribution workflow

- `dossiers`
- `dossier_sources`
- `investigations`
- `hypotheses`
- `opportunities`
- `opportunity_sources`
- `workspaces`
- `validation_definitions`
- `validation_runs`
- `evidence`
- `contributions`
- `contribution_outcomes`
- `collections`
- `collection_members`
- `lenses`
- `triage_events`

### 11.4 Storage rules

- GitHub IDs are authoritative identities where available.
- Renames update names but do not create new repository identities.
- Current state and immutable observations remain separate.
- Timestamps distinguish source time, observation time, and local update time.
- Replace-all child snapshots require a complete observation and observation
  ordering.
- Raw payload retention is configurable and bounded.
- Search indexes can be rebuilt from canonical stored documents.
- Secrets never enter exports, documents, embeddings, or logs.

## 12. CLI specification

`gitcontribute` is the working binary name.

### 12.1 Control

```text
gitcontribute init
gitcontribute configure
gitcontribute metadata
gitcontribute status
gitcontribute doctor
gitcontribute coverage
gitcontribute runs
gitcontribute jobs
gitcontribute job show <id>
gitcontribute job cancel <id>
```

### 12.2 Sources and crawling

```text
gitcontribute source add search --name NAME --query QUERY
gitcontribute source add gharchive --events LIST
gitcontribute source add repos OWNER/REPO...
gitcontribute source list
gitcontribute source show NAME
gitcontribute crawl NAME [--since DURATION] [--budget N]
gitcontribute tail NAME
```

### 12.3 Archive

```text
gitcontribute archive sync OWNER/REPO
gitcontribute archive sync OWNER/REPO --state open|closed|all
gitcontribute archive sync OWNER/REPO --numbers REFS
gitcontribute archive hydrate OWNER/REPO#NUMBER --with FACETS
gitcontribute archive refresh OWNER/REPO
gitcontribute archive threads OWNER/REPO
gitcontribute archive coverage OWNER/REPO
```

### 12.4 Search

```text
gitcontribute search repos QUERY [filters]
gitcontribute search issues QUERY [filters]
gitcontribute search prs QUERY [filters]
gitcontribute search threads QUERY [filters]
gitcontribute search code QUERY [filters]
gitcontribute search all QUERY [filters]
gitcontribute neighbors OWNER/REPO#NUMBER
gitcontribute clusters OWNER/REPO
gitcontribute cluster show ID
```

### 12.5 Dossiers and seeds

```text
gitcontribute dossier build OWNER/REPO [--since DURATION] [--include LIST]
gitcontribute dossier show OWNER/REPO
gitcontribute dossier export OWNER/REPO
gitcontribute seeds OWNER/REPO --from merged-prs,closed-prs,issues
```

### 12.6 Investigations and opportunities

```text
gitcontribute investigation start OWNER/REPO
gitcontribute investigation show ID
gitcontribute investigation list
gitcontribute hypothesis add INVESTIGATION
gitcontribute hypothesis update ID
gitcontribute duplicates check ID
gitcontribute collisions check ID
gitcontribute opportunity promote HYPOTHESIS
gitcontribute opportunity list
gitcontribute opportunity show ID
gitcontribute opportunity set-status ID STATUS
```

### 12.7 Workspaces and validation

```text
gitcontribute workspace create INVESTIGATION
gitcontribute workspace show ID
gitcontribute diff WORKSPACE
gitcontribute validation define INVESTIGATION
gitcontribute validation run ID
gitcontribute validation compare BASE_RUN CANDIDATE_RUN
gitcontribute evidence show INVESTIGATION
gitcontribute evidence export INVESTIGATION
```

### 12.8 Contribution preparation

```text
gitcontribute prepare issue OPPORTUNITY
gitcontribute prepare pr OPPORTUNITY --workspace ID
gitcontribute prepare review OPPORTUNITY --workspace ID
```

### 12.9 Lenses and collections

```text
gitcontribute lens add NAME --file PATH
gitcontribute lens list
gitcontribute lens show NAME
gitcontribute lens explain NAME RESULT
gitcontribute collection create NAME
gitcontribute collection add NAME REF...
gitcontribute collection list
```

### 12.10 Interfaces

```text
gitcontribute tui [OWNER/REPO]
gitcontribute mcp serve --transport stdio
```

### 12.11 CLI behavior requirements

- Every read command supports bounded output.
- Machine-oriented commands support `--json`.
- Pagination uses stable opaque cursors where relevant.
- Errors use stable categories and non-zero exit codes.
- Progress goes to stderr; structured results go to stdout.
- Search is local unless a refresh command is explicit.
- Commands print the source freshness and missing coverage when that affects
  interpretation.

## 13. MCP specification

### 13.1 Transport

- Stdio is the default v1 transport.
- Streamable HTTP is deferred until authentication and multi-user isolation
  are designed.
- The MCP adapter calls application services directly.

### 13.2 Read-only tools

- `search_repositories`
- `search_threads`
- `search_code`
- `get_repository`
- `get_thread`
- `get_repository_dossier`
- `find_neighbors`
- `find_clusters`
- `explain_match`
- `get_coverage`
- `get_investigation`
- `list_opportunities`
- `get_opportunity`
- `get_evidence`
- `get_job`

### 13.3 Local-write and network-read tools

- `start_crawl`
- `sync_repository`
- `hydrate_repository`
- `hydrate_thread`
- `build_repository_dossier`
- `start_investigation`
- `record_hypothesis`
- `check_duplicates`
- `check_collisions`
- `promote_opportunity`
- `create_workspace`
- `define_validation`
- `run_validation`
- `prepare_contribution`
- `cancel_job`

### 13.4 Resources

```text
github-index://repositories/{owner}/{repo}
github-index://threads/{owner}/{repo}/{number}
github-index://dossiers/{owner}/{repo}
github-index://investigations/{id}
github-index://opportunities/{id}
github-index://evidence/{investigation_id}
github-index://lenses/{name}
github-index://jobs/{id}
```

### 13.5 MCP behavior requirements

- Tools define JSON Schema inputs and structured output schemas.
- Results include stable IDs, URLs, source timestamps, and coverage.
- Lists use opaque cursor pagination.
- Long-running operations use MCP tasks where supported and durable job IDs as
  a compatibility-level application contract.
- Tools expose accurate read-only, destructive, idempotent, and open-world
  annotations.
- Search/get tools are read-only and do not auto-hydrate.
- Crawl/hydration tools write locally and access GitHub but do not mutate
  GitHub.
- Validation tools clearly disclose process execution.
- No tool posts, pushes, comments, opens issues, or creates PRs in v1.

## 14. Configuration

TOML is the initial configuration format.

Representative lens:

```toml
[lenses.active_go]
kinds = ["issue"]
states = ["open"]
languages = ["Go"]
exclude_archived = true
unassigned = true
updated_within = "30d"
min_stars = 20
max_results_per_repo = 3

[lenses.active_go.weights]
text_relevance = 0.25
repository_activity = 0.15
maintainer_responsiveness = 0.20
external_pr_merge_rate = 0.15
issue_clarity = 0.15
freshness = 0.10

[lenses.active_go.hydration]
promote_top = 25
facets = ["comments", "repository_health", "community"]
max_api_requests = 300
```

Configuration may define:

- paths and storage budgets;
- GitHub authentication sources;
- source definitions;
- event allowlists;
- crawl schedules and API budgets;
- hydration defaults;
- embedding providers and models;
- lenses and collections;
- validation execution policy;
- export redaction policy.

## 15. Security, privacy, and trust boundaries

### 15.1 GitHub data

- Public data can still contain personal information and secrets accidentally
  posted by users.
- Private repositories are out of initial scope unless a separate private-data
  design is approved.
- Raw payloads and embeddings require explicit retention policy.
- Exports must strip tokens, credentials, local paths where appropriate, and
  other configured sensitive fields.

### 15.2 Credentials

- Tokens come from environment, keyring, or GitHub CLI authentication.
- Tokens never enter SQLite documents, logs, JSON output, prompts, or exports.
- HTTP logging redacts authorization and sensitive query values.

### 15.3 Repository code

- Cloned code is untrusted.
- Indexing reads files but does not execute them.
- Validation execution is a separate explicit capability.
- Host execution requires confirmation and a visible command.
- Future unattended execution must use an intentionally designed isolation
  boundary with bounded filesystem, network, process, time, and resource
  access.

### 15.4 External mutations

- GitHub remains read-only in v1.
- Git push, issue creation, PR creation, review submission, and comments are
  outside the initial application contract.
- Future mutations require distinct tools, explicit authorization, dry-run
  material, and current live-state verification.

## 16. Reliability and performance requirements

- Interrupted crawls resume without duplicating canonical entities.
- Replaying a source partition is safe.
- Search remains available while crawl jobs run.
- SQLite writes use transactions and bounded connection behavior.
- Stale observations cannot overwrite newer current state.
- Replace-all child snapshots are applied only when complete.
- API retries are bounded and observable.
- Search queries have default and hard result limits.
- Code indexing has default and hard byte/file limits.
- Embedding and clustering are opt-in and independently retryable.
- Expensive derived artifacts record their source snapshot and can be rebuilt.
- The CLI and MCP return the same domain facts for equivalent operations.

## 17. Technology and libraries

Versions below are reference versions observed on 2026-07-16. Initial
scaffolding should pin exact versions and update them only with validation.

### 17.1 Core language

- Go 1.26.x.
- Standard library for contexts, HTTP, gzip, JSON, subprocesses, and testing.

### 17.2 Required or expected dependencies

| Area | Library | Reference version | Purpose |
|---|---|---:|---|
| MCP | `github.com/modelcontextprotocol/go-sdk` | v1.6.1 | Official MCP server/client SDK |
| GitHub REST | `github.com/google/go-github/v89/github` | v89.0.0 | Typed REST API client and response metadata |
| SQLite | `modernc.org/sqlite` | v1.54.0 | Pure-Go SQLite driver |
| SQL generation | `sqlc` | v1.31.1 | Generate typed Go code from SQL |
| Migrations | `github.com/pressly/goose/v3` | pin at scaffold | Transactional, versioned SQL migrations |
| CLI | `github.com/alecthomas/kong` | v1.16.0 | Structured CLI parsing |
| TOML | `github.com/pelletier/go-toml/v2` | v2.4.3 | Configuration encoding |
| Keyring | `github.com/zalando/go-keyring` | v0.2.8 | Platform credential storage |
| Rate limiting | `golang.org/x/time/rate` | pin at scaffold | API request pacing |
| Concurrency | `golang.org/x/sync` | pin at scaffold | Errgroups and duplicate-work suppression |
| Comparison | `github.com/google/go-cmp` | pin at scaffold | Precise test assertions and diffs |

### 17.3 TUI dependencies

The initial TUI should use the mature Charmbracelet stack if and when the TUI
enters scope:

- `github.com/charmbracelet/bubbletea` v1.3.10;
- `github.com/charmbracelet/bubbles` v1.0.0;
- `github.com/charmbracelet/lipgloss`.

Bubble Tea v2.0.8 is current as of this draft. Adopt one major version
deliberately and keep the TUI behind an adapter; do not copy Gitcrawl's TUI
architecture merely to preserve familiarity.

### 17.4 Optional dependencies

- `github.com/shurcooL/githubv4` for GitHub GraphQL if a direct typed wrapper is
  insufficient.
- `github.com/openai/openai-go/v3` for an optional OpenAI embedding or summary
  provider.
- Native `git` for complete contribution semantics.
- `rg` for fast checkout search.
- Tree-sitter for optional multi-language syntax indexing.
- SCIP for optional symbol/reference indexes.
- Semgrep and language-native analyzers as external validation adapters.
- Docker or Podman for a future constrained validation runner.

### 17.5 Libraries and systems to avoid initially

- An ORM over SQLite.
- A separate search server such as Elasticsearch or Meilisearch.
- A separate vector database.
- Replacing native Git worktree/diff behavior with a partial Git
  implementation.
- A distributed queue before local durable frontier requirements are proven.
- A web framework before a hosted product is in scope.

### 17.6 Dependency adoption rules

Before writing infrastructure code:

1. Search the Go standard library and maintained packages for the behavior.
2. Verify the current API and behavior against official documentation and
   source, not memory or generated summaries.
3. Prefer a focused dependency with a stable contract over a broad framework.
4. Keep external types inside adapters; expose product-owned domain contracts.
5. Pin versions, record licenses, and run vulnerability and update checks in
   CI.
6. Add a contract or integration test for behavior on which correctness
   depends.

Do not introduce a library for a trivial helper, and do not hand-roll protocol,
database migration, authentication, terminal, or concurrency behavior when a
well-maintained implementation already fits.

## 18. Independent implementation and source adoption

### 18.1 Relationship to Gitcrawl and Crawlkit

- GitContribute is not a Gitcrawl fork.
- GitContribute does not import Gitcrawl or Crawlkit as Go dependencies.
- GitContribute does not preserve their database, CLI, package, cloud, or
  release compatibility.
- Gitcrawl and Crawlkit are prior art and sources of concrete behavior,
  failure cases, tests, and selectively adaptable MIT-licensed code.
- The baseline studied for this design is Gitcrawl commit
  `a24bf74235f580758bc57e8ddca60ea2f51ad0c7` and Crawlkit v0.14.2.

### 18.2 Source adoption policy

Use this order for each capability:

1. Adopt an appropriate standard-library facility.
2. Adopt a mature maintained library behind a narrow adapter.
3. Implement the product-specific behavior directly.
4. Adapt a bounded portion of Gitcrawl or Crawlkit source when it is already
   well-tested and materially reduces correctness risk.

Direct source adoption must be an explicit decision, not a bulk import. The
adopted unit must have a clear owner boundary, no hidden dependency on upstream
internals, and tests that describe the behavior we intend to retain.

### 18.3 Requirements for directly adapted code

For copied or substantially adapted code:

- Retain notices required by the license with the adapted source or under
  `LICENSES/`.
- Place the code behind product-owned contracts, keep relevant tests, and
  remove unrelated upstream behavior.
- Do not require an automated subtree, vendor, or synchronization workflow.

Gitcrawl is MIT licensed, copyright 2026 OpenClaw. Crawlkit is MIT licensed,
copyright 2026 Vincent Koc. This specification is not a substitute for a
release-time license audit.

### 18.4 Gitcrawl behaviors used as design references

- SQLite repository/thread archive.
- Incremental issue/PR sync.
- Comment, review, and PR-detail hydration.
- Revisions, fingerprints, and observation ordering.
- Coverage and run records.
- Keyword, semantic, and hybrid search.
- Code document indexing.
- Related-thread and cluster behavior.
- Optional source-grounded thread and repository summaries with recorded
  provider, model, source revision, and generation time. Summaries must not
  upgrade a hypothesis or evidence status.
- Durable local cluster governance.
- TUI interaction model.
- Platform paths, config, status, metadata, and diagnostics.

These are behavioral references, not promises to retain Gitcrawl's
implementation or public contracts.

### 18.5 Excluded coupling

- Gitcrawl module or package imports.
- Crawlkit module or package imports.
- A Git fork relationship or requirement to merge upstream releases.
- Gitcrawl schema migration or CLI compatibility.
- Bulk source import, vendored source trees, or generated compatibility
  wrappers.
- Tests that exist only to preserve upstream behavior irrelevant to this
  product.

### 18.6 Features deferred for explicit design

- Cloud snapshot publication.
- Remote OAuth and authorization.
- D1/R2 remote corpus operation.
- Remote retention/deletion policy.
- Hosted multi-user behavior.

## 19. Prior art and relevant repositories

### 19.1 Closest architectural references

- [OpenClaw Gitcrawl](https://github.com/openclaw/gitcrawl): local-first
  GitHub issue/PR archive, hydration, FTS, embeddings, clustering, TUI, and
  portable/remote concepts.
- [OpenClaw Crawlkit](https://github.com/openclaw/crawlkit): shared crawling,
  configuration, storage, embedding, vector, progress, and remote primitives.
- [ecosyste.ms Repos](https://github.com/ecosyste-ms/repos): broad repository
  metadata ingestion, incremental refresh, and staged repository hydration.
- [ecosyste.ms Issues](https://github.com/ecosyste-ms/issues): GitHub issue/PR
  metadata, GH Archive ingestion, batched upserts, and per-repository refresh.
- [ecosyste.ms Timeline](https://github.com/ecosyste-ms/timeline): multi-billion
  public GitHub event timeline built from GH Archive.
- [OSS Insight](https://github.com/pingcap/ossinsight): GH Archive and GitHub
  API ingestion for large-scale GitHub analytics.

### 19.2 Contribution and mining references

- [Good First Issue](https://www.goodfirstissue.org/about): scheduled GitHub
  repository/issue indexing and beginner-oriented contribution discovery.
- [GrimoireLab Perceval](https://github.com/chaoss/grimoirelab-perceval):
  mature repository, issue, and PR extraction tooling.
- [Augur](https://github.com/augurlabs/augur): large-scale open-source
  repository collection and health metrics.
- [Issue Metrics](https://github.com/github-community-projects/issue-metrics):
  focused response-time and participation metrics for issues, PRs, and
  discussions.
- [OpenSSF Scorecard](https://github.com/ossf/scorecard): transparent,
  evidence-linked repository security-health checks and scoring conventions.
- [GH Archive](https://www.gharchive.org/): hourly archives of the public
  GitHub event timeline and BigQuery dataset.

### 19.3 Code intelligence and validation references

- [SCIP](https://github.com/scip-code/scip): language-neutral code intelligence
  protocol and indexes for definitions and references.
- [Tree-sitter](https://github.com/tree-sitter/tree-sitter): incremental parser
  infrastructure for optional multi-language syntax indexing.
- [Semgrep](https://github.com/semgrep/semgrep): established syntax-aware
  static-analysis engine suitable as an external validation adapter.

### 19.4 Library references

- [Official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [Official MCP TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
- [Official MCP Rust SDK](https://github.com/modelcontextprotocol/rust-sdk)
- [go-github](https://github.com/google/go-github)
- [sqlc](https://github.com/sqlc-dev/sqlc)
- [modernc SQLite](https://gitlab.com/cznic/sqlite)
- [Kong](https://github.com/alecthomas/kong)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [go-git](https://github.com/go-git/go-git), as an optional object-inspection
  reference rather than the default contribution Git implementation.
- [OpenAI Go](https://github.com/openai/openai-go)

## 20. Official protocol and API references

- [GitHub REST API](https://docs.github.com/en/rest)
- [GitHub Search REST API](https://docs.github.com/en/rest/search/search)
- [GitHub repository REST API](https://docs.github.com/en/rest/repos/repos)
- [GitHub issues REST API](https://docs.github.com/en/rest/issues/issues)
- [GitHub pull requests REST API](https://docs.github.com/en/rest/pulls/pulls)
- [GitHub events REST API](https://docs.github.com/en/rest/activity/events)
- [GitHub REST API rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api)
- [GitHub REST API best practices](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api)
- [GitHub GraphQL API](https://docs.github.com/en/graphql)
- [GitHub GraphQL rate and query limits](https://docs.github.com/en/graphql/overview/rate-limits-and-query-limits-for-the-graphql-api)
- [GitHub CLI manual](https://cli.github.com/manual/)
- [MCP specification 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25)
- [SQLite FTS5](https://www.sqlite.org/fts5.html)
- [Git worktree](https://git-scm.com/docs/git-worktree)

## 21. Delivery plan

### Phase 0: Baseline and architecture

- Establish repository, license notices, module, CI, formatting, and release
  skeleton.
- Start from a clean repository with no Gitcrawl or Crawlkit module dependency.
- Define product-owned corpus, crawl, application-service, CLI, and MCP
  contracts.
- Implement the smallest complete local corpus path using mature libraries.
- Use Gitcrawl tests and behavior as references for edge cases, adopting source
  only through the policy in Section 18.

Exit criteria:

- The module builds and tests without Gitcrawl or Crawlkit source or services.
- Corpus operations can be called without invoking CLI or MCP parsing.
- CLI, MCP, application, and persistence layers have explicit boundaries.
- Directly adapted source, if any, retains required license notices and
  relevant tests.

### Phase 1: Local archive kit

- Product-owned CLI.
- SQLite schema, observations, projections, and migrations.
- Repository sync and selective hydration.
- FTS/code search, coverage, runs, related threads, and clusters.
- Status, metadata, diagnostics, and JSON contracts.

Exit criteria:

- A repository can be synced, searched, hydrated, and inspected entirely
  through application services and CLI.
- Search performs no hidden network access.

### Phase 2: Discovery and frontier

- GitHub Search source.
- Query partitioning and overlap refresh.
- Explicit repository sources.
- Persistent frontier, budgets, leases, retries, and cancellation.
- Initial GH Archive hourly ingestion.

Exit criteria:

- A configured source can discover repositories/threads and resume safely.
- Replaying a source does not duplicate canonical entities.
- Oversized Search queries are split rather than silently truncated.

### Phase 3: Dossiers and lenses

- Repository metadata/health facets.
- Dossier builder and exports.
- Recent merged/closed/open PR seed extraction.
- Lenses, named signals, score explanation, and selective promotion.

Exit criteria:

- A user can rank a broad shallow corpus, explain each result, and build a
  source-backed dossier for selected repositories.

### Phase 4: Investigations and opportunities

- Investigation, hypothesis, source-link, duplicate, collision, and
  opportunity lifecycle models.
- Collections and local feedback.
- TUI views for opportunities and investigations.

Exit criteria:

- An unreported code finding and a GitHub issue can both become traceable
  opportunities with explicit status and source evidence.

### Phase 5: Workspaces, validation, and evidence

- Managed clones/worktrees.
- Explicit validation definitions and runs.
- Base-versus-candidate comparison.
- Evidence packets and proof-gap reporting.
- Initial constrained execution policy.

Exit criteria:

- A bug investigation can demonstrate a base failure and candidate success
  with reproducible recorded evidence.

### Phase 6: Contribution preparation

- Issue and PR preparation from actual evidence and repository templates.
- Diff review and merged-PR convention references.
- Outcome tracking.

Exit criteria:

- A prepared contribution contains only verified claims and can be reviewed
  without private investigation context.

### Phase 7: MCP

- Read-only search and resource surface.
- Structured outputs and opaque pagination.
- Crawl, hydration, dossier, investigation, workspace, and validation tasks.
- Accurate tool annotations and cancellation.

Exit criteria:

- An MCP client can execute the repository-audit workflow using the same
  application services and facts as the CLI.
- No MCP read tool causes hidden network access or process execution.

### Later phases

- Bubble Tea v2 migration.
- Syntax/symbol analyzer adapters.
- Containerized unattended validation.
- Portable stores and sanitized snapshots.
- Authenticated remote read service.
- Separately authorized GitHub write operations, if ever approved.

## 22. v1 acceptance criteria

The first product release is acceptable when:

- It installs as a Go CLI on Linux and macOS, with a documented Windows plan
  or tested Windows support before claiming it.
- It initializes and migrates a local SQLite corpus safely.
- It discovers repositories/threads through at least GitHub Search and explicit
  repository sources.
- It ingests a bounded GH Archive time window idempotently for configured event
  types.
- It incrementally syncs issues and PRs and selectively hydrates comments,
  reviews, and PR details.
- It indexes and searches repository conversations and code locally.
- It reports freshness and facet coverage.
- It builds a source-backed repository dossier.
- It records investigations, hypotheses, opportunities, and evidence.
- It checks duplicate issues/PRs and competing open work.
- It creates an isolated worktree and records explicit validation runs.
- It prepares issue/PR drafts without posting them.
- It exposes the core read operations and long-running jobs through MCP.
- Equivalent CLI and MCP requests return equivalent domain data.
- No local search silently accesses GitHub.
- No command executes repository-controlled code or mutates GitHub without an
  explicit capability and authorization.
- The dependency graph contains no Gitcrawl or Crawlkit module dependency.
- Directly adapted third-party code, if any, retains required license notices.

## 23. Open design questions

- How much raw GH Archive and GitHub payload data should be retained locally?
- What default time window creates a useful discovery corpus without excessive
  storage or bandwidth?
- Which repository-health signals are sufficiently reliable to ship as
  defaults?
- Should semantic embeddings remain limited to promoted repositories?
- What stable query/result schema should be shared by CLI JSON and MCP?
- What is the minimum safe execution boundary for unattended validation?
- Should Discussions be a first-class thread type in v1 or a later facet?
- When should Bubble Tea v2 be adopted?
- What project name, command name, module path, and public license should be
  finalized before implementation?

## 24. Immediate next decisions

Before implementation begins:

1. Confirm the product and binary name.
2. Choose the public module path and license.
3. Confirm the initial mature dependency set.
4. Define the first vertical slice:
   `init -> sync one repo -> local search -> dossier -> MCP read`.
5. Write architecture decision records for the corpus boundary, application
   service boundary, and execution safety boundary.

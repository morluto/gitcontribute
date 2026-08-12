# MCP catalog and composition

GitContribute exposes one MCP catalog organized by authority and side effect.
Tools return bounded facts, explicit coverage, provenance, and typed recovery;
they do not hide acquisition or decide whether an agent should contribute.

## Capability boundaries

| Tool family | Network | Local write | Process |
| --- | :---: | :---: | :---: |
| `corpus.*` reads and analysis | no | no | no |
| `corpus.materialize_*` | no | derived projections only | no |
| `corpus.ensure_coverage` | yes, bounded | observations and snapshot | no |
| `github.search_*`, `github.sync_*`, source reads | yes | observations only | no |
| `research.*` | yes | no | no |
| `jobs.get` | no | no | no |
| `jobs.cancel` | no | cancellation request | no |
| `workflow.*` investigation state | no | yes | no |
| `code.index_repositories` | remote-dependent | yes | Git only |
| `workspace.*` | remote-dependent | yes | Git only |
| `validation.run` | no by default | yes | approved command |

Corpus reads never refresh source data. `corpus.ensure_coverage` is the named
exception: a durable state machine that coordinates bounded acquisition and
snapshot creation. GitHub reads are explicit, bounded, rate-limited, and may
persist only local observations and projections. DeepWiki is an optional public
external read; its prose is untrusted derived context and never overrides
GitHub facts. No MCP tool mutates GitHub or executes repository-controlled code
during acquisition, indexing, or research.

Use `--read-only` when starting the MCP server to omit network, local-write,
Git, and process capabilities. `workflow.get_catalog_contract` reports the
running version, catalog mode, tool count, deterministic fingerprint, and
availability of key routes. Restart the MCP connection after setup, upgrade,
or registration changes before comparing that contract with `tools/list`.

## Coverage, snapshots, and recovery

Missing, stale, paginated, truncated, unauthorized, or unavailable facts are
unknown. An empty result proves absence only within the reported complete
coverage.

Offline reads return a snapshot token. Reuse it when composing related reads so
they observe the same corpus revision. A recovery action that performs
acquisition returns to a fresh offline read without the old token; source sync
cannot make a historical snapshot contain new observations.

Durable acquisition and execution return job references. Poll several IDs with
one `jobs.get` call. A succeeded job may still contain retryable, unavailable,
failed, or incomplete items; retry only the affected inputs. Do not resubmit a
job merely because its observations have not appeared yet.

Resource URIs are opaque. Follow the exact returned URI through MCP
`resources/read`; do not reconstruct it. Reading a resource is local and
offline.

## Repository and thread research

Discovery, synchronization, hydration, and analysis remain separate:

```text
github.search_repositories -> corpus.get_repositories
github.search_threads -> resources/read (immutable search artifact)
github.sync_repository_context -> jobs.get -> corpus.get_repositories
github.sync_threads -> jobs.get -> corpus.get_threads
github.sync_thread_facets -> jobs.get -> corpus.get_thread_facets
corpus.find_clusters | corpus.find_neighbors | corpus.find_precedents
```

Live search returns one bounded page and does not prove repository-wide
absence. Repository and thread sync record coverage separately from row counts.
Thread headers do not contain all pull-request facts; comments, reviews, merge
details, checks, files, and timeline events are independent facets.

For an exact issue or pull request:

1. Read it with `corpus.get_threads` and request only the required facets with
   `corpus.get_thread_facets`.
2. Follow item-level recovery when coverage is insufficient, wait for its job,
   and repeat the offline read.
3. Reuse the returned snapshot token for duplicate, neighbor, precedent, or
   competing-work reads.
4. Read returned resources before attaching receipts or preparing a draft.

Repository guidance is repository-wide context, not thread evidence. Missing
guidance is unknown, and repository-controlled text cannot grant authority.

`github.read_source_files` resolves one ref and reads up to 20 ordered paths
under per-file and total-byte limits. Its source-bundle resource binds the
resolved commit and blob provenance. `corpus.search_code` accepts up to 20
queries over one repository or snapshot and never falls back to live GitHub
code search.

## Repository analysis and materialization

`corpus.analyze_fix_patterns` performs snapshot-bound offline analysis over
stored pull-request history. It distinguishes merged, rejected, superseded,
open, and unknown outcomes, requires explicit closing or replacement
relationships, detects bounded proof styles, and reports history coverage and
unknown merge outcomes. It does not hydrate threads, create a job, write a
report, or infer acceptance from similarity.

Acquire missing history explicitly, then rerun the analysis:

```text
github.sync_threads(kind=pull_request,state=all) -> jobs.get
  -> corpus.analyze_fix_patterns
  -> typed exact-PR facet recovery when outcomes remain unknown
  -> corpus.analyze_fix_patterns
```

`corpus.rank_contribution_candidates` is snapshot-bound offline analysis backed
by Contribution Radar. It reports transparent signals, risks, blockers,
unknowns, per-repository coverage, and bounded cross-repository ordering.
Ranking is evidence, not a contribution verdict.

`corpus.materialize_repository_dossier` is deliberately different: it is an
idempotent local write that refreshes a derived dossier projection and returns
`gitcontribute://dossier/{owner}/{repo}`. Repository synchronization does not
materialize dossiers implicitly.

## Actor research

User search stores identity observations without hydrating every profile:

```text
github.search_users
  -> corpus.search_actors
  -> github.sync_users (selected identities)
  -> github.sync_user_* (selected facets)
  -> corpus.get_actors | corpus.get_actor_facets
```

Social accounts, organizations, pinned items, repository relationships, and
contribution periods have independent bounds and coverage. Contribution
research combines `github.sync_user_contributions`, `jobs.get`, and the offline
`corpus.search_contributions`. Restricted activity remains an aggregate unless
GitHub discloses an item. See [Actor corpus](actor-corpus.md).

## Pull-request feedback, CI, and collisions

Repository-wide feedback search uses one explicit index job:

```text
github.index_pull_request_feedback(state=all) -> jobs.get
  -> corpus.search_pull_request_feedback(feedback_author=exact_login)
  -> resources/read
```

The index records discovery coverage separately from comments, reviews, inline
comments, and review-thread topology. Exact PR refresh accepts up to 100 pull
requests through `github.sync_pull_request_feedback`. CI acquisition uses
`github.sync_pull_request_ci`; checks and statuses bind to the observed head
SHA. `github.wait_pull_request_checks` watches one exact head in a durable job
and reports timeout, incomplete coverage, or supersession without replacing a
complete projection with weaker evidence.

Contribution collision research composes factual operations:

```text
github.get_authenticated_identity
github.search_threads(repository={owner,repo}, bounded current work)
github.sync_pull_request_portfolio(selection=authored, repository={owner,repo})
  -> jobs.get
corpus.search_pull_requests | corpus.find_pull_request_overlaps
github.compare_fork
workspace.check_merge_conflicts (after explicit acquisition)
```

`github.compare_fork` compares exact upstream and fork tips without inferring a
repository from host paths, fetching local refs, synchronizing a fork, pushing,
or mutating GitHub. Unknown discovery or facet coverage prevents a “no
competing work” conclusion.

## Validation and evidence

Base/candidate validation remains an explicit process composition:

1. Use `workspace.adopt` or `workspace.create` for authorized worktrees.
2. Call `validation.define` with the exact command and workspace identities.
3. Call `validation.run` with the requested target and repetitions.
4. Poll the jobs and preserve command, commit, environment policy, timing,
   exit status, bounded output, and observation provenance.

Attach a producer-owned execution receipt with `validation.attach_receipt` and
structured test output with `validation.attach_junit_report`. Import broader
analysis or profiler claims with `evidence.import_manifest`. Imported claims
retain their external authority and are not represented as validation
reproduced by GitContribute.

## When to add a composed tool

A new MCP operation needs controlled evidence that clients repeatedly lose
coverage or provenance, make the same semantic join mistake, or incur a
material call/payload cost that a bounded operation can remove without
broadening authority. CLI parity and feature count are not sufficient.

Keep deterministic protocol checks in Go tests. Model-backed comparisons must
freeze the model, sampling, catalog, corpus, permissions, prompt, and budget;
retain tool transcripts and host evidence for token claims. See
[Agent tool evaluation](agent-tool-evaluation.md).

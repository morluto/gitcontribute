# Repository working agreements

## What we are building

GitContribute is a local-first research and validation workbench for coding
agents. It gives agents durable repository and GitHub facts, bounded acquisition
and workspace capabilities, and source-bound validation evidence. Agents do the
reasoning and contribution work; GitContribute provides the trustworthy local
context and explicit capabilities they need. It does not hide network access,
process execution, local writes, or GitHub mutation inside reads.

Treat missing, stale, incomplete, or superseded coverage as unknown rather than
as negative evidence. Keep imported external claims distinct from validation
reproduced by GitContribute. For the detailed contracts, read
`docs/architecture.md`, `CONTRIBUTING.md`, and, for MCP or evidence changes,
`docs/mcp-composed-workflows.md` and `docs/external-evidence-manifests.md`.

## Product boundaries

- Follow the contracts in `docs/architecture.md`; do not preserve Gitcrawl
  compatibility.
- Do not import Gitcrawl or Crawlkit modules.
- Prefer the standard library or mature maintained packages over custom
  protocol, migration, authentication, concurrency, and terminal machinery.
- Keep third-party types inside adapters. Domain and application contracts are
  owned by this repository.
- When copying third-party source, preserve notices required by its license
  with the adapted source or under `LICENSES/`.

## Side effects

- Corpus reads must not perform network access.
- GitHub access, local writes, process execution, and external mutations are
  separate capabilities.
- No GitHub mutation is in scope unless explicitly approved.
- Never execute repository-controlled code during crawling or indexing.

## Engineering rules

- Keep packages focused and interfaces narrow; avoid config bags and mode
  booleans.
- Pass `context.Context` through I/O and long-running operations.
- Make acquisition, sync, indexing, and projection writes idempotent; prevent
  stale observations from replacing newer projections.
- Replace child snapshots atomically only after complete retrieval.
- Run `gofmt` on changed Go files and focused tests while working; run
  `make verify` before opening a pull request.
- Add focused regression tests for ordering, resumability, cancellation, and
  side-effect boundaries.

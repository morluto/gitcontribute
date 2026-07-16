# Repository working agreements

## Product boundaries

- Implement the behavior in `SPEC.md`; do not preserve Gitcrawl compatibility.
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

## Go changes

- Keep packages focused and interfaces narrow; avoid config bags and mode
  booleans.
- Pass `context.Context` through I/O and long-running operations.
- Make crawl writes idempotent and prevent stale observations from replacing
  newer projections.
- Replace child snapshots atomically only after complete retrieval.
- Run `gofmt` on changed Go files and `go test ./...` before committing.
- Add focused regression tests for ordering, resumability, cancellation, and
  side-effect boundaries.

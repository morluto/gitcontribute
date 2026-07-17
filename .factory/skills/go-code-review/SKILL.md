---
name: go-code-review
description: Review Go code changes in gitcontribute, checking for storage invariants, side-effect boundaries, and testing coverage.
---

## Purpose

Review Go code changes in the gitcontribute repository. This skill enforces the
repository's working agreements from AGENTS.md and CONTRIBUTING.md.

## Review Guidelines

### Side effects
- Corpus reads must not perform network access.
- GitHub access, local writes, process execution, and external mutations are separate capabilities.
- No GitHub mutation unless explicitly approved.
- Never execute repository-controlled code during crawling or indexing.

### Go conventions
- Keep packages focused and interfaces narrow.
- Pass `context.Context` through I/O and long-running operations.
- Make crawl writes idempotent; prevent stale observations from replacing newer projections.
- Replace child snapshots atomically only after complete retrieval.

### Storage invariants
1. Record source observations even when an older observation loses projection ordering.
2. Update projections only when `(source_updated_at, observation_sequence)` wins.
3. Buffer paginated child data and replace it atomically only after retrieval completes.
4. Treat an empty complete child set as a valid replacement.
5. Give paginated queries a stable final tie-breaker and bind cursors to their query scope.
6. Keep multi-record imports and state transitions transactional.

### Testing
- Focused regression tests for ordering, resumability, cancellation, and side-effect boundaries.
- Use local HTTP servers, temporary repositories, and temporary databases.
- Do not depend on live GitHub state in the test suite.

# Runbooks

## Health Check

```sh
gitcontribute health
```

Checks SQLite database integrity, GitHub API connectivity, and local filesystem state.

## Deployment Observability

**CI Pipeline**: https://github.com/morluto/gitcontribute/actions

Monitor the CI workflow for build status, test coverage trends, and lint results.
Coverage reports are uploaded as artifacts on each run.

**Release Dashboard**: https://github.com/morluto/gitcontribute/releases

Track version history and release notes. Each release is built via GoReleaser
with cross-platform binaries and checksums.

## Database Integrity

If SQLite corruption is detected:

1. Stop all running gitcontribute processes
2. Run integrity check: `sqlite3 ~/.gitcontribute/corpus.db "PRAGMA integrity_check;"`
3. If corruption confirmed, restore from latest backup: `cp ~/.gitcontribute/corpus.db.bak ~/.gitcontribute/corpus.db`
4. Re-run health check

## Rate Limiting

If GitHub API rate limits are hit:

1. Check current limits: `gh api /rate_limit`
2. Wait for the reset window (shown in `X-RateLimit-Reset` header)
3. Reduce concurrent operations via `--concurrency` flag

## Circuit Breaker

The GitHub client uses a circuit breaker that opens after 5 consecutive failures.
When the circuit is open, all requests fail fast with `ErrCircuitOpen` rather
than retrying. After a 30-second cooldown, a single probe request is allowed.
If the probe succeeds, the circuit closes; if it fails, the circuit re-opens.

To check circuit status, enable debug logging:
```sh
GITCONTRIBUTE_LOG_LEVEL=debug gitcontribute sync owner/repo
```

## Job Reconciliation

If jobs appear stuck:

1. List active jobs: `gitcontribute jobs list --status running`
2. Check for lock conflicts: `gitcontribute jobs reconcile`
3. Force-release stale locks if the owning process is confirmed dead

## Migration Failures

If a Goose migration fails:

1. Check the migration version: `gitcontribute db version`
2. Rollback the failed migration: `goose -dir internal/corpus/migrations down`
3. Fix the migration SQL
4. Re-apply: `goose -dir internal/corpus/migrations up`

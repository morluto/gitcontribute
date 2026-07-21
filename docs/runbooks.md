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

1. Stop all running GitContribute and MCP processes.
2. Run `gitcontribute doctor --strict`. A timeout warning is not proof of
   corruption; an actual SQLite quick-check error is.
3. Inspect the candidate backup independently.
4. Restore through the supported command, which first creates a safety backup:

   ```sh
   gitcontribute corpus restore /safe/path/corpus.db --yes
   ```

5. Run `gitcontribute corpus inspect` and `gitcontribute doctor --strict`.

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

Do not run Goose directly against a user corpus. Use the product-owned lifecycle:

1. Stop running MCP processes and inspect without mutation:
   `gitcontribute corpus inspect --json`.
2. Preserve the backup path and checksum printed by the failed migration.
3. Fix the migration in a newer binary; never edit an already released
   migration in place.
4. Retry with `gitcontribute corpus migrate --yes`.
5. If recovery requires returning to the pre-migration database, use
   `gitcontribute corpus restore BACKUP --yes`. Reinstalling an older binary
   alone cannot roll back an advanced schema.

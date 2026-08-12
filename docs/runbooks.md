# Operational runbooks

These procedures use supported GitContribute commands. Run commands with
`--json` when the result will be inspected by automation.

## Diagnose a local installation

```sh
gitcontribute metadata --json
gitcontribute status --json
gitcontribute doctor --strict --json
```

`doctor` checks local configuration, the corpus, Git, and installed MCP
registrations. It does not synchronize repositories. A diagnostic timeout is a
warning unless a required check reports an error.

After setup or upgrade changes an MCP registration, restart the affected client
and call `workflow.get_catalog_contract` in the new session. Compare the server
version, mode, and catalog fingerprint with the expected installation.

## Inspect repository health

Repository health is an offline computation over stored observations:

```sh
gitcontribute health owner/repo --json
gitcontribute coverage owner/repo --json
```

Incomplete or stale coverage remains unknown. Synchronize the required source
facts explicitly before rerunning health; the health command itself never
contacts GitHub.

## Recover a durable job

List and inspect jobs without changing them:

```sh
gitcontribute jobs list --status running --json
gitcontribute jobs get JOB_ID --json
```

Request cancellation only when the operation should stop:

```sh
gitcontribute jobs cancel JOB_ID --json
```

Cancellation is cooperative and bounded. Inspect the job again to confirm its
terminal state. There is no manual lock-release or job-reconciliation command;
restart recovery is owned by the application.

## Diagnose GitHub acquisition failures

GitHub acquisition is explicit. First inspect the returned typed error and
item-level recovery. For provider rate limits, verify the authenticated
account's live limit independently when `gh` is configured:

```sh
gh api /rate_limit
```

Wait for the reported reset or retry only the affected bounded inputs. Reduce a
command's documented request/page bounds rather than launching parallel manual
chunks. Repeated provider failures may open the in-process circuit breaker for
30 seconds; retries during that interval fail fast. Debug logging can show the
request and retry boundary:

```sh
GITCONTRIBUTE_LOG_LEVEL=debug gitcontribute archive sync owner/repo
```

## Recover from database corruption

Do not replace a corpus until an actual SQLite check reports corruption. A
timeout warning alone is not proof.

1. Stop GitContribute CLI, TUI, and MCP processes using the corpus.
2. Run `gitcontribute doctor --strict --json` and
   `gitcontribute corpus inspect --json`.
3. Inspect the proposed backup independently.
4. Restore it through the supported lifecycle; restore first creates a safety
   backup of the current database:

   ```sh
   gitcontribute corpus restore /safe/path/corpus.db --yes
   ```

5. Repeat `corpus inspect` and `doctor --strict`.

## Recover from a schema migration failure

Never run Goose directly against a user corpus or edit an already released
migration.

1. Stop processes holding corpus leases.
2. Inspect without mutation: `gitcontribute corpus inspect --json`.
3. Preserve the backup path and checksum reported by the failed migration.
4. Fix the migration in a newer binary and retry:

   ```sh
   gitcontribute corpus migrate --yes
   ```

5. To return to the pre-migration database, restore the verified backup:

   ```sh
   gitcontribute corpus restore BACKUP --yes
   ```

Installing an older binary does not roll back an advanced schema.

## Inspect or remove stored data

Inspect scope before any deletion:

```sh
gitcontribute corpus list --json
gitcontribute corpus inventory owner/repo --json
gitcontribute corpus prune-code owner/repo
gitcontribute corpus remove-repository owner/repo
```

The prune and repository-removal commands show a plan by default. Review it,
then use their documented confirmation flag when deletion is intended. Code
pruning removes derived snapshots, not GitHub observations.

## Release failures

Releases are tag-triggered. Inspect the GitHub Actions run and the matching
GitHub release. One tag version must agree across the Go binaries, npm package,
`server.json`, MCP Registry metadata, and GitHub release. Do not repair a
partially published release by changing an immutable tag; fix the workflow and
publish a new version.

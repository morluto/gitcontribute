# Concurrency and ranking follow-up — 2026-09-08

## Need for jobs and SDK support

GitContribute already uses the latest stable official Go MCP SDK, **v1.7.0**
(released July 28). The newest prerelease checked was **v1.8.0-pre.2** (September 4).
Both the Go module registry and GitHub release metadata were checked, and both
versions' source was inspected. Neither supplies a task runtime or `TaskStore`
that can replace the application executor. Context7 had no matching task API
documentation, so this conclusion rests on the tagged source and official
release documentation, not on a missing search result alone.
[Stable release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0),
[prerelease](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0-pre.2).

The current Tasks extension standardizes durable handles, polling, results, and
cooperative cancellation. Servers still own durable creation and execution;
clients must opt into the extension. Protocol support would replace an adapter
surface, not SQLite persistence, admission limits, or process ownership.
Retain the executor for acquisition, indexing, and validation; ordinary bounded
reads stay synchronous. No second task implementation or prerelease upgrade was
added. The prerelease's cancellation and teardown fixes merit a separate upgrade
evaluation when adopting it.
[Tasks extension](https://modelcontextprotocol.io/extensions/tasks/overview).

## Reproduced defects and corrections

| Case | Observed before correction | Corrected behavior |
| --- | --- | --- |
| Abandoned queue | A single-slot executor had one running job and a queued job. After another corpus connection aged and reconciled its owner, the queued job remained queued forever. | Persist owner and queued row together. Reconcile queued and running jobs for missing/stale owners, while preserving live-owner work. |
| Reconciled worker | Reconciliation marked running work failed, but its callback remained active because the cancellation poll only checked cancellation timestamps and missing rows. | Terminal durable state also stops work. Missing-owner heartbeats stop the executor and further admission; terminal results cannot be overwritten. |
| Unrelated ranked evidence | An exact match, weak positive match, and disjoint title produced three findings. Competing-PR matches were automatically contradicting evidence. | Remove exact zero-score findings while preserving weak positive candidates and their order. Similarity produces inconclusive evidence, including competing PR candidates. |
| Hidden candidate cap | Among 201 threads, an older exact match fell outside the first 200 candidates. The resulting empty findings claimed complete status. | Propagate candidate truncation independently from finding count. Increasing the limit recovered the exact match; at the hard maximum, recovery does not suggest a smaller/repeated limit. |

The first three cases failed on the starting production code; the candidate-cap
case failed after zero-score filtering and before bound propagation. Focused
regressions passed after correction. Additional tests cover refusal to replace
a queued job's owner, loss of owner stopping admission, and preservation of live
owners. Existing job, cancellation, and ownership regressions were also run.

The ranking fixture is a small deterministic relevance check, not a broad
retrieval benchmark: exact and partial title overlap are retained in order,
disjoint tokens are excluded, and an older excluded match is recovered by an
explicit larger bound. Generic nearest-neighbor ranking and duplicate-v1 weights
are unchanged. The recent-first candidate cap can still miss older matches;
that limitation is now visible. An exact-sized bound conservatively reports
truncation even if there is no additional result.

## External validation

Separate standalone MCP server processes shared only the task-owned corpus.
A bounded `python3 -c "import time; time.sleep(8)"` validation command made job
states observable without running repository-controlled code. A 12-second
command timeout bounded each process. Six submissions exercised the default
four running slots plus two waiting admissions.

- A second process cancelled one running and one queued job; both reached the
  cancelled terminal outcome.
- Closing the submitting server's stdin cancelled and joined the remaining
  work; all six jobs were terminal and cancelled.
- A fresh submitting server was killed with four running and two queued jobs.
  After the ten-second owner lease expired, a new executor reconciled all six
  to failed. Work was not automatically replayed. Abrupt termination does not
  prove synchronous termination of child processes; these commands were bounded.
- The preserved Cobra investigation was queried again. Its tiny positive scores
  still produced candidates, with unknown source coverage and inconclusive
  evidence relations. Rounded `0.00` displays did not establish exact zero.

The probe initially used the wrong job response keys; it was corrected to read
`execution_state` and terminal `outcome` before the successful lifecycle run.
Those harness errors were not product defects. All task-owned runtime artifacts
and raw logs were removed after validation as requested; regression source and
this summary remain.

## Research and remaining limits

SQLite WAL supports concurrent readers but still serializes writes. Existing
reconciliation uses `BEGIN IMMEDIATE`, so no heartbeat can interleave between
liveness inspection and the corresponding updates. These fixes use that same
transaction rather than introducing another lock/lease owner.
[SQLite isolation](https://www.sqlite.org/isolation.html).

The Go race detector checks only executed paths; deterministic lifecycle tests
and a real multi-process workload complement it. Cross-process logical races
are not established or excluded by a clean race-detector run alone.
[Go race detector](https://go.dev/doc/articles/race_detector).

A broad ranking claim requires representative queries and independently judged
relevance, with precision and recall measured over the same collection. This
follow-up does not tune weights or claim a global precision/recall improvement
from a three-document fixture. Larger judged workloads, prolonged writer stalls,
lease-expiry under heavy load, and Windows process termination remain useful
follow-ups.
[IR evaluation](https://nlp.stanford.edu/IR-book/html/htmledition/information-retrieval-system-evaluation-1.html).

## Final validation

`make verify` passed after incorporating the current base's SQLite 1.56.0
update. The focused race command below also passed on that base:

```sh
go test -race -short -p=1 -parallel=4 -count=1 -timeout 120s ./internal/app ./internal/corpus -run 'Test.*Job|Test.*Owner|Test.*Reconcil|TestRelatedWork|TestDuplicateAndCollision'
```

The external workload preceded the dependency update. Final tests cover the
updated dependency, but the external multi-process probe was not repeated after
its task-owned runtime was removed. The Go MCP SDK version did not change.

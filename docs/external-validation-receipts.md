# External validation receipts

`internal/evidence/testdata/flameox-profiler-receipt.json` is a checked-in
Flameox-compatible example mapped to the producer-neutral
`gitcontribute.external-validation.v1` contract. It is fixture data only: the
repository does not depend on or execute Flameox.

The field mapping is intentionally direct:

| Profiler handoff | GitContribute receipt field |
| --- | --- |
| producer name | `producer` |
| profiler run identity | `provider`, `external_run_id` |
| source repository and commit | `repository`, `revision` |
| primary output digest | `artifact_sha256` |
| named output digests | `artifacts` |
| invoked command and workspace label | `argv`, `working_dir` |
| start/end and process result | `started_at`, `completed_at`, `exit_code`, `classification` |
| partial profiler output | `incomplete`, `limitations`, `truncated` |

Receipts are decoded with unknown-field rejection and validated for schema,
provenance, SHA-256 digests, timestamps, bounded output, incomplete status, and
round-trip preservation. Attaching a receipt records the supplied observation;
it never invokes the named producer.

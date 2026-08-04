# External evidence manifests

`evidence.import_manifest` accepts one bounded JSON document using
`gitcontribute.external-evidence.v1`. The importer is deliberately a handoff
boundary: it validates the producer-owned identity and digest, stores each
typed claim as local evidence, and preserves the producer's repository
revision, artifact digest, completeness, integrity, environment, and
limitations. It never reads a producer path, executes a command, or contacts
the producer.

The manifest must contain a producer, investigation, repository, revision,
observation time, completeness (`complete`, `incomplete`, or `unknown`),
integrity (`verified` or `unverified`), one or more claims, and a
`manifest_sha256`. The digest is SHA-256 over the canonical JSON representation
with `manifest_sha256` empty. Each claim is stored under a deterministic ID
derived from the manifest digest and claim content, so retries are idempotent.

Imported evidence remains external evidence. A verified producer claim is not
the same as a validation reproduced by GitContribute; incomplete, unverified,
stale, and unknown claims produce explicit evidence gaps in contribution
manifests.

For structured test execution, use `validation.attach_junit_report` with the
existing validation run ID. The JUnit importer retains a bounded raw XML copy
and digest, normalizes counts and test identities, and persists malformed
reports as incomplete diagnostic evidence. An incomplete or failing report
cannot satisfy a passing validation requirement.

# Ownership and failure-handling follow-up

Date: 2026-09-08. This pass continues the [search audit](search-audit.md)
against the local tree containing its fixes. It examines stored research
relationships, PR collision references, triage link resolution, and nearby
search coverage reads. It is not an exhaustive repository audit.

## Reproduced defects and changes

| Boundary | Reproduction before the fix | Ownership or behavior change |
| --- | --- | --- |
| Inbound PR references in research briefs | Seven table cases fail: colon and URL closing references are downgraded; quoted, inline, and fenced examples become evidence; an explicit pull URL matches an issue; a quoted closing claim overrides a real mention. | Use `relatedwork.Extract`, which already owns outbound research relationship parsing. Respect explicit thread kinds and preserve the brief's existing mentions/claims-to-close vocabulary. |
| PR collision scoring | An inbound blockquote or outbound inline-code reference gives an otherwise unrelated PR a 0.45 collision score. | The cluster-member adapter uses the same unquoted-prose extractor. Existing positive collision tests retain their expected scores. |
| Failed triage writes | Cancellation during lookup or an insert-rejection trigger clears the caller's carried links despite returning an error. | Resolve a copy and publish its link fields only after the insert succeeds. |
| Unreadable triage targets | A repository with an invalid stored numeric value is treated as missing, and an unlinked event is persisted successfully. | Propagate projection lookup errors; distinguish missing rows from query failures in carried-link checks. Remove the redundant repository-parser wrapper. |

These cases use isolated local SQLite corpora and public application/corpus
methods. The research fixture fails on unexpected GitHub access. Additional
tracking tests cover unreadable typed and untyped thread targets and successful
clearing of stale links. The three original tracking failure cases, seven
research cases, and both collision cases were observed failing before their
respective production fixes.

GitHub documents closing keywords and optional colons in
[Linking a pull request to an issue](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue).
The shared local extractor also recognizes full GitHub thread URLs. These are
lexical claims from stored prose; they do not establish that GitHub will close an
issue, that a PR targets the default branch, or that a merge occurred.

The research change removes per-PR regexp compilation but also performs the
shared extractor's Markdown filtering and relationship classification. No
latency improvement is claimed for this change without a benchmark.

## Follow-up inspection and limits

The follow-up traced both remaining cluster-reference consumers, the shared
relationship parser, the triage resolver's callers, and search coverage reads.
No additional demonstrated defect was selected in those boundaries after the
fixes above. Similar-looking URL adapters and reference equality checks were
not merged solely for matching source text: ambiguous references and exact
member identities have different contracts.

Search still reads thread coverage once per returned match. Its existing batch
reader requires an explicit facet list; an empty list means no results, not all
facets. Replacing this with a hard-coded list would silently omit future facets.
A bounded all-facet API and representative end-to-end measurements are needed
before selecting that optimization. No performance benefit has been measured
for it in this pass.

The earlier search audit records remaining relevance-dataset and scale-testing
opportunities. Exhausting possible improvements across a repository cannot be
proved by these passes; further changes should be selected from new behavioral
or profiling evidence.

## Validation

The independent review found no verified defects in the six changed production
and test files for this pass. It excluded the earlier search/radar changes and
the previously reviewed identity-helper removal. Focused behavioral tests and
the final `make verify` passed, including the uncached short suite, vet, full
lint, module tidiness, generated-output checks, and repository documentation
validation.

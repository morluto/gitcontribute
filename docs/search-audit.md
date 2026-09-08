# Search audit: relevance and filter ownership

Date: 2026-09-08. Baseline: `c1930cd`.

This pass started with an ownership/residue scan of the repository and narrowed
into offline search. It traced query compilation, corpus SQL, ranking, cursors,
counts, and application match explanations. It used isolated SQLite corpora and
stored synthetic observations, without accessing a user's corpus or fetching
GitHub data. It is not a complete security or architecture audit.

## Reproduced and fixed

| Problem | Evidence on the baseline | Change |
| --- | --- | --- |
| Hydration dilutes strong title matches | Among 102 threads, `connection timeout` ranks a body-only configuration example above `connection timeout during startup` after the latter receives 2,000 unrelated discussion words. | Relevance places all-term title matches first, then retains weighted BM25 within each group. The title predicate uses FTS5, including for any-term searches. Updated sorting remains chronological. |
| Different label selections share a cursor key | A cursor for the single label `bug,urgent` is accepted for the two labels `bug` and `urgent`. Trimming also aliases ` bug` and `bug`. | Encode filter values structurally and preserve their values; sort the copied label list because label order does not change selection. Unicode case folding is also avoided because SQLite's built-in [`lower()`](https://www.sqlite.org/lang_corefunc.html#lower) does not provide general Unicode folding. |
| Unknown merge selection returns known unmerged PRs | The corpus accepts `ParseMergeFilter("unknown")`, but search returns the known unmerged PR and reports a total of one instead of the two unknown PRs. | One merge predicate now owns both row selection and exact counts. Unknown outcomes are selected explicitly, and are not reported as excluded unknowns when explicitly requested. |

The three original regression tests failed against the baseline production
implementation before their fixes. Additional coverage checks any-term title
promotion, pagination between ranking groups, unchanged updated ordering, and
Unicode-distinct cursor filters. Existing tests cover title/label weights,
metadata filters, all/any term selection, literal operators and unmatched quotes,
facet replacement ordering, truncated evidence, and repository/code pagination.

The `unknown` merge correction is at the corpus API. This change does not add an
unknown selector to the public thread-search tool's existing nullable boolean
input.

## Ranking decision and cost

SQLite's [FTS5 BM25 documentation](https://www.sqlite.org/fts5.html#the_bm25_function)
explains that document length affects rank. Column weights increase term
frequency contributions but do not isolate a short title from the length of its
hydrated discussion. A larger title weight alone would still leave that problem
at larger document sizes.

The selected policy is intentionally explicit: a title matching every query
term precedes matches requiring other fields. This improves the reproduced
case, but can also promote a generic title above a more substantively relevant
body. It is a lexical retrieval rule, not evidence of semantic relevance. Scores
remain BM25 values, so callers should preserve result order instead of globally
re-sorting thread results by score. Match explanations and the architecture
contract describe the two groups.

The additional indexed title lookup has a cost. Three 300 ms benchmark samples
on the same Linux/amd64 host, using the 102-thread regression fixture, gave:

| Query | Baseline median | Changed median |
| --- | ---: | ---: |
| `connection timeout` (two matches) | 1.163 ms/op | 1.380 ms/op |
| `unrelated` (many matches, hydrated excerpt) | 27.388 ms/op | 27.402 ms/op |

These short samples are diagnostic, not a scale benchmark or proof of latency
parity. The broad-query samples were noisy. Reproduce with:

```sh
go test ./internal/corpus -run '^$' -bench BenchmarkThreadSearchRelevance -benchtime=300ms -count=3
```

## Remaining opportunities

1. Build a relevance dataset from actual failed searches and expected results.
   The regression fixture establishes the hydration defect; it does not establish
   that title-first ranking is best across real contribution-research tasks.
   Compare ranking policies using judged queries before further tuning weights.
2. Evaluate word forms and code identifiers separately. The current indexes use
   SQLite's default `unicode61` tokenizer. They do not configure stemming,
   substring search, or camel-case splitting. SQLite documents distinct
   [tokenizer options](https://www.sqlite.org/fts5.html#tokenizers); adopting one
   changes matching semantics and needs an index/projection design and a recall
   dataset, rather than an automatic fallback that silently broadens a query.
3. Continue profiling at larger sizes with realistic query selectivity. The
   follow-up below bounds excerpt work, but the full-text match/ranking stage
   still depends on corpus size. Small searches also incur extra planning cost.

The earlier punctuation hypothesis was rejected: SQLite ignores empty quoted
phrases within these queries, and the existing literal-query regression passes.
The retired crawl-frontier response fields were also left in place because
[ADR 0006](adr/0006-retire-crawl-frontier.md) explicitly retains them. Similar
looking helpers were not merged merely because their source bodies matched.

## Follow-up: bounded evidence work and shared owners

The next pass implemented the excerpt-work and ranking-ownership opportunities.
Its baseline was the local tree containing the title, merge, and cursor fixes
above, not the earlier Git commit alone.

Thread search now selects and materializes the filtered page of IDs before
building excerpts. Exact evidence lookup supplies a single ID to the same
attribution SQL. FTS row-ID constraints keep expensive excerpt generation away
from unrelated results. Page selection and count still share one read
transaction; this does not add writes, an index migration, or a network read.
The design uses SQLite's documented
[materialization boundary](https://www.sqlite.org/lang_with.html#materialization_hints)
to keep the page limit ahead of downstream evidence work.

Three 200 ms samples per case on the same host gave these medians:

| Workload | Before | After |
| --- | ---: | ---: |
| Selective query, 102-thread fixture | 1.295 ms | 2.192 ms |
| Broad query, 102-thread fixture | 26.751 ms | 3.064 ms |
| Scoped page, no background threads | 1.186 ms | 2.013 ms |
| Scoped page, 100 background threads | 9.574 ms | 2.244 ms |
| Scoped page, 1,000 background threads | 81.559 ms | 3.292 ms |
| Exact evidence, no background threads | 0.594 ms | 0.705 ms |
| Exact evidence, 100 background threads | 8.139 ms | 0.910 ms |
| Exact evidence, 1,000 background threads | 82.370 ms | 1.140 ms |

The scope fixture holds one selected thread and varies only the number of
matching hydrated threads in another repository. These are synthetic local
benchmarks, not production latency guarantees. The small-query overhead is a
real tradeoff; the gain comes from bounding evidence work as other matches grow.

```sh
go test ./internal/corpus -run '^$' -bench 'BenchmarkThreadSearch(HydratedScope|Relevance)' -benchtime=200ms -count=3
```

The accompanying read-only regression traverses every page in both relevance
and updated order, compares ranks and evidence with exact lookups, verifies
counts and corpus revision, and checks cancellation. Page and exact reads share
the evidence implementation; preservation of global BM25 values also relies on
inspection of the unchanged ranking expression, not an independent rank oracle.
Existing facet tests retain
coverage of complete/stale replacement and truncation boundaries.

Other duplicated owners were removed without changing their rules:

- Repository and code search each define their BM25 SQL expression once,
  including the expression used by cursor predicates and exact evidence reads.
- Radar candidate sorting and eligibility escalation use the same severity rule.
- `clustering.MemberRef.Equal` owns case-insensitive repository identity for
  both neighbor self-exclusion and the corpus's canonical-member guard.

This remains a focused improvement pass. Setup/release replacement helpers,
other domain state machines, and unrelated persistence paths were not refactored.

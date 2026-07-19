# Domain language

GitContribute uses these terms consistently across the CLI, MCP catalog,
application services, storage, and documentation.

- **Repository**: a stored projection of a GitHub repository and the source
  observations supporting it. Missing facets are unknown, not zero-valued facts.
- **Thread**: a stored GitHub issue or pull request. A thread is source material;
  it is not inherently a contribution opportunity.
- **Candidate**: a transient ranked view of a repository or thread. Ranking does
  not create workflow state or imply maintainer approval.
- **Investigation**: durable local work that evaluates one repository revision.
- **Hypothesis**: a persisted, testable claim recorded in an investigation.
- **Opportunity**: a persisted, scoped contribution promoted from a hypothesis.
  Raw search results and ranked threads must not be called opportunities.
- **Competing work**: existing issue or pull-request activity that may overlap a
  proposed contribution. It is distinct from a Git merge conflict.
- **Facet**: an independently observed and refreshed slice of source data, such
  as repository metadata, thread headers, comments, reviews, or PR details.

Tool namespaces describe authority and effects: `corpus.*` reads stored local
facts or deterministic derivations; `github.*` performs explicit GitHub reads
and persists observations; `jobs.*` manages durable asynchronous work;
`workflow.*` reads or changes local investigation state. Provider-derived prose
must carry provenance and must not override GitHub facts.

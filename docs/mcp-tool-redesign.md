# MCP tool design

GitContribute targets `github.com/modelcontextprotocol/go-sdk` `v1.7.0` and
negotiates MCP `2026-07-28`. Typed Go input and output values own protocol
semantics. The SDK performs input decoding and output-schema validation; schema
customization adds bounds, enums, defaults, and discriminated `oneOf` branches
without a parallel decoder.

## Facts before workflows

The public catalog exposes atomic, composable facts:

- `corpus.*` operations are offline reads and never refresh implicitly;
- `github.*` operations are explicit, bounded network reads that may update
  only the local corpus;
- `jobs.*` controls durable work;
- resource URIs are the canonical detailed read plane for persisted payloads;
- workspace and validation tools retain their separate local-write and process
  boundaries.

Tool descriptions state what a call reads or writes, its bounds, and what
unknown or incomplete output means. They do not prescribe an agent workflow.
Missing, paginated, truncated, stale, unauthorized, and unavailable facts stay
distinct from an observed empty value.

Batch inputs remain useful when every item has the same capability boundary.
They preserve input order and isolate per-item failures. Actor results use a
small item contract rather than embedding the catalog-wide recovery union in
every output schema.

## Actor primitives

`github.search_users` persists one bounded page of identity observations. It
does not fan out into profile reads. Exact profile and child facts use separate
tools:

```text
github.sync_users
github.sync_user_social_accounts
github.sync_user_organizations
github.sync_user_pinned_items
github.sync_user_repositories
github.sync_user_contributions
```

Selectors are a discriminated union: exactly one login or GitHub node ID.
Repository synchronization requires an explicit `owned`, `affiliated`, or
`contributed` relationship. Contribution synchronization requires an explicit
period no longer than one year. Pages, items, repositories, and total requests
are bounded in the input schema.

Offline consumers compose `corpus.search_actors`, `corpus.get_actors`,
`corpus.get_actor_facets`, and `corpus.search_contributions`. Results expose
observation and source timestamps, authorization scope, completeness, snapshot
identity, and opaque actor resource URIs. See [Actor corpus](actor-corpus.md).

## Resources and structured output

Small query results remain structured tool output. Larger or durable payloads
are read with MCP `resources/read` using the exact URI returned by a tool.
Actor resources use:

```text
gitcontribute://actor/{actor_id}
gitcontribute://actor/{actor_id}/facet/{facet}
```

Facet resources contain the stored typed payload and its provenance; reading a
resource never contacts GitHub. Job results expose structured progress and
artifact references rather than raw request or result blobs.

## Breaking catalog changes

The facts-first catalog removes overlapping workflow-shaped operations for
ranking candidates, preparing issue sets, building dossiers, mining or
previewing fix patterns, contribution preflight, related-work discovery, and
DeepWiki reads. Their component corpus, GitHub, resource, workspace, and
validation capabilities remain independently selectable where applicable.

Canonical names changed as follows:

| Previous name | Canonical name |
| --- | --- |
| `corpus.search_code_batch` | `corpus.search_code` |
| `corpus.list_pull_requests` | `corpus.search_pull_requests` |
| `github.sync_ci_failures` | `github.sync_pull_request_ci` |

There is no scalar code-search compatibility alias. The canonical code search
accepts several bounded queries over one shared offline snapshot.

## Side effects

MCP annotations reflect the actual capability boundary. Corpus reads are
read-only and idempotent. GitHub reads are open-world operations that persist
local observations. Local workflow writes, Git acquisition, and validation
execution remain separate. GitContribute exposes no GitHub mutation tool.

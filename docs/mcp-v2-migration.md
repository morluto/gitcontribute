# MCP v2 migration: repository-bound live acquisition

Version 2 removes the flat `owner` and `repo` fields from the two live
repository-acquisition tools. Both now require one nested `repository` object.
There are no compatibility aliases or mixed forms: callers must update every
request, recovery replay, and saved tool call before connecting to a v2 server.

## `github.search_threads`

Before (v1):

```json
{
  "owner": "acme",
  "repo": "rocket",
  "query": "cache eviction",
  "kind": "issue"
}
```

After (v2):

```json
{
  "repository": {"owner": "acme", "repo": "rocket"},
  "query": "cache eviction",
  "kind": "issue"
}
```

## `github.read_source_files`

Before (v1):

```json
{
  "owner": "acme",
  "repo": "rocket",
  "ref": "main",
  "files": [{"path": "README.md"}]
}
```

After (v2):

```json
{
  "repository": {"owner": "acme", "repo": "rocket"},
  "ref": "main",
  "files": [{"path": "README.md"}]
}
```

The tools still return an opaque artifact URI. Follow that URI only through MCP
`resources/read`; the resource reader remains local and offline.

## Scoped authored portfolios

`github.sync_pull_request_portfolio` accepts the same optional `repository`
scope only with `selection: "authored"`. The returned job follow-up and a
truncated `corpus.search_pull_requests` recovery retain that scope. Explicit
pull-request selections are already exact and reject `repository`.

```json
{
  "selection": "authored",
  "repository": {"owner": "acme", "repo": "rocket"},
  "state": "open",
  "limit": 20
}
```

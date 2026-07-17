# GitContribute

GitContribute is a local-first GitHub contribution research toolkit. It keeps
a durable local corpus of repositories, issues, pull requests, conversations,
and code so contributors and agents can discover, investigate, validate, and
prepare focused open-source contributions.

The project is an independent implementation. Gitcrawl and other systems are
used as design references; they are not runtime or build dependencies.

The current implementation is under active development. See [SPEC.md](SPEC.md)
for the product contract and delivery plan.

## Install

GitContribute requires Go 1.26 or newer. Install the CLI directly:

```sh
go install github.com/morluto/gitcontribute/cmd/gitcontribute@latest
```

Or build a checkout:

```sh
go build -o gitcontribute ./cmd/gitcontribute
```

Linux, macOS, and Windows builds are exercised in CI.

## Quick start

```sh
gitcontribute init
gitcontribute sync owner/repo
gitcontribute search "connection timeout" --repo owner/repo
gitcontribute dossier owner/repo
```

Useful follow-on operations include:

```sh
# Incremental or exact archive refreshes
gitcontribute archive sync owner/repo --since 168h
gitcontribute archive sync owner/repo --numbers 42,108

# Explicitly hydrate conversations and review data
gitcontribute archive hydrate owner/repo#42 --with issue_comments

# Offline contribution research
gitcontribute clusters owner/repo
gitcontribute neighbors owner/repo#42 --kind issue
gitcontribute export dossier owner/repo --format markdown
```

Run `gitcontribute --help` for the full CLI. The MCP server exposes the same
local search, dossier, coverage, clustering, and neighbor facts, plus explicitly
annotated repository sync and thread hydration operations.

Public GitHub data can be read anonymously. For authenticated reads, configure
an environment, operating-system keyring, or `gh auth token` source. Tokens are
resolved at runtime and are not stored in the corpus.

Local search is offline. Network reads happen only through explicit sync,
source, crawl, refresh, or hydration operations. Validation commands require
an explicit execution flag, and the v1 CLI never posts issues, opens pull
requests, pushes commits, or otherwise mutates GitHub.

## Development

The module selects its required Go toolchain automatically:

```sh
go test ./...
go run ./cmd/gitcontribute --help
```

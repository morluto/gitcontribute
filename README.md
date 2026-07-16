# GitContribute

GitContribute is a local-first GitHub contribution research toolkit. It keeps
a durable local corpus of repositories, issues, pull requests, conversations,
and code so contributors and agents can discover, investigate, validate, and
prepare focused open-source contributions.

The project is an independent implementation. Gitcrawl and other systems are
used as design references; they are not runtime or build dependencies.

The current implementation is under active development. See [SPEC.md](SPEC.md)
for the product contract and delivery plan.

## Development

The module selects its required Go toolchain automatically:

```sh
go test ./...
go run ./cmd/gitcontribute --help
```

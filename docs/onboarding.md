# Onboarding and npm distribution

The primary installation and onboarding entry point is:

```sh
npx gitcontribute@latest setup
```

GitContribute remains a native Go application. The `gitcontribute` npm package
contains a small Node.js launcher and precompiled binaries for macOS ARM64/x64,
Linux ARM64/x64, and Windows x64. Installation has no lifecycle script
and performs no binary download. The launcher chooses the host binary at run
time and forwards standard streams, arguments, signals, and its exit status.

## Setup contract

Setup is a local capability. It may create the GitContribute configuration and
corpus, register the MCP server with selected coding clients, and add an
explicit repository source. It does not synchronize a repository, access
GitHub, execute repository-controlled code, or mutate GitHub.

The setup engine plans and applies only GitContribute-owned entries:

- `[mcp_servers.gitcontribute]` in Codex TOML configuration;
- `mcpServers.gitcontribute` in Claude JSON configuration.

Unrelated configuration is preserved. Repeated setup is idempotent. `remove`
deletes only those entries; it never removes the GitContribute corpus or its
application configuration. `--dry-run` performs validation without writes,
and `--json` exposes per-step results for automation.

When invoked through npm, setup records an npm launcher such as:

```text
npx --yes --package=gitcontribute@0.1.0 -- gitcontribute mcp
```

It never records a temporary executable from the npm cache. Development builds
use `gitcontribute@latest`; released builds use their exact version so a client
configuration is reproducible. Re-running setup with a newer release updates
the registration. `--mcp-version latest` opts into following the latest npm
release instead.

## Release contract

One tag version controls the Go binaries and npm package. Release automation:

1. cross-compiles all supported native binaries with `CGO_ENABLED=0`;
2. injects the tag version into the Go executable;
3. assembles one npm package containing every binary;
4. verifies the package has no install lifecycle;
5. installs the tarball with `--ignore-scripts` and runs a smoke test;
6. enforces a 100 MB compressed-package ceiling;
7. publishes the npm package with provenance;
8. creates a matching GitHub release.

The npm environment must be configured for trusted publishing before the first
release. Package publication is an external mutation and is performed only by
the tag-triggered release workflow.

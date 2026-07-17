# Onboarding and npm distribution

The primary installation and onboarding entry point is:

```sh
npx gitcontribute@latest setup
```

The interactive wizard treats terminal and agent access as independent
capabilities. It offers to install a persistent `gitcontribute` command for the
CLI and TUI, then separately asks which MCP clients to configure. Terminal
installation is explicit: interactive users approve it in the setup plan, and
non-interactive callers must pass `--install-cli`.

```sh
# Terminal app and Codex MCP
npx gitcontribute@latest setup --install-cli --codex --yes

# Terminal app without MCP
npx gitcontribute@latest setup --install-cli --no-mcp --yes

# MCP without a persistent terminal command
npx gitcontribute@latest setup --codex --yes
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

Terminal installation is a separate, explicitly selected package-manager
capability. The client-registration engine plans and applies only
GitContribute-owned entries:

- `[mcp_servers.gitcontribute]` in Codex TOML configuration;
- `mcpServers.gitcontribute` in Claude JSON configuration.

Unrelated configuration is preserved. Repeated setup is idempotent. `remove`
deletes only those entries; it never removes the GitContribute corpus or its
application configuration. `--dry-run` performs validation without writes,
and `--json` exposes per-step results for automation.

### Execution phases and consent

Interactive setup follows an explicit sequence:

1. When bootstrapped through npx, ask whether to install the terminal app.
2. Select MCP clients independently; `none` is valid when installing only the
   terminal app.
3. Resolve GitHub authentication configuration without contacting GitHub.
4. Produce a dry-run plan. Planning never invokes npm or writes configuration,
   corpus, client, or repository-source state.
5. Ask for confirmation, then apply the selected effects.
6. Verify the resulting local installation with `doctor`.

Non-interactive setup never infers permission to install globally. It requires
`--install-cli`; `--yes` only accepts the explicitly selected plan.

Setup is not one cross-system transaction. The global npm prefix, application
configuration, corpus, and coding-client files are separate effects. Known
client-configuration errors are preflighted before npm runs. A later runtime
failure can still leave an earlier successful effect in place, and the report
lists each result separately before exiting unsuccessfully.

### MCP launcher policy

| Terminal capability | MCP capability | Registered MCP launcher |
| --- | --- | --- |
| Installed and verified | Selected | Absolute global `gitcontribute` command |
| Skipped under npx | Selected | Exact-version `npx --package=gitcontribute@VERSION` command |
| Selected | Skipped | No MCP launcher or client-file mutation |

If a requested terminal installation fails during combined setup, MCP can still
be configured with the pinned npx fallback. The terminal step remains failed,
so the overall setup command exits unsuccessfully even if MCP configuration
succeeds.

When setup installs the persistent terminal app, MCP clients use the verified
global executable:

```text
/absolute/npm/prefix/bin/gitcontribute mcp
```

When terminal installation is skipped, setup records an npm launcher such as:

```text
npx --yes --package=gitcontribute@0.1.0 -- gitcontribute mcp
```

It never records a temporary executable from the npm cache. Development builds
use `gitcontribute@latest`; released builds use their exact version so a client
configuration is reproducible. Re-running setup with a newer release updates
the registration. `--mcp-version latest` opts into following the latest npm
release instead.

MCP-only setup reports that the terminal command was not installed and prints
the exact global npm installation command. MCP remains usable through its
pinned launcher. Removing MCP registration does not uninstall a persistent CLI;
use `npm uninstall --global gitcontribute` for that separate capability.

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

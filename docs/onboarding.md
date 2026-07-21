# Onboarding and npm distribution

The primary installation and onboarding entry point is:

```sh
npx gitcontribute setup
```

The wizard identifies the resolved GitContribute version before any choice.

The interactive wizard presents three access modes: MCP, CLI, or Both.
Non-interactive callers select the same product choice with `--mode`.

```sh
# Both: CLI and Codex MCP
npx gitcontribute setup --mode both --codex --token-source none --yes

# CLI only
npx gitcontribute setup --mode cli --token-source none --yes

# MCP only; private runtime without a global command
npx gitcontribute setup --mode mcp --codex --token-source none --yes
```

GitContribute remains a native Go application. The `gitcontribute` npm package
contains a small Node.js launcher and precompiled binaries for macOS ARM64/x64,
Linux ARM64/x64, and Windows x64. Installation has no lifecycle script
and performs no binary download. The launcher chooses the host binary at run
time and forwards standard streams, arguments, signals, and its exit status.

## Setup contract

Setup is a local operation. It may create the GitContribute configuration and
corpus, register the MCP server with selected coding clients, and add an
explicit repository source. It does not synchronize a repository, access
GitHub, execute repository-controlled code, or mutate GitHub.

Global CLI installation is an explicitly selected package-manager effect in
CLI and Both modes. MCP mode instead installs a versioned private runtime
through a local file copy. The client-registration engine plans and applies only
GitContribute-owned entries:

- `[mcp_servers.gitcontribute]` in Codex TOML configuration;
- `mcpServers.gitcontribute` in Claude JSON configuration.

Codex setup also installs a `gitcontribute` discovery skill under
`~/.codex/skills/gitcontribute` so deferred MCP tools can be matched
implicitly for GitHub contribution workflows.

Unrelated configuration is preserved. Repeated setup is idempotent. `remove`
deletes only those entries; it never removes the GitContribute corpus or its
application configuration. `--dry-run` performs validation without writes,
and `--json` exposes per-step results for automation.

### Execution phases and consent

Interactive setup follows an explicit sequence:

1. Select MCP, CLI, or Both. The package runner
   is not presented as a product choice.
2. For MCP or combined setup, show supported clients in a multi-select.
   Detection is visible but does not select a new target. Existing
   GitContribute registrations may be preselected as prior intent.
3. Select GitHub authentication from described choices. Existing configuration,
   GitHub CLI availability, and environment-variable presence inform the
   default without resolving credentials or contacting GitHub.
4. Produce a dry-run plan. Planning never invokes npm or writes configuration,
   corpus, client, or repository-source state.
5. Ask for confirmation, defaulting to apply, then apply the selected effects.
6. Verify the applied plan: the corpus is readable and current, its integrity
   check passes, Git is available, and selected MCP registrations exactly match
   the installed command. Normal contention from another corpus writer does not
   make setup fail.

Interactive setup uses inline terminal forms rather than an alternate-screen
application. Active operations may show a spinner that settles into a durable
status line. `NO_COLOR` and `GITCONTRIBUTE_ACCESSIBLE=1` provide plain and
screen-reader-friendly operation respectively. Operational INFO logs are quiet
by default; set `GITCONTRIBUTE_LOG_LEVEL=info` or `debug` when diagnosing setup.

Non-interactive setup never infers access modes, client targets, authentication,
or permission to install globally. `--yes` requires `--mode` and
`--token-source`; MCP and Both also require explicit client targets. It accepts
only that resolved plan.

Setup is not one cross-system transaction. The private runtime or global npm
prefix, application configuration, corpus, and coding-client files are separate
effects. Known client-configuration errors are preflighted before installation.
If runtime or CLI installation fails, setup stops before writing application or
coding-client configuration.

### MCP runtime policy

| Mode | Installed artifact | Coding-agent MCP command |
| --- | --- | --- |
| MCP | Versioned private native binary | Absolute private path plus `mcp serve --transport=stdio` |
| CLI | Global `gitcontribute` command | None |
| Both | Global `gitcontribute` command | Absolute verified global path plus `mcp serve --transport=stdio` |

MCP-only setup copies the currently running native GitContribute executable
into GitContribute's platform data directory. It does not run a package
install or create a command on `PATH`. Typical destinations are:

```text
macOS:   ~/Library/Application Support/gitcontribute/Data/bin/VERSION/gitcontribute
Linux:   ~/.local/share/gitcontribute/bin/VERSION/gitcontribute
Windows: %LOCALAPPDATA%\gitcontribute\Data\bin\VERSION\gitcontribute.exe
```

Both uses the verified global CLI executable:

```text
/absolute/npm/prefix/bin/gitcontribute mcp serve --transport=stdio
```

No mode stores `npx`, `@latest`, or an npm-cache executable in coding-agent
configuration. Re-running MCP setup with a newer release installs that release
under its own versioned path and updates the selected registrations. When a
registration changes, setup reports the affected clients in
`restart_clients`; their active sessions must restart to replace older MCP
processes with the configured runtime.
`gitcontribute remove` deletes only selected coding-agent registrations. It
does not delete versioned private runtimes, uninstall the global CLI, or remove
application configuration or corpus data. Use
`npm uninstall --global gitcontribute` to remove the global installation.

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

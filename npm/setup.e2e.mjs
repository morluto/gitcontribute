import assert from "node:assert/strict";
import { chmod, copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { spawn, spawnSync } from "node:child_process";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

import platforms from "./platforms.json" with { type: "json" };

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const packageVersion = JSON.parse(await readFile(join(root, "package.json"), "utf8")).version;

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { encoding: "utf8", ...options });
  assert.equal(result.status, 0, result.stderr || result.stdout);
  return result;
}

function runAsync(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { ...options, stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.stderr.on("data", (chunk) => (stderr += chunk));
    child.on("error", reject);
    child.on("close", (status, signal) => resolve({ status, signal, stdout, stderr }));
  });
}

function startMCPStdio(command, env) {
  const child = spawn(command, ["mcp", "serve", "--transport=stdio"], {
    env,
    stdio: ["pipe", "pipe", "pipe"],
  });
  let buffer = "";
  const pending = new Map();
  const rejectPending = (error) => {
    for (const waiter of pending.values()) waiter.reject(error);
    pending.clear();
  };
  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk) => {
    buffer += chunk;
    let newline;
    while ((newline = buffer.indexOf("\n")) >= 0) {
      const line = buffer.slice(0, newline).trim();
      buffer = buffer.slice(newline + 1);
      if (!line) continue;
      let message;
      try {
        message = JSON.parse(line);
      } catch (error) {
        rejectPending(new Error(`invalid MCP response: ${error.message}: ${line}`));
        return;
      }
      const waiter = pending.get(message.id);
      if (waiter) {
        pending.delete(message.id);
        waiter.resolve(message);
      }
    }
  });
  child.stderr.on("data", () => {});
  child.on("error", rejectPending);
  child.on("close", (status, signal) => {
    rejectPending(new Error(`MCP process exited before response (status=${status}, signal=${signal})`));
  });
  return {
    request(id, method, params) {
      const response = new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
      child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
      return response;
    },
    notify(method, params) {
      child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", method, params })}\n`);
    },
    close() {
      return new Promise((resolve) => {
        const timer = setTimeout(() => {
          child.kill("SIGTERM");
          resolve();
        }, 2_000);
        child.once("close", () => {
          clearTimeout(timer);
          resolve();
        });
        child.stdin.end();
      });
    },
  };
}

function shellQuote(value) {
  return `'${value.replaceAll("'", `'\\''`)}'`;
}

function runScriptPTY(command, input, env) {
  return new Promise((resolve, reject) => {
    const child = spawn(
      "script",
      ["-qefc", `stty rows 40 columns 120; exec ${shellQuote(command)} setup`, "/dev/null"],
      { env, stdio: ["pipe", "pipe", "pipe"] }
    );
    let stdout = "";
    let stderr = "";
    let stageOutput = "";
    let stage = 0;
    let keyScheduled = false;
    let keyTimer;
    const interactions = [
      ["How do you want to use GitContribute?", "\r"],
      ["Which coding agents should GitContribute configure?", "\x1b[Z"],
      ["How do you want to use GitContribute?", "\x1b[B\r"],
      ["future GitHub syncs authenticate?", "\x03"],
    ];
    const advance = () => {
      if (keyScheduled || stage >= interactions.length) return;
      const [prompt, keys] = interactions[stage];
      if (!stageOutput.includes(prompt)) return;
      keyScheduled = true;
      stageOutput = "";
      keyTimer = setTimeout(() => {
        if (child.exitCode === null) child.stdin.write(keys);
        stage += 1;
        keyScheduled = false;
        advance();
      }, 100);
    };
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
      stageOutput += chunk;
      advance();
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
      stageOutput += chunk;
      advance();
    });
    child.on("error", reject);
    const timeout = setTimeout(() => child.kill("SIGKILL"), 20_000);
    child.on("close", (status) => {
      clearTimeout(timeout);
      clearTimeout(keyTimer);
      resolve({ status, stdout, stderr });
    });
    child.stdin.write(input);
  });
}

async function packCurrentPlatform(workspace) {
  const platform = platforms[`${process.platform}-${process.arch}`];
  assert.ok(platform, `unsupported test platform ${process.platform}-${process.arch}`);

  const staging = join(workspace, "package");
  const nativeDir = join(staging, "npm", "bin", "native", platform.target);
  await mkdir(nativeDir, { recursive: true });
  for (const path of ["package.json", "README.md", "LICENSE", "npm/platforms.json", "npm/bin/gitcontribute.cjs"]) {
    const target = join(staging, path);
    await mkdir(dirname(target), { recursive: true });
    await copyFile(join(root, path), target);
  }

  const executable = join(nativeDir, platform.binary);
  run("go", ["build", "-ldflags", `-X main.version=${packageVersion}`, "-o", executable, "./cmd/gitcontribute"], { cwd: root });
  if (process.platform !== "win32") await chmod(executable, 0o755);

  const packs = join(workspace, "packs");
  await mkdir(packs);
  run("npm", ["pack", "--silent", "--pack-destination", packs], { cwd: staging });
  return join(packs, `gitcontribute-${packageVersion}.tgz`);
}

async function createFakeGlobalNPM(workspace) {
  const fakeBin = join(workspace, "fake-bin");
  const globalPrefix = join(workspace, "global");
  const npmLog = join(workspace, "npm.log");
  await mkdir(fakeBin);
  const fakeNPM = join(fakeBin, "npm");
  await writeFile(
    fakeNPM,
    `#!/usr/bin/env node
const fs = require("node:fs");
const path = require("node:path");
const args = process.argv.slice(2);
fs.appendFileSync(process.env.FAKE_NPM_LOG, args.join(" ") + "\\n");
if (args[0] === "install" && args[1] === "--global") {
  const bin = path.join(process.env.FAKE_NPM_PREFIX, "bin");
  fs.mkdirSync(bin, { recursive: true });
  const command = path.join(bin, "gitcontribute");
  fs.writeFileSync(command, "#!/bin/sh\\nexit 0\\n", { mode: 0o755 });
  process.exit(0);
}
if (args[0] === "prefix" && args[1] === "--global") {
  process.stdout.write(process.env.FAKE_NPM_PREFIX + "\\n");
  process.exit(0);
}
process.exit(2);
`
  );
  await chmod(fakeNPM, 0o755);
  return { fakeBin, globalPrefix, npmLog };
}

async function startRegistry(latestVersion) {
  const requests = [];
  const server = createServer((request, response) => {
    requests.push({ method: request.method, url: request.url });
    if (request.method === "GET" && request.url === "/gitcontribute") {
      response.setHeader("content-type", "application/json");
      response.end(
        JSON.stringify({
          _id: "gitcontribute",
          name: "gitcontribute",
          "dist-tags": { latest: latestVersion },
          versions: { [latestVersion]: { name: "gitcontribute", version: latestVersion } },
        })
      );
      return;
    }
    response.statusCode = 404;
    response.end();
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  assert.ok(address && typeof address === "object");
  return {
    requests,
    url: `http://127.0.0.1:${address.port}`,
    close: () => new Promise((resolve, reject) => server.close((error) => (error ? reject(error) : resolve()))),
  };
}

test("npx upgrade completes without installing a persistent package", { skip: process.platform === "win32" }, async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-upgrade-e2e-"));
  const registry = await startRegistry("9.9.9");
  try {
    const tarball = await packCurrentPlatform(workspace);
    const runner = join(workspace, "runner");
    run("npm", ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]);

    const home = join(workspace, "home");
    await mkdir(home);
    const result = await runAsync(
      "npx",
      ["--yes", "gitcontribute", "upgrade", "--yes", "--json"],
      {
        cwd: runner,
        env: {
          ...process.env,
          HOME: home,
          XDG_CONFIG_HOME: join(home, ".config"),
          XDG_DATA_HOME: join(home, ".local", "share"),
          XDG_CACHE_HOME: join(home, ".cache"),
          XDG_STATE_HOME: join(home, ".local", "state"),
          npm_config_registry: registry.url,
          npm_config_audit: "false",
          npm_config_fund: "false",
          npm_config_update_notifier: "false",
        },
      }
    );
    assert.equal(result.status, 0, `${result.stderr || result.stdout}\nREGISTRY REQUESTS:\n${JSON.stringify(registry.requests)}`);

    const report = JSON.parse(result.stdout);
    assert.equal(report.context, "npx");
    assert.equal(report.status, "npx");
    assert.equal(report.latest, "9.9.9");
    assert.ok(report.stages.some((stage) => stage.name === "installation" && stage.status === "npx"));
    assert.deepEqual(registry.requests, [{ method: "GET", url: "/gitcontribute" }]);
  } finally {
    await registry.close();
    await rm(workspace, { recursive: true, force: true });
  }
});

test("npx upgrade activates the latest private MCP runtime without global installation", { skip: process.platform === "win32" }, async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-upgrade-activation-e2e-"));
  const registry = await startRegistry(packageVersion);
  try {
    const tarball = await packCurrentPlatform(workspace);
    const runner = join(workspace, "runner");
    run("npm", ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]);

    const home = join(workspace, "home");
    const oldRuntime = join(home, ".local", "share", "gitcontribute", "bin", "0.17.0", "gitcontribute");
    await mkdir(join(home, ".codex"), { recursive: true });
    await writeFile(
      join(home, ".codex", "config.toml"),
      `[mcp_servers.gitcontribute]\ncommand = ${JSON.stringify(oldRuntime)}\nargs = ["mcp", "serve", "--transport=stdio"]\n`
    );

    const globalPrefix = join(workspace, "global");
    const result = await runAsync(
      "npx",
      ["--yes", "gitcontribute", "upgrade", "--yes", "--json"],
      {
        cwd: runner,
        env: {
          ...process.env,
          HOME: home,
          XDG_CONFIG_HOME: join(home, ".config"),
          XDG_DATA_HOME: join(home, ".local", "share"),
          XDG_CACHE_HOME: join(home, ".cache"),
          XDG_STATE_HOME: join(home, ".local", "state"),
          npm_config_registry: registry.url,
          npm_config_prefix: globalPrefix,
          npm_config_audit: "false",
          npm_config_fund: "false",
          npm_config_update_notifier: "false",
        },
      }
    );
    assert.equal(result.status, 0, `${result.stderr || result.stdout}\nREGISTRY REQUESTS:\n${JSON.stringify(registry.requests)}`);

    const report = JSON.parse(result.stdout);
    assert.equal(report.context, "npx");
    assert.equal(report.current, packageVersion);
    assert.equal(report.latest, packageVersion);
    assert.equal(report.status, "restart required");
    assert.ok(report.stages.some((stage) => stage.name === "private-mcp-runtime" && stage.status === "verified"));
    assert.ok(report.stages.some((stage) => stage.name === "configured-runtime" && stage.status === "activated"));
    assert.ok(report.stages.some((stage) => stage.name === "activation" && stage.status === "restart_required"));
    assert.deepEqual(report.restart_clients, ["codex"]);

    const dataDir = process.platform === "darwin"
      ? join(home, "Library", "Application Support", "gitcontribute", "Data")
      : join(home, ".local", "share", "gitcontribute");
    const managed = join(dataDir, "bin", packageVersion, "gitcontribute");
    const codex = await readFile(join(home, ".codex", "config.toml"), "utf8");
    assert.ok(codex.includes(`command = ${JSON.stringify(managed)}`));
    assert.doesNotMatch(codex, /0\.17\.0|npx|npm-cache/);
    const metadata = run(managed, ["metadata", "--json"], { env: { ...process.env, HOME: home } });
    assert.equal(JSON.parse(metadata.stdout).version, packageVersion);

    const mcp = startMCPStdio(managed, {
      ...process.env,
      HOME: home,
      XDG_CONFIG_HOME: join(home, ".config"),
      XDG_DATA_HOME: join(home, ".local", "share"),
      XDG_CACHE_HOME: join(home, ".cache"),
      XDG_STATE_HOME: join(home, ".local", "state"),
    });
    try {
      const initialized = await mcp.request(1, "initialize", {
        protocolVersion: "2025-11-25",
        capabilities: {},
        clientInfo: { name: "gitcontribute-upgrade-e2e", version: "1" },
      });
      assert.equal(initialized.result.serverInfo.name, "gitcontribute");
      assert.equal(initialized.result.protocolVersion, "2025-11-25");
      mcp.notify("notifications/initialized", {});
      const tools = await mcp.request(2, "tools/list", {});
      assert.ok(tools.result.tools.some((tool) => tool.name === "corpus.search_code"));
    } finally {
      await mcp.close();
    }
    assert.deepEqual(registry.requests, [{ method: "GET", url: "/gitcontribute" }]);
    await assert.rejects(readFile(join(globalPrefix, "lib", "node_modules", "gitcontribute", "package.json")));
  } finally {
    await registry.close();
    await rm(workspace, { recursive: true, force: true });
  }
});

test("packaged setup can install the CLI without configuring MCP", { skip: process.platform === "win32" }, async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-setup-e2e-"));
  try {
    const tarball = await packCurrentPlatform(workspace);
    const runner = join(workspace, "runner");
    run("npm", ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]);

    const { fakeBin, globalPrefix, npmLog } = await createFakeGlobalNPM(workspace);

    const home = join(workspace, "home");
    await mkdir(home);
    const command = join(runner, "node_modules", ".bin", "gitcontribute");
    const result = run(
      command,
      ["setup", "--mode", "cli", "--token-source", "none", "--yes", "--json"],
      {
        env: {
          ...process.env,
          HOME: home,
          XDG_CONFIG_HOME: join(home, ".config"),
          XDG_DATA_HOME: join(home, ".local", "share"),
          XDG_CACHE_HOME: join(home, ".cache"),
          XDG_STATE_HOME: join(home, ".local", "state"),
          PATH: `${fakeBin}:${process.env.PATH}`,
          npm_command: "exec",
          FAKE_NPM_LOG: npmLog,
          FAKE_NPM_PREFIX: globalPrefix,
        },
      }
    );

    const report = JSON.parse(result.stdout);
    assert.ok(report.steps.some((step) => step.name === "cli" && step.status === "installed"));
    assert.equal(report.mcp_command, undefined);
    assert.ok((await readFile(npmLog, "utf8")).includes(`install --global gitcontribute@${packageVersion}`));
    await readFile(join(globalPrefix, "bin", "gitcontribute"), "utf8");
    await assert.rejects(readFile(join(home, ".codex", "config.toml"), "utf8"));
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

test("npx setup installs a private native MCP runtime and registers its absolute path", { skip: process.platform === "win32" }, async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-setup-e2e-"));
  try {
    const tarball = await packCurrentPlatform(workspace);
    const runner = join(workspace, "runner");
    run("npm", ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]);

    const home = join(workspace, "home");
    await mkdir(join(home, ".codex"), { recursive: true });
    const result = run(
      "npx",
      ["--yes", "gitcontribute", "setup", "--mode", "mcp", "--codex", "--token-source", "none", "--yes", "--json"],
      {
        cwd: runner,
        env: {
          ...process.env,
          HOME: home,
          XDG_CONFIG_HOME: join(home, ".config"),
          XDG_DATA_HOME: join(home, ".local", "share"),
          XDG_CACHE_HOME: join(home, ".cache"),
          XDG_STATE_HOME: join(home, ".local", "state"),
          npm_config_offline: "true",
          npm_command: "exec",
        },
      }
    );

    const report = JSON.parse(result.stdout);
    const dataDir = process.platform === "darwin"
      ? join(home, "Library", "Application Support", "gitcontribute", "Data")
      : join(home, ".local", "share", "gitcontribute");
    const managed = join(dataDir, "bin", packageVersion, "gitcontribute");
    assert.deepEqual(report.mcp_command, {
      command: managed,
      args: ["mcp", "serve", "--transport=stdio"],
    });
    assert.ok(
      report.steps.some(
        (step) => step.name === "mcp-runtime" && step.status === "installed" && step.path === managed
      )
    );
    assert.ok(report.steps.some((step) => step.name === "codex" && step.status === "configured"));
    const codex = await readFile(join(home, ".codex", "config.toml"), "utf8");
    assert.ok(codex.includes(`command = ${JSON.stringify(managed)}`));
    assert.match(codex, /args = \["mcp", "serve", "--transport=stdio"\]/);
    assert.doesNotMatch(codex, /npx|npm-cache/);
    const metadata = run(managed, ["metadata", "--json"], { env: process.env });
    assert.equal(JSON.parse(metadata.stdout).version, packageVersion);
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

test(
  "packaged interactive setup advances from MCP access to explicit agent targets",
  { skip: process.platform === "win32" },
  async () => {
    const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-setup-pty-e2e-"));
    try {
      const tarball = await packCurrentPlatform(workspace);
      const runner = join(workspace, "runner");
      run("npm", ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]);

      const home = join(workspace, "home");
      await mkdir(join(home, ".codex"), { recursive: true });
      const command = join(runner, "node_modules", ".bin", "gitcontribute");
      const input = "\x1b]11;rgb:0000/0000/0000\x07\x1b[1;1R";
      const env = {
        ...process.env,
        HOME: home,
        XDG_CONFIG_HOME: join(home, ".config"),
        XDG_DATA_HOME: join(home, ".local", "share"),
        XDG_CACHE_HOME: join(home, ".cache"),
        XDG_STATE_HOME: join(home, ".local", "state"),
        npm_command: "exec",
        TERM: "xterm-256color",
        GITCONTRIBUTE_E2E_COMMAND: command,
      };
      const result =
        process.platform === "darwin"
          ? spawnSync(
              "expect",
              [
                "-c",
                String.raw`set timeout 20
stty rows 40 columns 120
spawn -noecho $env(GITCONTRIBUTE_E2E_COMMAND) setup
exec stty rows 40 columns 120 < $spawn_out(slave,name)
expect {
  -exact "\033\[6n" { send "\033]11;rgb:0000/0000/0000\007\033\[1;1R" }
  timeout { exit 6 }
  eof { exit 7 }
}
expect {
  -re {How do you want to use GitContribute\?} { send "\r" }
  timeout { exit 2 }
  eof { exit 3 }
}
expect {
  -re {Which coding agents should GitContribute configure\?} { send "\033\[Z" }
  timeout { exit 4 }
  eof { exit 5 }
}
expect {
  -re {How do you want to use GitContribute\?} { send "\033\[B\r" }
  timeout { exit 8 }
  eof { exit 9 }
}
expect {
  -re {future GitHub syncs authenticate\?} { send "\003" }
  timeout { exit 10 }
  eof { exit 11 }
}
expect eof
catch wait result
exit [lindex $result 3]`,
              ],
              { encoding: "utf8", env }
            )
          : await runScriptPTY(command, input, env);

      assert.ok(
        result.status === 0 || (process.platform !== "darwin" && result.status === 130),
        result.stderr || result.stdout
      );
      const transcript = `${result.stdout}\n${result.stderr}`;
      assert.match(transcript, /How do you want to use GitContribute\?/);
      assert.match(transcript, /Which coding agents should GitContribute configure\?/);
      assert.match(transcript, /future GitHub syncs authenticate\?/);
      if (result.status === 0) {
        assert.match(transcript, /Setup cancelled; no changes were made\./);
      }
      await assert.rejects(readFile(join(home, ".codex", "config.toml"), "utf8"));
    } finally {
      await rm(workspace, { recursive: true, force: true });
    }
  }
);

test(
  "direct npx package runner ignores a stale executable on PATH",
  { skip: process.platform === "win32" },
  async () => {
    const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-npx-e2e-"));
    try {
      const tarball = await packCurrentPlatform(workspace);
      const npmCache = join(workspace, "npm-cache");
      const runner = join(workspace, "runner");
      run(
        "npm",
        ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]
      );

      const staleBin = join(workspace, "stale-bin");
      await mkdir(staleBin);
      const staleCommand = join(staleBin, "gitcontribute");
      await writeFile(
        staleCommand,
        `#!/bin/sh
printf '%s\\n' '{"name":"gitcontribute","version":"0.0.0-stale"}'
`
      );
      await chmod(staleCommand, 0o755);

      const home = join(workspace, "home");
      await mkdir(home);
      const result = run(
        "npx",
        ["--yes", `gitcontribute@${packageVersion}`, "metadata", "--json"],
        {
          cwd: runner,
          env: {
            ...process.env,
            HOME: home,
            PATH: `${staleBin}:${process.env.PATH}`,
            npm_config_cache: npmCache,
            npm_config_offline: "true",
            npm_config_audit: "false",
            npm_config_fund: "false",
            npm_config_update_notifier: "false",
          },
        }
      );

      assert.equal(JSON.parse(result.stdout).version, packageVersion);
    } finally {
      await rm(workspace, { recursive: true, force: true });
    }
  }
);

test("Both uses the installed CLI for MCP without a private runtime", { skip: process.platform === "win32" }, async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-setup-e2e-"));
  try {
    const tarball = await packCurrentPlatform(workspace);
    const runner = join(workspace, "runner");
    run("npm", ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]);
    const { fakeBin, globalPrefix, npmLog } = await createFakeGlobalNPM(workspace);

    const home = join(workspace, "home");
    await mkdir(join(home, ".codex"), { recursive: true });
    const command = join(runner, "node_modules", ".bin", "gitcontribute");
    const result = run(
      command,
      ["setup", "--mode", "both", "--codex", "--token-source", "none", "--yes", "--json"],
      {
        env: {
          ...process.env,
          HOME: home,
          XDG_CONFIG_HOME: join(home, ".config"),
          XDG_DATA_HOME: join(home, ".local", "share"),
          XDG_CACHE_HOME: join(home, ".cache"),
          XDG_STATE_HOME: join(home, ".local", "state"),
          PATH: `${fakeBin}:${process.env.PATH}`,
          npm_command: "exec",
          FAKE_NPM_LOG: npmLog,
          FAKE_NPM_PREFIX: globalPrefix,
        },
      }
    );

    const installedCommand = join(globalPrefix, "bin", "gitcontribute");
    const report = JSON.parse(result.stdout);
    assert.deepEqual(report.mcp_command, {
      command: installedCommand,
      args: ["mcp", "serve", "--transport=stdio"],
    });
    assert.ok(
      report.steps.some(
        (step) => step.name === "cli" && step.status === "installed" && step.path === installedCommand
      )
    );
    assert.ok(report.steps.some((step) => step.name === "codex" && step.status === "configured"));
    const codex = await readFile(join(home, ".codex", "config.toml"), "utf8");
    assert.match(codex, new RegExp(`command = "${installedCommand.replaceAll("\\", "\\\\")}"`));
    assert.doesNotMatch(codex, /command = "npx"/);
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

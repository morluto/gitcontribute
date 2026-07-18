import assert from "node:assert/strict";
import { chmod, copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
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

function shellQuote(value) {
  return `'${value.replaceAll("'", `'\\''`)}'`;
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

test("packaged setup can install the terminal app without configuring MCP", { skip: process.platform === "win32" }, async () => {
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
      ["setup", "--install-cli", "--no-mcp", "--token-source", "none", "--yes", "--json"],
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
    assert.ok(report.steps.some((step) => step.name === "terminal" && step.status === "installed"));
    assert.equal(report.launcher, undefined);
    assert.ok((await readFile(npmLog, "utf8")).includes(`install --global gitcontribute@${packageVersion}`));
    await readFile(join(globalPrefix, "bin", "gitcontribute"), "utf8");
    await assert.rejects(readFile(join(home, ".codex", "config.toml"), "utf8"));
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

test("project-local npx setup launches the packaged CLI and configures MCP", { skip: process.platform === "win32" }, async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-setup-e2e-"));
  try {
    const tarball = await packCurrentPlatform(workspace);
    const runner = join(workspace, "runner");
    run("npm", ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]);

    const home = join(workspace, "home");
    await mkdir(join(home, ".codex"), { recursive: true });
    const result = run(
      "npx",
      ["--yes", "gitcontribute", "setup", "--codex", "--token-source", "none", "--yes", "--json"],
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
    assert.equal(report.launcher, `npx --yes gitcontribute@${packageVersion} mcp`);
    assert.ok(report.steps.some((step) => step.name === "codex" && step.status === "configured"));
    assert.ok(
      report.steps.some(
        (step) =>
          step.name === "terminal" &&
          step.status === "not installed" &&
          step.message.includes(`npm install --global gitcontribute@${packageVersion}`)
      )
    );
    const codex = await readFile(join(home, ".codex", "config.toml"), "utf8");
    assert.match(codex, /command = "npx"/);
    assert.ok(codex.includes(`"gitcontribute@${packageVersion}"`));
    assert.doesNotMatch(codex, /--package=/);
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

test(
  "packaged interactive setup advances after keeping npx",
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
      const input = "\x1b]11;rgb:0000/0000/0000\x07\x1b[1;1R\x1b[B\r\x03";
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
  -re {How do you want to run GitContribute\?} { send "\033\[B\r" }
  timeout { exit 2 }
  eof { exit 3 }
}
expect {
  -re {Use GitContribute from coding agents} { send "\003" }
  timeout { exit 4 }
  eof { exit 5 }
}
expect eof
catch wait result
exit [lindex $result 3]`,
              ],
              { encoding: "utf8", env }
            )
          : spawnSync("script", ["-qefc", `${shellQuote(command)} setup`, "/dev/null"], {
              encoding: "utf8",
              env,
              input,
            });

      assert.equal(result.status, 0, result.stderr || result.stdout);
      const transcript = `${result.stdout}\n${result.stderr}`;
      assert.match(transcript, /How do you want to run GitContribute\?/);
      assert.match(transcript, /Use GitContribute from coding agents/);
      assert.match(transcript, /Setup cancelled; no changes were made\./);
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

test("packaged setup uses the installed terminal app for MCP", { skip: process.platform === "win32" }, async () => {
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
      ["setup", "--install-cli", "--codex", "--token-source", "none", "--yes", "--json"],
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
    assert.equal(report.launcher, `${installedCommand} mcp`);
    assert.ok(
      report.steps.some(
        (step) => step.name === "terminal" && step.status === "installed" && step.path === installedCommand
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

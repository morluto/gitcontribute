import assert from "node:assert/strict";
import { chmod, copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

import platforms from "./platforms.json" with { type: "json" };

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { encoding: "utf8", ...options });
  assert.equal(result.status, 0, result.stderr || result.stdout);
  return result;
}

async function packCurrentPlatform(workspace) {
  const packageJSON = JSON.parse(await readFile(join(root, "package.json"), "utf8"));
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
  run("go", ["build", "-ldflags", `-X main.version=${packageJSON.version}`, "-o", executable, "./cmd/gitcontribute"], { cwd: root });
  if (process.platform !== "win32") await chmod(executable, 0o755);

  const packs = join(workspace, "packs");
  await mkdir(packs);
  run("npm", ["pack", "--silent", "--pack-destination", packs], { cwd: staging });
  return join(packs, `gitcontribute-${packageJSON.version}.tgz`);
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
    assert.match(await readFile(npmLog, "utf8"), /install --global gitcontribute@0\.1\.1/);
    await readFile(join(globalPrefix, "bin", "gitcontribute"), "utf8");
    await assert.rejects(readFile(join(home, ".codex", "config.toml"), "utf8"));
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

test("packaged setup can configure MCP without installing the terminal app", { skip: process.platform === "win32" }, async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-setup-e2e-"));
  try {
    const tarball = await packCurrentPlatform(workspace);
    const runner = join(workspace, "runner");
    run("npm", ["install", "--prefix", runner, "--ignore-scripts", "--no-audit", "--no-fund", tarball]);

    const home = join(workspace, "home");
    await mkdir(join(home, ".codex"), { recursive: true });
    const command = join(runner, "node_modules", ".bin", "gitcontribute");
    const result = run(
      command,
      ["setup", "--codex", "--token-source", "none", "--yes", "--json"],
      {
        env: {
          ...process.env,
          HOME: home,
          XDG_CONFIG_HOME: join(home, ".config"),
          XDG_DATA_HOME: join(home, ".local", "share"),
          XDG_CACHE_HOME: join(home, ".cache"),
          XDG_STATE_HOME: join(home, ".local", "state"),
          npm_command: "exec",
        },
      }
    );

    const report = JSON.parse(result.stdout);
    assert.equal(
      report.launcher,
      "npx --yes --package=gitcontribute@0.1.1 -- gitcontribute mcp"
    );
    assert.ok(report.steps.some((step) => step.name === "codex" && step.status === "configured"));
    assert.ok(
      report.steps.some(
        (step) =>
          step.name === "terminal" &&
          step.status === "not installed" &&
          step.message.includes("npm install --global gitcontribute@0.1.1")
      )
    );
    const codex = await readFile(join(home, ".codex", "config.toml"), "utf8");
    assert.match(codex, /command = "npx"/);
    assert.match(codex, /--package=gitcontribute@0\.1\.1/);
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

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

import assert from "node:assert/strict";
import { chmod, copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

test("launcher selects the host binary and forwards arguments and exit status", async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-launcher-"));
  try {
    const packageDir = join(workspace, "package");
    const target = `${process.platform}-${process.arch}`;
    const binary = process.platform === "win32" ? "gitcontribute.cmd" : "gitcontribute";
    await mkdir(join(packageDir, "npm", "bin", "native", target), { recursive: true });
    await copyFile(join(root, "npm", "bin", "gitcontribute.cjs"), join(packageDir, "npm", "bin", "gitcontribute.cjs"));
    await writeFile(join(packageDir, "npm", "platforms.json"), JSON.stringify({ [`${process.platform}-${process.arch}`]: { target, binary } }));
    const fixture = join(packageDir, "npm", "bin", "native", target, binary);
    if (process.platform === "win32") {
      await writeFile(fixture, "@echo off\r\necho %*\r\nexit /b 7\r\n");
    } else {
      await writeFile(fixture, "#!/bin/sh\nprintf '%s\\n' \"$*\"\nexit 7\n");
      await chmod(fixture, 0o755);
    }
    const result = spawnSync(process.execPath, [join(packageDir, "npm", "bin", "gitcontribute.cjs"), "two words", "--flag=value"], { encoding: "utf8" });
    assert.equal(result.status, 7);
    assert.match(result.stdout, /two words --flag=value/);
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

test("published package has no install lifecycle", async () => {
  const pkg = JSON.parse(await readFile(join(root, "package.json"), "utf8"));
  for (const name of ["preinstall", "install", "postinstall"]) assert.equal(pkg.scripts?.[name], undefined);
});

test("launcher distinguishes source checkout from incomplete package", async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-launcher-missing-"));
  try {
    const target = `${process.platform}-${process.arch}`;
    const binary = process.platform === "win32" ? "gitcontribute.exe" : "gitcontribute";
    const makeFixture = async (name, sourceCheckout) => {
      const packageDir = join(workspace, name);
      await mkdir(join(packageDir, "npm", "bin"), { recursive: true });
      await copyFile(join(root, "npm", "bin", "gitcontribute.cjs"), join(packageDir, "npm", "bin", "gitcontribute.cjs"));
      await writeFile(join(packageDir, "npm", "platforms.json"), JSON.stringify({ [target]: { target, binary } }));
      if (sourceCheckout) {
        await writeFile(join(packageDir, "go.mod"), "module example.test/gitcontribute\n");
      }
      return spawnSync(process.execPath, [join(packageDir, "npm", "bin", "gitcontribute.cjs"), "setup"], { encoding: "utf8" });
    };

    const source = await makeFixture("source", true);
    assert.equal(source.status, 1);
    assert.match(source.stderr, /source checkout or local package/);
    assert.match(source.stderr, /npx --yes gitcontribute@latest setup/);
    assert.doesNotMatch(source.stderr, /report the incomplete npm artifact/);

    const installed = await makeFixture("installed", false);
    assert.equal(installed.status, 1);
    assert.match(installed.stderr, /native binary is missing/);
    assert.match(installed.stderr, /report the incomplete npm artifact/);
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

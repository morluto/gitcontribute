#!/usr/bin/env node
"use strict";

const { spawn } = require("node:child_process");
const { existsSync, readFileSync } = require("node:fs");
const { join } = require("node:path");

const manifest = JSON.parse(readFileSync(join(__dirname, "..", "platforms.json"), "utf8"));
const key = `${process.platform}-${process.arch}`;
const platform = manifest[key];

if (!platform) {
  const supported = Object.keys(manifest).sort().join(", ");
  console.error(`gitcontribute does not provide an npm binary for ${key}. Supported platforms: ${supported}.`);
  console.error("Build from source with: go install github.com/morluto/gitcontribute/cmd/gitcontribute@latest");
  process.exit(1);
}

const executable = join(__dirname, "native", platform.target, platform.binary);
if (!existsSync(executable)) {
  const packageRoot = join(__dirname, "..", "..");
  if (existsSync(join(packageRoot, ".git")) || existsSync(join(packageRoot, "go.mod"))) {
    console.error(`gitcontribute is running from a source checkout or local package at ${packageRoot}.`);
    console.error("Source packages do not include release-built native binaries. Run the published package with:");
    console.error("  npx --yes gitcontribute@latest setup");
    process.exit(1);
  }
  console.error(`gitcontribute native binary is missing for ${key}: ${executable}`);
  console.error("Reinstall the package, or report the incomplete npm artifact.");
  process.exit(1);
}

const child = spawn(executable, process.argv.slice(2), { stdio: "inherit", windowsHide: false });
for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
  process.on(signal, () => {
    if (!child.killed) child.kill(signal);
  });
}
child.on("error", (error) => {
  console.error(`failed to start gitcontribute: ${error.message}`);
  process.exitCode = 1;
});
child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exitCode = code ?? 1;
});

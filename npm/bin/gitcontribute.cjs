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
const signalHandlers = new Map();
for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
  const handler = () => {
    if (!child.killed) child.kill(signal);
  };
  signalHandlers.set(signal, handler);
  process.on(signal, handler);
}
child.on("error", (error) => {
  console.error(`failed to start gitcontribute: ${error.message}`);
  process.exitCode = 1;
});
child.on("exit", (code, signal) => {
  if (signal) {
    const handler = signalHandlers.get(signal);
    if (handler) process.off(signal, handler);
    try {
      process.kill(process.pid, signal);
    } catch {
      process.exitCode = 1;
    }
    return;
  }
  process.exitCode = code ?? 1;
});

import { spawn } from "node:child_process";

const expectedVersion = process.argv[2];
if (!expectedVersion) throw new Error("expected version argument is required");

// Public discoverability is a registry-metadata property: the version must be
// resolvable and the `latest` dist-tag must point at it. The published binary
// is already smoke-tested earlier in the release job via the local tarball, so
// re-resolving `@latest` through npx here would only re-download the native
// tarball to re-confirm metadata that `npm view` already proves. That extra
// round-trip was the flaky straggler that failed the 3.0.0 release.
const registry = "https://registry.npmjs.org";
const attempts = positiveInteger("GITCONTRIBUTE_NPM_PUBLICATION_ATTEMPTS", 30, 60);
const delayMS = positiveInteger("GITCONTRIBUTE_NPM_PUBLICATION_DELAY_MS", 5_000, 60_000);
const probeTimeoutMS = positiveInteger("GITCONTRIBUTE_NPM_PUBLICATION_PROBE_TIMEOUT_MS", 30_000, 120_000);
const npm = process.env.GITCONTRIBUTE_NPM_COMMAND || "npm";

for (let attempt = 1; attempt <= attempts; attempt += 1) {
  try {
    const latest = await output(npm, ["view", "gitcontribute", "dist-tags.latest", "--json", "--prefer-online", `--registry=${registry}`]);
    const published = await output(npm, ["view", `gitcontribute@${expectedVersion}`, "version", "--json", "--prefer-online", `--registry=${registry}`]);
    if (jsonString(latest) === expectedVersion && jsonString(published) === expectedVersion) {
      console.log(`npm release ${expectedVersion} is publicly discoverable`);
      process.exit(0);
    }
  } catch {
    // Registry propagation is expected to be transient immediately after
    // publication. The bounded retry loop owns those probes as one operation.
  }
  if (attempt < attempts) await new Promise((resolve) => setTimeout(resolve, delayMS));
}

throw new Error(`npm release ${expectedVersion} did not become publicly discoverable after ${attempts} attempts`);

function positiveInteger(name, fallback, maximum) {
  const value = process.env[name];
  if (value === undefined || value === "") return fallback;
  if (!/^[1-9][0-9]*$/.test(value)) throw new Error(`${name} must be a positive integer`);
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed > maximum) throw new Error(`${name} must be at most ${maximum}`);
  return parsed;
}

function jsonString(value) {
  try {
    const parsed = JSON.parse(value);
    return typeof parsed === "string" ? parsed : "";
  } catch {
    return "";
  }
}

function output(command, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { stdio: ["ignore", "pipe", "ignore"] });
    let stdout = "";
    let forceKill;
    const timeout = setTimeout(() => {
      child.kill("SIGTERM");
      forceKill = setTimeout(() => child.kill("SIGKILL"), 1_000);
    }, probeTimeoutMS);
    const finish = (callback, value) => {
      clearTimeout(timeout);
      clearTimeout(forceKill);
      callback(value);
    };
    child.stdout.setEncoding("utf8");
    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.on("error", (error) => finish(reject, error));
    child.on("close", (code, signal) => {
      if (code === 0) return finish(resolve, stdout.trim());
      finish(reject, new Error(`${command} exited with code ${code ?? "null"}${signal ? ` (${signal})` : ""}`));
    });
  });
}

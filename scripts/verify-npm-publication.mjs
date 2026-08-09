import { spawn } from "node:child_process";

const expectedVersion = process.argv[2];
if (!expectedVersion) throw new Error("expected version argument is required");

const registry = "https://registry.npmjs.org";
const attempts = positiveInteger("GITCONTRIBUTE_NPM_PUBLICATION_ATTEMPTS", 10, 30);
const delayMS = positiveInteger("GITCONTRIBUTE_NPM_PUBLICATION_DELAY_MS", 6_000, 60_000);
const npm = process.env.GITCONTRIBUTE_NPM_COMMAND || "npm";
const npx = process.env.GITCONTRIBUTE_NPX_COMMAND || "npx";

for (let attempt = 1; attempt <= attempts; attempt += 1) {
  try {
    const latest = await output(npm, ["view", "gitcontribute", "dist-tags.latest", "--json", "--prefer-online", `--registry=${registry}`]);
    const published = await output(npm, ["view", `gitcontribute@${expectedVersion}`, "version", "--json", "--prefer-online", `--registry=${registry}`]);
    if (jsonString(latest) === expectedVersion && jsonString(published) === expectedVersion) {
      const metadata = await output(npx, ["--yes", "--prefer-online", "gitcontribute@latest", "metadata", "--json"]);
      if (JSON.parse(metadata).version === expectedVersion) {
        console.log(`npm release ${expectedVersion} is publicly discoverable`);
        process.exit(0);
      }
    }
  } catch {
    // Registry propagation and fresh npx resolution are expected to be
    // transient immediately after publication. The bounded retry loop owns
    // those probes as one operation.
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
    const child = spawn(command, args, { stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "";
    child.stdout.setEncoding("utf8");
    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.on("error", reject);
    child.on("close", (code, signal) => {
      if (code === 0) return resolve(stdout.trim());
      reject(new Error(`${command} exited with code ${code ?? "null"}${signal ? ` (${signal})` : ""}`));
    });
  });
}

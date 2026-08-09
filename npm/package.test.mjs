import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

import platforms from "./platforms.json" with { type: "json" };

const root = new URL("..", import.meta.url).pathname;
const packageVersion = JSON.parse(await readFile(join(root, "package.json"), "utf8")).version;

test("build assembles every native binary into one npm tarball", async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-package-"));
  const artifacts = join(workspace, "artifacts");
  const packs = join(workspace, "packs");
  try {
    await mkdir(packs);
    for (const platform of Object.values(platforms)) {
      const directory = join(artifacts, platform.target);
      await mkdir(directory, { recursive: true });
      await writeFile(join(directory, platform.binary), `fixture:${platform.target}\n`);
    }
    const build = spawnSync(process.execPath, [join(root, "scripts", "build-npm-package.mjs")], {
      cwd: root,
      env: { ...process.env, GITCONTRIBUTE_ARTIFACTS: artifacts },
      encoding: "utf8",
    });
    assert.equal(build.status, 0, build.stderr || build.stdout);
    const packed = spawnSync("npm", ["pack", "--silent", "--pack-destination", packs], { cwd: root, encoding: "utf8" });
    assert.equal(packed.status, 0, packed.stderr || packed.stdout);
    const tarballs = await readdir(packs);
    assert.deepEqual(tarballs, [`gitcontribute-${packageVersion}.tgz`]);
    const listing = spawnSync("tar", ["-tzf", join(packs, tarballs[0])], { encoding: "utf8" });
    assert.equal(listing.status, 0, listing.stderr);
    for (const platform of Object.values(platforms)) {
      assert.match(listing.stdout, new RegExp(`package/npm/bin/native/${platform.target}/${platform.binary.replace(".", "\\.")}`));
    }
  } finally {
    await rm(join(root, "npm", "bin", "native"), { recursive: true, force: true });
    await rm(workspace, { recursive: true, force: true });
  }
});

test("release matrix is derived from the npm platform manifest", () => {
  const matrix = spawnSync(process.execPath, [join(root, "scripts", "release-platform-matrix.mjs")], {
    cwd: root,
    encoding: "utf8",
  });
  assert.equal(matrix.status, 0, matrix.stderr || matrix.stdout);
  assert.deepEqual(JSON.parse(matrix.stdout).include, Object.values(platforms));
});

test("MCP Registry metadata identifies the published package", async () => {
  const pkg = JSON.parse(await readFile(join(root, "package.json"), "utf8"));
  const server = JSON.parse(await readFile(join(root, "server.json"), "utf8"));
  assert.equal(pkg.mcpName, "io.github.morluto/gitcontribute");
  assert.equal(server.name, pkg.mcpName);
  assert.equal(server.version, pkg.version);
  assert.equal(server.repository.url, "https://github.com/morluto/gitcontribute");
  assert.deepEqual(server.packages, [{
    registryType: "npm",
    identifier: pkg.name,
    version: pkg.version,
    transport: { type: "stdio" },
  }]);
});

test("Release Please versions all MCP Registry metadata fields", async () => {
  const config = JSON.parse(await readFile(join(root, "release-please-config.json"), "utf8"));
  const registryVersionFields = config.packages["."]["extra-files"].filter(({ path }) => path === "server.json");
  assert.deepEqual(registryVersionFields, [
    { type: "json", path: "server.json", jsonpath: "$.version" },
    { type: "json", path: "server.json", jsonpath: "$.packages[*].version" },
  ]);
});

test("release verifies npm discovery before publishing MCP Registry metadata", async () => {
  const workflow = await readFile(join(root, ".github", "workflows", "release.yml"), "utf8");
  const npmVerification = workflow.indexOf("Verify npm publication is publicly discoverable");
  const mcpPublication = workflow.indexOf("Publish MCP Registry metadata");
  assert.ok(npmVerification >= 0, "release workflow must verify npm publication");
  assert.ok(mcpPublication > npmVerification, "MCP Registry metadata must follow npm discovery verification");
  assert.match(workflow, /node scripts\/verify-npm-publication\.mjs/);
});

test("publication verification requires the public latest tag and a fresh npx runtime", async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-publication-check-"));
  try {
    const client = join(workspace, "registry-client");
    const log = join(workspace, "calls.log");
    await writeFile(client, `#!/usr/bin/env node
const fs = require("node:fs");
const args = process.argv.slice(2);
fs.appendFileSync(process.env.GITCONTRIBUTE_TEST_CALL_LOG, JSON.stringify(args) + "\\n");
if (args[0] === "view" && args[2] === "dist-tags.latest") process.stdout.write('"1.2.3"\\n');
else if (args[0] === "view" && args[1] === "gitcontribute@1.2.3" && args[2] === "version") process.stdout.write('"1.2.3"\\n');
else if (args[0] === "--yes") process.stdout.write('{"version":"1.2.3"}\\n');
else process.exitCode = 1;
`);
    await chmod(client, 0o755);

    const result = spawnSync(process.execPath, [join(root, "scripts", "verify-npm-publication.mjs"), "1.2.3"], {
      encoding: "utf8",
      env: {
        ...process.env,
        GITCONTRIBUTE_NPM_COMMAND: client,
        GITCONTRIBUTE_NPX_COMMAND: client,
        GITCONTRIBUTE_NPM_PUBLICATION_ATTEMPTS: "1",
        GITCONTRIBUTE_TEST_CALL_LOG: log,
      },
    });
    assert.equal(result.status, 0, result.stderr || result.stdout);
    const calls = (await readFile(log, "utf8")).trim().split("\n").map(JSON.parse);
    assert.deepEqual(calls, [
      ["view", "gitcontribute", "dist-tags.latest", "--json", "--prefer-online", "--registry=https://registry.npmjs.org"],
      ["view", "gitcontribute@1.2.3", "version", "--json", "--prefer-online", "--registry=https://registry.npmjs.org"],
      ["--yes", "--prefer-online", "gitcontribute@latest", "metadata", "--json"],
    ]);
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

test("publication verification retries transient registry probe failures", async () => {
  const workspace = await mkdtemp(join(tmpdir(), "gitcontribute-publication-retry-"));
  try {
    const client = join(workspace, "registry-client");
    const state = join(workspace, "attempts");
    await writeFile(client, `#!/usr/bin/env node
const fs = require("node:fs");
const count = fs.existsSync(process.env.GITCONTRIBUTE_TEST_ATTEMPTS) ? Number(fs.readFileSync(process.env.GITCONTRIBUTE_TEST_ATTEMPTS, "utf8")) : 0;
fs.writeFileSync(process.env.GITCONTRIBUTE_TEST_ATTEMPTS, String(count + 1));
if (count === 0) process.exitCode = 1;
else if (process.argv[2] === "view" && process.argv[4] === "dist-tags.latest") process.stdout.write('"1.2.3"\\n');
else if (process.argv[2] === "view" && process.argv[3] === "gitcontribute@1.2.3" && process.argv[4] === "version") process.stdout.write('"1.2.3"\\n');
else if (process.argv[2] === "--yes") process.stdout.write('{"version":"1.2.3"}\\n');
else process.exitCode = 1;
`);
    await chmod(client, 0o755);

    const result = spawnSync(process.execPath, [join(root, "scripts", "verify-npm-publication.mjs"), "1.2.3"], {
      encoding: "utf8",
      env: {
        ...process.env,
        GITCONTRIBUTE_NPM_COMMAND: client,
        GITCONTRIBUTE_NPX_COMMAND: client,
        GITCONTRIBUTE_NPM_PUBLICATION_ATTEMPTS: "2",
        GITCONTRIBUTE_NPM_PUBLICATION_DELAY_MS: "1",
        GITCONTRIBUTE_TEST_ATTEMPTS: state,
      },
    });
    assert.equal(result.status, 0, result.stderr || result.stdout);
    assert.equal(await readFile(state, "utf8"), "4");
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
});

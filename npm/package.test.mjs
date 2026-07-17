import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
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

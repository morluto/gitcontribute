import { chmod, copyFile, mkdir, readFile, stat } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const artifacts = resolve(process.env.GITCONTRIBUTE_ARTIFACTS || join(root, "dist"));
const manifest = JSON.parse(await readFile(join(root, "npm", "platforms.json"), "utf8"));

for (const platform of Object.values(manifest)) {
  const source = join(artifacts, platform.target, platform.binary);
  const targetDir = join(root, "npm", "bin", "native", platform.target);
  const target = join(targetDir, platform.binary);
  try {
    const info = await stat(source);
    if (!info.isFile()) throw new Error("not a file");
  } catch (error) {
    throw new Error(`missing native artifact ${source}: ${error.message}`);
  }
  await mkdir(targetDir, { recursive: true });
  await copyFile(source, target);
  if (!platform.binary.endsWith(".exe")) await chmod(target, 0o755);
}

console.log(`assembled gitcontribute npm package from ${artifacts}`);

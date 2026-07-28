import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const manifest = JSON.parse(await readFile(join(root, "npm", "platforms.json"), "utf8"));

const include = Object.entries(manifest).map(([key, platform]) => {
  for (const field of ["target", "binary", "goos", "goarch"]) {
    if (typeof platform[field] !== "string" || platform[field] === "") {
      throw new Error(`platform ${key} is missing ${field}`);
    }
  }
  if (key !== `${platform.goos === "windows" ? "win32" : platform.goos}-${platform.goarch === "amd64" ? "x64" : platform.goarch}`) {
    throw new Error(`platform ${key} does not match its Go target`);
  }
  return platform;
});

process.stdout.write(JSON.stringify({ include }));

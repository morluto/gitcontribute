import { readFile } from "node:fs/promises";

const expected = process.argv[2];
if (!expected) throw new Error("expected version argument is required");
const pkg = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));
if (pkg.version !== expected) {
  throw new Error(`release tag version ${expected} does not match package.json ${pkg.version}`);
}
console.log(`release version ${expected} verified`);

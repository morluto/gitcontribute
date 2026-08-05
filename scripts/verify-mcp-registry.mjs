import { readFile } from "node:fs/promises";

const expectedVersion = process.argv[2];
const packageJSON = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));
const serverJSON = JSON.parse(await readFile(new URL("../server.json", import.meta.url), "utf8"));
const packageName = packageJSON.mcpName;

if (packageName !== "io.github.morluto/gitcontribute") {
  throw new Error(`package.json mcpName ${packageName ?? "<missing>"} does not identify this server`);
}
if (serverJSON.name !== packageName) {
  throw new Error(`server.json name ${serverJSON.name} does not match package.json mcpName ${packageName}`);
}
if (expectedVersion && serverJSON.version !== expectedVersion) {
  throw new Error(`server.json version ${serverJSON.version} does not match release ${expectedVersion}`);
}
if (serverJSON.version !== packageJSON.version) {
  throw new Error(`server.json version ${serverJSON.version} does not match package.json ${packageJSON.version}`);
}
if (serverJSON.repository?.url !== "https://github.com/morluto/gitcontribute" || serverJSON.repository?.source !== "github") {
  throw new Error("server.json repository does not identify the GitHub source repository");
}
if (!Array.isArray(serverJSON.packages) || serverJSON.packages.length !== 1) {
  throw new Error("server.json must describe exactly one npm package");
}

const [pkg] = serverJSON.packages;
if (pkg.registryType !== "npm" || pkg.identifier !== packageJSON.name || pkg.version !== serverJSON.version || pkg.transport?.type !== "stdio") {
  throw new Error("server.json npm package metadata is not aligned with package.json");
}

console.log(`MCP Registry metadata verified for ${serverJSON.name} ${serverJSON.version}`);

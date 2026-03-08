import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const extensionRoot = path.resolve(__dirname, "..");
const repoRoot = path.resolve(extensionRoot, "..", "..");
const adapterRoot = path.resolve(repoRoot, "adapters", "cursor-bridge");
const extensionNodeModulesRoot = path.join(extensionRoot, "node_modules");
const stageRoot = fs.mkdtempSync(path.join(os.tmpdir(), "vibedeck-vsix-"));

function resolveVsceEntry() {
  const candidate = path.join(extensionRoot, "node_modules", "@vscode", "vsce", "vsce");
  if (!fs.existsSync(candidate)) {
    throw new Error(`vsce entry not found: ${candidate}`);
  }
  return candidate;
}

function normalizeCliPath(value) {
  return value.replace(/\^(.)/g, "$1").trim();
}

function resolveMaybeAbsolute(value) {
  const normalized = normalizeCliPath(value);
  return path.isAbsolute(normalized)
    ? normalized
    : path.resolve(extensionRoot, normalized);
}

function parseOutPath(argv) {
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--out" && index + 1 < argv.length) {
      return resolveMaybeAbsolute(argv[index + 1]);
    }
    if (arg.startsWith("--out=")) {
      return resolveMaybeAbsolute(arg.slice("--out=".length));
    }
  }

  const extensionPkg = readJson(path.join(extensionRoot, "package.json"));
  return path.resolve(
    repoRoot,
    "artifacts",
    "vsix",
    `vibedeck-bridge-${extensionPkg.version}.vsix`,
  );
}

function copyDirectory(source, destination) {
  fs.mkdirSync(destination, { recursive: true });
  for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
    const from = path.join(source, entry.name);
    const to = path.join(destination, entry.name);
    if (entry.isDirectory()) {
      copyDirectory(from, to);
      continue;
    }
    fs.copyFileSync(from, to);
  }
}

function writeJson(filePath, value) {
  fs.writeFileSync(filePath, JSON.stringify(value, null, 2) + "\n");
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function resolveInstalledPackageRoot(root, packageName) {
  const resolved = path.join(root, ...packageName.split("/"));
  if (!fs.existsSync(resolved)) {
    throw new Error(`package root not found: ${packageName} -> ${resolved}`);
  }
  return resolved;
}

function vendorRuntimePackage(packageName, sourceRoot, destinationRoot, seen) {
  if (seen.has(packageName)) {
    return;
  }
  seen.add(packageName);

  copyDirectory(sourceRoot, destinationRoot);
  const pkg = readJson(path.join(sourceRoot, "package.json"));
  for (const dependencyName of Object.keys(pkg.dependencies ?? {})) {
    if (seen.has(dependencyName)) {
      continue;
    }
    const dependencySourceRoot = resolveInstalledPackageRoot(
      extensionNodeModulesRoot,
      dependencyName,
    );
    const dependencyDestinationRoot = path.join(
      stageRoot,
      "node_modules",
      ...dependencyName.split("/"),
    );
    vendorRuntimePackage(
      dependencyName,
      dependencySourceRoot,
      dependencyDestinationRoot,
      seen,
    );
  }
}

function buildStage() {
  const extensionPkg = readJson(path.join(extensionRoot, "package.json"));
  const adapterPkg = readJson(path.join(adapterRoot, "package.json"));

  copyDirectory(path.join(extensionRoot, "dist"), path.join(stageRoot, "dist"));
  fs.copyFileSync(path.join(extensionRoot, "README.md"), path.join(stageRoot, "README.md"));
  fs.copyFileSync(path.join(extensionRoot, ".vscodeignore"), path.join(stageRoot, ".vscodeignore"));

  const stagedExtensionPkg = {
    ...extensionPkg,
    dependencies: {
      ...extensionPkg.dependencies,
      "@vibedeck/cursor-bridge": adapterPkg.version,
    },
  };
  writeJson(path.join(stageRoot, "package.json"), stagedExtensionPkg);

  const seen = new Set(["@vibedeck/cursor-bridge"]);
  const stagedAdapterRoot = path.join(stageRoot, "node_modules", "@vibedeck", "cursor-bridge");
  copyDirectory(path.join(adapterRoot, "dist"), path.join(stagedAdapterRoot, "dist"));
  fs.copyFileSync(path.join(adapterRoot, "README.md"), path.join(stagedAdapterRoot, "README.md"));
  writeJson(path.join(stagedAdapterRoot, "package.json"), adapterPkg);

  for (const dependencyName of Object.keys(stagedExtensionPkg.dependencies ?? {})) {
    if (dependencyName === "@vibedeck/cursor-bridge") {
      continue;
    }
    const dependencySourceRoot = resolveInstalledPackageRoot(
      extensionNodeModulesRoot,
      dependencyName,
    );
    const dependencyDestinationRoot = path.join(
      stageRoot,
      "node_modules",
      ...dependencyName.split("/"),
    );
    vendorRuntimePackage(
      dependencyName,
      dependencySourceRoot,
      dependencyDestinationRoot,
      seen,
    );
  }
}

function runVsce(outPath) {
  fs.mkdirSync(path.dirname(outPath), { recursive: true });
  const vsceEntry = resolveVsceEntry();
  const result = spawnSync(process.execPath, [vsceEntry, "package", "--out", outPath], {
    cwd: stageRoot,
    stdio: "inherit",
  });
  if (result.status !== 0) {
    throw new Error(`vsce package failed: ${result.status ?? "unknown"}`);
  }
}

try {
  buildStage();
  runVsce(parseOutPath(process.argv.slice(2)));
} finally {
  fs.rmSync(stageRoot, { recursive: true, force: true });
}
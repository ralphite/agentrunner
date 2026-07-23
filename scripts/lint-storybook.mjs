import { execFileSync } from "node:child_process";
import fs from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..");
const frontendRoot = path.join(repoRoot, "webui/frontend");
const require = createRequire(path.join(frontendRoot, "package.json"));
const ts = require("typescript");
const manifestPath = path.join(frontendRoot, "src/storybook/storyManifest.ts");
const baselinePath = path.join(frontendRoot, "storybook-missing-baseline.json");
const storyIndexPath = path.join(frontendRoot, "storybook-static/index.json");
const sourceBaselinePath = path.join(frontendRoot, "storybook-baseline.json");
const allowedRoots = new Set([
  "Foundations",
  "Components",
  "Features",
  "Pages",
  "CUJs",
  "Demos",
  "Future",
]);
const errors = [];

function fail(message) {
  errors.push(message);
}

async function loadManifest() {
  const source = fs.readFileSync(manifestPath, "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
    fileName: manifestPath,
    reportDiagnostics: true,
  });
  for (const diagnostic of transpiled.diagnostics ?? []) {
    fail(ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n"));
  }
  const encoded = Buffer.from(transpiled.outputText).toString("base64");
  return import(`data:text/javascript;base64,${encoded}`);
}

function normalizeImportPath(source) {
  return `./${source.replace(/\.tsx$/, ".stories.tsx")}`;
}

function readGitFile(revision, file) {
  try {
    return execFileSync("git", ["show", `${revision}:${file}`], {
      cwd: repoRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    });
  } catch {
    return "";
  }
}

function previousBaseline() {
  const relativeBaseline = path.relative(repoRoot, baselinePath);
  let trackedAtHead = true;
  try {
    execFileSync("git", ["cat-file", "-e", `HEAD:${relativeBaseline}`], {
      cwd: repoRoot,
      stdio: "ignore",
    });
  } catch {
    trackedAtHead = false;
  }
  if (!trackedAtHead) return [];

  const working = fs.readFileSync(baselinePath, "utf8");
  const head = readGitFile("HEAD", relativeBaseline);
  const previous = working === head ? readGitFile("HEAD^", relativeBaseline) : head;
  if (!previous) return [];
  try {
    return JSON.parse(previous).missing ?? [];
  } catch {
    return [];
  }
}

const { storyManifest } = await loadManifest();
const storyIndex = JSON.parse(fs.readFileSync(storyIndexPath, "utf8"));
const sourceBaseline = JSON.parse(fs.readFileSync(sourceBaselinePath, "utf8"));
const entries = storyIndex.entries ?? {};
const targetIds = new Set();
const sourceExports = new Set();
const availableExports = new Set(
  sourceBaseline.productionComponents.files.flatMap((file) =>
    file.exports.map((exportName) => `${file.source}#${exportName}`),
  ),
);
const coveredStoryIds = new Set();
const missing = [];

// Story/Page harnesses are only safe when production components cannot bypass
// the injected AppServices boundary. Keep this structural rule in the same
// gate as Story coverage so a later "quick" AR/EventSource/storage call cannot
// silently reconnect an isolated canvas to ambient browser state.
for (const file of sourceBaseline.productionComponents.files) {
  const absolute = path.join(frontendRoot, file.source);
  const source = fs.readFileSync(absolute, "utf8");
  const sourceFile = ts.createSourceFile(
    absolute,
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const violations = [];
  const visit = (node) => {
    if (
      ts.isPropertyAccessExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "AR"
    ) {
      violations.push(`direct AR.${node.name.text}`);
    }
    if (
      ts.isNewExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "EventSource"
    ) {
      violations.push("direct EventSource");
    }
    if (
      ts.isIdentifier(node) &&
      (node.text === "localStorage" || node.text === "sessionStorage")
    ) {
      violations.push(`ambient ${node.text}`);
    }
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "fetch"
    ) {
      violations.push("direct fetch");
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  for (const violation of new Set(violations)) {
    fail(`${file.source}: bypasses AppServices (${violation})`);
  }
}

for (const target of storyManifest) {
  if (!target || typeof target !== "object") {
    fail("manifest target must be an object");
    continue;
  }
  if (targetIds.has(target.componentId)) {
    fail(`duplicate componentId: ${target.componentId}`);
  }
  targetIds.add(target.componentId);
  const sourcePath = path.join(frontendRoot, target.source);
  if (!fs.existsSync(sourcePath)) {
    fail(`${target.componentId}: source does not exist: ${target.source}`);
  }
  const sourceExport = `${target.source}#${target.exportName}`;
  sourceExports.add(sourceExport);
  if (!availableExports.has(sourceExport)) {
    fail(`${target.componentId}: production export does not exist: ${sourceExport}`);
  }
  if (!target.cells || Object.keys(target.cells).length === 0) {
    fail(`${target.componentId}: no coverage cells`);
    continue;
  }

  for (const [cellId, cell] of Object.entries(target.cells)) {
    const key = `${target.componentId}/${cellId}`;
    if (cell.status === "missing") {
      missing.push(key);
      continue;
    }
    if (cell.status === "n-a") {
      if (!cell.reason || !cell.evidence || !cell.owner) {
        fail(`${key}: N/A requires reason, evidence and owner`);
      }
      continue;
    }
    if (cell.status !== "covered") {
      fail(`${key}: unknown status ${String(cell.status)}`);
      continue;
    }
    const entry = entries[cell.storyId];
    if (!entry || entry.type !== "story") {
      fail(`${key}: storyId not found in built index: ${cell.storyId}`);
      continue;
    }
    if (entry.importPath !== normalizeImportPath(target.source)) {
      fail(
        `${key}: Story must colocate with source (${entry.importPath} != ${normalizeImportPath(target.source)})`,
      );
    }
    coveredStoryIds.add(cell.storyId);
  }
}

for (const file of sourceBaseline.productionComponents.files) {
  for (const exportName of file.exports) {
    const key = `${file.source}#${exportName}`;
    if (!sourceExports.has(key)) {
      fail(`visible production export missing from manifest: ${key}`);
    }
  }
}

for (const entry of Object.values(entries)) {
  if (entry.type !== "story") continue;
  const root = entry.title.split("/")[0];
  if (!allowedRoots.has(root)) {
    fail(`${entry.id}: invalid taxonomy root ${root}`);
  }
  if ((root === "Demos" || root === "Future") && entry.tags?.includes("test")) {
    fail(`${entry.id}: ${root} Story must opt out with !test`);
  }
  if (!coveredStoryIds.has(entry.id)) {
    fail(`${entry.id}: orphan Story is not referenced by a manifest cell`);
  }
}

missing.sort();
const renderedBaseline = `${JSON.stringify({ schemaVersion: 1, missing }, null, 2)}\n`;
if (process.argv.includes("--update-baseline")) {
  fs.writeFileSync(baselinePath, renderedBaseline);
  console.log(`storybook manifest: wrote ${path.relative(repoRoot, baselinePath)} (${missing.length} missing)`);
} else {
  const currentBaseline = fs.existsSync(baselinePath)
    ? fs.readFileSync(baselinePath, "utf8")
    : "";
  if (currentBaseline !== renderedBaseline) {
    fail("missing baseline is stale; run `npm run manifest:storybook:update`");
  }
  const previous = new Set(previousBaseline());
  const regressions = missing.filter((item) => !previous.has(item));
  if (previous.size > 0 && regressions.length > 0) {
    fail(`MISSING baseline increased: ${regressions.join(", ")}`);
  }
}

if (errors.length > 0) {
  for (const error of errors) console.error(`storybook lint: ${error}`);
  process.exit(1);
}

console.log(
  `storybook lint: ${storyManifest.length} targets, ${Object.keys(entries).length} stories, ${missing.length} missing`,
);

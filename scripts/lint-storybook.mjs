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
const missingBaselineSchemaVersion = 2;
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

function normalizeImportPath(target) {
  return `./${target.storySource ?? target.source.replace(/\.tsx$/, ".stories.tsx")}`;
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
  if (!trackedAtHead) return { schemaVersion: 0, missing: [] };

  const working = fs.readFileSync(baselinePath, "utf8");
  const head = readGitFile("HEAD", relativeBaseline);
  const previous = working === head ? readGitFile("HEAD^", relativeBaseline) : head;
  if (!previous) return { schemaVersion: 0, missing: [] };
  try {
    const parsed = JSON.parse(previous);
    return {
      schemaVersion: parsed.schemaVersion ?? 0,
      missing: parsed.missing ?? [],
    };
  } catch {
    return { schemaVersion: 0, missing: [] };
  }
}

const {
  storyManifest,
  privateVisibleExclusions = [],
  semanticStateRequirements = [],
  workbenchStories = [],
  globalStatePairs = [],
} = await loadManifest();
const storyIndex = JSON.parse(fs.readFileSync(storyIndexPath, "utf8"));
const sourceBaseline = JSON.parse(fs.readFileSync(sourceBaselinePath, "utf8"));
const entries = storyIndex.entries ?? {};
const targetIds = new Set();
const semanticStateIds = new Set();
const sourceDeclarations = new Set();
const availableExports = new Set(
  sourceBaseline.productionComponents.files.flatMap((file) =>
    file.exports.map((exportName) => `${file.source}#${exportName}`),
  ),
);
const availablePrivateVisible = new Set(
  sourceBaseline.productionComponents.files.flatMap((file) =>
    file.privateVisibleCandidates.map(
      (declarationName) => `${file.source}#${declarationName}`,
    ),
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
  const declarationName = target.exportName ?? target.componentId;
  const sourceDeclaration = `${target.source}#${declarationName}`;
  sourceDeclarations.add(sourceDeclaration);
  const availableDeclarations = target.exportName
    ? availableExports
    : availablePrivateVisible;
  if (!availableDeclarations.has(sourceDeclaration)) {
    fail(
      `${target.componentId}: production ${target.exportName ? "export" : "private visible declaration"} does not exist: ${sourceDeclaration}`,
    );
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
    const expectedImportPath = normalizeImportPath(target);
    if (entry.importPath !== expectedImportPath) {
      fail(
        `${key}: Story source mismatch (${entry.importPath} != ${expectedImportPath})`,
      );
    }
    coveredStoryIds.add(cell.storyId);
  }
}

for (const requirement of semanticStateRequirements) {
  const key = `${requirement.componentId}/${requirement.state}`;
  if (semanticStateIds.has(key)) {
    fail(`duplicate semantic state requirement: ${key}`);
  }
  semanticStateIds.add(key);
  if (!targetIds.has(requirement.componentId)) {
    fail(`${key}: component target does not exist`);
  }
  if (
    !requirement.source ||
    !requirement.evidenceSelector ||
    !requirement.storyId ||
    !requirement.evidence ||
    !requirement.owner
  ) {
    fail(`${key}: source, evidenceSelector, storyId, evidence and owner are required`);
    continue;
  }
  const evidencePath = path.join(frontendRoot, requirement.source);
  if (!fs.existsSync(evidencePath)) {
    fail(`${key}: semantic state source does not exist: ${requirement.source}`);
    continue;
  }
  const evidenceSource = fs.readFileSync(evidencePath, "utf8");
  if (!evidenceSource.includes(requirement.evidenceSelector)) {
    fail(
      `${key}: evidence selector not found in ${requirement.source}: ${requirement.evidenceSelector}`,
    );
  }
  const entry = entries[requirement.storyId];
  if (!entry || entry.type !== "story") {
    fail(`${key}: storyId not found in built index: ${requirement.storyId}`);
  }
  if (!coveredStoryIds.has(requirement.storyId)) {
    fail(`${key}: semantic state Story must also be referenced by a component coverage cell`);
  }
}

const exclusionDeclarations = new Set();
for (const exclusion of privateVisibleExclusions) {
  const key = `${exclusion.source}#${exclusion.declarationName}`;
  if (exclusionDeclarations.has(key)) {
    fail(`duplicate private-visible exclusion: ${key}`);
  }
  exclusionDeclarations.add(key);
  if (!availablePrivateVisible.has(key)) {
    fail(`private-visible exclusion does not exist in source baseline: ${key}`);
  }
  if (sourceDeclarations.has(key)) {
    fail(`private-visible declaration cannot be both target and exclusion: ${key}`);
  }
  if (!exclusion.reason || !exclusion.evidence || !exclusion.owner) {
    fail(`${key}: private-visible exclusion requires reason, evidence and owner`);
  }
}

for (const file of sourceBaseline.productionComponents.files) {
  for (const exportName of file.exports) {
    const key = `${file.source}#${exportName}`;
    if (!sourceDeclarations.has(key)) {
      fail(`visible production export missing from manifest: ${key}`);
    }
  }
  for (const declarationName of file.privateVisibleCandidates) {
    const key = `${file.source}#${declarationName}`;
    if (!sourceDeclarations.has(key) && !exclusionDeclarations.has(key)) {
      fail(`private visible declaration is unclassified: ${key}`);
    }
  }
}

for (const workbench of workbenchStories) {
  const key = `${workbench.kind}:${workbench.storyId}`;
  const entry = entries[workbench.storyId];
  if (!entry || entry.type !== "story") {
    fail(`${key}: storyId not found in built index`);
    continue;
  }
  const expectedRoot = workbench.kind === "cuj" ? "CUJs" : "Demos";
  if (entry.title.split("/")[0] !== expectedRoot) {
    fail(`${key}: expected ${expectedRoot} taxonomy root, got ${entry.title}`);
  }
  if (entry.importPath !== `./${workbench.source}`) {
    fail(
      `${key}: Story source mismatch (${entry.importPath} != ./${workbench.source})`,
    );
  }
  if (!workbench.evidence || !workbench.owner) {
    fail(`${key}: evidence and owner are required`);
  }
  coveredStoryIds.add(workbench.storyId);
}

const globalPairIds = new Set();
for (const pair of globalStatePairs) {
  const key = `global-pair:${pair.pairId}`;
  if (globalPairIds.has(pair.pairId)) {
    fail(`duplicate global state pair: ${pair.pairId}`);
  }
  globalPairIds.add(pair.pairId);
  const entry = entries[pair.storyId];
  if (!entry || entry.type !== "story") {
    fail(`${key}: storyId not found in built index: ${pair.storyId}`);
  }
  if (!coveredStoryIds.has(pair.storyId)) {
    fail(`${key}: canonical Story must be referenced by component coverage`);
  }
  if (
    !Array.isArray(pair.states) ||
    pair.states.length < 2 ||
    !pair.theme ||
    !pair.viewport?.width ||
    !pair.viewport?.height ||
    !pair.evidenceSelector ||
    !pair.evidence ||
    !pair.owner
  ) {
    fail(`${key}: states, theme, viewport, evidenceSelector, evidence and owner are required`);
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
const renderedBaseline = `${JSON.stringify({ schemaVersion: missingBaselineSchemaVersion, missing }, null, 2)}\n`;
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
  const previousSnapshot = previousBaseline();
  const previous = new Set(previousSnapshot.missing);
  const regressions = missing.filter((item) => !previous.has(item));
  if (
    previousSnapshot.schemaVersion === missingBaselineSchemaVersion &&
    previous.size > 0 &&
    regressions.length > 0
  ) {
    fail(`MISSING baseline increased: ${regressions.join(", ")}`);
  }
}

if (errors.length > 0) {
  for (const error of errors) console.error(`storybook lint: ${error}`);
  process.exit(1);
}

console.log(
  `storybook lint: ${storyManifest.length} targets, ${semanticStateRequirements.length} semantic states, ${globalStatePairs.length} global pairs, ${privateVisibleExclusions.length} private exclusions, ${Object.keys(entries).length} stories, ${missing.length} missing`,
);

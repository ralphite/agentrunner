import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
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
const reviewLedgerPath = path.join(
  frontendRoot,
  "storybook-review-ledger.json",
);
const missingBaselineSchemaVersion = 3;
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
  storyReviewFamilies = [],
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
const reviewFamilyByStoryId = new Map();
const missing = [];
const requiredReviewAxes = new Set([
  "role-name-state",
  "keyboard-focus",
  "pointer-touch",
  "disabled-busy-error",
  "live-region",
  "motion",
  "contrast-theme",
  "zoom-overflow",
]);
const reviewIds = new Set();
const reviewPrefixes = new Set();

for (const family of storyReviewFamilies) {
  if (!family || typeof family !== "object") {
    fail("Story review family must be an object");
    continue;
  }
  if (!family.reviewId || reviewIds.has(family.reviewId)) {
    fail(`invalid or duplicate Story review id: ${String(family.reviewId)}`);
  }
  reviewIds.add(family.reviewId);
  if (!family.titlePrefix || reviewPrefixes.has(family.titlePrefix)) {
    fail(
      `invalid or duplicate Story review prefix: ${String(family.titlePrefix)}`,
    );
  }
  reviewPrefixes.add(family.titlePrefix);
  const axes = new Set(family.axes ?? []);
  if (
    axes.size !== requiredReviewAxes.size ||
    [...requiredReviewAxes].some((axis) => !axes.has(axis))
  ) {
    fail(`${family.reviewId}: Story review must decide every required axis`);
  }
  if (
    !["ALIGNED", "FIXED", "INTENTIONAL"].includes(family.visualVerdict) ||
    !["PASS", "UNTESTED", "GAP", "INTENTIONAL"].includes(family.codexParity)
  ) {
    fail(`${family.reviewId}: invalid visual or Codex parity verdict`);
  }
  if (
    !family.decision ||
    !family.codexEvidence ||
    !family.agentEvidence ||
    !family.reviewedAt ||
    !/^[a-f0-9]{64}$/.test(family.reviewedDigest ?? "") ||
    !family.owner
  ) {
    fail(
      `${family.reviewId}: decision, evidence, reviewedAt, reviewedDigest and owner are required`,
    );
  }
  const reviewerRoles = new Set(family.reviewedBy ?? []);
  for (const role of [
    "visual-design",
    "interaction-a11y",
    "contract-evidence",
  ]) {
    if (!reviewerRoles.has(role)) {
      fail(`${family.reviewId}: missing reviewer role ${role}`);
    }
  }
}

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
  const reviewMatches = storyReviewFamilies.filter(
    (family) =>
      entry.title === family.titlePrefix ||
      entry.title.startsWith(`${family.titlePrefix}/`),
  );
  if (reviewMatches.length !== 1) {
    fail(
      `${entry.id}: expected exactly one Story review family, got ${reviewMatches.length}`,
    );
  } else {
    reviewFamilyByStoryId.set(entry.id, reviewMatches[0]);
  }
}

const targetRefsByStoryId = new Map();
const targetCells = [];
for (const target of storyManifest) {
  const targetKey = `${target.source}#${target.exportName ?? target.componentId}`;
  const firstCoveredStoryId = Object.values(target.cells).find(
    (cell) => cell.status === "covered",
  )?.storyId;
  const targetReviewId = firstCoveredStoryId
    ? reviewFamilyByStoryId.get(firstCoveredStoryId)?.reviewId
    : undefined;
  for (const [cellId, cell] of Object.entries(target.cells)) {
    const reviewId =
      cell.status === "covered"
        ? reviewFamilyByStoryId.get(cell.storyId)?.reviewId
        : targetReviewId;
    if (cell.status !== "missing" && !reviewId) {
      fail(
        `${target.componentId}/${cellId}: no Story review family joins this target cell`,
      );
    }
    targetCells.push({
      targetKey,
      componentId: target.componentId,
      productionSource: target.source,
      cellId,
      status: cell.status,
      ...(cell.status === "covered" ? { storyId: cell.storyId } : {}),
      ...(cell.status === "n-a"
        ? {
            reason: cell.reason,
            evidence: cell.evidence,
            owner: cell.owner,
          }
        : {}),
      reviewId: reviewId ?? null,
    });
    if (cell.status !== "covered") continue;
    let refs = targetRefsByStoryId.get(cell.storyId);
    if (!refs) {
      refs = new Map();
      targetRefsByStoryId.set(cell.storyId, refs);
    }
    let ref = refs.get(targetKey);
    if (!ref) {
      ref = {
        targetKey,
        componentId: target.componentId,
        productionSource: target.source,
        cells: [],
      };
      refs.set(targetKey, ref);
    }
    ref.cells.push(cellId);
  }
}

const semanticRefsByStoryId = new Map();
for (const requirement of semanticStateRequirements) {
  const refs = semanticRefsByStoryId.get(requirement.storyId) ?? [];
  refs.push(`${requirement.componentId}/${requirement.state}`);
  semanticRefsByStoryId.set(requirement.storyId, refs);
}
const pairRefsByStoryId = new Map();
for (const pair of globalStatePairs) {
  const refs = pairRefsByStoryId.get(pair.storyId) ?? [];
  refs.push(pair.pairId);
  pairRefsByStoryId.set(pair.storyId, refs);
}
const workbenchByStoryId = new Map(
  workbenchStories.map((story) => [story.storyId, story]),
);
const storyEntries = Object.values(entries)
  .filter((entry) => entry.type === "story")
  .sort((a, b) => a.id.localeCompare(b.id));
const sharedReviewSources = [
  ".storybook/main.ts",
  ".storybook/manager.ts",
  ".storybook/preview.tsx",
  "src/tw.css",
  "src/ui/Button.tsx",
  "src/ui/Field.tsx",
  "src/ui/IconButton.tsx",
  "src/ui/IconLink.tsx",
  "src/ui/LifecycleStatus.tsx",
];

function sha256(parts) {
  const hash = createHash("sha256");
  for (const part of parts) hash.update(part).update("\0");
  return hash.digest("hex");
}

const inventoryClassification = JSON.stringify({
  targets: storyManifest,
  semanticAssertions: semanticStateRequirements,
  pairedAssertions: globalStatePairs,
  workbenchStories,
  privateExclusions: privateVisibleExclusions,
});

const familyLedger = storyReviewFamilies
  .map((family) => {
    const familyEntries = storyEntries.filter(
      (entry) => reviewFamilyByStoryId.get(entry.id)?.reviewId === family.reviewId,
    );
    const storySources = [
      ...new Set(familyEntries.map((entry) => entry.importPath.replace(/^\.\//, ""))),
    ].sort();
    const productionSources = [
      ...new Set(
        familyEntries.flatMap((entry) =>
          [...(targetRefsByStoryId.get(entry.id)?.values() ?? [])].map(
            (ref) => ref.productionSource,
          ),
        ),
      ),
    ].sort();
    const { reviewedDigest, ...reviewDecision } = family;
    const digestParts = [
      JSON.stringify(reviewDecision),
      inventoryClassification,
      ...familyEntries.map((entry) =>
        JSON.stringify({
          id: entry.id,
          title: entry.title,
          name: entry.name,
          importPath: entry.importPath,
          tags: [...(entry.tags ?? [])].sort(),
        }),
      ),
      ...[...sharedReviewSources, ...storySources, ...productionSources].map((source) => {
        const absolute = path.join(frontendRoot, source);
        return `${source}\n${fs.existsSync(absolute) ? fs.readFileSync(absolute, "utf8") : ""}`;
      }),
    ];
    const targetIds = [
      ...new Set(
        familyEntries.flatMap((entry) =>
          [...(targetRefsByStoryId.get(entry.id)?.values() ?? [])].map(
            (ref) => ref.componentId,
          ),
        ),
      ),
    ].sort();
    const familyDigest = sha256(digestParts);
    if (reviewedDigest !== familyDigest) {
      fail(
        `${family.reviewId}: review digest is stale (${reviewedDigest} != ${familyDigest}); fresh reviewers must approve and explicitly update REVIEWED_FAMILY_DIGESTS`,
      );
    }
    return {
      ...family,
      familyDigest,
      storyCount: familyEntries.length,
      storyFiles: storySources,
      targetIds,
    };
  })
  .sort((a, b) => a.reviewId.localeCompare(b.reviewId));

const storyLedger = storyEntries.map((entry) => {
  const family = reviewFamilyByStoryId.get(entry.id);
  const workbench = workbenchByStoryId.get(entry.id);
  const pairRefs = globalStatePairs
    .filter((pair) => pair.storyId === entry.id)
    .map((pair) => ({
      pairId: pair.pairId,
      states: pair.states,
      theme: pair.theme,
      viewport: pair.viewport,
      evidenceSelector: pair.evidenceSelector,
    }));
  return {
    storyId: entry.id,
    title: entry.title,
    name: entry.name,
    source: entry.importPath.replace(/^\.\//, ""),
    kind: workbench?.kind ?? entry.title.split("/")[0].toLowerCase(),
    tags: [...(entry.tags ?? [])].sort(),
    targetRefs: [
      ...(targetRefsByStoryId.get(entry.id)?.values() ?? []),
    ]
      .map((ref) => ({ ...ref, cells: [...ref.cells].sort() }))
      .sort((a, b) => a.targetKey.localeCompare(b.targetKey)),
    semanticRefs: [...(semanticRefsByStoryId.get(entry.id) ?? [])].sort(),
    pairRefs,
    workbenchRef: workbench
      ? {
          kind: workbench.kind,
          evidence: workbench.evidence,
          owner: workbench.owner,
        }
      : null,
    review: family
      ? {
          status: "REVIEWED",
          reviewId: family.reviewId,
          axes: family.axes,
          visualVerdict: family.visualVerdict,
          agentEvidence: family.agentEvidence,
          reviewedBy: family.reviewedBy,
          reviewedAt: family.reviewedAt,
        }
      : null,
    interactionA11y: {
      status: "REVIEWED",
      mode: entry.tags?.includes("play-fn") ? "PLAY_AND_REVIEW" : "MANUAL_REVIEW",
      evidence:
        entry.tags?.includes("play-fn")
          ? "Built index play-fn plus family interaction/a11y review; final gate result is recorded in QA-92."
          : "Family interaction/a11y review; no automated PASS is inferred from Story presence.",
    },
    codex: family
      ? {
          verdict: family.codexParity,
          evidence: family.codexEvidence,
          reason:
            family.codexParity === "PASS"
              ? ""
              : "No family-wide PASS is inferred from inventory or a one-sided screenshot.",
        }
      : null,
    viewportTheme: {
      status: "REVIEWED",
      pairedAssertions: pairRefs,
      evidence:
        pairRefs.length > 0
          ? "Exact global pair is retained in the manifest and final Storybook gate."
          : "Canonical Story plus toolbar/global review; no extra Phone/Dark Story copy and no Codex PASS inferred.",
    },
  };
});

const storyFiles = [
  ...storyLedger
    .reduce((files, story) => {
      let file = files.get(story.source);
      if (!file) {
        file = {
          source: story.source,
          titles: new Set(),
          storyIds: [],
          reviewIds: new Set(),
          targetIds: new Set(),
        };
        files.set(story.source, file);
      }
      file.titles.add(story.title);
      file.storyIds.push(story.storyId);
      if (story.review?.reviewId) file.reviewIds.add(story.review.reviewId);
      for (const ref of story.targetRefs) file.targetIds.add(ref.componentId);
      return files;
    }, new Map())
    .values(),
]
  .map((file) => ({
    source: file.source,
    titles: [...file.titles].sort(),
    storyIds: [...file.storyIds].sort(),
    reviewIds: [...file.reviewIds].sort(),
    targetIds: [...file.targetIds].sort(),
  }))
  .sort((a, b) => a.source.localeCompare(b.source));

const coveredCellCount = targetCells.filter(
  (cell) => cell.status === "covered",
).length;
const notApplicableCellCount = targetCells.filter(
  (cell) => cell.status === "n-a",
).length;
const uniqueTargetStoryIds = new Set(
  targetCells
    .filter((cell) => cell.status === "covered")
    .map((cell) => cell.storyId),
);
const reviewLedger = {
  schemaVersion: 1,
  policy:
    "Inventory proves closure only. REVIEWED is a manual family decision joined to every exact Story and target cell; it never upgrades Codex parity or automated gate status by itself.",
  summary: {
    reviewFamilies: storyReviewFamilies.length,
    storyFiles: storyFiles.length,
    stories: storyEntries.length,
    reviewedStories: reviewFamilyByStoryId.size,
    productionTargets: storyManifest.length,
    targetCells: targetCells.length,
    coveredCells: coveredCellCount,
    notApplicableCells: notApplicableCellCount,
    missingCells: missing.length,
    uniqueTargetStoryIds: uniqueTargetStoryIds.size,
    workbenchStories: workbenchStories.length,
    semanticAssertions: semanticStateRequirements.length,
    pairedAssertions: globalStatePairs.length,
    privateExclusions: privateVisibleExclusions.length,
  },
  families: familyLedger,
  storyFiles,
  stories: storyLedger,
  targetCells: targetCells.sort(
    (a, b) =>
      a.targetKey.localeCompare(b.targetKey) ||
      a.cellId.localeCompare(b.cellId),
  ),
  semanticAssertions: semanticStateRequirements.map((requirement) => ({
    assertionId: `${requirement.componentId}/${requirement.state}`,
    ...requirement,
    reviewId:
      reviewFamilyByStoryId.get(requirement.storyId)?.reviewId ?? null,
    reviewStatus: "REVIEWED",
  })),
  pairedAssertions: globalStatePairs.map((pair) => ({
    ...pair,
    reviewId: reviewFamilyByStoryId.get(pair.storyId)?.reviewId ?? null,
    reviewStatus: "REVIEWED",
  })),
  exclusions: privateVisibleExclusions.map((exclusion) => ({
    exclusionId: `${exclusion.source}#${exclusion.declarationName}`,
    ...exclusion,
    reviewStatus: "REVIEWED",
    verdict: "INTENTIONAL",
  })),
};

missing.sort();
const renderedBaseline = `${JSON.stringify({ schemaVersion: missingBaselineSchemaVersion, missing }, null, 2)}\n`;
const renderedReviewLedger = `${JSON.stringify(reviewLedger, null, 2)}\n`;
if (process.argv.includes("--update-baseline")) {
  fs.writeFileSync(baselinePath, renderedBaseline);
  fs.writeFileSync(reviewLedgerPath, renderedReviewLedger);
  console.log(`storybook manifest: wrote ${path.relative(repoRoot, baselinePath)} (${missing.length} missing)`);
  console.log(
    `storybook review: wrote ${path.relative(repoRoot, reviewLedgerPath)} (${storyFiles.length} files, ${storyEntries.length} stories)`,
  );
} else {
  const currentBaseline = fs.existsSync(baselinePath)
    ? fs.readFileSync(baselinePath, "utf8")
    : "";
  if (currentBaseline !== renderedBaseline) {
    fail("missing baseline is stale; run `npm run manifest:storybook:update`");
  }
  const currentReviewLedger = fs.existsSync(reviewLedgerPath)
    ? fs.readFileSync(reviewLedgerPath, "utf8")
    : "";
  if (currentReviewLedger !== renderedReviewLedger) {
    fail("Story review ledger is stale; run `npm run manifest:storybook:update`");
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
  `storybook lint: ${storyManifest.length} targets, ${targetCells.length} cells, ${semanticStateRequirements.length} semantic states, ${globalStatePairs.length} global pairs, ${privateVisibleExclusions.length} private exclusions, ${storyFiles.length} Story files, ${storyEntries.length} Stories, ${reviewFamilyByStoryId.size} reviewed, ${missing.length} missing`,
);

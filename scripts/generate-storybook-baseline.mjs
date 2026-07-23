import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..");
const frontendRoot = path.join(repoRoot, "webui/frontend");
const require = createRequire(path.join(frontendRoot, "package.json"));
const ts = require("typescript");
const srcRoot = path.join(frontendRoot, "src");
const componentsRoot = path.join(srcRoot, "components");
const outputPath = path.join(frontendRoot, "storybook-baseline.json");

function walk(dir) {
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .flatMap((entry) => {
      const absolute = path.join(dir, entry.name);
      return entry.isDirectory() ? walk(absolute) : [absolute];
    })
    .sort();
}

function relative(file) {
  return path.relative(frontendRoot, file).split(path.sep).join("/");
}

function isProductionComponent(file) {
  return (
    file.endsWith(".tsx") &&
    !/\.(?:test|spec|stories)\.tsx$/.test(file)
  );
}

function hasExport(node) {
  return node.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword) ?? false;
}

function componentNames(sourceFile) {
  const exported = [];
  const privateVisible = [];
  const record = (name, isExported) => {
    if (!name || !/^[A-Z]/.test(name)) return;
    (isExported ? exported : privateVisible).push(name);
  };

  for (const statement of sourceFile.statements) {
    if (ts.isFunctionDeclaration(statement) || ts.isClassDeclaration(statement)) {
      record(statement.name?.text, hasExport(statement));
      continue;
    }
    if (!ts.isVariableStatement(statement)) continue;
    const isExported = hasExport(statement);
    for (const declaration of statement.declarationList.declarations) {
      if (
        ts.isIdentifier(declaration.name) &&
        declaration.initializer &&
        (ts.isArrowFunction(declaration.initializer) ||
          ts.isFunctionExpression(declaration.initializer))
      ) {
        record(declaration.name.text, isExported);
      }
    }
  }

  return {
    exports: [...new Set(exported)].sort(),
    privateVisibleCandidates: [...new Set(privateVisible)].sort(),
  };
}

function baseCallName(expression) {
  let current = expression;
  while (ts.isPropertyAccessExpression(current) || ts.isCallExpression(current)) {
    current = ts.isPropertyAccessExpression(current) ? current.expression : current.expression;
  }
  return ts.isIdentifier(current) ? current.text : "";
}

function testDeclarationCount(sourceFile) {
  let count = 0;
  const visit = (node) => {
    if (ts.isCallExpression(node)) {
      const base = baseCallName(node.expression);
      if ((base === "it" || base === "test") && !ts.isCallExpression(node.parent)) {
        count += 1;
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  return count;
}

function storageCalls(sourceFile) {
  const constants = new Map();
  for (const statement of sourceFile.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (
        ts.isIdentifier(declaration.name) &&
        declaration.initializer &&
        ts.isStringLiteral(declaration.initializer)
      ) {
        constants.set(declaration.name.text, declaration.initializer.text);
      }
    }
  }

  const calls = [];
  const visit = (node) => {
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      ts.isIdentifier(node.expression.expression) &&
      (node.expression.expression.text === "localStorage" ||
        node.expression.expression.text === "sessionStorage") &&
      ["getItem", "setItem", "removeItem"].includes(node.expression.name.text)
    ) {
      const argument = node.arguments[0];
      const key =
        argument && ts.isStringLiteral(argument)
          ? argument.text
          : argument && ts.isIdentifier(argument) && constants.has(argument.text)
            ? constants.get(argument.text)
            : argument?.getText(sourceFile) ?? "";
      const position = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile));
      calls.push({
        storage: node.expression.expression.text,
        operation: node.expression.name.text,
        key,
        line: position.line + 1,
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  return calls;
}

function hashExpressions(sourceFile) {
  const expressions = [];
  const visit = (node) => {
    if (
      ts.isBinaryExpression(node) &&
      node.operatorToken.kind === ts.SyntaxKind.EqualsToken &&
      node.left.getText(sourceFile).endsWith("location.hash")
    ) {
      const position = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile));
      expressions.push({
        line: position.line + 1,
        expression: node.right.getText(sourceFile),
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  return expressions;
}

const sourceFiles = walk(srcRoot).filter((file) => /\.(?:ts|tsx)$/.test(file));
const parsed = new Map(
  sourceFiles.map((file) => [
    file,
    ts.createSourceFile(
      file,
      fs.readFileSync(file, "utf8"),
      ts.ScriptTarget.Latest,
      true,
      file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    ),
  ]),
);

const productionComponents = walk(componentsRoot)
  .filter(isProductionComponent)
  .map((file) => {
    const text = fs.readFileSync(file, "utf8");
    return {
      source: relative(file),
      lines: text === "" ? 0 : text.split(/\r?\n/).length - (text.endsWith("\n") ? 1 : 0),
      ...componentNames(parsed.get(file)),
    };
  });

const tests = sourceFiles.filter((file) => /\.(?:test|spec)\.(?:ts|tsx)$/.test(file));
const productionSources = sourceFiles.filter(
  (file) => !/\.(?:test|spec|stories)\.(?:ts|tsx)$/.test(file),
);
const storage = productionSources
  .flatMap((file) =>
    storageCalls(parsed.get(file)).map((call) => ({
      source: relative(file),
      ...call,
    })),
  )
  .sort((a, b) => a.source.localeCompare(b.source) || a.line - b.line);
const routes = productionSources
  .flatMap((file) =>
    hashExpressions(parsed.get(file)).map((route) => ({
      source: relative(file),
      ...route,
    })),
  )
  .sort((a, b) => a.source.localeCompare(b.source) || a.line - b.line);

const baseline = {
  schemaVersion: 1,
  productionComponents: {
    count: productionComponents.length,
    lines: productionComponents.reduce((sum, component) => sum + component.lines, 0),
    files: productionComponents,
  },
  tests: {
    files: tests.length,
    declarations: tests.reduce(
      (sum, file) => sum + testDeclarationCount(parsed.get(file)),
      0,
    ),
  },
  storage,
  routes,
};
const rendered = `${JSON.stringify(baseline, null, 2)}\n`;

if (process.argv.includes("--check")) {
  const current = fs.existsSync(outputPath) ? fs.readFileSync(outputPath, "utf8") : "";
  if (current !== rendered) {
    console.error("storybook baseline is stale; run `npm run baseline:storybook`");
    process.exit(1);
  }
  console.log("storybook baseline: current");
} else {
  fs.writeFileSync(outputPath, rendered);
  console.log(`storybook baseline: wrote ${path.relative(repoRoot, outputPath)}`);
}

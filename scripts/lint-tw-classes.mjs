#!/usr/bin/env node
// QA-0718 层 1 静态审计:JSX 引用的自定义类 ↔ 构建产物 CSS 对账。
//
// 背景:Tailwind 迁移丢样式的事故形态高度一致——组件 className 引用
// 自定义 kebab-case 类(rd-hero / page-action / fd-gap …),而编译后的
// CSS 里没有任何对应规则,浏览器静默降级,只有"恰好走到该视图×该数据
// 形态×该主题"时才被肉眼看到(QA-0718 实录:rd-* 六组、Scheduled 整页、
// fd-gap、preflight 全是这么漏的)。这类问题可机械对账,不该靠眼睛。
//
// 机制:
//  - 真相以 dist/assets/index-*.css(Tailwind 编译产物)为准,不是
//    tw.css 源文件——嵌套/层叠/utility 展开后的最终结果。
//  - 首建时的存量缺口记录在 qa/tw-class-baseline.txt(每行一个类名),
//    作为待消化 backlog:在 baseline 里的 MISS 只告警不 fail。
//  - **不在 baseline 里的新 MISS 立即 fail**——新代码不许再引入
//    无样式类。补上样式后应同时从 baseline 删行(脚本会提示可删项)。
//  - 豁免表:状态/修饰类(.on/.active 等由父选择器成对定义,单独
//    grep 不到的情况极少;dist 对账下它们通常也能命中)、第三方。
//
// 用法:node scripts/lint-tw-classes.mjs   (需先 npm run build)
import fs from "node:fs";
import path from "node:path";

const FRONT = path.join(import.meta.dirname, "..", "webui", "frontend");
const SRC = path.join(FRONT, "src");
const BASELINE_FILE = path.join(import.meta.dirname, "..", "qa", "tw-class-baseline.txt");

const distDir = path.join(FRONT, "dist", "assets");
const cssFile = fs.existsSync(distDir)
  ? fs.readdirSync(distDir).filter((f) => /^index-.*\.css$/.test(f)).map((f) => path.join(distDir, f))[0]
  : null;
if (!cssFile) {
  console.error("lint-tw-classes: dist css not found — run `npm run build` in webui/frontend first");
  process.exit(2);
}
const CSS = fs.readFileSync(cssFile, "utf8");

const baseline = new Set(
  fs.existsSync(BASELINE_FILE)
    ? fs.readFileSync(BASELINE_FILE, "utf8").split("\n").map((l) => l.trim()).filter((l) => l && !l.startsWith("#"))
    : [],
);

const EXEMPT = new Set(["hljs", "language-mermaid", "mermaid"]);

const files = [];
(function walk(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p);
    else if (e.name.endsWith(".tsx") && !e.name.includes(".test.")) files.push(p);
  }
})(SRC);

// 提取 className 字面量 token;只保留纯 kebab-case 候选(tailwind 的
// 修饰/任意值语法带 [ ] : / ! 等,不会匹配)。
const used = new Map(); // class -> Set(file:line)
for (const f of files) {
  const lines = fs.readFileSync(f, "utf8").split("\n");
  lines.forEach((line, i) => {
    for (const m of line.matchAll(/className\s*=\s*\{?\s*["'`]([^"'`]*)["'`]/g)) {
      for (const tok of m[1].split(/\s+/)) {
        if (!tok || !/^[a-z][a-z0-9-]*$/.test(tok)) continue;
        if (EXEMPT.has(tok)) continue;
        if (!used.has(tok)) used.set(tok, new Set());
        used.get(tok).add(`${path.relative(SRC, f)}:${i + 1}`);
      }
    }
  });
}

// dist css 里出现过 `.foo` 即视为有规则命中(选择器、嵌套、组合均可)。
const hasRule = (cls) => new RegExp(`\\.${cls.replace(/[-]/g, "\\-")}[ ,:{.)>~+\\[]`).test(CSS) || CSS.includes(`.${cls}{`);

const miss = [...used.keys()].filter((c) => !hasRule(c)).sort();
const newMiss = miss.filter((c) => !baseline.has(c));
const knownMiss = miss.filter((c) => baseline.has(c));
const healed = [...baseline].filter((c) => !miss.includes(c)).sort();

console.log(`lint-tw-classes: ${used.size} custom-class candidates · ${miss.length} without any rule in dist css`);
if (knownMiss.length) console.log(`  baseline backlog(已知待补,不 fail): ${knownMiss.length}`);
if (healed.length) {
  console.log(`  可从 baseline 删除(已修复): ${healed.join(", ")}`);
}
if (newMiss.length) {
  console.error(`\nlint-tw-classes: ${newMiss.length} NEW unstyled class(es) — 新代码不许引入无样式类:`);
  for (const c of newMiss) {
    console.error(`  .${c}`);
    for (const s of [...used.get(c)].slice(0, 3)) console.error(`      ${s}`);
  }
  console.error("\n在 tw.css 定义它,或(确属无样式诉求的语义锚点)加入 qa/tw-class-baseline.txt 并注明原因。");
  process.exit(1);
}
console.log("lint-tw-classes: OK — no new unstyled classes");

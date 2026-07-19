#!/usr/bin/env node
// QA-76 · UI 事实声明 × 系统真相对账器(docs/QA.md QA-76,QA-0718 复盘产物)。
//
// 性质:确定性 oracle(不是场景探索)。webui 的每条"事实声明"都有权威
// 真相源;本脚本把系统开进 QA-76 的语义状态 S1–S4,同时读**声明侧**
// (webui diff API——UI 卡与 rail 的数据源)与**真相侧**(git),断言
// 一致。任何 mismatch = 语义 bug,正是 QA-0718 用户实机三连撞的级别:
// 幽灵 diff(S2)、重启后失踪(S3)、清零不同步(S4)。
// S5/S5b(子 agent 终态与不可见审批,G39 红锚)在 QA.md 登记,待
// G39 INC 落地后接入。
//
// 用法(qa-consistency.yml 驱动,两阶段夹一次 daemon 重启):
//   node check.mjs <webui-base> <ar-bin> <workspace-dir> fresh      # S1+S2,输出 SID_B=
//   (workflow 重启 daemon+webui)
//   SID_B=<sid> node check.mjs <webui-base> <ar-bin> <workspace-dir> restarted  # S3+S4
// 前置:workspace 已 git init + 空提交;arwebui 已起;GEMINI_API_KEY 可用;
// spec 位于 <workspace-dir>/../base.yaml。
import { execFileSync, execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const [BASE, AR, WS, PHASE] = process.argv.slice(2);
if (!BASE || !AR || !WS || !["fresh", "restarted"].includes(PHASE)) {
  console.error("usage: check.mjs <webui-base> <ar-bin> <workspace-dir> fresh|restarted");
  process.exit(2);
}
const findings = [];
const note = (kind, what, detail) => {
  findings.push({ kind, what, detail });
  console.log(`${kind === "mismatch" ? "✗ MISMATCH" : kind === "observation" ? "· obs" : "✓"} ${what}${detail ? " — " + detail : ""}`);
};

const api = async (p) => {
  const r = await fetch(BASE + p);
  if (!r.ok) throw new Error(`${p} -> ${r.status}`);
  return r.json();
};
const gitFiles = () =>
  new Set(
    execSync(`git -C ${JSON.stringify(WS)} status --porcelain`, { encoding: "utf8" })
      .split("\n").map((l) => l.slice(3).trim()).filter(Boolean),
  );
const diffFiles = async (sid, scope) => {
  const d = await api(`/api/sessions/${sid}/diff?scope=${scope}`);
  if (!d.known || !d.isRepo) return { unknown: true, files: new Set() };
  const files = new Set();
  for (const m of (d.diff || "").matchAll(/^diff --git a\/(\S+) b\//gm)) files.add(m[1]);
  for (const f of d.untracked || []) files.add(typeof f === "string" ? f : f.path);
  for (const f of d.files || []) files.add(typeof f === "string" ? f : f.path);
  return { unknown: false, files };
};
const eq = (a, b) => a.size === b.size && [...a].every((x) => b.has(x));
const show = (s) => (s.size ? [...s].sort().join(",") : "∅");

const newSession = (prompt) => {
  const out = execFileSync(AR, ["new", "--detach", "--workspace", WS, path.join(WS, "..", "base.yaml"), prompt], { encoding: "utf8" });
  const sid = out.trim().split("\n").pop().trim();
  if (!/^20\d{6}-/.test(sid)) throw new Error("ar new: no sid in output: " + out);
  return sid;
};
const waitIdle = async (sid, ms = 150000) => {
  const t0 = Date.now();
  while (Date.now() - t0 < ms) {
    const ss = await api("/api/sessions");
    const s = ss.find((x) => x.id === sid);
    if (s && s.status.startsWith("waiting:input")) return;
    await new Promise((r) => setTimeout(r, 1500));
  }
  throw new Error(`session ${sid} not idle after ${ms}ms`);
};

let sidA = "", sidB = process.env.SID_B || "";

if (PHASE === "fresh") {
  // ---- S1 写盘对账:turn 后外部写入,working-tree 声明必须 == git ----
  sidA = newSession("Reply with exactly the word hi. Do not use any tools.");
  await waitIdle(sidA);
  fs.writeFileSync(path.join(WS, "a.txt"), "hello\n");
  {
    const git = gitFiles();
    const wt = await diffFiles(sidA, "working-tree");
    if (wt.unknown) note("mismatch", "S1 working-tree unknown", "git 有改动而声明侧不可知");
    else if (!eq(wt.files, git)) note("mismatch", "S1 working-tree ≠ git", `api=${show(wt.files)} git=${show(git)}`);
    else note("ok", "S1 working-tree == git", show(git));
    const lt = await diffFiles(sidA, "last-turn");
    // scope 字面语义 = "自最近人类 turn 开始";turn 结束后的外部写入是否
    // 计入属产品语义灰区——记 observation 供裁决,不作硬门。
    note("observation", "S1 last-turn(A) after external write", `${lt.unknown ? "unknown" : show(lt.files)}`);
  }

  // ---- S2 脏接手不谎报(幽灵 diff 回归锚,QA-0718 用户实撞) ----
  sidB = newSession("Reply with exactly the word hi. Do not use any tools.");
  await waitIdle(sidB);
  {
    const lt = await diffFiles(sidB, "last-turn");
    if (!lt.unknown && lt.files.size > 0)
      note("mismatch", "S2 幽灵 diff:新会话 last-turn 含接手前改动", show(lt.files));
    else note("ok", "S2 新会话 last-turn 为空(不谎报 Edited)", "");
    const wt = await diffFiles(sidB, "working-tree");
    const git = gitFiles();
    if (wt.unknown || !eq(wt.files, git)) note("mismatch", "S2 working-tree ≠ git", `api=${wt.unknown ? "unknown" : show(wt.files)} git=${show(git)}`);
    else note("ok", "S2 working-tree == git", show(git));
  }
} else {
  if (!sidB) { console.error("restarted phase needs SID_B env"); process.exit(2); }
  // ---- S3 重启后不失踪(workflow 已重启 daemon+webui,journal replay) ----
  const ss = await api("/api/sessions");
  if (!ss.find((x) => x.id === sidB)) note("mismatch", "S3 重启后会话失踪", sidB);
  else note("ok", "S3 重启后会话在列", sidB.slice(0, 15));
  {
    const wt = await diffFiles(sidB, "working-tree");
    const git = gitFiles();
    if (wt.unknown || !eq(wt.files, git)) note("mismatch", "S3 重启后 working-tree ≠ git", `api=${wt.unknown ? "unknown" : show(wt.files)} git=${show(git)}`);
    else note("ok", "S3 重启后 working-tree == git", show(git));
    const lt = await diffFiles(sidB, "last-turn");
    if (!lt.unknown && lt.files.size > 0) note("mismatch", "S3 重启后 last-turn 幽灵复活", show(lt.files));
    else note("ok", "S3 重启后 last-turn 空/unknown(回退卡走 working-tree)", "");
  }

  // ---- S4 commit 清零:真相侧 commit,声明侧必须跟随 ----
  execSync(`git -C ${JSON.stringify(WS)} add -A && git -C ${JSON.stringify(WS)} -c user.email=qa@local -c user.name=qa commit -qm consistency-S4`);
  {
    const wt = await diffFiles(sidB, "working-tree");
    const git = gitFiles();
    if (git.size !== 0) note("mismatch", "S4 git 未清零", show(git));
    if (!wt.unknown && wt.files.size > 0) note("mismatch", "S4 commit 后声明侧仍有 Changes", show(wt.files));
    else note("ok", "S4 commit 后声明侧清零", "");
  }
}

const outDir = process.env.OUT_DIR || ".";
fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(path.join(outDir, `consistency-${PHASE}.json`), JSON.stringify({ findings, sidA, sidB }, null, 2));
if (PHASE === "fresh") console.log(`\nSID_A=${sidA}\nSID_B=${sidB}`);
const bad = findings.filter((f) => f.kind === "mismatch");
console.log(`\n${PHASE}: ${bad.length} mismatch / ${findings.length} checks`);
process.exit(bad.length ? 1 : 0);

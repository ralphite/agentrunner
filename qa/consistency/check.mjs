#!/usr/bin/env node
// QA-76 · UI 事实声明 × 系统真相对账器(docs/QA.md QA-76,QA-0718 复盘产物)。
//
// 性质:确定性 oracle(不是场景探索)。webui 的每条"事实声明"都有权威
// 真相源;本脚本把系统开进 QA-76 的语义状态 S1–S4,同时读**声明侧**
// (webui diff API——UI 卡与 rail 的数据源)与**真相侧**(git),断言
// 一致。任何 mismatch = 语义 bug,正是 QA-0718 用户实机三连撞的级别:
// 幽灵 diff(S2)、重启后失踪(S3)、清零不同步(S4)。
// S5/S5b(子 agent 终态与 child approval 可见性)在 QA.md 登记；机制侧
// 有 INC-81/98.3g 孪生，shared-store 闸门 B 由 QA-88 TH-09 常设复验。
// 本脚本仍只负责 diff/status 声明对账，不重复启动多-agent 场景。
//
// 用法(qa-consistency.yml 驱动,两阶段夹一次 daemon 重启):
//   node check.mjs <webui-base> <ar-bin> <workspace-dir> fresh      # S1+S2,输出 SID_B=
//   (workflow 重启 daemon+webui)
//   SID_B=<sid> node check.mjs <webui-base> <ar-bin> <workspace-dir> restarted  # S3+S4
//   node check.mjs <webui-base> <ar-bin> <workspace-dir> approval   # S7(需 ../ask.yaml)
// 前置:workspace 已 git init + 空提交;arwebui 已起;GEMINI_API_KEY 可用;
// spec 位于 <workspace-dir>/../base.yaml。
import { execFileSync, execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const [BASE, AR, WS, PHASE] = process.argv.slice(2);
if (!BASE || !AR || !WS || !["fresh", "restarted", "approval"].includes(PHASE)) {
  console.error("usage: check.mjs <webui-base> <ar-bin> <workspace-dir> fresh|restarted|approval");
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

const newSession = (prompt, spec = "base.yaml") => {
  const out = execFileSync(AR, ["new", "--detach", "--workspace", WS, path.join(WS, "..", spec), prompt], { encoding: "utf8" });
  const sid = out.trim().split("\n").pop().trim();
  if (!/^20\d{6}-/.test(sid)) throw new Error("ar new: no sid in output: " + out);
  return sid;
};
const waitStatus = async (sid, prefix, ms = 150000) => {
  const t0 = Date.now();
  while (Date.now() - t0 < ms) {
    const ss = await api("/api/sessions");
    const s = ss.find((x) => x.id === sid);
    if (s && s.status.startsWith(prefix)) return;
    await new Promise((r) => setTimeout(r, 1500));
  }
  throw new Error(`session ${sid} not ${prefix} after ${ms}ms`);
};
const waitIdle = (sid, ms) => waitStatus(sid, "waiting:input", ms);

let sidA = "", sidB = process.env.SID_B || "", sidC = "";

if (PHASE === "approval") {
  // ---- S7 审批语义对账(QA-0719 第十四轮远程首验的自动化孪生) ----
  // 声明侧:status pill(waiting:approval)与 diff API;真相侧:journal
  // approval_requested/responded + git。Approve ⇒ 命令真执行;Deny ⇒
  // 真未执行。spec 用 <ws>/../ask.yaml(bash: action ask)。
  const send = async (sid, text) => {
    const r = await fetch(`${BASE}/api/sessions/${sid}/send`, {
      method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ text }),
    });
    if (!r.ok) throw new Error(`send -> ${r.status}`);
  };
  const openApproval = async (sid) => {
    const evs = await api(`/api/sessions/${sid}/events`);
    const responded = new Set(
      evs.filter((e) => e.type === "approval_responded").map((e) => e.payload.approval_id),
    );
    const open = evs.filter((e) => e.type === "approval_requested" && !responded.has(e.payload.approval_id));
    return open.length ? open[open.length - 1].payload.approval_id : null;
  };
  const decide = async (sid, approvalId, decision, reason = "") => {
    const r = await fetch(`${BASE}/api/sessions/${sid}/approve`, {
      method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ approvalId, decision, reason }),
    });
    if (!r.ok) throw new Error(`approve -> ${r.status}`);
  };

  sidC = newSession("Use the bash tool to run exactly this command: touch s7-approved.txt — then reply with the single word done.", "ask.yaml");
  await waitStatus(sidC, "waiting:approval");
  note("ok", "S7 审批声明=真相:status waiting:approval", sidC.slice(0, 15));
  let ap = await openApproval(sidC);
  if (!ap) note("mismatch", "S7 journal 无未决 approval_requested", "status 声明待批而真相侧无记录");
  else {
    await decide(sidC, ap, "approve");
    await waitIdle(sidC);
    const git = gitFiles();
    const wt = await diffFiles(sidC, "working-tree");
    if (!git.has("s7-approved.txt")) note("mismatch", "S7 approve 后命令未真执行", `git=${show(git)}`);
    else if (wt.unknown || !wt.files.has("s7-approved.txt")) note("mismatch", "S7 approve 后声明侧缺文件", `api=${wt.unknown ? "unknown" : show(wt.files)}`);
    else note("ok", "S7 approve ⇒ 命令真执行(git+API 双侧)", "s7-approved.txt");
  }

  await send(sidC, "Now use the bash tool to run exactly this command: touch s7-denied.txt — nothing else.");
  await waitStatus(sidC, "waiting:approval");
  ap = await openApproval(sidC);
  if (!ap) note("mismatch", "S7 deny 腿 approval_requested 缺失", "");
  else {
    await decide(sidC, ap, "deny", "QA S7 deny leg");
    await waitIdle(sidC);
    const git = gitFiles();
    const wt = await diffFiles(sidC, "working-tree");
    if (git.has("s7-denied.txt") || (!wt.unknown && wt.files.has("s7-denied.txt")))
      note("mismatch", "S7 deny 后命令仍执行了", `git=${show(git)} api=${wt.unknown ? "unknown" : show(wt.files)}`);
    else note("ok", "S7 deny ⇒ 命令真未执行(双侧无痕)", "");
  }
} else if (PHASE === "fresh") {
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
fs.writeFileSync(path.join(outDir, `consistency-${PHASE}.json`), JSON.stringify({ findings, sidA, sidB, sidC }, null, 2));
if (PHASE === "fresh") console.log(`\nSID_A=${sidA}\nSID_B=${sidB}`);
const bad = findings.filter((f) => f.kind === "mismatch");
console.log(`\n${PHASE}: ${bad.length} mismatch / ${findings.length} checks`);
process.exit(bad.length ? 1 : 0);

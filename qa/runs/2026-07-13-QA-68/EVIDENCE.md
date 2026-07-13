# QA-68 Web UI iOS session 细节收口

## 结论

PASS。指定 prompt 的真实 session 已完成并保留；三 worker child 已全部打开。
五枚 finding 修复后，在 390×844 与 1280×900 均无页面横向 overflow，Browser
dev logs 的 warning/error 为 `[]`。

## Audit steps

| Health | Step | Evidence |
|---|---|---|
| 🟢 Healthy | live deploy/health | `health.json`：daemonUp/versionMatch=true，Go 1.26.5 |
| 🟢 Healthy | 指定 prompt completed session | `prompt-session.events.jsonl`、`prompt-session.inspect.json`、`10-prompt-session-mobile-after.png` |
| 🟢 Fixed | mobile sidebar / Environment 互斥 | before `../2026-07-13-ios-sessions-ui-audit/07-sidebar-mobile-before.png`；after `08-sidebar-mobile-after.png` |
| 🟢 Fixed | failed session 只说一次 | before `../2026-07-13-ios-sessions-ui-audit/09-failed-mobile-before.png`；after `07-failed-mobile-after.png` |
| 🟢 Fixed | activity marker/count | before `../2026-07-13-ios-sessions-ui-audit/11-team-activity-mobile-before.png`；after `02-team-activity-mobile-after.png` |
| 🟢 Fixed | child link 窄屏分栏 | before `../2026-07-13-ios-sessions-ui-audit/12-subagents-expanded-mobile-before.png`；after `03-subagents-expanded-mobile-after.png` |
| 🟢 Fixed | child header agent identity | `04/05/06-worker-*-mobile-after.png`；inspect spec=`worker_a/b/c` |
| 🟢 Healthy | recovery | `09-recovery-mobile-after.png`：单卡、标题/正文/按钮可读 |
| 🟢 Healthy | Changes | `11-changes-mobile-after.png`：366×820 contained，14 files/+932 |
| 🟢 Healthy | New session + Project/Access/Model picker | `12`～`15`：所有 popover 在 viewport 内 |
| 🟢 Healthy | desktop responsive | `16-prompt-session-desktop-after.png`：1280×900，无 overflow |
| 🟢 Healthy | deep link/reload + console | worker_b reload 后标题仍正确；warning/error=[] |

## Finding → fix

1. 两个 mobile overlay 可叠开 → `App` 把 navigation 状态传入 `SessionView`，
   mobile sidebar 打开即关闭 Environment。
2. failed 同时显示 provider failure 与泛化 terminal failure → 具体 journal
   failure 成为唯一 actionable card；标题、hint 块级排版。
3. activity summary 有原生/custom 双 marker，数量裸 `6` → summary 显式 flex、
   marker suppression、`×6` + `aria-label="6 activities"`。
4. child link 与 token 文案黏连 → chip 使用 wrap/flex gap，link 独立 shrink-0。
5. child header 暴露 parent machine slug → inspect `spec` 优先；metadata 未就绪时
   用 `Sub-agent · call N` fallback。

## 保留数据

- Prompt session：`20260713-082616-session-6d0d93bfcf9daa38`
- Team parent：`20260713-070914-run-the-three-worker-qa-delega-3dcf`
- 三 child：parent 下 `sub-call_1_1-a1` / `1_2-a1` / `1_3-a1`
- Workspace：`/Users/yadong/dev2/agentrunner/webui/runtime/ws/ws-20260713-012616`
- 未 close、未删除、未清理任何 session/workspace/journal。

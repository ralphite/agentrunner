# QA-93 goal × plan first-run journey — PASS（2026-07-24）

事故回归验证（对照 20260724-194450-…-af266b173beb1815 的五类问题）。
部署：daemon+webui 均 6d3b7daf-133626（versionMatch=true），真 Gemini
Flash Medium，webui 全流程真浏览器驱动（Home → +菜单 Goal + Plan mode →
project agentrunner (New worktree) → Send）。

## 场景 A 硬断言（全 PASS）
1. journal `mode_changed{to:plan,cause:startup}` + `goal_attached`；attach
   program input **引用 user 消息**（无 `<goal>` 全文重复）、含
   "do not call exit_plan_mode" 调和句 → events 导出 + dom-assertions.json。
2. `goal_completion_claimed` + `goal_checkpoint{pass}` + `goal_achieved
   {satisfied, checks:2}`；miss continuation 不含 "inspect the workspace
   state"（plan 版 evidence 句生效）。
3. 终态单一陈述：action-row `⊘ Goal achieved in 00:29` 唯一在场；无
   GOAL COMPLETE 横幅、无 `Goal check N · passed`/`Goal achieved ·
   satisfied` 顶层 chips（miss check 折叠进 "Worked for 23s"）。
4. 全程（live turn 中与终态）无 "Loading changes…" 骨架、无
   `.changes-outcome-skel` 节点。
5. composer 底部留白 16px（≥12px）。
- 观察项：**exit_plan_mode 调用 0 次**（事故会话 3 次 + 用户两次追问）；
  agent 直接在对话交付 plan 后 goal_complete。

## 场景 B（UI 抽验 PASS + scripted 锚）
GoalOptions（Plan · read-only + verifier "go test ./..."）内联警示逐字：
"Plan (read-only) mode can't run commands — this check only starts working
after you approve leaving plan mode."。checkpoint 短路（miss detail 人话、
零 effect/deny 噪音）由 TestGoalVerifyPlanModeShortCircuit 钉住。

会话保留共享 store 供复查（不 close 不删）。

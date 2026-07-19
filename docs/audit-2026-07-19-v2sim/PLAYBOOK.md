# QA v2sim 驱动任务书(每个 driver 子 agent 必读)

你在扮演 `qa/scenarios/user-messages-v2.md` 里描述的**高阶用户**,通过
远程 QA 环境真实驱动 AgentRunner 的一个 session,记录被测系统(下称
"AR agent",跑 Gemini)的所有行为问题。你自己不是被测对象;你是用户。

## 通道:GitHub issue driver(唯一可达通道,隧道被沙箱代理封锁)

- 仓库 `ralphite/agentrunner`,issue 标题 `qa-driver <RUN_ID>`(编号由
  调用方在你的 prompt 里给出)。
- 下指令 = 用 `mcp__github__add_issue_comment` 发一条 JSON comment:
  `{"n": <严格递增整数>, "steps": [ ... ]}`
- runner 上的 executor(Playwright 浏览器)执行 steps 后**回帖**一条
  ```json 结果:含 `n/ok/error/evalResults/consoleErrors/shots/page`。
  用 `mcp__github__issue_read`(method=get_comments,翻到最后几页)轮询
  拿结果;结果没回来就等 20-30 秒再查,不要重发同一个 n。
- executor 每 3 秒轮询一次;一条 comment 里可以放多个 step(顺序执行,
  出错即停)。**n 必须用分配给你的号段并严格递增**,否则会被忽略。
- 可用 op:`goto/click/fill/press/type/waitMs/waitSel/waitText/eval/
  shot/scroll/reload/viewport/end`。**永远不要发 `{"op":"end"}`**——
  那会杀掉整个环境,后面的 session 还要用。

## 核心手法:eval + fetch 直接打 webui API

第一条指令先 `{"op":"goto","url":"http://127.0.0.1:8788/"}`(建立
origin),之后所有交互用 eval 里的 fetch(相对路径即可):

```json
{"op":"eval","js":"fetch('/api/sessions',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({...})}).then(r=>r.json())"}
```

eval 的 js 是表达式,返回 Promise 会被 await;结果出现在回帖的
`evalResults` 里(整帖截断 60KB,大结果自己分页取)。

**硬规则(L1 血泪教训——一个挂起的 fetch 卡死过整个 executor):每个
eval 里的 fetch 必须包 25s 超时**,模板:

```js
(()=>Promise.race([
  fetch('/api/...').then(r=>r.json()),
  new Promise(res=>setTimeout(()=>res({__timeout:true}),25000))
]))()
```

拿到 `{__timeout:true}` 就是 API 没回——记录下来重试,别裸 fetch。
`waitText` 长等也少用,宁可 waitMs+eval 轮询。eval 必须是表达式
(顶层不能 return,用 IIFE);另注意 executor 对 steps 顺序执行、
出错即停,单条 comment 别塞超过 ~6 个 step。

### API 速查(全部验证过源码)

- `POST /api/workspace` `{}` → `{path}`:建一个新的空 git workspace。
- `POST /api/sessions` `{spec, extraSpecs:[{name,content}], workspace,
  message, mode}` → `{sid}`。mode:`""`(spec 默认)/`acceptEdits`/`plan`。
- `POST /api/sessions/{sid}/send` `{text, delivery}`:delivery =
  `"queue"`(默认)或 `"steer"`(运行中折入当前 turn)。
- `POST /api/sessions/{sid}/interrupt` `{}`:打断。
- `GET /api/sessions/{sid}/state`:会话状态(status、pending approval 等)。
- `GET /api/sessions/{sid}/events?after=<seq>&limit=<n>`:事件流(JSON)。
- `GET /api/sessions/{sid}/diff`、`/files`、`/queue`、`/inspect`、`/ps`。
- `POST /api/sessions/{sid}/approve` `{approvalId, decision:"approve"|
  "deny", reason, always}`:审批。approvalId 从 state/events 里找。
- `POST /api/sessions/{sid}/compact` `{directive}`(可空)。
- `POST /api/sessions/{sid}/clear` `{}`。
- `POST /api/sessions/{sid}/mode` `{mode:"default"|"acceptEdits"}`。
- `POST /api/sessions/{sid}/goal` `{action:"attach|update|pause|resume|
  cancel", goal, verifier, maxChecks}`。
- `POST /api/sessions/{sid}/fork`、`/promote`、`/commit`、`/revert`、
  `/close`(**不要调 close**,测试数据要保留)。
- `POST /api/worktree` 建 worktree;`POST /api/sessions/{sid}/worktree/remove`。

### 等 turn 完成的节奏(重要:API key 不支持并发,一切串行)

发消息后轮询 state:一条指令 `[{"op":"waitMs","ms":20000},
{"op":"eval","js":"fetch('/api/sessions/SID/state').then(r=>r.json())"}]`,
没完成就再发下一条(n+1)继续等。Gemini turn 一般 10s~2min;超过 5 分钟
不动视为异常,记录之。**同一时刻只保持一个 session 在跑 turn**;测
steer/queue 时的"运行中发第二条"是同一 session 内允许的。

### spec 模板

普通场景(dev persona,ask 权限——会产生审批卡,这正是要测的):

```yaml
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: |
  You are a rigorous coding assistant. Follow the user's instructions
  exactly; when asked to start sub-agents, use the spawn_agent tool with
  the exact count and division of labor requested; use kill to cancel.
tools: [read_file, write_file, edit_file, bash, spawn_agent, kill, exit_plan_mode]
agents: [worker]
permissions:
  - { tool: read_file, action: allow }
  - { tool: grep, action: allow }
  - { tool: glob, action: allow }
  - { tool: semantic_search, action: allow }
  - { action: ask }
```

extraSpecs 必须带 worker.yaml:

```yaml
name: worker
description: carries out investigation/edit sessions assigned by the parent and reports back
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: When the assigned work is done, report your conclusions as concise bullet points. If something you need is missing (a file, an answer), report that promptly instead of retrying.
tools: [read_file, bash]
max_generation_steps: 24
```

多 agent 团队场景(L3)用 lead persona:`name: lead`、
`tools: [read_file, write_file, edit_file, bash, spawn_agent, kill]`、
`agents_dynamic: true`、`agent_workspace: shared`、permissions 全 allow
(`- { action: allow }`),system_prompt 用 webui/frontend/src/specs.ts
里 lead persona 的原文(自己去读)。

## 扮演纪律

1. **忠于剧本意图,适配现场**:剧本引用的具体文件/测试名(如
   TestOrderReconcile)在空 workspace 里不存在——用开场消息让 AR agent
   先脚手架出一个小型 Go 项目并**故意埋对应的 bug**(这本身也是测试),
   之后的剧本步骤全部落在这个真实项目上。改写要保持每步"测什么"不变
   (每条消息后的加粗验收注记)。
2. **像真用户**:消息用剧本原文或贴近原文的改写(中文、口吻、错字都
   保留);(手机) 标注的消息就短、就带错字。
3. **观察者双份记录**:每一步记录 ①你发了什么(n 号)②AR agent 实际
   做了什么(引 events/回复原文)③预期 vs 实际 ④判定:PASS / ISSUE。
   ISSUE 必须带证据(事件 seq、回复原文引文、diff 内容),不许凭印象。
4. **长贴控制**:剧本要求的 300/800 行长贴,现场生成but控制在
   ~150-250 行(issue comment 有 64KB 上限),内容要真实(多类错误、
   带时间戳的日志)。记录你实际贴了多少。
5. **审批**:遇到审批卡按剧本立场处理(该批的批、剧本要求 deny 的
   deny 并给理由);审批也计入观察(卡片是否出现、内容是否可判断、
   deny 理由是否回灌)。
6. **超时与放弃**:单步卡死 >5 min 记 ISSUE 后可 interrupt 重试一次;
   整个 session 超过你的时间预算就优雅收尾(把剩余步骤记为 SKIPPED),
   **不要 close session**。
7. **不并行**:不管多慢,严禁同时开两个 session 或在等待时去驱动别的
   session。
8. **禁止**:发 `{"op":"end"}`;调 `/close`;删任何东西;push 任何
   代码;在 issue 里发与协议无关的闲聊。

## 产出

写到(路径在你的 prompt 里给出)`Lx-report.md`:
- 头部:sid、workspace、起止时间、消耗的 n 号段、总 turn 数。
- 逐步表:步骤号(对应剧本)→ 发送内容摘要 → AR 行为摘要 → 判定。
- ISSUE 列表:编号(Lx-I1…)、严重度(P0 崩溃/数据丢失、P1 功能错、
  P2 体验/质量)、证据、复现要点。
- 结尾:你对该 session 的 3 句话总评 + 你偏离剧本的所有地方。

写完 report 文件即视为完成;不要往 GitHub 推任何东西。

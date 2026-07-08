# arweb — AgentRunner 测试驾驶舱(web/ 内部设计文档)

**这是什么**:一个极简、自包含的本机 web app,用来以"真实用户"的方式
驱动 AgentRunner 的全部会话能力——chat、插话排队、interrupt、起/杀
子 agent、图片输入、审批、崩溃恢复观察——作为手工测试与 QA 观察面。
UI 追求朴素,功能追求全覆盖。

**这不是什么**:不是产品的一部分,不进 `docs/` 三层产品定义,不参与
`scripts/check.sh` 闸门,不做多用户/鉴权/部署。

---

## 1. 铁律(本目录的不变量)

- **I1 依赖面 = `ar` CLI 公开契约。** arweb 只通过 `os/exec` 调用
  `ar` 子命令,依赖其 argv、stdout/stderr、退出码。绝不 import 仓库
  内部 Go 包,绝不直读 `$XDG_DATA_HOME` 数据目录,绝不自己解析
  daemon socket 协议。CLI 白名单见 §4。
- **I2 反向零依赖。** 仓库其余部分(代码、qa/ 脚本、docs/、check.sh)
  不得引用 web/。web/ 是独立 Go module(`module arweb`),根目录的
  `go vet ./...`、`go test ./...`、golangci-lint 均触及不到它;唯一的
  交面是根 `gofmt -l .` 会走到本目录的 .go 文件(保持 gofmt 干净即可)。
- **I3 零外部依赖。** 后端 stdlib-only;前端单文件 `static/index.html`,
  vanilla JS + 内联 CSS,零框架、零 CDN、零网络资源。
- **I4 只听 127.0.0.1,无鉴权。** 本机测试舱,永不对外暴露。
- **I5 journal 是时间线的唯一事实源。** 聊天时间线由
  `ar events --json` 的 journal 事件渲染;`ar attach --json` 的实时流
  只用作流式装饰(text_delta 打字效果)。两者不一致时以 journal 为准
  ——这与 DESIGN.md 的"journal 即真相"哲学同构,但 arweb 只站在
  CLI 契约这一侧。

## 2. 架构

```
浏览器(单页 index.html)
   │  HTTP/JSON(操作与轮询) + SSE(流式装饰)
   ▼
arweb(Go,无状态壳;web/ 目录,127.0.0.1:8787)
   │  os/exec:每个请求 → 一次 ar 子命令(直接 argv,无 shell)
   ▼
ar CLI ── unix socket ──> ar daemon(会话宿主)
```

- arweb **自身无持久状态**:一切事实在 AgentRunner 的 journal 里。
  `web/runtime/`(gitignored)只存便利产物:生成的 spec 文件、上传的
  图片、临时 workspace、daemon.log。
- daemon 托管:启动时默认 spawn `ar daemon`(继承 arweb 的环境,含
  `--env-file` 载入的 `GEMINI_API_KEY`);若 700ms 内退出,视为外部
  已有 daemon,标记 external 并继续。`--no-daemon` 关闭托管。退出时
  杀掉自己拉起的 daemon(外部的不动)。
- 探活:CLI 没有专门的 ping 动作,用 `ar interrupt __arweb_probe__`
  的 stderr 区分——含 "is the daemon running" 为不可达;其余(如
  unknown session)为可达。探针会话名不存在,副作用为零。

## 3. 数据流

- **时间线(真相)**:前端持 `cursor=最后已见 seq`,每 1s
  `GET /api/sessions/{sid}/events?after=cursor`;后端跑
  `ar events --json` 并按 seq 过滤后返回增量。事件按 §5 映射渲染。
- **流式(装饰)**:选中会话时开 `GET /api/sessions/{sid}/stream`
  (SSE);后端 spawn `ar attach --json <sid>` 逐行透传。前端只消费
  `text_delta`(临时打字气泡)与 `approval_request`(提前亮审批卡);
  durable 版本到达(journal 的 assistant_message / approval_requested)
  即丢弃临时件。断线只影响装饰,不影响真相。
- **排队语义**:实现是 journal-on-boundary(见 QA-02 / QA.md):忙时
  `send` 的 `input_received` 在安全边界才落 journal。因此发送后先渲染
  本地 pending 气泡("已投递,排队中"),等 journal 出现同文本的
  `input_received` 再落实,不视为丢失。

## 4. API ↔ CLI 映射(依赖白名单)

| HTTP | ar 命令 | 说明 |
|---|---|---|
| GET  /api/health | `ar --version` + interrupt 探针 | daemon 状态、版本 |
| POST /api/daemon/start | `ar daemon`(spawn) | 重启托管 daemon |
| GET  /api/sessions | `ar sessions list` | 解析表格为 JSON |
| POST /api/sessions | `ar new --detach --workspace W [--mode M] <specdir>/base.yaml "msg"` | spec 文本落 runtime/specs/,worker 等旁置文件同目录(CLI 按 SpecPath 兄弟解析) |
| POST /api/workspace | (mkdir) | 造空临时 workspace,返回绝对路径 |
| GET  /api/sessions/{sid}/events?after=N | `ar events --json <sid>` | journal 增量 |
| GET  /api/sessions/{sid}/state | `ar events --state <sid>` | 折叠状态(D2 后:`handles` 键、`closed` 标记、无终态 status) |
| GET  /api/sessions/{sid}/inspect | `ar inspect --json <sid>` | 含子 session 树、工具关键参数(detail)、被拒调用 |
| GET  /api/sessions/{sid}/ps | `ar ps <sid>` | 在飞后台工作(tab 分列;空集输出仍以 "no tasks" 开头) |
| GET  /api/sessions/{sid}/stream | `ar attach --json <sid>` | SSE 透传 |
| POST /api/sessions/{sid}/send | `ar send --detach [--image f]... <sid> "text"` | 图片为服务器侧路径 |
| POST /api/sessions/{sid}/interrupt | `ar interrupt <sid>` | 带外打断(运行中转向;**待命处 no-op**,只落审计行,裁决 #11) |
| POST /api/sessions/{sid}/close | `ar close <sid>` | 关会话 |
| POST /api/sessions/{sid}/kill | `ar kill <sid> <handle>` | 用户直杀后台 handle |
| POST /api/sessions/{sid}/approve | `ar approve <sid> <id> approve\|deny [reason]` | 审批 |
| POST /api/sessions/{sid}/agent | `ar agent <sid> <specdir>/agent.yaml` | session 内换 agent(决策 #32);spec 文本落 runtime/specs/,下一条消息生效 |
| POST /api/upload | (存文件) | multipart → runtime/uploads/,返回路径 |

`--detach` 的由来:INC-2 起 `ar new`/`ar send` **默认跟随本轮并把回复
渲染到 stdout**(面向人);程序性消费方一律 `--detach` 回到 ack-only
形式(new → stdout 纯 session id;send → "delivered")。arweb 的回复
从 journal 轮询取,与 qa/ 脚本同一姿势。

错误约定:ar 非零退出 → HTTP 502,body `{error, stderr}`,前端原样
展示 stderr(测试舱要看得到真实报错)。

## 5. 时间线渲染模型(journal type → UI)

| journal 事件 | UI 元素 |
|---|---|
| input_received | 用户气泡(text + 图片 ref chip;source≠user 加"控制"标) |
| generation_started | 轮次分隔线 "第 N 轮" |
| assistant_message | 助手气泡(渲染 parts 里的 text;非 text part 忽略,工具活动另有事件;finish=blocked 加"可见截断"chip) |
| activity_started (kind=tool) | 工具卡(name + args,折叠;background=true 加"后台"标) |
| activity_completed / failed / cancelled | 回填对应工具卡:结果/错误/取消 + usage 徽章 |
| spawn_requested | 子 agent 卡(agent、task、child_session 短 id) |
| subagent_completed | 子 agent 完结 chip(reason + tokens) |
| approval_requested | 审批卡(批准/拒绝按钮 + 理由输入);approval_responded 后固化结论 |
| waiting_entered / resolved | 底部状态条(kind 只剩 input=待命可输入 / approval;D2 删了 tasks/timer) |
| mode_changed / context_compacted | 灰色系统 chip |
| limit_exceeded | "可见截断"chip(kind ∈ tokens\|generation_steps\|malformed_tool_call;会话不终结,待命可续发) |
| spec_changed | "换 agent → 名字·模型"chip(决策 #32) |
| session_closed / actor_crashed | 终态/崩溃 chip + 状态条;reason ∈ closed\|killed,kill 标记带 source ∈ user\|parent(D2;task_completed 事件已删) |
| 其他未列类型 | 兜底灰色单行 `seq type`(事实不静默丢弃) |

悬摆审批 = journal 里 approval_requested 尚无同 id 的
approval_responded → 审批卡保持可操作。

**子 agent 审批上卷(2026-07-07)**:子 loop 无 Out sink,但共享父的
approvals resolver——子的 ask 以 SSE `approval_request` 事件出现在
**父的 attach 流**(`text` 字段 = 请求方 agent 名 + 子会话 id),且
**不落父 journal**。因此审批卡是双通道去重渲染(approvalCard,按
approval id):父自身的 ask 走 journal(带 gate 徽章)+ SSE;子的 ask
只走 SSE,应答后本地固化。approve 一律 POST 到父 sid(broker 按
approval id 路由)。

## 6. 安全注意

- exec 一律直接 argv,无 shell 展开;sid、handle、approval id 校验
  `^[A-Za-z0-9._-]+$`。
- spec 旁置文件名只取 `filepath.Base` 且须匹配 `*.ya?ml`。
- 上传限 10MB,落 runtime/uploads/,文件名清洗。
- 只绑定 127.0.0.1(I4);无 CSRF/鉴权,因不对外。

## 7. 开放问题(进 PROGRESS 排期或搁置)

- 子 session journal 深挖:CLI 的会话前缀解析不达 `sub/` 层;先靠
  `ar inspect --json` 的树 + 父 journal 的 spawn/settle 事件;若需要
  逐子时间线,再议 CLI 是否加公开动作(那属于产品增量,走 docs/ 流程)。
- SSE 断线自动重连与 attach 进程回收的边角。
- fork / barrier / drive(IterationDriver)面板:暂不做,驾驶舱聚焦
  会话交互测试。

# AgentRunner 真实使用审计报告 — 2026-07-07

**性质**：8 个测试 agent 并行、像真实用户一样用 `ar` CLI 真实使用产品全功能面，
全程真实 Gemini API（`gemini-flash-latest`），**零 mock、零 scripted provider、零造假**。
目标：找出所有问题，不改任何产品代码。触发原因：用户在 web UI 上几条消息就把
一个会话搞死（`gemini: message with role "assistant" has no parts`），且重开即再死。

**规模**：**578 个真实 agent loop**（`generation_started`；另有 835 个 `activity_completed`
含工具活动），跨 131 个顶层会话 + 38 个子 agent 会话，8 个功能域全覆盖。

**逐 agent 详细使用日志**（含每条发给模型的消息原文、系统逐步反应、独立验证）见
本目录 `agent1..8-report.md`。

---

## 一、执行摘要（TL;DR）

产品**内核是扎实的**，但有**一条会吞掉真实用户的主干裂缝**和**两个安全边界失效**，
外加若干韧性与体验缺口。

- **用户遇到的会话死亡不是偶然，是必然**：任何真实的"vibe-coding"式连续开发，只要
  跑到较深轮次或让模型产出较多内容，就会触发空-parts assistant 消息，永久毒化会话
  历史、把会话钉死在 error 且无法恢复。**4 个马拉松会话 100% 命中**（死在 turn 24/首轮/
  turn 38/turn 40+）。这与 web UI 用户的原始截图同源。
- **workspace 安全沙箱对 bash 完全失效**：启用 bash 的会话可读写工作区外任意文件，
  包括用户的 `~/.ssh` 私钥。**我本人独立复现**。
- **凭据 redaction 名不副实**：`.npmrc`/`.netrc` 经 `read_file` 明文回灌给模型并落盘，
  尽管 SPEC 明确标注该硬排除表 ✅ 已收口。**我本人独立复现**。
- **好消息（同样重要）**：daemon 协议层极其健壮（14 种畸形输入 + 并发风暴零崩溃）；
  存储层扛住 3+ 次 `kill -9`（journal 全部合法、seq 连续、snapshot 可解析）；核心
  对话、多轮、工具面、子 agent 编排、审批流、goal/loop/best-of-N 驱动、观察面绝大多数
  **真实可用**；报错质量普遍优秀。**"几条消息搞崩"的根因是空-parts bug，不是协议层
  或存储层。**

### 现有 QA 为什么全绿却漏掉这一切

| QA 的做法 | 造成的盲区 | 漏掉的 bug |
|---|---|---|
| 消息都是"一句话作答/数文件"级轻指令 | 从不触发长输出/深度 thinking，2048 token 上限永远够用 | 空-parts 死亡（生成型主干） |
| 会话最多 2–3 轮 | 从不到 turn 20+，长会话续命路径无覆盖 | 深轮次死亡、compaction 后行为 |
| `qa/lib.sh` 全程单脚本进程跑完 daemon + 所有命令 | socket 路径短、daemon 不跨进程组、从不真重启 | A2-2 长路径、审批不持久、crash 不自愈 |
| 并发子 agent 用 routing/scripted provider（决定论孪生） | 关键路径不走真实 LLM | 一切 thinking/空-parts 相关 bug |
| 安全验收侧重 file 工具的 path 逃逸 | bash 的 command 参数不被边界检查覆盖 | bash 越界读写（A4-1） |
| MCP 只在单测夹具里连过 | 没有端到端 spec→run 的真实路径 | `mcp:` spec 字段根本不存在（A6-1） |

---

## 二、问题清单（按严重度排序）

严重度：🔴 critical（会话死亡/数据或凭据泄漏/安全绕过/招牌功能不可用）·
🟠 major（功能不 work/韧性缺口）· 🟡 minor（体验/文案/可观测性）。
「✔ 已亲验」= 汇总者本人独立复现确认。

### 🔴 Critical

#### [C1] 空-parts assistant 消息永久毒化会话历史 → 会话死亡且不可恢复 ✔ 已亲验
- **来源**：A1-1、A7-1（web UI 用户原始 bug 同源）；命中率 **4/4 马拉松会话**。
- **现象**：某个 LLM turn 返回的 assistant 消息被 adapter 折叠成 `parts: null`
  （抓到一次 `output_tokens=1868` 却 `parts=0`——模型确实生成了内容却被丢空）。
  该空消息写入 journal 后，**下一轮组装历史把它发回 Gemini**，被
  `gemini: message with role "assistant" has no parts`（`retryable:false`）拒绝 →
  `session_closed reason=error`。
- **第二层（永久死亡）**：`send` 重开被接受（回 `delivered`）但立刻重跑同一失败
  activity、再送污染历史、再死；`ar resume` 拒绝 `task session already completed (error)`。
  **无任何恢复路径。**
- **复现**：
  ```bash
  SID=$(ar new --workspace ws base.yaml "write a number guessing game in python, single file, then run it once to verify")
  ar send $SID "add a scoring system and a play-again loop"
  ar send $SID "now convert it to a web app. use just js and html"   # 注入空 assistant(表面无异常)
  ar send $SID "open index.html in a headless check ..."             # 立刻死
  ar send $SID "hello"                                               # 重开→立刻再死
  ```
- **根因线索**：写入侧无空-parts 保护（`internal/provider/gemini/gemini.go` 的
  `mapPart` 对非 text/非 functionCall 的 part 走 default 丢弃；纯 thought 输出即空），
  且请求侧 `toContent()`（gemini.go:238-239）遇空 parts 直接 error 而非修补。
- **修复方向**：(1) 写入侧 parts 为空时合成占位 part，绝不落 `parts:null`；
  (2) 请求侧组装历史时过滤/修补空 parts；(3) 把这类历史错误标为可修复而非钉死会话。

#### [C2] bash 工具完全绕过 workspace 边界，可读写任意文件（含 ~/.ssh 私钥）✔ 已亲验
- **来源**：A4-1。汇总者独立复现：`cat <工作区外文件>` 成功返回并落 journal。
- **现象**：`read_file ../outside/.env` 被 floor 正确拦（`denied: path escapes workspace`），
  但 bash 的 `cat ../outside/.env`、`echo PWNED > <ws外路径>`、`ls $HOME/.ssh`（列出真实
  `id_ed25519` 等）全部成功。
- **根因**：`internal/pipeline/permission.go` 的 hardFloor 只解析 file 工具的 `args.Path`，
  bash 的参数是 `command`，逃逸检查从不作用于它。**任何启用 bash 的会话，ws 边界形同虚设**
  ——prompt injection 可读 `~/.aws/credentials`、`~/.ssh/id_*`，写/删任意用户文件。

#### [C3] .npmrc/.netrc 凭据经 read_file 明文泄漏并落盘（SPEC 声称已 redact）✔ 已亲验
- **来源**：A4-2。汇总者独立复现：`.npmrc` 的 authToken 明文在 journal 出现 2 次。
- **现象**：`read_file .npmrc` → `← ok {"content":"…_authToken=sk-…"}`，模型原样回显。
  `.netrc` 同样明文泄漏。
- **矛盾**：`docs/SPEC.md:82`「凭据 redaction + 硬排除表（含 .netrc/.npmrc 等）」标 ✅
  （S2/S7 收口）。
- **根因**：`.netrc/.npmrc` 硬排除只在 `internal/snapshot/snapshot.go`（快照/索引）生效；
  `read_file`（`internal/tool/exec.go`）走的 `redact.FromEnv()` 只按**环境变量值**
  （`_API_KEY/_TOKEN/_SECRET` 后缀）redact，对文件内任意 token 无效。

#### [C4] fork 出的会话吞掉发给它的每一条消息（招牌功能"续跑 fork"不可用）
- **来源**：A5-2。两个独立 fork 均 100% 复现。
- **现象**：`ar send <fork-sid> "…"` 回 `delivered`，但无 `input_received` 事件、文件不写、
  对话状态里没有这条消息；消息 durably 堆在 `inbox.jsonl` 却永不消费。`ar resume` 也不消费。
  **既违反"崩溃不丢输入"铁律（消息被 ack、fsync 进 inbox 却永不处理），又让 fork/rewind
  时间旅行的招牌能力完全不可用。**
- **根因（已定位）**：`session_started` 的 `Conversational` 标志——`internal/event/types.go:114`
  注释「a send-driven revival wires the inbox **iff this is true**」。正常会话
  `Conversational=True`（send 生效），fork **MISSING**（吞消息）。fork 创建时未置
  `Conversational=true`。

#### [C5] daemon socket 路径超 macOS 104 字节即 `bind: invalid argument`，无可诊断提示
- **来源**：A2-2。**6 个 agent 独立撞上并复现**。
- **现象**：`XDG_DATA_HOME` 较长（socket 全路径 >104 字节）时 daemon 启动即
  `daemon: listen unix …/daemon.sock: bind: invalid argument` 退出，**封杀所有 daemon
  依赖功能**（conversational 全家、submit/resume、远程审批）。短路径即恢复。
- **问题**：报错是底层 syscall 原文，无"路径太长"提示；普通用户一头雾水。
- **修复方向**：socket 落固定短目录（如 `$TMPDIR` 下哈希短名），与数据目录解耦；或启动
  时预检长度给可操作报错。

#### [C6] MCP 全域对用户不可达：文档化的 `mcp:` spec 字段在代码里不存在（SPEC 标 ✅）
- **来源**：A6-1。
- **现象**：任何含 `mcp:` 的 spec → `yaml: unmarshal errors: field mcp not found in
  type agent.AgentSpec`。`AgentSpec` 无 `mcp` 字段，`KnownFields(true)` 硬拒；且没有
  任何 CLI 构造点给 `Loop.MCP` 赋值、没有代码建立 stdio transport。`internal/mcp` 包对
  产品是死代码，只被单测触达。
- **矛盾**：`docs/SPEC.md:127`「MCP stdio 全生命周期」标 ✅（S5）、`DESIGN.md §9` 有完整
  `mcp:` 语法。

### 🟠 Major

- **[M1] mid-turn crash 后会话不自动 resume，卡死 `running` 且状态撒谎**（A5-3、A8-2）：
  打断在飞 LLM/bash 后重启 daemon，会话永久 `running`、零进展，被打断的问题无人应答；
  `sessions list`（读磁盘）一直报 running（末事件却非在飞活动）。恢复**可行但须用户显式**
  `ar resume`/`ar send`，且无任何提示。**直接对应用户"会话死后无法恢复"的观感。**
- **[M2] 审批 broker 仅存内存，daemon 重启后 pending 丢失，`ar approve` 死路**（A4-5）：
  重启后 `sessions list` 仍 `waiting:approval`，但 `ar approve` 报 `no pending approval`
  exit 1，会话卡死；CLI 通知却一直指引用 `ar approve`。
- **[M3] crash 回执触发模型失控重跑，runtime 无护栏**（A5-1）：单次 mid-bash crash 后
  billed 从几百飙到 **27123 tokens**（14 gen-steps 狂调工具），真实 API 花费失控。
- **[M4] error-closed 会话 send 仍回 `delivered` 才失败**（A1-3、A7-2）：用户看到
  `delivered` 以为成功，实则后台立刻同错再 close。应即时拒绝或明示"会话处于错误态"。
- **[M5] send 的 `idem_key` 被完全忽略 → 重复投递**（A8-1）：`handleSend`（daemon.go:465-516）
  从不读 `cmd.IdemKey`（idem 只在 run/drive 实现）；网络抖动/重试 → 消息重复 → agent
  重复响应 + 重复计费。
- **[M6] tool 结果实时 stdout 回显不经 redaction**（A4-3）：模型消息层与 journal 已
  redact，但 `ar run` 流式打印的 `← ok {"content":"…真实密钥…"}` 是明文；stdout 重定向到
  日志/CI artifact 即泄漏。
- **[M7] daemon 重启后磁盘上 idle 会话无法 close/send**（A1-2、A5-4）：重启前创建、磁盘
  `waiting:input` 的会话，`ar close`/`send` 报 `no live … could not be resumed`，但
  `sessions list` 仍显示 waiting:input（on-disk 状态与 daemon 内存注册表脱节）。
- **[M8] handoff_agent 正常终止返回非零退出码（exit 1）**（A3-1）：`internal/cli/run.go:229`
  `if result.Reason != "completed"` 把 `reason:handoff` 误判为失败，CI/脚本会把成功的
  handoff 当失败。
- **[M9] daemon `ar new` 对加载失败的 spec 返回成功 + 派发幽灵 session ID**（A6-2）：
  坏 spec（缺 model.provider 等）经 daemon `ar new` 时 rc=0 + 打印一个 sid，但 session
  目录从不创建、daemon 无 error；随后 `ar send <该 sid>` 报 `no live session`。前台
  `ar run` 对同一 spec 诚实报错。根因：handleRun 在 LoadSpec **之前** mint ID 并
  Encode(SessionStart)，CLI 收到即 detach，真正的错误 emit 进没人读的流。

### 🟡 Minor（体验/文案/可观测性）

- **[m1]** `schedule: interval` 漏写 `interval` 字段 → 静默变 back-to-back 热循环，无校验
  无警告，无界 loop 会全速打满 API（A2-3）。
- **[m2]** 前台 loop（`ar drive`）对**单次** Ctrl-C 无反应且继续起下一迭代；只有双击
  Ctrl-C 或 SIGTERM 能停，均不直观、文档无载（A2-1，根因 drive.go:66 丢弃 interrupts channel）。
- **[m3]** 多数子命令（resume/close/interrupt）不支持 `--help`，反把它当业务参数执行（A1-4）。
- **[m4]** `ar send --image` 要求 `--image` 在 sid 之前，放后面报 usage 静默不发（A1-5）。
- **[m5]** `ar fork` usage 文案把 `--workspace` 写在位置参数之后，实际须在之前（A5-5）；
  `ar attach --json` 同类。
- **[m6]** `ar close` 无法关闭已持久化但非 live 的会话（A5-4）；间接卡住手动 barrier。
- **[m7]** plan mode 退出后无 `mode_changed to default` 事件，可观测性不对称（A4-4）。
- **[m8]** `ar attach` 回放审批提示的 approval-id 字段为空，照提示无法 approve（A4-6）。
- **[m9]** `ar ps` 空状态（无 sessions）报 `no sessions found (open …: no such file…)` 再
  打 usage，新用户首次运行即见报错（A3-2）。
- **[m10]** 协议 unknown-command 错误的 known 列表漏 `drive`（A8-4）。
- **[m11]** 协议 `kill` 不存在的 handle 谎报成功 `killing <handle>`（A8-3）。
- **[m12]** memory 层级合并只向上 walk 到 git 根、从不向下扫子目录；用户在项目根开会话、
  子目录放模块级 CLAUDE.md 会以为生效（A6-4，设计如此但易踩）。
- **[m13]** 合并后外层 CLAUDE.md 标签渲染成超长绝对路径（memory.go:80 的 Rel fallback）（A6-5）。

---

## 三、真实通过、经得起拷打的部分（同等重要）

避免只见树木——以下都是**真实 API 下实测 PASS**，且不少经受了极端拷打：

**核心对话（A1）**：多轮续聊、close→send 重开且上下文完好、idle interrupt=close、
>10KB 长消息折叠为 file part（埋在第 200 行的 token 准确抽出）、图片输入（模型真读到
PNG 像素里的报错文字）、特殊字符（反引号/`$`/引号/emoji/换行逐字保留）、忙时投递按序
不丢不乱、**无密钥明文泄漏进 daemon 日志**。

**task/driver（A2）**：one-shot task、**自主修复 cobra 注入的失败测试**（复现→定位→改源→
验证全闭环）、goal mode verifier 三态达标即停、loop mode interval 跑满、best-of-N 隔离
worktree 并行 + per-attempt 判定、预算耗尽 LimitExceeded 显式可见、interrupt 部分产出
留存、错误路径报错质量优秀（未知 provider 还列出可选项）。

**工具面 + 子 agent（A3）**：bash 前后台（超长输出头尾截断、标记可见）、文件工具（磁盘
真实正确、中文 UTF-8 完好）、spawn 前后台（立即拿 handle 不阻塞、子进程真死无孤儿）、
task_kill/CLI kill 双路径、blackboard（note 带来源跨 agent 传递）、publish_artifact
（CAS ref 与 shasum 一致）、semantic_search（BM25 真实相关）、3 并发 handle 不串。

**权限安全（A4）**：审批 allow/deny 主链路、拒绝理由回灌、deny 规则硬拦、plan mode
只读面 + exit_plan_mode 审批、acceptEdits（edit 自动放行/execute 仍审批）、hooks +
trust（未 trust 不跑、trust 后跑、exit 2 拦截 + stderr 回灌）、bypass 不跳 hooks、
WAITING_APPROVAL 期间 send 排队不唤醒后按序消费、**env 值 redaction 有效**、
**file 工具的 workspace 边界有效**、配置严格解析（拼错字段加载即报错、fail-closed）。

**持久化恢复（A5）**：**存储层扛住 3+ 次 kill -9 与多次 fork**——7 会话 journal 全部
JSON 合法、seq 严格连续、snapshot 全可解析；crash 矩阵三态（idle/在飞 bash/在飞 LLM）
自愈、in-doubt 三类处置精确（LLM 重发/执行不重跑）、fork 的 workspace 与 history 回滚
+ 原会话隔离三不变量成立、events/inspect 数字自洽、attach 补读→live 无缝。

**daemon 协议 + 观察面（A8）**：**14 种畸形输入注入 + 60 并发 + 200 快连关全部零 panic
零崩溃**、优雅停机 cooperative cancel + socket 清理、kill-9 后 stale socket 自动重建、
**notifier 跨重启去重零重复**、attach 多订阅、协议层 interrupt 实际可用（证伪 G12）、
40MB 超大 payload daemon 自保不崩。

**生态（A6）**：CLAUDE.md 注入生效（行为学 + 注入双证）、层级合并（外层先内层后）、
skills 目录注入 + 按需 read_file body、坏 skill 体面降级 + 清晰 warning。

**长会话/并发（A7）**：自动 compaction 触发且摘要准确、3 会话交错零串扰、6 会话快开快关
fd 精确回收无泄漏、compaction 前的记忆检查点保真。

---

## 四、方法论与可信度说明

- **四重交叉印证的非-bug**：测试中多次出现"daemon 静默死亡/会话卡 running/context
  canceled"。Agent 3、4、5、7 **各自独立**排查后一致确认：这是**测试 harness 缺陷**，
  非产品 bug——`nohup ar daemon &` 启动的 daemon 落在该次 Bash 调用的进程组内，调用
  超时（exit 143）时 Claude Code 的 Bash 工具对整个进程组发 SIGKILL 连带杀死 daemon。
  Agent 3 用双 fork+setsid 彻底 detach 后 daemon 稳定存活满 101 秒、spawn 0/3 崩溃，
  证伪。**真实用户用 `ar daemon` 常驻不踩此坑；`qa/lib.sh` 全程单进程也不踩。**
  报告已据此撤回若干误报——这提高了保留问题的可信度。
- **两个 critical 由汇总者亲自复现**（C2 bash 越界、C3 凭据泄漏），非单一 agent 孤证。
- **空-parts 死亡（C1）三重来源印证**：web UI 用户原始截图、Agent 1 拿到 journal 级根因
  链、Agent 7 在 4/4 马拉松会话复现并抓到 `output_tokens=1868 / parts=0` 的关键帧。

---

## 五、测试规模与 turn 统计

| 指标 | 数值 |
|---|---|
| 真实 agent loop（`generation_started`） | **578**（顶层 494 + 子 agent 84） |
| 含工具的 activity（`activity_completed`） | 835 |
| 顶层会话数 | 131 |
| 子 agent 会话数 | 38 |
| events.jsonl 文件数（去重后） | 169 |
| 功能域覆盖 | 8/8（SPEC A–J） |
| provider | 真实 Gemini `gemini-flash-latest`（anthropic 因无 key 而 BLOCKED） |

统计口径：跨所有测试 XDG 目录扫描 `*/agentrunner/sessions/*/events.jsonl`（含子 agent
深层目录），realpath 去重，`grep '"type":"generation_started"'`。

---

## 六、建议优先级

1. **先堵 C1（空-parts 死亡）**——这是唯一一个每个真实用户都会撞上、且导致数据不可
   恢复的主干裂缝。写入侧空-parts 保护 + 请求侧修补，二者都做。
2. **再堵 C2/C3（安全边界与凭据）**——bash 越界与凭据泄漏是安全红线，且 C3 与 SPEC 的
   ✅ 直接矛盾，需同步修 SPEC 或修实现。
3. **C4（fork 续跑）根因已定位**（一行 `Conversational=true`），修复成本低、收益是招牌
   功能复活。
4. **C5（daemon 路径）**——socket 与数据目录解耦，一劳永逸。
5. **C6 与 SPEC 校准**——要么接线 MCP，要么把 SPEC 的 ✅ 改成实际状态；避免"文档说有、
   代码没有"的信任裂缝。
6. **M1–M9 韧性缺口**——尤其 M1（crash 不自愈 + 状态撒谎）与 M2（审批不持久）会放大
   用户"卡住了/救不回来"的挫败感。
7. **QA 纪律升级**——引入"真实 LLM + 深轮次（20+）+ 生成型任务 + 长 XDG 路径 + 真 daemon
   重启"的 e2e 门，把本次盲区补进常规回归。

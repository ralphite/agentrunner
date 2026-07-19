# AgentRunner 全量功能清单（2026-07-19 盘点）

**这是什么**：对 origin/main（edd29e6）代码事实的地毯式功能盘点——四路并行
读码（CLI / 工具面 / Web UI / 内核）+ SPEC/JOURNEYS 交叉核对。分级组织，
叶子节点一句话。与 SPEC.md 的区别：SPEC 是验收登记簿，本文是"用户视角
能摸到的每一个功能"的展平清单，含 SPEC 未列的小能力与隐藏行为。

**审阅提示**：`⚠` = 盘点中注意到的可疑/易踩坑设计，供后续逐项 review。

---

## 1. 会话内核（session 生命周期与输入）

### 1.1 会话形态
- 静止模型：session 没有终态状态机，"结束"是从 journal 形状推导出的静止（quiescence），任何 session 随时可续。
- 续聊：agent 答完进入待命，同一 session 无限追问，上下文完整延续。
- close：`ar close` 只写一个"关闭标记"，任何显式 send / compact / clear 都会清标记复活会话。
- stop：`ar stop` 远程拆掉托管中的 run 并写"stopped 可复活"标记（区别于 interrupt 和 close）。
- interrupt：`ar interrupt` / Esc 只打断当前轮的活动（idle 时是 no-op），永不结束 session、不丢历史。
- kill 标记语义：被用户 kill 的会话/子 agent 记录来源，自动恢复路径永不越过标记，只有用户显式操作能复活。
- 可见截断：token/步数预算耗尽以可见截断收场，session 转 idle、补预算后可继续。
- 失败标记：provider 类干净失败记为可见可重启的 FailureMark，不会被误当成崩溃。
- 会话内换 agent：`ar agent` 运行中切换 spec（SpecChanged 事件），下条消息生效，session 不绑定 agent。
- session ID：64-bit 随机后缀、CLI 支持唯一前缀寻址、子会话按 `-sub-` 全 id 结构化寻址、路径遍历/symlink 逃逸被拒。
- 交付契约 outputs：spec 声明的产物（name/path/required）在会话收尾未显式 publish 时自动从 workspace 文件 publish，required 缺失把结局降级为 contract_violation。

### 1.2 消息投递
- 忙时排队：运行中发消息默认排队（queue），在安全边界按序消费，不丢不乱序。
- steer 投递：`ar send --steer` / webui 切换，把消息注入当前轮的下一个安全边界，本轮内即生效。
- 排队列表：`ar queue` 列出尚未消费的排队输入（含 command-id 与 revoked 态）。
- 排队撤回：`ar unqueue <sid> <cmd-id>` 撤回一条未消费的排队消息，迟到撤回是 no-op。
- retry：`ar retry` 把最后一条用户输入作为新 turn 幂等重发（附件从 CAS 读回）。
- stdin 管道：run/new/send/optimize 的文本参数可由管道 stdin 提供，`-` 显式占位，tty 下 `-` 报错不阻塞。
- 回复就地可见：new/send 默认跟随本轮渲染到 idle 再脱离，`--detach` 恢复异步，并发 send 各自只渲染自己那一轮。

### 1.3 多模态输入
- 图片输入：`ar send --image`（可重复、须 image/*、单文件 ≤5MiB），CAS ref 入 journal、组装时 inflate。
- 任意文件附件：`ar send --file` 附 PDF 等任意文件，sniff MIME，Gemini inline_data / Anthropic document block。
- 长贴折叠：>10KB 的粘贴文本自动转为 file part，不撑爆上下文。
- 语音听写：`ar dictate <audio>` 一次性 provider 调用转写音频（--context 消歧、--mime、--max-bytes），不建 session。
- Prompt 优化：`ar optimize "draft"` 一次性 LLM 改写草稿（--context 解析指代），不碰 daemon/journal。
- ⚠ `ar new` 开场消息不支持附件/折叠（与 send 不对称，显式记档推迟）。

### 1.4 上下文管理
- 自动 compaction：上下文超阈值自动触发摘要压缩，摘要空则拒绝落盘（不静默丢史）。
- 手动 compact：`ar compact <sid> [focus]` 立即压缩且可带保留指示。
- clear：`ar clear` 丢弃上下文前缀（journal 保留全量）。
- microcompact：不调 LLM，把久远可重算的 read-class 工具结果在装配视图渲染为占位符，先于 autocompact 生效。
- 结构化输出（客户端）：`ar new --json-schema <path>` 校验回复 JSON、失败重发纠正、`--json-schema-max-retries` 限次。
- 结构化输出（原生）：spec `output_schema` 走 Gemini 原生 response_schema（仅 tool-less 轮），Anthropic 显式降级。
- LLM 自动标题：托管 session 开局后异步蒸馏 3–6 词标题（auto 永不覆盖 manual/fork 标题，失败回退首行）。
- 记忆注入：CLAUDE.md 从 workspace 向上到 git root 层级合并，冻结进 session 前缀。
- 记忆写回：`ar remember` 追加到项目 CLAUDE.md 的 Remembered 段，并同时作为 program 输入让当前会话立即遵守。
- spec 调参面：`model.thinking{enabled,budget_tokens}`、`compact_at_tokens`、`microcompact_at_tokens`、`max_tokens` 都是 spec 作者可调项。

### 1.5 会话内自主形态
- in-session goal：`ar goal attach` 给正在聊的 session 挂目标，miss 时程序回灌反馈在**同一上下文**续跑（不起新 session）。
- goal 三种裁决：command verifier（每边界跑、唯一裁判）/ llm_judge（有完成声明才调、零声明零成本）/ 自证（goal_complete 声明被边界接受）。
- goal 控制面：pause / resume / update（可扩预算重武装 exhausted 目标）/ cancel / status，全部 journal 留痕。
- goal 预算：max-checks 检查预算（默认兜底 20）防空转，耗尽保留 goal 可 update 复活。
- in-session schedule：`ar schedule attach --every/--cron` 让会话周期性自唤醒跑 standing prompt（零 send、context 延续）。
- schedule 语义：漏 slot 折恰好一次 catch-up、busy 时记 skip 不打断、pause 不补偿、close 撤 timer 但 schedule 越标记存活、max-wakes 到期自动摘除。
- webhook 唤醒：`ar hook create` 铸造 per-session 的 HTTP ingress URL+token，外部事件经 `POST /hooks/<id>` 作为机器输入唤醒会话。

## 2. CLI 面（40 个子命令 + version/help）

### 2.1 运行与会话
- `ar run <spec> "prompt"`：前台一次性跑到终止（--workspace/--mode/--max-generation-steps/--json）。
- `ar new <spec> "msg"`：起 daemon 托管的对话 session 并跟随首轮（--detach/--json-schema/--mode/--workspace）。
- `ar send <sid> "msg"`：向 session 投消息（--image/--file/--steer/--detach）。
- `ar resume <sid>`：前台进程内恢复被打断/崩溃的 session（spec 从 journal 来，无需参数）。
- `ar submit <spec> "prompt"`：把一次性 run 或 --drive 系列交 daemon 托管，--idem 幂等键重连不重开。
- `ar drive <driver.yaml>`：前台跑 IterationDriver 系列，--retry 从旧 driver 会话新起同 spec 系列。
- `ar close / interrupt / stop`：三种停法（标记关闭 / 打断当前轮 / 拆托管 run）。
- `ar retry / queue / unqueue / answer`：重发最后输入 / 列排队 / 撤回排队 / 回答结构化提问（`q:n`、`q:1,3`、`q:text=`、--skip）。
- `ar compact / clear / remember / mode / agent`：压缩 / 清空 / 写记忆 / 切权限模式 / 换 agent spec。
- `ar goal <sid> attach|update|status|pause|resume|cancel`：目标管理（--verify/--verify-llm/--max-checks）。
- `ar schedule <sid> attach|status|pause|resume|cancel`：周期唤醒管理（--every/--cron/--max-wakes）。

### 2.2 观察与控制
- `ar sessions [list]`：列全部 session 及状态形状（--json 带 workspace/title/kind/schedule，--limit/--offset 分页）。
- `ar inspect <sid>`：全量事实报告——timeline、每次调用裁决、usage、stats、goal、progress、artifacts、子树递归（--json）。
- `ar events <sid>`：dump 原始事件日志或 --state 的 folded state（--json）。
- `ar ps <sid>`：列在飞后台工作（纯 journal 读，无需 daemon）。
- `ar attach <sid>`：补读全部历史再实时跟随，Ctrl-C 脱离（--json/--replay-only，隐藏别名 --no-follow）。
- `ar approve <sid> <id> approve|deny [reason]`：应答待批审批，--always 同时把 allow 规则写回用户配置。
- `ar kill <sid> <handle>`：取消一个后台 handle（子 agent 或后台 bash）。
- `ar diff <sid>`：显示自最近人类轮以来的工作区改动（--scope last-turn/--json）。
- `ar artifacts <sid> list|read <stream>[@vN]`：列/读已发布 artifacts（版本寻址、--json）。

### 2.3 环境与基建
- `ar daemon`：起常驻 runtime（--detach 后台化、--http 开 webhook ingress、重复启动幂等）。
- `ar init [path]`：写带注释的示例 spec（--driver 出 driver 模板），拒绝覆盖。
- `ar trust <dir>`：把工作区标记为本机可信（project hooks/规则/tools 才生效）。
- `ar doctor`：预检 OS 沙箱后端（bubblewrap/Seatbelt），network all/none 双档探针，失败非零退出并给修复指引。
- `ar hook create|list|revoke`：管理 webhook ingress（token 只显示一次、仅存哈希）。
- `ar dictate / ar optimize`：语音转写 / 草稿改写（一次性 provider 调用）。
- `ar fork <sid> <barrier>` / `ar barrier <sid>`：从 barrier 分叉新 session（--list 列 barriers、--workspace 指定 worktree）/ 手动打 barrier 点。
- `ar version / help`：版本 / 帮助（`help <cmd>` 转发到各命令专用帮助）。
- `ar record-fixture / ar accept`：开发者命令——录制真实 provider 交互为 fixture / 跑验收场景出 JSON 报告。

### 2.4 CLI 横切行为
- flag 重排：已定义 flag 可后置于位置参数（`send sid "msg" --image x.png`），`--` 终止扫描（2026-07-19 PLAN 5.4 起 inspect/events/sessions 同一纪律,手写分拣已除）。
- 退出码约定：0 完成 / 1 运行失败 / 2 用法或 spec 错误；`-h/--help` 算成功退出 0。
- .env 自动加载：run/drive/daemon/resume/dictate/optimize 从 cwd（部分还从 workspace 根）读 .env 补缺失环境变量，从不覆盖。
- 信号语义：前台第一次 Ctrl-C = steering interrupt，第二次或 SIGTERM = 硬取消；daemon SIGTERM = 优雅停机（loop driver 无终态、boot sweep 复活）。
- stuckHint：close/kill/mode 等失败时探测 session 状态并指出出路（resume 后再操作等）。
- daemon 缺失统一提示 `agentrunner daemon --detach` 启动指引。
- 隐藏开关：`AGENTRUNNER_DEBUG=1` 开 debug 日志；`AGENTRUNNER_SCRIPTED_FIXTURE` 指定 scripted provider 的 fixture。
- socket 回退：数据目录路径过长时 unix socket 自动落到 `$TMPDIR/ar-<hash>.sock`。
- ~~sessions 手写解析器~~（2026-07-19 PLAN 5.4 迁 flag 包+parseFlags）。
- ~~run -o 静默忽略~~（2026-07-19 PLAN 5.4 改显式报错并指路 record-fixture）。
- flag 补遗：submit 另有 --workspace/--mode/--json；drive 另有 --workspace/--json；dictate/optimize 各有 --model/--provider；queue --json；retry --detach；hook create --name、hook list 可按 session 过滤；accept --stage/--plain/--report。
- ~~goal --max-checks help 写 10 实际 20~~（2026-07-19 PLAN 5.4 文案改 20）。

## 3. 模型工具面（26 个内置工具）

### 3.1 文件读写
- read_file：读 workspace 文件，offset/limit 分页，默认 2000 行 / 50KB 截断并给续读提示。
- read_file 读媒体：按内容 sniff 识别图片/PDF，字节入 CAS、模型收到真实像素/文档 part（5MB 上限）。
- write_file：整文件创建或覆盖（连父目录一起建），返回 lines_added/removed 行统计。
- edit_file：精确唯一字符串替换（replace_all 可多处；空 old + 不存在路径 = 建新文件），同样返回 lines_added/removed。
- edit_file 隐藏别名：接受 Claude Code 习惯的 `old_string`/`new_string`/`all` 字段，防静默误创建。

### 3.2 执行
- bash：workspace 根跑 shell，强制 OS 沙箱（沙箱不可用 fail-closed 拒跑），输出 30KB 头尾截断。
- bash 后台：`background=true` 立即拿 handle，结果以消息回流；运行中可用 output 工具看有界 tail。
- bash notify 门：后台任务 `notify: always|on_fail|none` 控制结果是否回流（none = fire-and-forget）。
- bash 取消：SIGTERM→宽限→SIGKILL 按进程组杀，以进程组真实消失为取消终态。
- bash 凭据回报：被沙箱扣留的凭据 env 变量名列在结果 `credential_env_withheld` 里，让失败可解释。
- output：按 handle 查后台工作进度（含实时 output_tail）。
- kill：按 handle 取消后台工作或子 agent。

### 3.3 搜索
- grep：RE2 正则搜内容，支持 path 子目录限定 / case_insensitive / glob 过滤 / output_mode(content|files_with_matches|count) / -A/-B/-C 上下文 / multiline 跨行 / max_results（默认 100、上限 200）。
- glob：按 glob 模式列文件（`**` 跨目录且可匹配零段，path 可限定子目录），上限 1000 条。
- keyword_search（原 semantic_search,2026-07-19 PLAN 5.2 如实改名）：BM25 词法相关性搜索（identifier-aware 分词），惰性建全树共享内存索引（max_results 默认 8、上限 20）。
- 搜索横切：三者共享凭据文件/vendored 树排除表，snippet 全过 redaction。

### 3.4 网络
- web_fetch：抓 URL 转可读文本（HTML→text、512KB wire / 50KB 出、最多 5 跳 redirect）。
- web_fetch 安全：每跳拒绝 link-local/cloud-metadata IP；内容包 BEGIN/END untrusted 定界符防注入；network 收容下 fail-closed。
- ⚠ web search 没有实现（GAPS G18）。

### 3.5 编排与协作
- spawn_agent：非阻塞派生子 agent 拿 handle（目录 agent 名或 inline role 二选一）。
- spawn_agent 参数：inputs 把 artifact 落进子 workspace、replaces 显式退休前任（~~depends_on~~ 2026-07-19 PLAN 5.3 砍除）。
- handoff_agent：交棒给另一 agent 并结束本 run（承接权限与剩余预算）。
- send_message：树内 durable 消息（发 parent / 兄弟 / 自己的 handle），空闲的收件方被唤醒。
- publish_note / read_notes：共享 blackboard 按 topic 发/按序读便签（跨父子可见）。
- publish_artifact：发布版本化 artifact（同 stream 累积版本，CAS ref）。
- artifacts_list / artifacts_read：列/读本 session 已发布 artifacts（分页、整数 version 参数寻址历史版本——`@vN` 语法仅 CLI 有、二进制只回元数据）。

### 3.6 交互与控制流
- ask_user：wait-class 提问 park 会话等回答（自由文本 / 2–4 选项 / multi_select / allow_free_text 选项外兼收自由文本 / 结构化 questions[] ≤4 问）。
- exit_plan_mode：提交 plan 摘要请求离开只读规划模式（需用户批准）。
- progress_update：整表替换会话进度 checklist（≤50 条，pending/running/done/failed），inspect 与 webui 消费。
- goal_complete / goal_status：声明目标达成（边界裁决、verifier 优先于声明）/ 查询当前 goal 状态。
- schedule_next / finish_series：loop 自定步调声明下一次延迟 / 声明系列完成（人审把关）。
- skill：按名加载 skill 正文（去 frontmatter、拒路径穿越）；context:fork 的 skill 变一次性子 agent。
- ⚠ finish 工具显式裁掉（待命本身就是待命）。

### 3.7 工具横切
- 审批分类：每个工具定义 read/edit/execute/wait class，供权限模式与 in-doubt 处置。
- 未广告工具防御：spec 未列的工具在 dispatch 前被拒（defense-in-depth）。
- 全部工具输出有 per-tool 上限截断且带 truncated 标记。
- 工具错误渲染为模型可见的 error result，loop 继续不中断。

## 4. 子 agent 编排

- 后台 spawn：全部非阻塞（阻塞路径已删除），子跑在 goroutine、结果作消息回灌父。
- 静止回执：子静止时父 turn 被激活处理回执（先回先处理、可多次），receipts 投递模式 steer/turn_end 可配。
- 杀子 agent 的全部路径：`kill` 工具 / `ar kill` CLI / spawn `replaces` / webui Background 区 kill 按钮 / 父被 stop 级联 / interrupt steer 后模型主动杀。
- kill 来源标记：用户 kill 的子只有用户可复活，parent kill 的父可复活。
- 树预算：子预算 = min(父剩余公平份额, 子 cap)，reserve-then-settle 防并发超卖，整树总花费受控。
- 深度/扇出上限：默认 spawn 深度 ≤2、单会话 spawn ≤8，超限是模型可见的 DENY 非崩溃。
- 权限只窄不宽：子权限 = 父与子规则交集、mode 取更窄，冻结于 spawn 时刻。
- escalate 提权：子申请超父权限强制走用户审批，批准仅替换 permission 层（floor/gate 保留），拒绝降级交集。
- 动态角色：`agents_dynamic` 开启后模型可 inline 起草角色（name/instructions/tools/permissions），不得声明 hooks/MCP/预算。
- 内置 agent 库：explore/plan 只读 agent 随发行 embed，spec 列名即可 spawn，内置优先同名文件。
- workspace 隔离：子默认拿 spawn 时快照物化的独立 worktree（注入隔离须知），显式 shared 才共享目录。
- durable delegation：delegation-id 跨 revive 复用、workspace assignment、settlement 状态（~~DAG/lease~~ 2026-07-19 PLAN 5.3 砍除:零消费）。
- 静止子唤醒：send_message 唤醒静止的子，同 journal 同 context 延续，绝不另起炉灶。
- 用户直达子：`ar send <child-sid>` / webui 点进成员直接指挥任一子 agent。
- 子进度镜像：成员事件带标签入树根 hub，CLI attach 可过滤、webui 子会话实时 SSE。
- 子审批路由：子的审批上浮到根宿主，crash 后重挂接（等审批的子不重放工作）。
- handoff：一 turn 只允许转控一次，成功后本 agent 停止行动。
- 父崩溃结算：从子 fold 结算（子已静止交付真实回执，子随进程死记 crash cancellation 带真实花费）。

## 5. 权限与安全

### 5.1 规则与模式
- permission rules：tool/path/command/network 四维规则，user > project > spec 拼接，first-match 裁决。
- modes：default / plan（只读+计划审批）/ acceptEdits（edit 自动放行）/ bypass（跳权限不跳 hooks），未知类 fail-closed。
- mode 运行中切换：`ar mode` / webui pill 点击在安全边界切 default↔acceptEdits（plan/bypass 仅启动期）。
- plan mode：只读工具面 + exit_plan_mode 审批跃迁，拒绝则留在 plan。
- bash 逐段匹配：复合命令按顶层分隔符拆段、逐段取最严裁决（一段 allow 不放行整条），wrapper（timeout/env 等）剥离后再匹配。
- 只读命令集：内置只读 bash 命令免提示放行，但显式 deny 规则仍然胜出。
- protected paths：acceptEdits 下写 .git/.claude/rc 等敏感路径仍要审批（只上收自动放行、不 deny）。
- hard floor：workspace 逃逸、plan 模式写/执行、凭据文件读——先于一切规则与 mode（bypass 也不可越）。

### 5.2 审批
- 审批流：ask → WAITING_APPROVAL 挂起 → 批准/拒绝，拒绝理由回灌模型可见。
- 常设审批：同 session 内"允许且不再问"对精确同判据的后续 ask 自动作答。
- 规则写回：`--always` / webui Always allow 把精确 allow 规则写回 user 配置，下个 session 生效。
- 远程审批：daemon 托管的审批可从 CLI/webui 异地应答，crash 后审批可复活。
- ⚠ WAITING_APPROVAL 挂起期间来消息只排队不唤醒（G3 余项）。

### 5.3 hooks
- pre/post tool hooks：pre exit 2 阻塞（stderr 作为模型可见理由），post stdout 成 activity note，超时按进程组收割。
- lifecycle hooks 8 事件：session_start/end、user_prompt_submit（可阻塞）、stop、subagent_start/stop、pre_compact（可阻塞）、post_compact。
- hooks 信任门：project 层 hooks 必须 `ar trust` 后才会运行；hooks 不重放。

### 5.4 沙箱与凭据
- OS 沙箱强制：bash/verifier/command-tool 必须在 Seatbelt（macOS）/ Bubblewrap（Linux）内跑，能力缺失 fail-closed。
- 文件系统收容：writable 仅 workspace 根 + 隔离 TMP + git 元数据，HOME/TMP/XDG 隔离，凭据路径显式 deny。
- network 棘轮：任一 spec 声明 network=none 即全树收容且不可回退，bash 进 loopback-only netns，web_fetch fail-closed。
- 凭据 redaction：`*_API_KEY/_TOKEN/_SECRET` 值（≥8 字符且非占位串）在 journal/fixture/模型可见面替换为 `[REDACTED:VAR]`。
- 凭据硬排除表：.env*/.pem/.ssh/.aws 等永不进快照、不进索引、read_file 硬拒——三处排除表 lockstep。
- 凭据 env 剥离：沙箱/hook 子进程默认扣留凭据变量，root spec `sandbox.env_passthrough` 按名放行（首封 seal、子永不放宽）。
- 信任模型：未 trust 的 workspace 其 project settings 收紧为 ask、hooks/command-tools 不生效。
- 注入对抗：web_fetch 定界符 + machine/hook 输入强制 untrusted 框定 + trust 钳制 + 不做宏展开。

## 6. 持久化与恢复

- journal 纯 fold：state = fold(events)，Apply 纯函数不读墙钟，未知事件类型响亮报错。
- snapshot-resume：fold 快照 + 索引游标只读真尾，offset/hash 校验，索引可重建。
- schema 兼容：子状态命名空间独立版本号，旧 snapshot 缺新投影自动全量 fold，破坏性冲突拒绝且不改源。
- durable CommandLog：send/close/kill 等全部命令 caller-minted command-id 幂等、fsync 后 ack、跨 restart 自动重放。
- blob-before-event：附件/artifact 字节先入 CAS 再写事件，崩溃只留孤儿 blob 不留悬挂 ref。
- genesis 守卫：无合法创世事件的 journal 不可 resolve/list/send/resume。
- crash 恢复：resume 单一自愈——execute 类效果绝不重跑、渲染 interrupted-by-crash，只读类可重跑，session 继续。
- boot sweep 四路：daemon 启动自动接续 mid-turn stranded 会话、重挂 loop drive 并补漏 slot、复活有 pending 命令的会话、按 pgid 清扫孤儿 bash 进程。
- 显式重开 vs 自动路径：send 对任何 session 成立（含带标记的），自动路径永不越 close/kill 标记。
- shadow repo 并发 flock：同 GIT_DIR 的快照操作跨进程单写，diff 用私有 index 并发只读。
- crash 注入 harness：`AGENTRUNNER_CRASH=after:<EventType>:<n>` 与 `point:<name>[:<n>]` 两种注入形式，供崩溃矩阵测试。

## 7. Workspace 与时间旅行

- workspace 边界：所有路径经 realpath 归一，`..`/symlink 逃逸即拒。
- shadow repo 快照：独立 GIT_DIR 对用户 repo 和 agent 双向不可见，相同树去重，无 git 时优雅降级。
- worktree 物化：fork/best-of-N/isolated child 各得 `git archive` 原子解包的独立 worktree。
- CheckpointBarrier：安全边界/turn 收尾自动 + `ar barrier` 手动打点，记跨流向量 + 快照 ref + 在飞工作处置。
- fork：`ar fork` 从 barrier 切分复制成新 session（单创世、随行 CAS verbatim 复制、独立 worktree、in-flight 按 cancel_at_fork 落实）。
- rewind：fork 后显式切换到新分支继续（旧分支保留）。
- diff：基于 barrier 快照对比的只读评审面（working-tree / last-turn 两个 scope）。
- webui scratch workspace：不选项目时自动创建一次性 scratch 工作区目录，侧栏把这类会话归组显示为 "Scratch"。
- ⚠ Claude Code 式 scratchpad 辅助目录（workspace 外草稿区）没有实现，仅归档评审文档列为对标空白。
- ⚠ best-of-N 胜者晋升（自动 apply diff/fork 接管）没有实现，v0 靠用户手动（G15）。
- ⚠ 多根 workspace（--add-dir 类）没有实现（G17）。

## 8. 驱动形态（driver）

- driver-goal：批式 headless 目标驱动——每轮 fresh child run + verifier 三态打分 + 停滞检测（patience）+ carry 传递。
- driver-loop：interval 固定节奏 / cron / self_paced 三种 cadence，durable absolute tick，跑到 max_iterations/预算/取消。
- best-of-N：N 个隔离 worktree 从同一 base 快照并行尝试、各自树内评分、pass 优先选优、败者留档。
- verifier 四态：command（exit 0 = pass，另可配 metric_regex 捕获组 + threshold 变打分制）/ llm_judge（rubric 严格评分）/ human（走 ask 路径）/ 自证，聚合取最弱。
- overlap 策略：撞 tick 按 skip（留痕跳过）或 coalesce（折成一次 catch-up）处置。
- 失败处置：on_child_failure 三态 stop（默认，结束系列）/ surface（算作 spent iteration 继续）/ retry（独立子库立即重试、无 backoff），重试花费计入预算。
- cron 跨重启：daemon 崩溃/优雅停机都不写终态，boot sweep 重挂并按 overlap 策略恰好补跑一次漏 slot。
- series memory：迭代结论文件注入下一轮 prompt（8KiB 注入时截断）。
- Scheduled Retry：从旧 DriverStarted 的 spec 新建 series，绝不向旧 journal 注入消息。
- ⚠ driver 子执行与递归 session 的收敛还没完成（driver 形态与 in-session 形态并存，E1 进行中）。
- ⚠ overlap:interrupt 策略显式推迟。

## 9. 生态接入

- MCP 连接：stdio 与 streamable HTTP 两种 transport，工具名 `mcp__<server>__<tool>` 命名空间。
- MCP 能力：resources/prompts 协议工具自动追加、structured/multimodal result 保真、list_changed 感知、断连自动重连恢复。
- MCP 安全：secret 只按 env 名引用（不进 spec/journal）、allowed_tools 白名单、写操作走通用审批流、ReadOnlyHint 只影响权限面。
- ⚠ MCP 交互式 OAuth 登录 / refresh-token 持久化显式裁掉（runtime 不持久化 secret）。
- skills：`.claude/skills/<name>/SKILL.md` 只注入目录行（body 按需加载），context:fork 的 skill 展开为一次性子 agent。
- 自定义 slash 命令：`.claude/commands/*.md` 的 `/name` 在 new/send 两路宏展开进 journal。
- 自定义 command tools：JSON manifest（name/command/timeout/params）把本地命令包装成模型工具，args 走 stdin JSON、固定命令过全权限管线 + OS 沙箱。
- command tools 分层：user 层恒载、project 层需 trust、撞内置拒载、user 压 project。
- notifier：生命周期时刻（run 结束/审批/迭代）经用户配置的 shell 命令投递通知，journal-before-send 跨重启去重。

## 10. Daemon 与远程面

- daemon 托管：owner-only 0600 unix socket，run idem-key 与 command-id 双幂等，优雅停机。
- daemon 后台化：`--detach` re-exec + setsid，stdio 落 daemon.log，父进程等 socket 起来才返回。
- webhook ingress：`--http` 开 `POST /hooks/<id>`——bearer token 常量时间比对、未鉴权限流 429、body 上限 413、无存在性 oracle、机器输入不能复活带标记会话（410）、X-Command-Id 幂等重投。
- attach 直播：先补读 journal 历史再订阅实时，成员事件折叠为一行 announcement，成员审批仍上浮。
- 远程审批/远程 stop：daemon approve/stop 通道，approve 可跨 crash 复活审批。
- wire 协议命令面：ping/run/drive/attach/approve/send/close/interrupt/stop/compact/clear/remember/kill/agent/mode/unqueue/answer/goal-*/schedule-*。

## 11. Web UI

### 11.1 信息架构与侧栏
- Projects → sessions 分组：按 workspace 分组、折叠态双写（localStorage + 服务端 overlay）、Scratch 归组、Pinned 独立区、另有无 workspace 会话的扁平 Sessions 区。
- 会话行：状态点（未读/运行/审批/搁浅/崩溃）、hover 预览卡（项目/分支/状态）、pin/archive 快捷钮。
- 会话行菜单：Pin / Rename / Mark read / Archive / Copy session ID / Copy link。
- Project 组菜单：Open in VS Code/Finder/Terminal、Rename project、Copy path、Mark all read、Archive all。
- 大历史渐进加载：首 40 条立即可操作、后台 80/页补齐、refresh 不重入。
- 归档：Show/Hide archived 切换 + Settings 里按项目浏览归档会话。
- 底部 daemon 徽标：连接状态三态显示，离线可点击重启 daemon。

### 11.2 New session（composer 首页）
- 落地页：项目感知标题 + 4 张 suggestion 卡（Explore/Build/Review/Fix）预填草稿。
- Project chip：搜索历史工作区、最近 5 个、New project（scratch 或已有目录）、不选项目。
- Start-in chip：Local 或 New worktree（尊重所选 ref）；Branch chip 可搜索、local 模式真实 checkout。
- Access pill：Full access / Ask to approve / Auto-accept edits / Plan，记忆上次选择。
- Model pill：模型（4 个 Gemini + Claude Sonnet 5）/ Effort 五档 / Speed 子菜单 / 自定义 model id / thinking budget 覆盖。
- `+` 菜单：附件、Goal、Plan mode、Automation（Loop/Best-of-N/Background run/Agent persona 五选 + YAML 编辑）。
- Goal/Loop/Best-of-N launcher：prompt + 验证命令/interval（内联 cadence 校验）+ 轮数/尝试数。
- ~~Environment chip / Plugins 组占位~~（2026-07-19 PLAN 5.1 已移除）。

### 11.3 Composer 交互
- 附件：文件选择、粘贴图片、拖拽（前端 10MB 本地拦截不上传，服务端 413 兜底不留半文件）、缩略图管理。
- @-mention：`@query` 检索工作区文件名插入引用。
- 语音听写：服务端 ar dictate 优先、浏览器 SpeechRecognition 回退。
- Optimize：Sparkles 按钮 LLM 改写草稿，单步 undo 保留原稿。
- 运行中投递切换：Queue|Steer 切换 + ⌘⏎ 按相反模式发送一次。
- 草稿按 session 持久化；Enter 发送 / Shift+Enter 换行；空输入时 Send 变 Stop。
- slash 命令 14 个：/goal /loop /bestof /optimize /plan /compact /clear /mode /diff /fork /model /reasoning /interrupt /resume。

### 11.4 Timeline 渲染
- Markdown：GFM 表格/删除线/任务列表、raw HTML 强制转义（无注入面）、代码块语法高亮 + Wrap/Copy、mermaid 围栏懒加载渲染成图。
- 用户消息：>10 渲染行折叠 Show more/less、附件缩略图、Sent as goal 标记、来源 peer 链接。
- Worked fold：完成的 turn 折叠为 "Worked for N"，内部连续工具调用聚合（Ran commands ×3）。
- 工具卡 per-tool 结构化视图：bash Shell 块、read/write/edit MiniDiff、grep/glob、spawn_agent 子会话链接、web_fetch untrusted 标记、未知工具回退 JSON。
- 内联图片：工作区相对路径解析渲染、点击 Lightbox 全屏翻页。
- 轮辅助行：Copy、Share 深链、Continue in new session、Worked duration、goal 判决。
- 系统事件：program/control 输入默认藏进 system events 开发者视图，绝不冒充用户消息。
- pending 气泡（queued/steering）、typing 指示、jump-to-bottom、compaction 分隔线。

### 11.5 审批与提问
- 内联审批卡：人类摘要 + Details 折叠原始 args/gates + Approve once / Always allow / Deny（可附理由），⌘↵/⌘⌫ 快捷键。
- AskForm：结构化 ask_user 渲染成单/多选 + free-text 表单，Submit/--skip。
- queued 消息列表：逐条 Withdraw 撤回按钮。

### 11.6 Changes / git 操作
- Changes 面板：Working tree / Last turn 双 scope（缺 durable 基线时如实报 unavailable）、逐文件 diff、inline/split 切换、hunk 折叠与上下文展开、文件树跳转、generated 文件计数隐藏。
- Changes 出口卡：Edited N files 摘要、Undo（确认后 revert 回 HEAD）、Review 跳转、单文件定位。
- git 操作：Commit / Commit & push / Push（结构化错误分类）、git init 使未初始化目录可追踪、单文件或整树 revert。
- worktree 一等公民：New worktree 落共享数据根、Apply to project（git 原生 clean-or-nothing 干跑通过才落）、Remove worktree（脏树二次确认）。

### 11.7 Supervision（环境栏）
- Environment 区：Changes 概览、Worktree 行（路径/Apply/Open/Remove）、Create branch、Commit or push。
- Goal 区（编辑/暂停/恢复/取消 + 实时判决）、Progress checklist（N/M）、Background work（逐条 kill）、Artifacts 查看器、Agents 子树（递归、状态点、点开成员完整时间线）、Attention 汇总（审批 + recovery）。
- Run details：inspect 全量投影（usage/billed、per-tool 统计、provider capabilities、raw JSON）。

### 11.8 会话操作与导航
- 顶栏：返回父会话、Stop/Resume/Retry、Fork from checkpoint、Environment 开关。
- `…` 菜单：Pin/Rename/Archive/Copy link/View 切换/Create checkpoint/Continue in new session/Switch agent/Close session。
- 失败 banner（Technical details + Retry）、terminal 提示、GoalBanner 实时时钟。
- deep link：`#<sid>` / `#run:<id>` / `#scheduled` hash 路由，重启后同链接直达；非法 sid 立即 Not found。
- FindBar：⌘F 会话内查找接管浏览器搜索（↑/↓、Enter/⇧Enter 匹配导航 + N/M 计数）。

### 11.9 Scheduled 与后台 runs
- Scheduled 页：全部 driver/schedule 会话列表——cadence 人话（Every 30m / Saturdays at 4:00 AM）+ next run 推算、All/Active/Finished 过滤、搜索、行级操作菜单。
- Create 菜单：One-time run / Goal / Repeating / Best of N 四种建法 + 三张 suggestion 卡。
- RunModal：submit 或 drive 的全参数表单（interval/cron/best-of-N、YAML 高级编辑、内联校验）。
- RunView：后台 run 的 SSE 日志流、iteration 分隔、终局判决、Stop。

### 11.10 全局设施
- Settings：General（daemon 状态 + 重置默认）、Appearance（主题/字号/对比度/diff 标记/动效/语法高亮）、快捷键表、Git 模板、Worktrees 清单、Configuration、Archived。
- Command Palette（⌘K）：会话模糊搜索 + 命令（新建 session/New run/Scheduled/Settings/Trust/切主题/Toggle archived 等），⌘1–9 跳会话。
- 快捷键：⌘⌥N 新会话、⌘B 侧栏、⌘,、⌘F、⌘⌥↑/↓ 切会话、? 帮助。
- 系统 launcher：Open in VS Code/Finder/Terminal（app 白名单 + workspace 白名单双门禁、不过 shell）。
- 主题 system/light/dark、移动端抽屉/scrim/键盘避让、桌面通知、未读数入 title、Toast/ErrorBoundary。
- 健康面：/api/health 报版本一致性/daemon 状态/sandbox 后端，DaemonAlert 离线红条一键重启。
- ~~Settings Branch prefix / PR merge method（Not wired 摆设）~~（2026-07-19 PLAN 5.1 已移除）。
- ⚠ webui 的 schedule 投影是 internal/driver cadence 逻辑的手工 stdlib 镜像（双实现漂移风险，设计上有意为之）。

## 12. 安装分发与运维

- 一行安装：`curl … install.sh | sh` 多平台探测（linux x86_64/arm64、macOS 两 arch）、sha256 校验、版本化目录 + symlink 切换（永不原地覆盖）。
- 私有 repo 路径：GITHUB_TOKEN 下载支持；损坏下载硬失败不动既有安装。
- 沙箱依赖交付：install.sh Linux 自动装 bubblewrap + AppArmor userns 放开 + 真实 probe 验证（AR_SKIP_SANDBOX_DEPS/AR_REQUIRE_SANDBOX 开关）。
- release CI：打 v* tag → 单 runner 交叉编译 4 target → 真产物 smoke → 发布稳定命名资产。
- CI 预置环境：GitHub Actions 上配好 API keys，qa-blackbox（真浏览器黑盒）/ phone-webui（Tailscale 手机远程驾驶）/ qa-all / sandbox-doctor / remote-qa-env 等 workflow 可直接 dispatch。
- arwebui：优先用兄弟目录的 ar 二进制（避 PATH 同名冲突），`-ar/-addr` 启动参数。

## 13. 开发者与测试基建

- scripted provider + record-fixture：录真实 provider 流量为 fixture（凭据红化）、按序回放并逐步断言请求漂移。
- accept harness：嵌入式 YAML 验收场景（s1–s7）带独立 scratch dir、TUI/plain 渲染、JSON 报告。
- crash 注入：命名注入点环境变量，支撑崩溃矩阵测试。
- 确定性并发测试：routing provider 支撑并发子 agent 场景孪生。
- `./scripts/check.sh`：一步全绿标准（含 lint-docs/lint-wiring/deadcode 基线等）。
- qa/ 脚本库：40+ 个 run-qaXX.sh 真实 API 验收场景，结果归档 qa/runs/。

## 14. 显式未实现 / 裁掉（防止误当遗漏）

- web search 工具（G18，搜索后端未选型）。
- 屡崩升级策略（同因连续 crash 的 retry{max,backoff} 升级为失败回执）——UJ-21 愿景未落地，kernel crash 只标 dead 不自动重启（GAPS G22②）。
- HTTP/WS 全 API 壳（仅 webhook ingress 单端点已做）。
- 云 workspace 生命周期 / IDE 集成（G11 裁掉待重启）。
- 多根 workspace（G17）。
- best-of-N 胜者自动晋升（G15，手动）。
- scratchpad 辅助草稿目录（对标空白，零实现）。
- MCP 交互式 OAuth / refresh token 持久化。
- finish 工具、overlap:interrupt、`ar new` 开场附件（均显式记档）。

# INC-5 核心工具面补全:web_fetch / ask_user

状态:执行中(2026-07-09 起)。
来源:用户指令「impl them, make them reusable」——把"自己执行、装得进
数据模型"的缺口工具补齐,并把补法做成可复用的模式(数据位 + 复用既有
轨道,而不是特例代码)。

**scope 修订(2026-07-09)**:初稿含 grep/glob;成文当天发现并行增量
INC-3(2855404)已将其落地(含 index 谓词导出、QA-11)。本 INC 收窄为
**web_fetch + ask_user**,并承接 CODEX-PARITY §3 排出的「web fetch /
search——G18b,设计优先,先 DESIGN 增量」:本工作纸即该 DESIGN 增量。

## 动机与 journey 锚

| 工具 | journey 锚 | GAPS 锚 |
|---|---|---|
| web_fetch | UJ-01(即问即答需要外部文档佐证时;本 INC 给 UJ-01 步骤 2 补一句可选步) | G18b(未 spec,牵动 network 与注入面) |
| ask_user | UJ-06「agent 主动提问(wait-class)」 | G20(设计已定,无工具定义、无应答路径) |

**明确不做**(理由记档):
- `web_search`:客户端版需要外部搜索后端(API key / 引擎选型),
  provider 服务端版落在「provider 执行类工具不过 L2」的例外类别
  (CAPABILITY-REVIEW 已记档)——两条路都不是"加个 def"量级,G18b 中
  search 部分维持开放,单独成增量。
- `finish`:§17 记档维持不动(待命本身就是待命,增量价值待真实反馈)。
  本 INC 只解冻 ask_user——它有 UJ-06 直接压着,且 wait-class 语义
  DESIGN §5 已写明,不属"预做"。
- pipeline `globMatch`(规则)与 exec `globToRegexp`(工具)双 glob
  实现暂不合并:语义各自锚定(规则侧 `(?is)` 封大小写/换行逃逸,
  工具侧面向文件定位),硬统一反而耦合两个演化方向;记档观察。

## Spec delta(SPEC.md C·工具面)

| 功能点 | 旧状态 | 新状态 | 验收锚 |
|---|---|---|---|
| web fetch | ❌ 未 spec(G18b) | ✅(web_fetch,客户端执行) | 单测 + 规则匹配测试 · QA-12 |
| web search | ❌(与 fetch 同行) | ❌(拆出单列,维持开放) | — |
| ask_user(wait-class 提问) | 🧊 G20 | ✅(finish 维持记档) | scripted 孪生(park/answer/interrupt/resume)· QA-12 |

D·权限与安全补一行:network 规则覆盖"带网 read-class 工具"(web_fetch
恒带 `all`、收容棘轮下 fail closed)——见 Design delta。

## Design delta

### 1. tool 定义新增 `network` 数据位(§5「tool 定义本身是数据」的扩展)

`Def` 增可选字段 `network: "all"`。语义:该工具**在进程内产生网络
出口**。`networkScope` 从"只认 execute class"改为数据驱动:

- def.network 非空且未收容 → effect 带 `all`(network 规则可匹配);
- 收容棘轮生效 → executor 对该工具 **fail closed**(in-process fetch
  无法被 netns 包住,拒跑而非静默出网)——与 bash 的 fail-closed、
  MCP 的"恒记 all(边界诚实)"同一族纪律。

**不变量对照**:§5 network 资源类粗体条款说"rules 的 `network` 模式
匹配 effect 的出口范围——未受限的 execute effect 带 `all`"。本 delta
是**覆盖面扩展**(多一类带网工具如实携带出口范围),不反转任何语义;
与 MCP 条目("恒记 all")同性质。措辞随实现同 commit 修订,不走
不变量变更流程(没有旧保证被削弱;边界诚实反而更完整)。

### 2. web_fetch(read class + network 数据位)

- http/https only(逐跳校验),重定向上限 5,响应上限 ~512KB 读入、
  提取正文后按工具输出截断;text/html 做轻量 HTML→text(去
  script/style、标签折行、实体解码),text/* 与 JSON 原样,其余
  content-type 报模型可见错误。
- 结果 payload 带 `untrusted_content` 标记字段+提示行(注入面第一道
  软防线;威胁模型成文仍挂 G16,不在本 INC)。
- class=read:default/plan mode 均放行(与"研究先行"一致);要收紧的
  spec 写 `{tool: web_fetch, action: ask}` 或 `{network: "*", ...}`
  规则——本 INC 附规则匹配测试证明这两种写法都真的拦得住。
- 本地/私网地址**不**默认封禁(单机 dev 工具,fetch localhost 是正当
  用例);云形态的 SSRF 策略挂 G11 展开,不预做。
- 无 durable timer 变化:read class 无 wall-clock(与现状一致),
  HTTP client 自带 30s 超时兜底。

### 3. ask_user(wait-class,落实 DESIGN §5 原文)

§5 已定语义:「wait-class 即"向用户提问"类工具,execute = 进入
`WAITING_INPUT` 待命而非阻塞 activity,跨崩溃不被 in-doubt 误杀」、
「向用户提问=待命等 inbox 里的 user_message」。落地形状:

- **park**:doTools 对 ask_user 分流(同 exit_plan_mode 的"不进
  executor"族):并发批其余 call 全部落定**之后**,journal
  `WaitingEntered{kind: input, detail: {call_id, question}}`,drive
  循环经 decide() 进 doWait。等待注册表**维持两种 kind 不变**
  (决策 #31 不触碰)——ask park 就是"带未决问题的待命",靠 Detail
  区分,与 WAITING_APPROVAL 靠 Detail 携带请求载荷同构。
- **应答路径 = inbox 本身**(G20 缺的那条):park 期间第一条
  `InputReceived{source: user}` 由 fold 配对为该 call 的 tool result
  (`{"answer": <text>}`),随附图片按正常 user 消息紧随其后入对话;
  journal `WaitingResolved{input, "answered"}`;decide() 看到 call 已
  resolve → **同一 turn 继续**(这正是 ask_user 相对"结束 turn 提问"
  的增量价值)。后台 settlement(source 带前缀的子回执)**不**配对、
  只当唤醒,问题保持未决。
- **interrupt**:注册表 input 行既有 `superseded_by_interrupt` 生效;
  fold 对带未决问题的该 resolution 渲染 call result
  `"[interrupted by user]"`(IsError),与审批 denied_by_interrupt
  同款,turn 按既有 interrupt 缝收束。
- **crash-resume**:park 无 activity → 无 in-doubt;fold 里 Waiting
  未决 → decide() → doWait → 重新等待。**headless(UserInputs 为 nil,
  如 one-shot)**:走既有"standby lives in the journal"缝,run 返回、
  park 持久化,后续 `ar send` 走 resume 应答——一问一答跨进程天然成立。
- **约束**:一批至多一个 ask_user 进入 park;同批第二个 ask_user 返回
  模型可见错误(决策 #9 风格),不排队多问。
- CLI/daemon **零新协议动词**:`send` 就是应答(G20 所谓"应答路径"
  由 fold 配对补上,不是新命令)。

### 4. 不变量总对照

- 原则 3(一切副作用过同一条 pipeline):两个工具全是客户端执行的
  普通 tool_call effect,四关卡照走。✓
- 决策 #31(等待注册表两种 kind):不动,ask 用 Detail。✓
- §2 统一 inbox:答案就是 InputReceived,不加新输入通道。✓
- 凭据红线:web_fetch 输出过 redact。✓
- 决策 #13(tool 定义即数据):新增 network 数据位强化该决策。✓

## 验收(双闸门)

**闸门 A(scripted/单测,进 check.sh)**:
- internal/tool:web_fetch(httptest:200 文本、HTML 提取、重定向、
  非 http(s) 拒绝、超大响应截断、收容下 fail closed)。
- networkScope:web_fetch effect 带 "all";`{network:"*"}` 与
  `{tool:web_fetch}` 规则均匹配且能 ask/deny;收容后 executor 拒跑。
- internal/agent(scripted provider):ask_user park→send 应答→同 turn
  继续;park 中 interrupt→result "[interrupted by user]";park 中
  crash→resume 重新 park→应答;headless park→run 返回→resume+send 应答;
  同批双 ask_user→第二个报错;settlement 不配对。

**闸门 B(真实 API,Gemini)**:新增 **QA-12 web_fetch + ask_user
即问即答**——本地 http 服务页 web_fetch 佐证 + ask_user 中途提问、
脚本 `ar send` 应答、agent 用答案收尾。断言只钉 runtime 红线:两类
工具调用事件在 journal、waiting_resolved(answered) 出现、session 正常
idle、无 crash。结果归档 `qa/runs/<日期>-QA-12/`。

## 实施步骤(一步 = 一个可合并提交,全绿才 commit,立即 push origin/main)

1. **INC-5.1 web_fetch**:Def.network 数据位 + networkScope 数据驱动
   + def + 实现 + 规则匹配测试;SPEC/DESIGN 行齐。
   完成标志:check.sh 绿 + httptest/规则场景全过。
2. **INC-5.2 ask_user**:def + doTools 分流 + doWait 应答分支 + fold
   配对 + 全套 scripted 测试;SPEC/DESIGN §17/GAPS G20 行齐。
   完成标志:check.sh 绿 + 上表 6 个 scripted 场景全过。
3. **INC-5.3 收口**:JOURNEYS(UJ-01 可选步)/QA.md(QA-12+矩阵)/
   GAPS(G18b 收窄、G20 关闭)/CODEX-PARITY 对照行/LOG 齐活;
   qa/run-qa12.sh 真实 API 跑通归档;对抗 review;工作纸移
   archive/increments/。

## review 裁决

中等增量,不到里程碑级,裁掉三视角全量 review;但 INC-5.2 动 loop 的
等待/fold 机制(并发与恢复敏感),收口时做一轮**正确性/并发聚焦**的
对抗 review(基准 = DESIGN §2/§5/§6 + 本工作纸语义表),P0/P1 修完
才关闭。

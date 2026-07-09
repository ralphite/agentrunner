# arweb 开发台账(loop 驱动)

图例:`[ ]` 未动 `[~]` 进行中 `[x]` 完成(代码绿 + 真实验证过)

**loop 执行纪律(每轮迭代)**:
1. 取下面第一个未完成项(按序,不跳跃;一轮做一个 milestone 或其
   收尾余项)。
2. 实现;`cd web && gofmt -l . && go vet ./... && go test ./...` 全绿。
3. **真实验证**:真 `ar` + 真 daemon + 真 Gemini(`../.env`),按该
   milestone 的"真验"栏逐字执行并观察;证据摘要写进变更记录。
   没做真验的项不许打 `[x]`。
4. 更新本文件(勾选 + 变更记录追加一行),`git commit && git push`。
5. 全部 milestone `[x]` 后:收官检查(README 走查一遍),结束 loop。

---

## M0 蓝图与骨架
- [x] DESIGN.md / PROGRESS.md / README.md / .gitignore
- [x] 独立 module(arweb, stdlib-only)+ server 骨架 + 单文件 UI 壳
- [x] /api/health(版本 + daemon 探活)、daemon 托管(spawn/external 判定)
- [x] fake-ar 单测框架(exec 层不打真 API)
- 真验:`go run . --env-file ../.env` 起服务,浏览器打开首页,health
  绿点,sessions 列表能显示(空或已有)。

## M1 会话只读面(journal 观察器)
- [x] GET sessions / events?after / state / inspect / ps 五端点
- [x] 时间线渲染:DESIGN §5 全映射(用户/助手气泡、工具卡回填、
      轮次线、spawn/settle、waiting 状态条、兜底行)
- [x] 原始 journal / 折叠状态 / inspect 树 三个查看面板
- 真验:用 CLI 手工建一个真实会话跑两轮(真 Gemini),网页端完整
  重现时间线与状态;`ar events` 输出与页面逐事件对照无缺漏。

## M2 会话读写(chat 主链路)
- [x] POST sessions(spec+worker 落盘、workspace helper、mode)
- [x] send(含排队 pending 气泡)、interrupt、close;错误面(stderr 透出)
- [x] 新会话表单(base.yaml/worker.yaml 预填、空 workspace 一键造)
- 真验:全程网页操作——新建会话(真 Gemini)问答两轮上下文衔接;
  忙时插话排队生效(QA-02 式);interrupt 打断长 bash 后会话可续聊。

## M3 编排面(子 agent / 审批 / 图片)
- [x] ps 面板 + kill 按钮(用户直杀);spawn/subagent 事件卡
- [x] 审批卡(approve/deny + 理由),用 ask 权限 spec 真验
- [x] 图片上传 + send --image(真 vision 读图)
- 真验:网页指挥"起恰好 2 个子 agent"并 ps 可见、杀一个、另一个
  自然完成回灌;ask 模式下批准/拒绝各走一次;传 qa/fixtures 截图
  问答正确。

## M4 流式与打磨
- [x] SSE attach 透传;text_delta 打字气泡(journal 到达即落实)
- [x] 断线/daemon 重启的 UI 表达(探活红点 + 一键重启 daemon + 守护自愈)
- [x] 会话 URL hash 持久化、自动滚动、状态 pill 精化
- 真验:流式回答肉眼可见逐字出;kill -9 daemon → 页面红点 → 一键
  重启 → 同会话续聊无缝(QA-08a 式)。

## M5 压轴终验与收官
- [x] QA-09 式场景全程网页操作:图 + 恰好 3 子并行 + 先回先处理 +
      杀 C 换 D + 汇总 + 崩溃重启续聊 + 让它写 SUMMARY.md
- [x] README 快速上手走查(按文档从零起服务);已知问题清单
- 真验:上述场景一次成(允许按 QA §0.1 重跑一次);全 milestone 勾满。

## M6 父子交互面(用户新需求 2026-07-07:实时看父子交互)
- [x] 子 agent 审批上卷实时闭环:SSE approval_request(仅此通道有,
      父 journal 无子审批事件)→ 实时审批卡,标注请求方(agent 名 +
      子会话全 id);批准/拒绝+理由经父 sid 路由回子;与父自身审批
      的 journal 卡按 approval id 去重
- [x] 子会话链接 + 实时时间线(INC-1 子会话寻址落地:spawn/settle
      卡带"打开子会话 ↗",子页只读模式 + "← 父会话"导航;1s 轮询
      即实时——在飞子的 bash 运行中卡实测可见,完成后同页自动更新)
- [ ] 子事件进 attach 流(打字级流式装饰)——产品缺口,提案 P1②
- [ ] 父/用户向在飞子 agent 发第二条消息——**产品缺口**,见提案 P2
- 真验:worker spec 带 `{tool: bash, action: ask}`;网页指挥 spawn →
  审批卡实时弹出 → 批准路径(子执行 echo 回灌父)+ 拒绝路径(理由
  逐级穿透:用户→子→父转述)各走一次。

## M7 契约同步(2026-07-08:跟进 INC-2 + REVIEW-001 D 系手术,9a9f18e..5eb1e76)
- [x] 后端:`new`/`send` 加 `--detach`(INC-2 后默认跟随渲染回复,驾驶舱
      要 ack-only);新增 POST /api/sessions/{sid}/agent → `ar agent`
      (换 agent,决策 #32),spec 落盘抽共用 writeSpecDir(旁置 worker 同支持)
- [x] 前端:默认 spec 工具名 task_kill→kill(+output);删 task_completed
      渲染(事件已删);session_closed 读 reason(closed|killed)+source
      (user|parent);新增 spec_changed chip;waiting kinds 清到 input/
      approval;limit_exceeded 改"可见截断"表达(kind 三态、会话不终结);
      assistant_message finish=blocked chip;「换 agent」按钮+对话框;
      词汇 task→后台工作/handle
- 真验:真 Gemini 全程(API+Chrome)——见变更记录轮 8。

## M8 UI overhaul(2026-07-08 用户 goal:full features + easy to use,像 Claude Code app)
第一步(轮 9):无 close 概念、全英文(铁律 I6/I7)、UI-GAPS.md 盘点。
主体(轮 10):
- [x] 视觉系统现代化:Claude-app 风格(暖色 accent、圆角/阴影、系统字体)、
      light/dark 双主题(prefers-color-scheme + 手动切换,localStorage 持久)、
      空态引导、聚焦态、侧栏会话卡带状态点
- [x] Markdown 渲染器(零依赖 vanilla:标题/列表/行内 code/代码块/引用/链接/粗斜体;
      XSS 先转义后套标签);assistant 消息改全宽 AR 头像 + markdown 块(user 仍气泡)
- [x] Composer 能力面:agent 模板下拉(dev/auditor/reviewer/chat/custom)+ model 下拉
      (gemini-flash-latest/2.5-pro/2.5-flash/custom id)+ mode 只读显示;选择即 `/agent`
      切换,reflectSpec 从 session_started/spec_changed 反同步
- [x] 生命周期状态面:classifyStatus 覆盖全集状态词上色(waiting/running/stranded/
      closed/killed/completed/…);usage 徽章(inspect billed tokens,点开 = tree);
      stranded/marked 复活提示条 + maybeStranded 交叉引用 sessions-list 权威 status
- [x] fork/barrier 面板:header barrier 按钮(`ar barrier`)+ fork 对话框(`ar fork --list`
      列 barrier → `ar fork` 分叉,新会话进列表并打开);后端 /barrier /barriers /fork 端点
- [x] one-shot(submit)+ trust:new session 对话框 Conversation/One-shot 分段 + trust 勾选;
      后端 startOneShot(submit run 后台跑完,读 session id 即返回——submit 的 run 绑客户端连接,
      不能像 attach 那样断开)+ handleNewSession 支持 trust/oneshot
- [x] drive 驱动面板(goal/loop/best-of-N):header Driver 按钮 + 对话框(mode 分段 +
      task + agent/model + goal verifier command/metric/threshold + loop interval +
      best-of-N n + max_iterations + budget);后端 POST /api/drive 生成 agent.yaml+
      driver.yaml,exec `ar drive --json` 前台流式,stdout(child run 的 protocol
      events)以 NDJSON 透传 + stderr 结论合成 driver_stderr 行;前端 fetch-stream
      读 NDJSON,每个 child run session_start=一轮 iteration,driver 结论从 stderr 解析
- 真验:见变更记录轮 9/10/11。

## 产品增量提案(动 internal/,须按 docs/PROCESS.md 走三层 delta,待用户拍板)

- **P1 子会话观察面**:②寻址部分 **已落地为 INC-1**(2026-07-07,
  见 docs/archive/increments/INC-1;events/--state/inspect/ps/
  attach-replay 对子会话全 id 生效)。余项 ①`childLoop` 接 Out sink
  (tee 到父的 hub sink,`protocol.Event.Session` 填 childSession),
  attach 一个父即得全树实时流——打字级流式装饰用,轮询已覆盖功能面。
- **P2 父→子第二条消息**:子 run 是一次性 task(无 inbox/mailbox),
  当前只有 task 一条输入 + kill 两种交互。增量:给 spawn 子挂
  UserInputs(复用 conversational 的 mailbox 语义)+ 投递面(用户侧
  `ar send <child-id>`;可选父模型侧新工具 `send_to_agent(handle,
  text)`)。动运行时核心语义(waiting/边界/恢复),需完整走增量流程。

---

## 已知问题(web/ 之外的发现,按铁律不在此修)

1. **[产品 bug] Gemini 空 assistant parts 毒化会话**(2026-07-07,会话
   20260707-231439-task-1dfc):模型某轮返回空内容 → `assistant_message`
   以 `parts: []` 落 journal → 之后每轮组装历史都被 gemini adapter 拒绝
   (`message with role "assistant" has no parts`,internal/retryable=false)
   → session_closed(error),revive 后同样历史同样死,**会话永久不可
   恢复**。已提独立修复任务(task_572ca493);涉及落账侧(不落空消息)
   与组装侧(过滤空消息救活存量)两个面。该会话是用户实际使用驾驶舱
   时踩到的——驾驶舱作为测试工具第一天就抓到真 bug。

## 变更记录(每轮追加;只记真实发生并验证过的事)

| 日期 | 轮次 | 动作 | 真验结果 |
|---|---|---|---|
| 2026-07-08 | 12 | drive 三模式补验 + 易用性打磨:best-of-N(schedule parallel)、loop(schedule interval)真验;driver 顶层 session 从会话列表滤除(web-driver id);同名会话 meta 加启动时间区分 + hover 完整 id | 真 Gemini:best-of-N(n=3,verifier test -f result.txt)→ 3 并行 attempt(iteration 1/2/3 + 3 bash 卡)+「driver satisfied · 3 iterations」;loop(interval 2s,max_iter 3,无 verifier)→ 3 轮 + progress.txt 3 行 +「driver max_iterations · 3 iterations」(warn 色)+ pill driver:max_iterations;列表滤除后无 driver 噪音(anyDriver=false),meta 显示 23:41/23:36 区分同名会话 |
| 2026-07-08 | 11 | M8 drive 驱动面板(goal/loop/best-of-N):后端 /api/drive(生成 agent+driver.yaml,exec ar drive --json 前台流式 NDJSON 透传)+ buildDriverYAML(yamlStr 借 JSON 引用安全转义);前端 Driver 对话框 + fetch-stream driver 视图;+2 单测(TestBuildDriverYAML/TestDriveStreamsNDJSON) | 真 Gemini goal 模式全程(ws-drive2):Driver 对话框 goal/task/verifier(test wc-l>=3)/max_iter 6 → runDriver fetch-stream。先抓真实 drive --json 事件流(background capture)发现 stdout 只有 child run 的 protocol events(session_start→bash→message→run_end reason=completed)、driver 结论只在 stderr——据此修 driverRender:每个 child session_start=一轮 iteration(编号 1/2/3),child run_end(completed)不冒充 driver,driver 结论从 stderr driver_stderr 行解析。修后渲染:iteration 1/2/3 分隔 + 每轮 bash 工具卡(done)+ agent 消息(行内 code)+ 单个「■ driver satisfied · 3 iterations」结论 + pill driver:satisfied;progress.txt 实达 3 行 tick(verifier 真通过)。已知噪音:drive 产生的顶层 driver session 在 sessions list 显示 unreadable(CLI 无法 fold driver session,非 web bug) |
| 2026-07-08 | 10 | M8 主体:视觉现代化+双主题、markdown 渲染、composer 能力面(agent/model 选择器)、生命周期面(stranded/usage/revive)、fork/barrier、one-shot/trust;后端加 /barrier /barriers /fork 端点 + startOneShot + trust/oneshot;fake-ar 单测扩容(barrier/fork/submit/trust/inspect-usage,+6 测试);隔离 /tmp/arw2 避开用户 8787 环境 | 真 Gemini 全程 Chrome(端口 8890,XDG=/tmp/arw2/d):①markdown——新会话要求 h2+bullet+行内 code,DOM 验证 h2「Capabilities」/2 bullet/`default_api:*` code/MD_RENDER_OK,usage ▸2753 tok ②composer——Agent 下拉切 auditor→spec_changed chip→下条消息 [AUDITOR] 身份;model 下拉反同步 ③双主题——切 dark 全页深色 markdown 清晰 ④fork/barrier——barrier 打点 bar-m37;fork 对话框列 6 barrier(自动+手动)→选 bar-t2 分叉→新会话 stranded ⑤生命周期——fork 会话 stranded pill+复活提示条;send REVIVED_OK→journal seq17-24 复活(新 LLM+回复+idle)→pill 转 waiting:input、提示消失 ⑥one-shot——UI 建 submit 会话,write_file 工具卡(done)+回复行内 code,ws-os7/hello.txt=ONESHOT_OK;curl+fetch+CLI 三路径交叉验证 completed。**坐实 stranded 状态真实存在**(fork 会话),推翻调研 subagent 的误判。已知:频繁重启 arweb 有 daemon 交接窗口,期间 submit 会撞 activity_cancelled(非产品 bug) |
| 2026-07-08 | 9 | M8 第一步(用户三点拍板之 1/2):close 概念全移除(按钮/端点/白名单/fake-ar 桩,铁律 I7)、UI 全英文化(词汇对齐 journal/CLI,含默认 spec,铁律 I6);UI-GAPS.md 全量欠缺盘点成文待确认 | fake-ar 单测绿;真 Gemini 全程 Chrome(隔离 XDG=/tmp/awui,端口 8890):新会话表单(英文默认 spec、make empty workspace 一键)→ 两轮问答(turn 1 工具介绍 → 指令回显 ENGLISH_UI_OK),session started chip/turn 线/cli·you·agent tag/waiting: input pill/composer 全英文;sesshead 五按钮无 close;pending→queued→you 落账链路走通;console 零错误零警告 |
| 2026-07-08 | 8 | M7 契约同步(INC-2 + D 系手术):new/send --detach、/agent 端点+「换 agent」对话框、task_kill→kill/output、事件映射更新(spec_changed/session_closed source/可见截断/waiting kinds)、词汇清理;fake-ar 单测新增 agent 场景 | 真 Gemini 会话 af84(API+Chrome 双路):new --detach 秒回 sid、两轮暗号"紫罗兰"衔接;send --detach→"delivered"、journal 轮询渲染回复;/agent dev→auditor→dev(带 worker 旁置)三连,journal spec_changed 各恰一条、【审计员】身份即换即答、上下文延续;spawn worker 非阻塞(ps 面板 handle call_4_0)→网页 kill→父 [kill] control 气泡+activity_cancelled+subagent_completed(error),子 journal session_closed{killed,source:user},子页只读视图"已杀"pill+"会话被杀·来源 user"chip(38 轮 output busy-poll 卡全渲染);interrupt 待命处 no-op 只落 [interrupt] 审计行、会话仍 waiting(坐实 cli.go:115 help 过时,已另立任务);close→session_closed{closed,source:user};整页重载(3×spec_changed+kill 标记全量重放)console 零错误 |
| 2026-07-08 | 7 | INC-1 子会话寻址(产品增量,按 PROCESS.md 全流程:工作纸→实现→三层收口→归档):resolveSessionDir 支持 -sub- 分段映射;web 链接化 + 子页只读模式 | CLI:`ar events <child全id>` 在子在飞时输出其 journal(实抓 seq1-8),ps/inspect/--state 同工;scripted ×2(含孙级嵌套);Chrome:spawn/settle 卡"打开子会话 ↗"→子页(← 父会话导航、只读、无 SSE)→在飞 bash"运行中"卡实拍→45s 后同页自动更新为完成+LIVE_OK+任务完成 chip。记档:internal/tool TestBashCancelLeavesNoSessionOrphans 在 main(4974932)pre-existing FAIL(D 系手术中间态),与本增量无关 |
| 2026-07-07 | 6 | M6 子审批上卷实时闭环(SSE approval_request 渲染 + approvalCard 双通道去重 + 本地固化);产品增量 P1/P2 提案成文 | 真 Gemini 父子会话(worker bash=ask):子跑 bash 触发 ask 上卷→驾驶舱**实时**弹审批卡(请求方: worker + 子会话全 id + args);批准→子执行→CHILD_NEEDS_OK 回灌父;拒绝(理由"用户不允许这条命令")→子收到 denied+理由→汇报父→父转述用户,四级穿透;两路径审批卡各自正确固化(已批准(你)/已拒绝(你): 理由) |
| 2026-07-07 | 5 | M5 压轴收官:QA-09 式场景全程网页操作,一次成(会话 20260708-000600-ready-1151,168 事件) | 图(CI 截图内容穿透进子任务书)+恰好 3 子并行(spawn 卡×3、在飞面板×3);先回先处理(A 无工具最先回→父第 4 轮消化并确认 A 推理正确:Go 版本过低;B 第 5 轮);"杀C换D"消息忙时排队(排队中→边界落账);kill C(journal `[kill call_2_2]` control)→spawn D→四路总汇总表(A 成功/B_OK/C 已取消未重启/D_OK);pkill -9 daemon→3s 自愈→续聊答"A"(崩溃前历史完整);write_file 落 SUMMARY.md(文件内容与会话结论一致,四行结局;journal write_file@159);全程 4 spawn/4 settle。README 从零走查:build→serve(daemonUp)→index 200 ✓。全部 milestone 勾满,loop 收官 |
| 2026-07-07 | 4 | M4 真验(代码在前几轮已就位+自愈是本轮新增) | ①流式:JS 探针客观捕获——send 后 6.15s state.typingEl 出现且带增量文本(SSE text_delta);此前 M2 已两次目击 streaming 气泡截图 ②崩溃恢复:pkill -9 arbin daemon→3s 内 auto-respawn(arweb 日志"managed daemon died; auto-respawned")→同会话网页续聊,暗号"紫葡萄"正确复述(QA-08a 式);红点+「重启 daemon」按钮此前两次真实目击,API /daemon/start 真验 started ③hash 直达/自动滚动/状态 pill(idle 绿/run 黄/appr 紫/closed 灰)全程在用 |
| 2026-07-07 | 3 | M3 真验 + 三个健壮性修复:hashchange 监听(同页 hash 导航原先不切会话)、daemon 守护自愈(托管 daemon 被外杀 1s 内自动重启,3 次/分钟节流)、测试环境 binary 改名 arbin 避开并行 session 的 pkill 误伤 | ①spawn:网页指挥起恰好 2 子,在飞面板 2 行+kill 按钮;网页杀 A→journal `[kill call_10_0]` source=control→activity_cancelled+subagent_completed(canceled);B 自然完成回灌;父第 12 轮激活消化;模型自作主张重启 A(模型行为,再杀+叫停后正确汇总"A 被取消,B 汇报 B_DONE")②审批:ask 权限 spec,审批卡带全 gate 徽章(permission:ask rule 1: tool=bash→ask)+args;拒绝(理由达模型:denied: 测试拒绝路径)→批准→bash 执行→APPROVAL_TEST ③图片:upload API+chip+send --image,build-error.png 三要素全对(command.go/1234/EnableTraverseRunHooks2);journal ref-not-bytes(单行 372B,sha256 CAS ref) |
| 2026-07-07 | 0 | M0 落地:module/server(9 端点+SSE)/单文件 UI/fake-ar 单测 ×9/docs。M1–M5 的代码骨架同时就位,待逐项真验 | health 绿(daemon 托管成功);sessions 列表 OK;真 Gemini 全链路 smoke:POST /api/sessions 建会话→"1+1=?"→journal 里 ASST"2"→waiting:input。注意:XDG_DATA_HOME 过长会使 daemon socket bind 失败(macOS 104B 限制),测试用 /tmp/aw1 |
| 2026-07-07 | 1 | M1 真验(代码已在轮 0 就位,本轮纯验证,零代码改动) | CLI 建真实两轮 Gemini 会话(暗号"红苹果"第二轮复述→上下文衔接);`ar events --json` vs web /events 20 事件逐一 MATCH(seq+type);after=13 过滤→7 条;state=waiting:input、inspect 树(2 llm entries+usage billed 1058)、ps 空、sessions 双会话均对;Chrome 实测 UI:时间线气泡/轮次线/source 标签(cli/你)/状态 pill/三查看面板/系统事件开关(#4 barrier、#5-6 effect、#7-8 activity 兜底行)全部正确渲染 |
| 2026-07-07 | 2 | M2 真验全程 Chrome 网页操作;修 close 为双击确认(原生 confirm 冻结渲染进程、毁自动化);发现产品级 bug 记入已知问题 #1 | ①新会话表单:造空 workspace 一键、默认 spec、开场消息→真 Gemini 会话 42f4,两轮暗号"蓝海豚"衔接 ✓;②QA-02 式排队:sleep 20 在飞时插话→气泡"排队中"(pending)→bash 完成卡(QUEUE_TEST_DONE)→插话消化答"三加四等于七",bash 无 Cancelled ✓;③interrupt:sleep 30 在飞 8s 时点按钮→bash 卡"已取消"(部分输出留存)→[interrupt] 来源气泡→第 7 轮解释→续聊"OK" ✓;SSE 打字气泡实测出现并被 journal 落实替换(M4 部分提前验证);close 双击确认在已关闭会话上走通 UI 路径 |

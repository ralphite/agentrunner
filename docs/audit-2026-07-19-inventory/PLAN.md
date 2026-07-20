# 修复 Plan（2026-07-19 评审收口 · loop 工作队列）

**来源**：docs/audit-2026-07-19-inventory/FEATURES.md 盘点 + 双盲评审交集
（会话内 report，未落盘）。本文件是 loop 的 durable 队列：每轮迭代取下一
未完成项，按下方协议执行并勾选。

## 用户裁决（硬约束）

- **Driver 不是产品功能**：driver 只是 loop/goal 的内部实现抽象，从未被
  认可为 user-facing；目标态 = 用户面只有「会话 + 挂在会话上的
  goal / loop(schedule) / best-of-N」，不存在任何 user-facing 的
  Driver/run/series 概念。
- dictate / optimize **保留不动**（不降级、不撤出 CLI）。
- phone-webui workflow **保留不动**（在用，不改 cron）。

## 默认裁决（AskUserQuestion 未送达，按推荐口径执行，可随时改口）

- Driver CLI 面最终删除，分两步：先撤 webui driver 投影/新建入口；
  in-session 等价确认后删 `ar drive`/`ar submit --drive`；旧 driver
  journal 永远只读可查（sessions/inspect/events 不受影响）。
- lease/DAG/relay：先做消费方评估，确认零/低消费再砍；有真实消费方则
  只报告不动。
- QA 共享 store 政策：维持现状，不碰 CLAUDE.md。

## Loop 协议（每轮迭代同一纪律）

1. 取下方队列第一个未勾选项。
2. **复核问题仍真实**（读码确认；不真实 → 在本文件记"复核不成立"并勾掉）。
3. 按 docs/PROCESS.md 走增量流程（三层 delta；动 DESIGN 不变量走不变量
   变更流程，单独 commit）。
4. 实现 + `./scripts/check.sh` 全绿 + 相关测试。
5. commit + push origin/main（CLAUDE.md git 规则）。
6. 勾选并在条目后追加一行结果摘要。

## 队列

### Phase 0 · 布防
- [x] 0.1 FEATURES.md 纠错应用（屡崩升级移入 §14、补 outputs 契约/spec
      调参面/枚举修正等 ~25 处）——2026-07-19 本 commit。
- [x] 0.2 DESIGN/LOG 登记裁决："driver 是实现抽象、无 user-facing 面"
      ——2026-07-19：工作纸 INC-80 立项（不变量变更单独成文）+ LOG 记档；
      DESIGN §15 决策落表按 PROCESS 与实现同 commit（INC-80.4）。

### Phase 1 · 用户可感的洞
- [x] 1.1 G39 子审批不可见死锁：child 审批上浮父会话 Attention
      ——2026-07-19 INC-81.1：复核=approve 路由本已通、缺口纯可发现性；
      inspect 树递归 in-flight child + answer_with，webui approval stack
      /Attention 持久浮出；闸门 A 两侧孪生绿，闸门 B 挂 G39 待真机复验。
- [x] 1.2 G3 审批挂起期间消息只排队不唤醒——2026-07-19 INC-70 Option B
      落码：park 中 user-class 消息=转向式拒批（denied_by_steer+保序
      flush+同边界入 context），machine/revoked 不触发；闸门 A 三孪生绿，
      闸门 B 挂 G3 注记待真机。

### Phase 2 · Driver 去 user-facing（核心，拆多轮）
- [x] 2.1 盘点 driver 暴露面全集并映射 in-session 等价物——2026-07-19
      结论入 INC-80.1：series runner 齐备但零接线，goal/interval/cron
      可先合流，best-of-N/retry/self_paced 是硬缺口（/bestof 被硬阻塞）。
- [x] 2.2a E1③ 第一步（opt-in）——2026-07-19 落码：`drive/submit
      --series` 走 RunSeries（SupportsSeries 路由，不支持形态响亮拒绝）；
      hostResumeDrive/boot-sweep 按 journal 头双分派（readSeriesSpec）；
      scanDriveSessions 收编 series 会话、scanStrandedSessions 排除；
      `ar resume` 拒绝并指路。四孪生锚入 SPEC F 表。
- [x] 2.2c 翻默认——2026-07-19：eligible 三类（goal/interval/cron 无
      retry）默认走 RunSeries（CLI 前台 + daemon host 双路径），--series
      变"强制并校验"；retry 双头（CLI drive --retry 读 series 头、webui
      parseDriverRetryInfo 识别 session_started+series_started）；legacy
      写侧仅剩 bestof/retry/self_paced。钉子：
      TestDriveDefaultsToMergedStreamForEligibleSpec/
      TestDriveRetryReadsSeriesHead。
- [x] 2.2b series runner 补三形态（分三 commit：①retry ✅ 2026-07-19
      ——runSeriesIteration attempt 循环、per-attempt spawn 词汇
      `iter-N-aM`、spend 全和、settled 失败重分类；②self_paced ✅
      2026-07-19——pace 工具装配、awaitSeriesTick 分支（durable timer
      仅 wake hint、tick 恒零）、applySeriesPaceIntent（finish 人审/
      clamp/on_no_intent）、ResumeSeries pace 再推导、shutdown 免终态
      名单收编；三孪生绿；
      ③parallel ✅ 2026-07-19——base ref pin 在 **SeriesStarted**（open
      前快照，优于原案的 SeriesIteration 载体）、series 版本 bump 1→2、
      driveSeriesParallel（worktree 物化/丢失拒判/pass 压 score 选择）、
      SeriesEnded.BestIter 为 fold 权威、parallel×retry 组合留 legacy；
      三孪生绿）。legacy 写侧仅剩 parallel×retry 一个组合。
- [x] 2.3 撤 webui 的 driver/run 概念面——2026-07-19 主体落地：series
      SESSION 行成为 Scheduled 的 canonical（run 行在 session 落列表后
      让位，两 vitest 钉住）；/loop、/bestof 启动后 landInSeries 直接落
      会话（run 流仅 sid 未知时兜底）；用户可见 "driver" 文案清除
      （RunModal 两处 tooltip）。残余（RunView 兜底流与 RunModal 高级
      表单的存废）并入 2.4 与 CLI 传输决策一起裁。
- [x] 2.4 CLI 入口处置——2026-07-19：`ar drive` 降为 webui transport
      （help 撤宣传、物理保留因 thin-shell 教义，删除挂 HTTP 壳
      backlog）；决策 #41 落 DESIGN §15、§17 E1 四步全落。**INC-80
      三视角对抗 review 完成**：正确性 P0×2+P1（sweep 收编/错形态复活
      /in-flight skip）、安全 P1（journal 脱敏）、契约 P1×4（版本兼容
      /SPEC 矛盾/QA 登记/工作纸对账）全修并推 main；P2 批记档。闸门 B
      = QA-77 五场景已登记待真机轮。

### Phase 3 · webui 双实现拆弹
- [x] 3.1 `ar sessions --json` 长出 cadence/next_run_at——2026-07-19：
      engine 权威实现落 internal/driver/cadence.go + internal/cron.Phrase，
      legacy 与 series 双形态投影，webui 删 driverSchedule/
      parseDriverJournal/自研 cron 引擎/TTL 缓存（仅换键名转发）；瞬态
      run 的 YAML 本地 cadence 暂存至 2.3 撤 runs 面。

### Phase 4 · "停"动词收敛
- [x] 4.1 修文实矛盾：DESIGN §12 "stop teardown-no-mark" vs 实现/CLI help
      "stopped 标记"——2026-07-19：裁决代码为真相（loop.go abort 路径落
      可复活 SessionClosed{stopped} 标记），DESIGN §12/SPEC stop 行改写为
      "落可复活 stopped 标记，自动路径不越、send 复活"，LOG 记档；docs-only。
- [x] 4.2 动词面收敛——2026-07-19 INC-82（不变量变更单独成文）：收回
      INC-74 "WaitingEntered 清标记"条款，重开信号只剩 GenerationStarted
      （真实输入起 turn）；compact/clear=维护手势，closed 会话上照常执行
      但不复活（标记存活、状态诚实报 closed，send 随时续）；动词模型收敛
      为两概念：打断（interrupt 无标记）/关闭（close/stop/kill 同族标记
      同规则），stop=打断+标记组合。钉子
      TestMaintenanceAfterCloseKeepsMark；DESIGN §12/§恢复、SPEC:24 改写。

### Phase 5 · 减法与碎屑（每项独立一轮）
- [x] 5.1 占位 UI 移除——2026-07-19:Plugins 五件套、Environment chip、
      Settings Branch prefix/PR merge method(含 GitPrefs 瘦身)全删;
      测试/QA-69/SPEC/FEATURES 锚同步。原则:UI 只承诺已接线的能力。
- [x] 5.2 semantic_search 改名 keyword_search——2026-07-19:defs 改名+
      description 如实(BM25 lexical,not embeddings);tool.Canonical 别名
      迁移(旧 spec/journal/中途 resume 全兼容);builtin specs/init 模板/
      QA 脚本/webui timeline 双 case/SPEC/DESIGN/parity 文档同步;钉子
      TestKeywordSearchToolEndToEnd(spec 故意用旧名钉 alias 全链)。
- [x] 5.3 lease/DAG 评估后砍除——2026-07-19:depends_on(仅静止校验无
      调度)与 lease_id(零读者)砍,team fold v2;**逐层 relay 复核有真实
      消费方(孙辈 durable mail 承重件)保留**;delegation 本体/workspace/
      replaces 保留。评估全文见 LOG。
- [x] 5.4 CLI 碎屑批修——2026-07-19:sessions/inspect/events 统一
      parseFlags(撤两处手写分拣),run -o 显式报错指路 record-fixture
      (补钉子),goal --max-checks help 10→20。
- [x] 5.5 `ar new` 开场附件——2026-07-19:--image/--file 全链(CLI→
      daemon→Loop.ingestOpening,blob-before-event 同形);钉子
      TestOpeningImageAttachmentEndToEnd;超长开场折叠仍记档推迟。
- [x] 5.6 manual rename 落 journal——2026-07-19:durable control `title`+
      `ar title` CLI+webui /rename 端点;前端 localStorage 层退役(在飞
      乐观 overlay+旧 key 一次性迁移);钉子 TestManualTitleControl。
- [x] 5.7 结构化输出合并——2026-07-19:spec output_schema 单入口;非原生
      provider 自动引 INC-26 客户端校验为内部 fallback(--json-schema
      flags 退役);钉子 TestSpecSchemaFallback*;QA-33 重裁挂闸门 B。
- [x] 5.8 best-of-N 胜者晋升——2026-07-19:`ar promote`+webui Apply
      winner(shadow repo base→winner patch,clean-or-nothing 不 staged);
      顺手补 timeline series 事件渲染;G15 消解;钉子
      TestPromoteWinner*+timeline series 测试。
- [x] 5.9 时间旅行减负——2026-07-19:BarrierHandle.Policy 字段裁除(从未
      有第二个值),fork 一律取消在飞 handle;DESIGN §13/词表/SPEC 瘦身;
      旧 journal 字段 decode 忽略无迁移。**队列至此清空。**

### Phase 6 · 生命周期动词全面拆除（2026-07-19 用户裁决:全删）

**用户裁决（硬约束,推翻 Phase 4 的"两概念收敛"方向）**:产品里根本
没有"活着/关闭"这些概念——静止模型下会话只有"在干活/在等你",永远
可续。close/stop/kill/interrupt 这一族生命周期动词**全部从用户面删除**
(尤其 UI);它们不是用户引入的设计。Phase 4 把它打磨成"打断/关闭"
两概念是方向性错误,本 Phase 拆除。

**默认裁决（已向用户声明,可改口）**:(a) "Stop 当前生成"手势保留
(Esc/按钮——affordance 不是概念;CLI interrupt 仅作 transport);
(b) 停运行中的 loop/best-of-N=对系列落 cancelled 终态(领域内事实),
不给会话盖章;(c) kill 只留模型内部工具,agent 自管子任务。

- [x] 6.0 INC-83 工作纸——2026-07-19 成文(docs/increments/
      INC-83-no-lifecycle-verbs.md):旧不变量(决策 #30 用户面部分)、
      "每个停的需求的正确归属"对照表、新表述、波及面→6.1–6.6 映射、
      契约自审(旧 journal 兼容读、thin-shell 不破、#31 不动)。
- [x] 6.1 series cancel——2026-07-19:复核发现机制已在(ctx cancel→
      SeriesEnded,无 SessionClosed,sweep 跳过 Ended);词汇 stopped→
      **cancelled**(读侧双词兼容旧 journal);钉子
      TestSeriesUserCancelWritesCancelledTerminal。
- [x] 6.2 CLI 撤出——2026-07-19:help/命令表不再宣传 close/stop/kill
      (物理保留为内部 transport,-h 安全性质保留);interrupt help=唯一
      手势措辞;stuckHint/barrier 指路清词。webui 迁移(6.3)后 close/
      kill 全删、stop 留 series-cancel transport(并入 6.5 收敛)。
- [x] 6.3 webui 概念面清除——2026-07-19:Close session 菜单/Background
      kill 按钮删,菜单 Stop=interrupt 唯一手势,Scheduled 行动词=
      "Cancel series…"(域内终态,不再 running 限定);/close /kill 路由
      与 AR client 方法删;closed 提示语改中性;vitest 599 绿。状态
      词汇(closed/stopped)的投影清词归 6.4。
- [ ] 6.4 状态投影清词:sessions/inspect 不再输出 closed/stopped/
      killed(旧 journal 的标记折出统一的中性形态,如 waiting:input);
      Quiescence reason 词汇同步;hook ingress 410 门重裁(hook revoke
      即停,不再看会话标记)。
- [ ] 6.5 内核收敛:SessionClosed 用户写侧全删(interrupt 不落标记;
      agent kill 工具的 parent-kill 子会话标记保留为内部);INC-82 的
      fold 规则随之简化;child revive 门只剩内部 kill 语义。
- [ ] 6.6 文档全链:DESIGN 决策 #30/§12 重裁、SPEC/JOURNEYS/QA 清词、
      FEATURES v1.3、GAPS 记档;LOG 追加。

### 明确不做
- dictate/optimize 降级（用户否决）。
- phone-webui cron 移除（用户在用）。
- webhook ingress 重构（冻结即可）。
- QA 共享 store 政策调整（维持现状）。

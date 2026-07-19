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
- [ ] 2.3 撤 webui 的 driver/run 概念面：Scheduled 页 = 带 schedule/goal
      的会话视图；新建一律走 in-session 形态。
- [ ] 2.4 删 `ar drive`/`ar submit --drive`（2.2/2.3 确认等价后）。

### Phase 3 · webui 双实现拆弹
- [x] 3.1 `ar sessions --json` 长出 cadence/next_run_at——2026-07-19：
      engine 权威实现落 internal/driver/cadence.go + internal/cron.Phrase，
      legacy 与 series 双形态投影，webui 删 driverSchedule/
      parseDriverJournal/自研 cron 引擎/TTL 缓存（仅换键名转发）；瞬态
      run 的 YAML 本地 cadence 暂存至 2.3 撤 runs 面。

### Phase 4 · "停"动词收敛
- [ ] 4.1 修文实矛盾：DESIGN §12 "stop teardown-no-mark" vs 实现/CLI help
      "stopped 标记"——先对齐文档与代码。
- [ ] 4.2 动词面收敛：目标两个用户概念（打断 / 关闭且不被 compact/clear
      复活），stop 并入；涉及不变量变更流程。

### Phase 5 · 减法与碎屑（每项独立一轮）
- [ ] 5.1 占位 UI 移除：Plugins 五件套、Environment chip、Settings 的
      Branch prefix / PR merge method（Not wired）。
- [ ] 5.2 semantic_search 改名 keyword_search（description 如实；含
      defs/迁移与测试）。
- [ ] 5.3 lease/DAG/depends_on/逐层 relay：先消费方评估，零消费则砍
      （保留 spawn/receipt/kill/直接子 revive）。
- [ ] 5.4 CLI 碎屑批修：sessions 迁回 flag 包、run -o 显式报错、
      inspect/events 解析统一、goal help 默认值 10→20 陈旧文案。
- [ ] 5.5 `ar new` 补开场附件（与 send 对称）。
- [ ] 5.6 webui manual rename 落 journal（SessionTitled{manual}），删
      localStorage 层。
- [ ] 5.7 结构化输出合并：spec output_schema 单入口，--json-schema 客户端
      校验降为无原生能力时的内部 fallback。
- [ ] 5.8 best-of-N 胜者晋升：复用 INC-49 Apply-to-project 补
      "Apply winner"（以会话形态呈现）。
- [ ] 5.9（可裁）时间旅行减负：处置向量写死 cancel_at_fork、DESIGN 条款
      瘦身。

### 明确不做
- dictate/optimize 降级（用户否决）。
- phone-webui cron 移除（用户在用）。
- webhook ingress 重构（冻结即可）。
- QA 共享 store 政策调整（维持现状）。

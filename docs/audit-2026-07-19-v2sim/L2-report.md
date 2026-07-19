# L2 · 异步派活(Codex cloud 形态)—— QA 报告

- 场景: L2(手机短句、并行 worktree、best-of-N;三路限流方案 A/B/C)
- Driver: ralphite/agentrunner issue #30, run 29703154416(新环境,executor lastN 从 0 起)
- n 号段: 200–299(严格递增)
- 起始时间: 2026-07-19 ~20:51Z
- 铁律: 全程串行(API key 不支持并发)、未 close、未发 {"op":"end"}、未 push;所有 sid 保留

## sid / workspace / worktree 台账
- 通道: n200 goto+health(30 sessions, 0 console err, no overflow, ~5s roundtrip)。PASS。
- workspace(主): `/home/runner/work/agentrunner/agentrunner/runtime/ws/ws-20260719-205246`(n201)
- **base session(脚手架/主工作区)**: sid `20260719-205428-go-6b877e83fe45d27c`, specDir `s1784494468819942556`(n202, mode=acceptEdits, permissions all-allow)
- **worktree A** (Redis token bucket, branch ratelimit/a-redis-token-bucket): `/home/runner/.local/share/agentrunner/worktrees/ws-20260719-205246-ratelimit-a-redis-token-bucket-20260719-205701`(n205 PASS)
- **worktree B** (进程内滑动窗口, branch ratelimit/b-inproc-sliding-window): `.../ws-20260719-205246-ratelimit-b-inproc-sliding-window-20260719-205701`(n205 PASS)
- **worktree C** (envoy ratelimit, branch ratelimit/c-envoy-config): `.../ws-20260719-205246-ratelimit-c-envoy-config-20260719-205701`(n205 PASS)
- session A sid: **20260719-205801-a-9fc77e45374db707**(n206;turn 完成 n208 status=waiting)
- session B sid: **20260719-210007-b-7ca656d79f297291**(n209, specDir s1784494807614392932;running)
- session B: turn 完成 n212 status=waiting(title "进程内滑动窗口限流设计与测试")
- session C sid: (n213 已发,待 sid)

### 编排/串行观察
- 三路 session 各绑定各自 worktree workspace,严格串行:A 开场 turn 完成后才开 B。UI 侧边栏可见各 worktree 会话(title 自动命名如 "Token Bucket Rate Limiter Implementation")。全程 0 console err,无横向溢出(scrollW=winW=1280)。
- 观察: /api/worktree 一条 eval 内串行建 3 个 worktree 全成功,返回 {path,repo,branch};落盘到 ~/.local/share/agentrunner/worktrees/<repo>-<branch>-<ts>。PASS。

## 逐步记录

### 剧本步骤1:三路并行 worktree + 对比表(优先级1)
三路 session 各自在自己 worktree 内完成开场 turn(串行),各自产出真实代码+bench+报告。抓取到的**真实对比数据**(来自各 session assistant_message 原文,非臆造):

| 维度 | A: Redis token bucket | B: 进程内滑动窗口 | C: envoy 配置 |
|------|----------------------|------------------|--------------|
| 代码量(git diff --stat) | 5 files, **251 insertions**(main.go/middleware.go/limiter.go/limiter_test.go/bench_test.go) | 2 新文件, **265 行**(limiter.go 144 + server_test.go 121) | 纯配置(envoy.yaml/ratelimit-config.yaml/README),无业务代码 |
| p99 代理(中间件 ns/op) | 单租户 **602.9 ns/op** / 多租户 611.2 ns/op(608 B, 8 allocs);核心 memBucket.Allow 83.87 ns/op 0 alloc | 单租户 **1273 ns/op**(1072B,12 allocs) / 多租户 3221 ns/op(6568B,24 allocs) | 本仓无法 bench(限流在 sidecar/外部 ratelimit service,不在 Go 进程) |
| 运维成本 | 高:Redis 集群可用性、连接池调优、阈值监控/动态配置 | **近零**:纯标准库、无外部依赖 | 高:需部署 envoy + 独立 ratelimit service(gRPC)+ Redis 后端 |
| 故障半径 | 小:per-tenant 隔离;Redis 宕→fail-open 降级 | 局限进程内;无网络 I/O;但多实例无法共享配额 | 数据面 envoy 故障影响所有流量;ratelimit svc 宕可配 fail-open |

**关键洞察**:数据上 **A 的中间件延迟(603ns)反而优于 B(1273ns)**——B 并非全面胜出;B 的优势在运维成本(零依赖)与部署简单,不是延迟。→ 这给了步骤3"用数据说别拿形容词"很好的压力测试点。
判定 **PASS**:三路并行 worktree 编排成立、各产出真实可 bench 的横向对比材料。数据质量高、口径一致(都给了 ns/op+allocs+运维+故障半径)。

### 剧本步骤2:砍掉 C 路(evnoy 那路不用做了)(优先级2)
- n214:C 运行中(实为刚好已完成 config turn)发 steer "evnoy 那路不用做了 太重 资源给另外俩 你先停"(手机短句+错字 evnoy 原样)。send API 返回 `status:"delivered"`。
- **观察**:C 的 turn 在 steer 到达前 ~8s 已完成(progress readme done, waiting_entered seq154 @21:04:12);steer @21:04:20 到达一个刚 idle 的 session。此后:/queue 为空、events 无任何新事件(仍停在 seq154)、status 保持 waiting、无 assistant 回应、无 turn。仅 UI 标题挂一个 "(1)" 未读徽标。
- **判定 → ISSUE L2-I1(P2,语义/反馈缺失)**:对一个刚进入 idle 的 session 发 delivery=steer,API 回 `delivered` 但消息被**静默丢弃**——不入 queue、不产生 input_received 事件、不触发 turn、无任何 ack。用户的"停/砍掉这一路"指令在 path 恰好完成的竞态下丢失且零反馈。Codex-cloud 式"运行中砍一路"若 path 抢先跑完,cut 指令无落点。(注:功能上 C 已完成产物,砍与不砍对产物无实质影响,但**指令静默失败**本身是可观测性缺陷。)

### 剧本步骤3:B比A好在哪 用数据说 别拿形容词(优先级3)
- n219:手机短句把 A/B/C 数据汇总喂给 coordinator(base session 20260719-205428),问 "B比A好在哪 用数据说 别拿形容词糊我"。default(queue)delivery → idle session **正常起了 turn**(与步骤2 的 steer-drop 对照鲜明:queue 到 idle 会起 turn,steer 到刚 idle 被丢)。
- AR 回复(n220 原文摘录):**没有**天真宣称"B 延迟更低"(裸数据 A 602.9ns < B 1273ns,B 并不占优);而是识别出"A 的 602.9ns bench 大概率是 local/Mock Redis",指出生产 Redis 单次 RTT 0.1-1ms(1e5~1e6 ns)且有网络长尾,B 的 1273ns 纯内存确定性、无 p99/p999 长尾;代码量 265 vs 251 基本持平(+14 行);运维 B 零依赖 vs A 需 Redis HA 集群=SPOF。
- **判定 PASS(强)**:数据纪律好——用真实 ns/op + 行数说话,诚实承认裸延迟 A 更快,再用"bench 口径(缺网络 I/O)"这一正确工程论证反转结论,而非形容词糊弄。唯一瑕疵:生产 RTT 数值是估算(明确标注为估算,可接受);输出用了 $...$ LaTeX 记号,手机场景略突兀(cosmetic)。

### 剧本步骤4:promote B 到主工作区(优先级4)
- n221:`POST /api/sessions/{B}/promote` → **HTTP 400 `... is not a best-of-N session`**。
- **产品语义澄清(源码 internal/cli/promote.go)**:`ar promote` 只对 `Series.Kind=="best_of_n"` 的会话生效——winner attempt 树在 `<session>/wt/att-<N>`,从 round 的 pinned base snapshot(Series.BaseRef)物化,base→winner patch 在 workspace 的 shadow repo 里生成、clean-or-nothing UNSTAGED 落到主工作区。我这条 user journey 用的是 `POST /api/worktree` + 独立 session(非 best-of-N series),因此 promote 架构上不适用。
- **→ L2-I2(P2,产品形态/发现性)**:AR 有**两条**并行探索路径且互不通:①`POST /api/worktree`+独立 session(worktree 存在,但**无 promote/聚合**);②best-of-N series(fork/run 式 fan-out,支持 promote)。Codex-cloud "best-of-N 选优 promote" 是①之外的另一套一等公民。手动三 worktree 的自然 journey 无法 promote,用户容易踩空(错误信息本身清晰,但不指路"该怎么把这路合进主干")。
- **正确的等价能力**:worktree session 用 `POST /api/sessions/{sid}/apply`(worktree "Apply-to-project" 补集,源码 handleApply/ar_test.go):把该 worktree 的改动 clean-or-nothing UNSTAGED 应用到主 checkout,冲突则 409 且主干原封不动。用它完成"B 合入主工作区"的意图(见步骤4b)。

### 建模差异(L2 与产品形态)观察
- 剧本假设"一个 agent 编排 N 路 worktree 并汇总对比"(Codex 从单一会话 fan-out)。AR 的 `POST /api/worktree` 只建 worktree,每路由**独立 session** 驱动;没有一个原生的 orchestrator session 自动聚合 N 路结果。对比表由用户(或手动喂数据的 coordinator session)编。→ 记为 L2 适配观察,非缺陷:并行探索的**基础设施**齐备,**编排层聚合**需用户参与。

## 通道/环境日志
(边跑边追加)

## ISSUE 汇总
(边跑边追加)

## 收尾状态(由主控补记)

驱动 agent 在完成剧本 1-4 步后被意外终止(**非用户操作**——用户已确认没有停它;疑似客户端断连/harness 中断,时间点与主控向用户弹确认、界面断开重合)。剩余步骤当时标记:
- 步骤 5(通宵 goal + 静默通知策略): SKIPPED(未执行)
- 步骤 6(结果?): SKIPPED
- 步骤 7(bench 回归 bisect 归因): SKIPPED
核心覆盖(三路 worktree 编排、砍一路、数据对比、promote)已完成。
**更正(L5 后补测)**:5-7 步由 L2b 补测轮完成,见 L2b-report.md。

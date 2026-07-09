# INC-D3 web_fetch / web_search 内置工具（G18b）— 设计稿

> **状态（2026-07-09 更新）：web_fetch 已实现并经安全 review 对齐；程序
> 补齐。** 本设计稿的不变量升级(收容棘轮 → egress 类统一 fail-closed)已
> 正式落为 **DESIGN 决策 #33**(§4 流程),§5/§18.5 同步。实现在 INC-5
> (web_fetch 主体)+ 本纸驱动的安全对齐:**M1** class read→execute
> (default 需审批,不静默出网;附修 plan 泄漏 + in-doubt 重跑)、**M2**
> link-local/metadata 无条件封禁(Dialer.Control 作用于已解析 IP,覆盖
> 重定向每跳)。**S1 host allowlist**(spec 级默认 deny/ask)= 开发者待裁
> 增强(execute 审批 + URL 可见是单机 dev 可辩护的弱替代);**web_search**
> 仍延后(需外部搜索 API)。安全 review 全文结论见 LOG 2026-07-09
> 「web_fetch 补程序」条。收容 fail-closed 不变量经 review 确认无绕过。
>
> 本纸使命完成,归档只读。

## 动机与 journey 锚
GAPS **G18b**（内置工具面余项）+ UJ-01。今天靠 bash+curl（在
`network=none` 收容下被拦、无红线、无截断纪律）。对标 Codex：web search
默认开（cached/live 两档）+ web fetch 内置。

## 现状（网络许可/沙箱）
- spec `sandbox.network:none` 经 `Loop.applySandbox` 棘轮式调
  `Executor.ContainNetwork`,使 bash 跑在 `unshare -r -n`（netns,仅
  loopback）；宿主无 netns 且要 none 时 bash **fail-closed** 拒跑；棘轮
  共享整棵树、永不放宽。
- 许可管线把 network 建成 effect 资源类：`pipeline.Effect.Network`,由
  `Loop.networkScope` 计算——execute-class 携 "all"（未收容）/""（已收容）；
  `mcp__` 恒 "all"（进程外,netns 不覆盖）。生效收容记进
  `EffectResolved.Containment`；mcp 与宿主内工具**不能谎称被 netns 收容**。
- 工具 = 数据 + 执行臂：def 是 defs/*.json,执行在 Execute switch；最接近
  的进程内新工具样板 = `semanticSearch`。

## 不变量冲突（核心）
收容棘轮（§18.5）今天只保证 **bash** fail-closed。一个用 `net/http` 在
**宿主 Go 进程内**跑的 web 工具,其出口**不被 `unshare -n` 覆盖**（netns
只包住 bash 子进程）——会在 `network=none` 已棘轮全树时**仍带出口**,
**静默违反"收容=全树无出口"**。

- **不变量升级**：从"bash fail-closed"升级为"**所有 egress 类 tool 统一
  fail-closed under containment**"。web 工具纳入 network 资源类模型：
  携 `Network="all"`、受 network deny/ask 规则与审批闸门治理、containment
  记账诚实（无 netns backend 覆盖进程内出口 → 收容下这类工具 fail-closed,
  镜像 bash）。
- **G16 条款补全**：不可信 web 内容加**来源/信任标记**、**不自动跟随跨
  allowlist 重定向**、可疑重定向**呈现而非静默跟随**。

## 最小安全 MVP（只做 web_fetch）
- 只交付 `web_fetch`（HTTP GET 单 URL）；execute-class；**spec 级 host
  allowlist**（默认 deny/ask）；收容下 fail-closed；size-cap + 截断 +
  redact；内容包"untrusted web content"分隔标记。
- `web_search` 需外部搜索 API + 凭据 → 延后（def 预留）。

## 波及面
- DESIGN §18.5 收容棘轮不变量升级 + §5 network 资源类 + §10 生态；
  **走不变量变更流程**。G16 落条款。
- 代码：defs/web_fetch.json（class=execute,{url, max_bytes?}）；exec.go
  `webFetch`（allowlist 校验 → containment fail-closed 检查 → net/http GET
  size-limited → redact → 包裹 untrusted 标记 → okResult）；pipeline
  network 资源类把 web_fetch 标 Network="all"；containment 下 egress 类
  统一 fail-closed。
- SPEC C 行 web fetch/search；GAPS G18/G16；QA（allowlist 命中/非命中拒绝/
  network=none fail-closed/network deny 拦截,真 HTTP）。

## 验收
- 孪生：allowlist 命中放行、非命中拒绝、收容下 fail-closed、network deny
  规则拦截、size-cap 截断、内容过 redact。
- 真实 API QA：真 HTTP + 真 provider,四场景。

## review 裁决
安全视角必做（egress 不变量、注入、redirect、allowlist）。本纸仅设计。

# INC-32 auto mode 设计稿（分类器作为 effect pipeline 的 policy 源，#57/§3.3）

**状态：📐 awaiting-review——设计先行，本稿即变更单，未裁决不实现。**

## 动机与 journey 锚

CLAUDECODE-PARITY §2 #57 / §3.3（SPRINT #18）。Claude Code 2.1.200 起
默认 auto mode：权限判定交给双层模型系统（server 端 prompt-injection
探针扫工具输出 + Sonnet transcript 分类器两阶段快筛/CoT），黑白名单
`autoMode.{allow,soft_deny,hard_deny}`（hard_deny 无条件），连拒 3 次
回退人审。对方动机=遥测显示 **93% 的 ask 被人反射式批准**——审批太多
时人形同虚设，治理精度反而下降。

我们哲学相反且**不推翻**：无人值守 fail-closed（决策 #34 headless
ask→deny）。auto mode 的移植形态不是"取消治理"，而是给 permission
关卡加一个**新 policy 源**：判定器从「规则 + mode default + 人」扩为
「规则 + **分类器** + mode default + 人」，且分类器判定**天然
journaled**（对方的分类器判定不进审计链——这是我们的结构性反超点）。

**对我们的真正价值**：headless/goal driver 的无人值守自动性正门。
现状无人值守只有两极——ask→deny（决策 #34,安全但寸步难行）或
bypass（裸放行）。auto 提供中间态：**受审计、可回退、有黑白名单兜底
的自动放行面**。

## 判定序（安全序,与 INC-16/17/18 精确咬合）

现状 `pipeline.Check` 序：hardFloor → 显式 rules（user>project>spec,
bash 逐段聚合取最严,INC-16）→ protected-path 收紧（INC-18）→
modeDefault（per-class）。

**核心裁决：分类器只接手 would-ask 面。**

```
hardFloor ──deny──────────────────────────────► Deny（恒先,不经分类器）
    │
显式 rules ──deny──────────────────────────────► Deny（用户意图,不经分类器）
    │        ──allow─────────────────────────► Allow（用户意图,不经分类器）
    │        ──ask──┐
protected/modeDefault ──ask──┤
    │                        ▼
    │              【auto mode 且分类器可用】
    │                autoMode.hard_deny 命中 ──► Deny（无条件,不进分类器）
    │                autoMode.allow 命中 ──────► Allow（白名单跳分类器）
    │                autoMode.soft_deny 命中 ──► Ask（分类器无权放行,必须人审）
    │                其余 ──► 分类器 {allow → Allow（auto-approved,记judge）
    │                              ask   → Ask（升级人审）
    │                              deny  → Deny（soft,模型可见理由,loop 继续）}
    │                分类器不可用/超时/输出不合 schema ──► Ask 原样交人
    │                                     （headless 则按决策 #34 收紧 deny）
    └─ modeDefault allow ──────────────────────► Allow（不经分类器）
```

- 显式 deny/allow 与 hardFloor **永不**经分类器——分类器只处理"本来
  要问人"的那批判定,判定面最小、成本最小、语义最忠实（对方 auto 也
  是替代 ask,不替代 rules）。
- **fail-closed 不变式**：分类器故障/超时/凭据缺失/输出不合 schema,
  一律回到 Ask（在场交人,headless 按 #34 收紧 deny）。**绝不因分类器
  故障而放行。**
- 连拒回退：fold 统计分类器**连续** deny 次数,达阈值（默认 3）该
  session 自动跃迁 auto→default（ModeChanged event,fold 可见,可手动
  切回）。防分类器与模型顶牛空转烧预算。

## 分类器的架构地位：管线机件,不是 effect

同 hooks 的地位（DESIGN §5「hooks 是管线机件,不是 effect」）：在
permission 关卡内同步调用,**不递归进管线自身**——否则分类 llm_call
又要过关卡,无穷递归。与 hooks 的差异并各自处置：

| 维度 | hooks | 分类器 |
|---|---|---|
| 成本 | 本地脚本,不计预算 | 真 LLM 调用,**usage 入 session 预算**（settle 侧补记,不做预留——单次分类成本上界小且有限） |
| 恢复 | 判定不重放,读记录值 | **同构**：EffectResolved 已落读值;未落 in-doubt 上浮,绝不静默重过 |
| 配置 | settings hooks.* | spec/settings `auto_mode.*`（model/阈值/黑白名单） |

- **model 可配**,默认 = 便宜档（gemini-flash;Anthropic 侧 haiku 类）,
  与 session 主 model 解耦。
- **输出结构化**：分类器被要求返回 `{verdict: allow|ask|deny,
  confidence, reason}`,用 INC-26 的 `internal/structured` 校验;不合
  schema = 分类器不可用（→Ask,fail-closed）。reason 落 journal 且
  deny 时作为模型可见错误理由（DESIGN §5「每种关卡结果都定义模型看到
  什么」——分类器 deny = error tool_result,loop 继续）。

## 输入面（v0 收敛为单层 transcript 分类）

分类器输入 = ①effect 摘要（tool/args/class/network scope/protected
命中/逐段拆分结果——复用 INC-16 的 splitCompound 产物）②最近 K 条
对话摘要（用户意图上下文,K 默认 6）③规则命中情况（哪层 ask 的、
protected 与否）。

对方的**双层**里 server 端 prompt-injection 探针（扫 tool 输出）v0
**不做**,拆余项 32c：它的 seam 在 post-hook 位（工具输出入 context
前）,是另一个独立增量;两阶段快筛/CoT 也留作 32b 的内部优化,v0 单发。

## Journal 与恢复面（零新发明,全复用既有纪律）

- 分类器判定落 `EffectResolved.GateResults` 追加字段：
  `classifier: {verdict, confidence, reason, model, usage}`——event
  schema **追加**,不改既有字段语义。
- ask 路径不变：分类器判 ask 时,`ApprovalRequested` 照常携带此前
  关卡判定（含分类器判定）,人审应答后 `EffectResolved` 终态汇总。
- 恢复：与 hooks 完全同构——判定不重放,恢复读记录值;进了关卡没
  `EffectResolved` 的 effect 走既有 in-doubt 上浮。
- G5/INC-17 咬合：人对分类器放行的事后否决 = 既有 `approve --always`
  反向（写 deny 规则),显式规则先于分类器,下次直接 Deny——**人恒可
  覆盖分类器**,且覆盖是持久规则不是一次性。

## Mode 语义（DESIGN §5「mode 是 loop 行为」咬合）

auto 是第 5 个 run mode（default/plan/acceptEdits/bypass/auto）,
mode=数据：
- 工具面过滤：同 default（全面）;advertised 面不变（缓存稳定）。
- prompt 注入：无（v0;可选注入一句"审批由分类器辅助"透明提示,裁决点⑥）。
- 跃迁规则：user 显式切换（既有机制）;连拒 N 次自动跃迁 auto→default
  （event 记录）;与 bypass 互斥（bypass 无 ask 可言,亦不跳 hooks 的
  既有裁决不动）。
- headless（driver）：ask 先经分类器,**仍 ask 者按决策 #34 收紧
  deny**——无人值守的 fail-closed 底线不动,auto 只是把"问人"面里
  分类器有把握的部分变成受审计的放行/拒绝。

## 触不变量吗？（PROCESS §4 对照）

- EffectResolved 追加字段、新增 mode 枚举、autoMode 规则层：**追加**,
  不改既有事件/判定语义。
- 但 permission 关卡判定器的构成变化 + "模型参与治理判定"是治理面
  重大变更 → 本稿按 PROCESS §4 走单独 review,**未裁决不实现**。
- 收容棘轮/floor/预算 reserve-then-settle/审批持久化全不动。

## 考虑过并否决

1. **分类器作为普通 effect 进管线**——否：无穷递归 + 关卡内等关卡的
   预算死锁;hooks 先例已确立"管线机件"地位。
2. **分类器替代 modeDefault**——否：read-class 也过分类器,成本爆炸
   且无收益;只接手 would-ask 面最小、语义最忠实。
3. **分类器可推翻显式 deny/allow**——否：显式规则=用户意图,模型不得
   覆盖人的持久决定（G5 语义反转）。
4. **auto 默认开启（对齐对方 2.1.200 默认）**——否：我们的默认是
   fail-closed;auto 是显式 opt-in mode（spec `mode: auto` 或 CLI
   `--mode auto`）。

## 依赖与前置（全部已就位）

G5 判定持久化（INC-17）✅ · 规则工程三件套（INC-16）✅ · protected
paths（INC-18）✅ · structured 校验（INC-26）✅ · mode 机制（S2/S3）✅
· lifecycle hooks（INC-15,连拒回退的观察点）✅。缺：分类器 prompt 与
评测集（32b 交付物,误放行率红线先定后上）。

## 实施拆分（裁决通过后,三个独立增量）

1. **32a pipeline 侧**（S/M）：auto mode 枚举 + autoMode 规则层 +
   分类器接缝（interface,可注入 fake）+ EffectResolved 追加字段 +
   连拒回退跃迁。孪生全 scripted（fake classifier,判定序矩阵：显式
   规则不经分类器/hard_deny 恒拒/故障回 Ask/连拒跃迁/headless 收紧）。
2. **32b classifier provider**（M）：gemini-flash 实现 + structured
   校验 + 预算入账 + fail-closed + 快筛/CoT 两阶段（可选）。评测集
   `qa/classifier-eval/`（N 个 effect 场景 golden 判定,误放行率红线
   =0 于 hard 类场景）。QA 真机：真 Gemini 分类器对危险命令（deny+
   理由入 journal）与良性命令（allow,EffectResolved 含 judge）的
   端到端判定。
3. **32c 注入探针层**（M,余项）：tool 输出扫描(post-hook 位)。

## 交用户的裁决点

① 要不要 auto mode（价值主张=无人值守自动性正门,成本=每 would-ask
一次分类调用）。② 判定序「只接手 would-ask」是否正确。③ 连拒回退
阈值默认 3？④ headless auto 语义（分类器过筛后仍 ask→deny）是否符合
fail-closed 哲学。⑤ 实施拆分粒度（32a/32b/32c）。⑥ auto 下要不要
prompt 注入透明提示。

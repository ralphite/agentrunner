# INC-86 默认 medium thinking + 移除 no-thinking(修 gemini-flash-latest 拒绝 thinkingBudget:0)

## 动机与 journey 锚

**锚:UJ-14(loop mode)+ UJ-18(spawn)+ 全体 Gemini 会话。** 用户在 live
webui 用 `/loop` 测真实例子时,第 1 轮 child 立刻 `gemini [provider_invalid]:
Error 400 INVALID_ARGUMENT`。

**根因(真机真值表,ar-live 稳定复现,每档多次):**
- `thinking 禁用(thinkingBudget:0)→ ❌ 400`;`thinking 启用 → ✅ OK`,与
  max_tokens 无关(8192/4096/2048 禁用全 400)。
- 即 **gemini-flash-latest 当前拒绝 `thinkingBudget:0`**(Gemini 侧模型指针
  近期变更;同二进制几小时前 session 20260721-080739 thinking-off 还好)。
- 我们 [gemini.go `toConfig`](internal/provider/gemini/gemini.go) 对任何未开
  thinking 的 spec 都发 `thinkingConfig{thinkingBudget:0}` 去"关思考"→中招。

**爆炸半径(不止 loop):** webui 默认 `DEFAULT_EFFORT="off"`(budget 0)→
**所有默认 Gemini Flash 会话(含普通聊天)当场 400**;`buildDriverAgent`/
`DEFAULT_WORKER` 硬编码无 thinking → **全部 loop/goal/best-of-N + spawn 子
agent** 也 400。

## Spec delta

`SPEC.md` C 表(webui)对应行补注/或新增一条:reasoning effort 默认 medium、
移除 "off" 档;所有 webui 生成 spec(主 agent/worker/driver agent)恒带
thinking。挂 UJ-14/18,锚 QA-0721b。

## Design delta

**不触后端不变量。** 本增量为 webui 前端 spec 生成层(specs.ts)修复:effort
模型删 off、默认 medium、生成器补 thinking。DESIGN 无需改。

**遗留(后续 provider 加固,单列):** 后端 gemini `toConfig` 的 budget:0 分支
未动——CLI 手写 thinking-off spec 仍会 400。真正根治应让 provider 对拒绝
budget:0 的模型(flash-latest)也走 pro 那条"给最小正 budget/交给模型"路径,
而非硬发 0。列 GAPS G41,待裁决(触及"会话死亡"防御逻辑,需单独 review)。

## 验收

**枚举型交付物(webui/frontend):**

| # | 改动 | 断言 |
|---|---|---|
| 1 | EffortId/EFFORT_LEVELS 删 "off";DEFAULT_EFFORT off→medium | vitest 绿;pill 默认显 Medium |
| 2 | modelBlock 永不发无 thinking 块(budget≤0 兜底 medium) | 生成 spec 恒含 thinking |
| 3 | buildDriverAgent→modelBlock(medium);DEFAULT_WORKER/DEFAULT_DRIVER_AGENT 补 thinking | 三处 model 行均 budget_tokens:6144 |
| 4 | Composer /reasoning 别名 off/none→light;pill 恒显 effort 标签 | tsc/vitest 绿 |
| 5 | Composer.effort.test 默认断言 off→medium | vitest 630/630 |

**闸门 A:** 前端 `npm run build`(tsc+vite)绿 + vitest **630/630** 绿。

**闸门 B(QA-0721b,真机):** 部署到 live 8809 后,**真浏览器 `/loop`**(与用户
同路径)跑真实代码走查(editable_mermaid2,interval 30s,3 轮)。断言:
- child 不再 INVALID_ARGUMENT;worker 正常 Running→完成;
- 3 轮迭代真实推进(每轮审一个文件、追加 REVIEW_NOTES.md);
- 对照:修前同 `/loop` 第 1 轮即 child_failed(session 20260721-162833)。

**QA 结论:PASS**(2026-07-21,真浏览器 /loop,session 20260721-165144-loop)。
child 零 INVALID_ARGUMENT;Iteration 1/2 Completed(worker 27k/39k tok)、
Iteration 3 overlap-skip、`max_iterations` 干净收尾;真项目
`/Users/yadong/dev2/editable_mermaid2/REVIEW_NOTES.md` 累积 2 条真实审查发现
(mermaidParser 界定符/边 ID bug、MermaidRenderer useEffect 循环+securityLevel
XSS)。对照:修前同 /loop 第 1 轮即 child_failed(session 20260721-162833)。

## 实施步骤

1. INC-86.1:前端改 + 测试更新;build+vitest 绿;commit+push;deploy 8809。
   完成标志:pill 默认 Medium、生成 spec 带 thinking。**已完成(0450b893)。**
2. INC-86.2:真浏览器 /loop 复验(闸门 B),delta 并回 SPEC/GAPS(G41)/LOG/
   QA;工作纸归档;commit+push。

## review 裁决

裁三视角对抗 review。理由:改动面=前端 spec 生成字符串 + effort 枚举 + 两处
UI 引用,零后端/控制流/并发;build+vitest 全覆盖;真实行为由闸门 B 真浏览器
直证。provider 根治(G41)另立项时再按不变量流程走。

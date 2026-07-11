# INC-51 Web UI Markdown 渲染增强（react-markdown + GFM 表格 + 语法高亮 + line-wrap）

**状态：🔧 A 闸绿，B 闸（真浏览器）待验（2026-07-11，SPRINT-handa-parity
#20 / 并行轮，worktree 子 agent A）。** 方案以 HANDA-PARITY §2 第 #20 行
（review CONFIRMED）为准。A 闸=vitest 5 新例（`Markdown.test.tsx`）+ 既有
103 例全绿 + `tsc -b` + `vite build` 打包通过 + 仓库根 `scripts/check.sh`
全绿；B 闸=真浏览器 DOM 断言（表格/高亮/line-wrap/转义），由集中验收做。
**additive webui 增强，不触 DESIGN 不变量**（无 §15 决策、无各章粗体条款
变更）。规模 M，小增量——三视角重 review 裁掉（理由见末节），唯一安全红线
（禁 raw HTML 注入）以单测正面钉住。

## 动机与 journey 锚

UJ-24（Web UI 驾驶 AgentRunner）的单一 task thread 里，assistant/runtime
消息正文用自研极简 `Markdown.tsx` 渲染——**无表格、无语法高亮、无
mermaid、无 line-wrap 控件**。对标 Codex/Claude 的富文本答复，纯 `<span>`
式表格与未高亮代码块观感落后（HANDA-PARITY #20，"已实现两份 markdown-*.md
参考"）。本增量换用成熟 markdown 引擎补齐表格与高亮，并加代码块 line-wrap
开关，同时**守住"禁 raw HTML"安全红线**。

## Spec delta

- SPEC.md「N · Web UI 消费面」区新增一行（紧随 INC-36 用户消息折叠行）：
  **Web UI Markdown 渲染增强**（react-markdown + remark-gfm 表格/删除线/
  任务列表 + 按需 highlight.js 语法高亮 + 每代码块 line-wrap 开关；**禁
  raw HTML**：react-markdown 默认转义，无 rehype-raw，无 HTML 注入面；
  对外 `<Markdown text>` prop 不变）。锚 = `frontend vitest
  Markdown.test.tsx`（表格/高亮/line-wrap/raw-HTML 转义 4 断言）· B 闸真
  浏览器 DOM 断言。状态记 ⚠️（A 闸绿、B 闸待），收口转 ✅。
- HANDA-PARITY §2 #20 行 + SPRINT-handa-parity #20 行状态随后（收口时
  由集中合并者跟改，避免并行轮抢改冲突）。

## Design delta

**无。** 本增量是纯前端 additive 增强：

- 不改任何 daemon/loop/event/journal 契约，不动 §15 决策表，不动各章粗体
  不变量。
- **安全不变量维持而非变更**："禁 raw HTML 注入"是既有性质（旧
  `Markdown.tsx` 靠"从不 `dangerouslySetInnerHTML`、只构 React 节点"实现）。
  新实现用 react-markdown 默认转义（**不引入 rehype-raw**）延续同一性质，
  CSP/离线红线不变（react-markdown / remark-gfm / highlight.js 均纯 npm 包
  打进 bundle，无外部请求）。故不走 PROCESS §四不变量变更流程。

若后续 mermaid 懒加载尾巴引入运行时 `import()`，需复核离线/CSP 条款——本
轮不做（见余项）。

## 验收（A 闸孪生 · 枚举逐项对锚）

`webui/frontend/src/components/Markdown.test.tsx`（`@vitest-environment
jsdom`，`@testing-library/react` 交互）：

| 交付项（枚举） | 断言锚 |
|---|---|
| GFM 表格 | `renders GFM pipe tables into a bordered .cx-table`：`table.cx-table` 存在、th=`[Head A, Head B]`、td=`[r1c1, r1c2]` |
| 语法高亮（已注册语言） | `syntax-highlights fenced code with highlight.js token spans`：`pre.md-hljs code.hljs` 带 `language-js`、`.hljs-keyword`="const"、`.hljs-number`="1"、header lang 标签="js" |
| 未注册语言不崩 | `leaves an unregistered language unhighlighted but still a block`：仍是 `pre.md-hljs` 块、含原文、无 `.hljs-keyword` |
| line-wrap 开关 | `toggles line-wrap on the code body`：默认 `overflow-x-auto` 且无 `whitespace-pre-wrap`；点 Wrap→`whitespace-pre-wrap`+`break-words`；再点→复原 `overflow-x-auto` |
| **禁 raw HTML（安全红线）** | `escapes raw HTML — no injection surface`：`<img onerror>`/`<script>` 不生成 live 元素、无元素带 `onerror` 属性、innerHTML 无 `<img`/`<script` live 标签、payload 仅作转义文本存活、合法 `**bold**` 仍渲染 |

裁掉项显式声明：
- **mermaid**：本轮不做（见余项），无对应锚。
- **diff fence 专属渲染**：`diff` 语言已注册走通用 highlight（+/- 行着色），
  不做 Codex 式并排 diff 视图，无独立锚。
- **copy 工具栏**：沿用旧实现的 Copy 按钮（既有能力，非本增量新增），line-wrap
  为新增按钮并列其左。

**B 闸（真浏览器，交由集中验收）**：`arwebui` 起真 daemon，跑一个让模型
输出含"表格 + 多语言代码块（js/go/python）+ 超长单行代码 + 形如
`<script>` 的字面文本"的真实 turn，在浏览器 DOM 断言：(1) 表格渲染为
`.cx-table` 且横向可滚不撑宽；(2) 代码块出 `.hljs-*` 着色 span、header 有
lang 标签；(3) 点 Wrap 后 `<pre>` 由横滚变软换行、再点复原；(4) 字面
`<script>` 显示为可见文本、DOM 中无 `<script>`/`<img>` 元素、无 `onerror`
属性（安全红线）；(5) light/dark 两主题 token 配色都可读（token 色映射到
既有 `--violet/--blue/--green/--amber/--ink-2/--dim/--red` 主题变量）。

## 实施步骤（已落）

1. 依赖：`+react-markdown@9 +remark-gfm@4 +lowlight@3 +unist-util-visit@5`
   （运行时）、`+@testing-library/react +jsdom`（devDep，不进 bundle）；
   **不用 `rehype-highlight`**（它静态 `import {common}` 拉全 35 语言、活
   引用无法 tree-shake）。
2. `src/components/highlight.ts`：自写精简 rehype 插件，`createLowlight`
   （跑 `highlight.js/lib/core`，零语言）+ 按需注册 19 语言（bash/c/cpp/
   csharp/css/diff/dockerfile/go/ini/java/javascript/json/markdown/python/
   rust/sql/typescript/xml/yaml，别名 tsx/jsx/shell/… 随语法自带）。只处理
   `pre>code`，未注册语言原样返回不抛。
3. `src/components/Markdown.tsx`：`<Markdown text>` 内部换 `<ReactMarkdown
   remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>`；组件覆盖
   映射到既有 class（`.md-p/.md-h/.md-list/.md-quote/.cx-table`）保持观感；
   `pre` 覆盖为 `CodeBlock`（slim header + lang 标签 + Wrap 开关 + Copy；
   body 为高亮 span，Wrap 态 `whitespace-pre-wrap break-words` ↔ 默认
   `whitespace-pre overflow-x-auto`）；`a` 覆盖加 `target=_blank rel=noreferrer`。
   两处调用点（Timeline.tsx assistant/runtime）零改动。
4. `src/styles.conv.css`（A6 段）：加 highlight.js token 配色（映射主题变量，
   双主题可用，不引外部 hljs 主题 CSS 保离线）；块内 `<code>` 去 `.md code`
   inline chip 化。
5. A 闸：vitest 108 全绿 + `tsc -b` + `vite build` 通过；`scripts/check.sh`
   全绿。**dist 不提交**（由集中合并者三增量统一 clean rebuild）。

## 余项

- **mermaid 图**：作可选懒加载尾巴（`import("mermaid")` on demand），避免
  bundle 膨胀；需先复核离线/CSP 条款（动态 chunk 仍须内嵌无外部请求）。
  本轮不做，记 HANDA-PARITY #20 尾项。
- **bundle 观测**：本增量 JS 866 KB（gzip 245 KB，较基线 613 KB +253 KB，
  主要为 react-markdown/micromark/mdast/hast 管线 + highlight.js core+19 语言；
  `common` 35 语言已确认 tree-shake 掉）。若日后预算吃紧，可把 highlight 拆
  懒加载 chunk。

## review 裁决

小增量，**裁掉三视角重 review**（正确性/并发无新并发面；契约无 DESIGN
delta）。唯一横切风险是**安全（HTML 注入）**——不做独立 review，改由
`Markdown.test.tsx` 的 `escapes raw HTML` 例正面钉死（无 live 标签/无
`onerror` 属性/payload 仅转义文本），并在本纸 Design delta 显式记"禁
rehype-raw"为延续既有安全性质。收口时若 B 闸暴露注入或渲染缺陷，按 runtime
bug 修复重跑。

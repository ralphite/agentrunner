# INC-36 用户消息折叠（HANDA-PARITY #23）

## 动机与 journey 锚

长粘贴（日志/diff/多段指示）把 timeline 撑成一屏用户消息，读对话要
翻页。对标 handa user-message-collapse：用户气泡渲染高度超 ~10 行默认
折叠，`Show more`/`Show less` 展开收起。journey 锚：UJ-04（贴日志）/
UJ-24（webui thread 可读性），不新增 journey。HANDA-PARITY §2 #23。

## Spec delta

- SPEC I 区加一行：webui 用户消息折叠（>10 渲染行折叠 + Show
  more/less；按渲染高度测量，wrap 后的长行也计；纯前端态）。

## Design delta

- 无（webui 展示层，DESIGN 无此层不变量）。

## 验收

- A 闸：frontend build + 既有 vitest 全绿（INC-23 先例：组件 DOM
  测量不入 jsdom 单测）。
- B 闸（真浏览器，共享 daemon）：>10 行消息默认折叠且 Show more 全
  文展开可收起；短消息无折叠钮；窄宽 wrap 导致的超高同样折叠。

## 实施步骤

1. Timeline.tsx `CollapsibleUserText`（max-height 钳 10 行 +
   scrollHeight 探测 + ResizeObserver 随宽度重测）+ styles.css；
   build 绿 → 真浏览器验证 → commit。

## review 裁决

小增量（S，单组件+样式），裁掉三视角 review；B 闸真浏览器覆盖交互。

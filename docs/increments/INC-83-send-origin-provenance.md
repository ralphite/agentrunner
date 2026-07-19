# INC-83 · send 溯源:origin 透传(提案,待裁决)

**日期**:2026-07-19 · **来源**:QA-0719 091500 深审实证——webui 发出的
消息在 journal 里标 `source:"cli"`(arwebui 的 send 走 runAR CLI 转发),
审计时无法区分 webui/CLI 真实入口,一次排查被误导(见 LOG 同日条目)。

## 动机与 journey 锚

UJ-24(webui 驾驶)/审计溯源:journal 是 source of truth,但 input 的
入口通道信息在 webui→CLI 转发处丢失。排障与安全审计(如"这条消息谁
从哪发的")需要真实入口。

## 为什么不能直接改 source

`protocol.UserClassSource`(INC-12.3,决策 #30 canonical)白名单:
`""|user|cli|unix-socket`。source 同时承担 **user-class 语义**(revive
权、last-turn baseline 判定)。把 webui 消息的 source 改成 "webui" 会
使其变非 user-class——行为破坏;扩白名单则动决策 #30,须走不变量流程。
结论:source 语义保持不动。

## Spec delta(拟)

- `daemon.Command` 增可选 `Origin string`(webui|cli|schedule|hook|…,
  空=cli 兼容);`ar send` 增 `--origin` flag(默认空)。
- `InputReceived` payload 增可选 `origin` 字段,fold/投影透传;
  webui timeline 的消息 tooltip 可显示入口。
- arwebui `handleSend`/`runAR send` 传 `--origin webui`。
- source/trust/principal 全部不变;`UserClassSource` 不动。

## Design delta

无不变量变更(纯附加字段)。事件 schema 兼容性:旧 journal 无 origin
→ 空值,渲染回退到 source。

## 验收(拟)

- Go:webui handleSend 发送后 journal 的 input_received 携
  origin:"webui";CLI 直发无 origin(或 "cli")。
- 真机:webui 发一条消息,`ar events` 导出可见 origin,timeline
  tooltip 显示入口。

**状态**:提案。落地前需用户/owner 裁决字段命名与展示位置。

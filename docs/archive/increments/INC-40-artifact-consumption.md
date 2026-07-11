# INC-40 artifact 消费面（HANDA-PARITY #11）

## 动机与 journey 锚

`publish_artifact` 只有发布半边：模型不能读回自己（或前任/队友）发布
的 artifact，人侧无 CLI/webui 消费面——发布物只能翻 CAS 目录。对标
handa artifacts_list/read（窗口分页）。journey 锚：UJ-06/18/24
（artifact 发布与监督消费），不新增。PARITY §2 #11（review
CONFIRMED 纯 additive：ArtifactPublished 已 journaled 已 fold，CAS
读 API 齐备）。

## Spec delta

- SPEC C 区加行：artifacts_list / artifacts_read（loop 内部 read 工具：
  list=fold `Published` 快照+store 补 bytes；read=stream[@version]，
  UTF-8 文本分页 offset/max_bytes+next_offset，二进制回 metadata；
  不过管线）。
- SPEC I 区：CLI `ar artifacts <sid> list|read <stream>[@vN]`；webui
  Supervision Artifacts 区 + 只读查看器（收口时并入现有行或加行）。

## Design delta

- 无新事件、无不变量。工具走 goal/progress 同 seam（drive-goroutine
  快照 + `l.Artifacts` 直读）；list 以 fold `Published` 为真相（只列
  journaled 事实，orphan blob 不入列——publish 的 crash 语义保持）。

## 验收

- 孪生：publish→list 见 stream/version→read 全文/分页/越界/
  version 寻址/二进制 metadata/无 store 报错。
- B 闸（真 Gemini）：模型 publish 后自发 read 回内容并引用；CLI
  list/read 人验；webui Artifacts 区渲染。归档 qa/runs/。

## 实施步骤

1. defs + loop seam + runArtifactsTool + 孪生 → check 绿。
2. CLI 子命令 + webui Artifacts 区/查看器 + 真验 → 文档齐活 → commit。

## review 裁决

M 增量、纯 additive 读面，裁掉三视角 review；双闸覆盖。

---

## 执行记录（2026-07-11 收口）

两步并轮完成。B 闸真 Gemini 一会话全链（publish→list→read→
READBACK 与 CLI 逐字一致）+ CLI 人验 + webui DOM 断言/截图（console
0 错误），证据 `qa/runs/2026-07-11-INC40/`。模型自发用
progress_update 维护 checklist（INC-37 自然采用佐证）。SPEC C/LOG/
SPRINT 齐活；SPRINT #11 ✅，批 1 五项全落。

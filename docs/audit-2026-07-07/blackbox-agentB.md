# 黑盒报告 B:一次性编码任务(让 AI 写并跑通脚本)

结局:**做成了**(merge.py 真实生成、独立验证可用),但撞了 **6 堵墙**、靠两次运气才通关。对只看 --help 的新手"几乎不可能自己走通"。

## 摸索链(逐步撞墙)
1. 根目录无 README → 只能硬碰工具。
2. `ar --help`/`-h`/`help` 全 `unknown command`,顶层 usage 20+ 命令零解释。靠 usage 尾巴 `[<spec.yaml> "task"]` 猜出 `run` 最像跑任务。
3. `run -h` 只列 flag,不说 spec 内容。
4. 逐字段报错反推 spec:`name`→缺 `model.provider`→缺 `model.id`→缺 `system_prompt`(这条报错友好,给了字段名)。
5. 模型名盲试:`gemini-2.5-flash`/`1.5-flash`/`2.0-flash` 全 404,`gemini-flash-latest` 才中。
6. spec 全过但 AI 只"说要创建 hello.py",不落盘 → `-json` 发现**默认一个工具都没有**。
7. 系统性反推字段:故意写错 `tools:` → 报错**吐出完整工具白名单** `[bash edit_file ... write_file]`(真名 write_file/bash),也暴露 permissions/sandbox 等。
8. 加 `tools:[write_file,read_file,bash]` 仍不落盘 → `-json` 发现 `tool_call write_file → {"error":"denied by policy"}`。
9. `-mode bypass` 才放行 → 真写文件、真跑 bash、结果回灌。独立验证脚本可用。

## 问题(按杀伤力)
### [BB-B-1] 工具默认被禁,且无处得知怎么放行 🔴致命
默认 spec 下 AI 无工具(空谈);加 `tools:` 仍 `denied by policy`;真正开关是 `-mode bypass`,但 --help/报错/示例**从不讲"工具被禁→怎么放行"**。四个 mode 值只有名字零解释,新手不可能知道 bypass 是让 AI 真写文件的钥匙。(注:spec 里 `permissions:[{action:allow}]` 也能放行,但同样无任何引导/示例,naive 用户发现不了。)

### [BB-B-2] spec.yaml 无示例/模板/文档,字段全靠报错反推 🔴致命
从 name 到能跑撞 4 堵纯字段墙;无 README、run -h 不给结构、无 `ar init`。`tools` 合法值靠"故意写错让报错吐白名单"才拿到——纯运气,非引导。

### [BB-B-3] 模型 id 无默认/无候选,盲试到不 404 🟠严重
`model.id` 必填不给合法值,猜 4 次才中。provider 选定后无推荐默认 id。

### [BB-B-4] `-h`/`help`/`--help` 全不认;`ar trust -h` 直接崩 🟠严重
顶层三种求助全 `unknown command`;`ar trust -h` 把 -h 当文件 → `lstat -h: no such file or directory` 崩溃,观感"工具很脆"。

### [BB-B-5] 顶层 usage 列 20+ 命令零解释,无法判断用哪个 🟡糙

### [BB-B-6] 工具被拒/轮数不够时 AI 退回"贴代码让你自己跑"——看着成功实则没做 🟡糙(有迷惑性)
磁盘什么都没有却像完成了,新手易误判。

## 成本
约 25 分钟、~16 条命令(~10 次真实 run)、6 堵墙:无入口→model.provider→model.id(+盲试4次)→system_prompt→默认无tools(空谈)→有tools仍denied(靠bypass)。

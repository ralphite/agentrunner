# Agent 6 报告: 生态接入 (MCP / skills / memory)

**核心结论**: memory (CLAUDE.md 注入 + 层级合并) 和 skills 两个 ✅ 域完全通过、行为学证据确凿。但标为 ✅ 的 **MCP 域在产品里根本没有接线** —— DESIGN.md §9 文档化的 `mcp:` spec 字段在 `AgentSpec` 里不存在,任何用户都无法配置 MCP。更糟的是 daemon 的 `ar new` 对加载失败的 spec 会**返回成功并派发一个幽灵 session ID**。

## 一、测试记录(逐消息使用日志)

### 场景 1: CLAUDE.md 注入生效
**夹具** `ws1/CLAUDE.md`(无 `.git`):
```
# 项目暗号规则（硬性）
- 每次回答的第一行必须原样输出：MEMO-OK
```
1. `$ ar new --workspace ws1 base.yaml "用一句话解释什么是二分查找。"`
   → 派发 `20260707-233957-task-3a1b`,一 turn 后 waiting:input。
   → 模型回复: `MEMO-OK\n二分查找是一种在有序数组中…折半…的搜索算法。`(**第一行确实是 MEMO-OK**)
   → 验证: events.jsonl 的 `session_started.memory` 字段确认注入 `<memory># CLAUDE.md (.)…MEMO-OK…</memory>`。行为+注入双证。
2. `$ ar send <sid> "你的 instructions 里有没有关于'回答第一行'的任何要求？如果有，把那条规则原文复述一遍。"`
   → 模型: `MEMO-OK\n我的 instructions 中确实有一条…原文如下：- 每次回答的第一行必须原样输出：MEMO-OK…`(既遵守又能逐字引用)
   **判定: PASS**

### 场景 2: CLAUDE.md 层级合并
**夹具**: `ws2/`(git init,repo 根)外层 CLAUDE.md(暗号 OUTER-42);`ws2/sub/CLAUDE.md`(暗号 INNER-99)。workspace 设为 `ws2/sub`。
1. `$ ar new --workspace ws2/sub base.yaml "用一句话解释什么是快速排序。"`
   → 模型回复末尾: `OUTER-42\nINNER-99`(**两层都遵守**)
   → 验证: memory 里两个 `# CLAUDE.md` 段都在,外层先渲染、内层后渲染——与 `internal/memory/memory.go`「outermost first,nearest last」一致。
2. 反向对照 `$ ar new --workspace ws2 base.yaml "1+1等于几？只回答数字。"`
   → 验证: memory 只含 OUTER-42,**不含 INNER-99**——memory 只向上 walk 到 git 根、从不向下扫子目录(设计如此,见 A6-4 观察)。
   **判定: PASS**

### 场景 3: skills 加载与使用
**夹具** `ws3/.claude/skills/git-oneline/SKILL.md`(带 frontmatter):正文含 `git log --oneline -n 5` 和暗号 `SKILL-SECRET-7788`。
1. `$ ar new --workspace ws3 base.yaml "我想把最近的 git 提交压成单行摘要。你有没有相关的 skill 可以参考？如果有，读它的正文并严格按正文要求作答。"`
   → 模型 turn1: `read_file(path=".claude/skills/git-oneline/SKILL.md")`(**自主发现并读正文**)
   → 模型 turn2: 给出 `git log --oneline -n 5`,并输出 `**SKILL-SECRET-7788**`
   → 验证: `session_started.skills` 只注入目录行;body 靠 read_file 按需加载(与设计一致);secret 出现证明 body 真被读到。
   **判定: PASS**

### 场景 3b: skills 坏格式降级
**夹具** `ws3b/.claude/skills/` 下: good/(正常)、broken/SKILL.md(缺 frontmatter 围栏)、empty-dir/(无 SKILL.md)。
1. `$ ar new --workspace ws3b base.yaml "列出你能看到的所有 skill 的名字。"`
   → 模型只列出 good;daemon 日志 `WARN "skill discovery issues; continuing with the well-formed ones" err="skills: malformed frontmatter in broken"`。会话未崩。
   **判定: PASS**(体面降级 + 清晰 warning)

### 场景 4: MCP stdio 接入 —— 被测物根本没有接线
**夹具** `mcp_designdoc.yaml`,严格照抄 DESIGN.md §9 line 672 文档化语法:
```yaml
mcp:
  - name: demo
    transport: stdio
    command: ["python3", "/tmp/echo_mcp.py"]
    allowed_tools: [echo, add]
```
1. `$ ar run --workspace ws4 mcp_designdoc.yaml "只回复 OK 两个字"`
   → `spec …: yaml: unmarshal errors:\n  line 6: field mcp not found in type agent.AgentSpec`
   → 验证: `internal/agent/spec.go` AgentSpec **无 mcp 字段**,`dec.KnownFields(true)`(line 118)硬拒;全仓无任何 `Loop{}` 构造点设置 `.MCP`;go-sdk MCP client 只在 `internal/mcp/mcp.go` 内被引用且无 caller 建立 stdio transport。→ `internal/mcp` 对产品是死代码,只被单测触达。
   **判定: FAIL**(功能对用户完全不可达,却标 ✅)
2. 另试 map 形式(`mcp: {demo: …}`)、`mcp_servers:` 键 —— 全部 `field … not found`。
   **场景 5/6(断连恢复、写审批)BLOCKED**: 无用户可达路径接入 MCP。

### 场景 7: MCP server 命令不存在
`$ ar run … mcp_nonexistent.yaml` → 同样 `field mcp not found`,走不到执行层。

### 场景 8a: MCP http transport(🧊)
`$ ar run … mcp_http.yaml` → `field mcp not found`。体面拒绝、不崩。

### 场景 8b: 记忆写回 # remember(❌ G9)
1. `$ ar new --workspace ws5 base.yaml "# remember 我偏好用 tab 缩进而不是空格"`
   → 模型当普通对话回复"已记住";`ws5/` 下**没有生成 CLAUDE.md**;无崩溃。G9 正确地未实现。
   **判定: PASS**(正确的"未实现"行为)

## 二、发现的问题

### [A6-1] MCP 全域对用户不可达:文档化的 `mcp:` spec 字段在代码里不存在 🔴 critical
- SPEC H 域标 ✅"MCP stdio 全生命周期",实为零可用。
- 复现: 任何含 `mcp:` 的 spec → `field mcp not found in type agent.AgentSpec`。
- 期望依据: docs/DESIGN.md §9 line 672-676 + §10「### MCP」+ docs/SPEC.md:127 ✅。
- 实际: AgentSpec 无 mcp 字段;KnownFields(true) 硬拒;无任何 CLI 构造点给 Loop.MCP 赋值;无代码建立 stdio transport。internal/mcp = 死代码。
- 证据: agent6/mcp_designdoc.yaml、mcp_mapform.yaml、mcp_serverskey.yaml;internal/agent/spec.go:47-89,118。

### [A6-2] daemon `ar new` 对加载失败的 spec 返回成功 + 派发幽灵 session ID 🟠 major
- 复现(daemon 起着): `ar new --workspace ws1 mcp_designdoc.yaml "用 echo 工具重复 hello mcp"` → **rc=0**,stdout 打印 sid,stderr 提示 send 用法;但 sessions/ 目录没创建、events.jsonl 不存在、daemon 日志无 error;`ar send <该id> "hello?"` → `no live session … could not be resumed`。
- **非 MCP 专属**: 仅"缺 model.provider"的 spec 同样复现——前台 `ar run` 诚实报错,daemon `ar new` rc=0 + 幽灵 ID。
- 根因: `internal/daemon/daemon.go` handleRun——line 588 在加载 spec **之前** mint ID;line 633 立即 Encode(KindSessionStart);真正 s.Run(内含 LoadSpec)在 goroutine 里失败时 hub.Emit(KindError),但 CLI new 收到 SessionStart 就 detach 退出(internal/cli/conversation.go:54-56),**错误 emit 进没人读的流**。
- 证据: agent6/dnew_out.txt、dnew_err.txt、bp_out.txt、bp_err.txt。

### [A6-3] 独立复现 [A2-2]:超长 XDG 路径 daemon 启动失败 🔴(已登记,不重计)

### [A6-4] (观察,非 bug) memory 合并只向上不向下,易违用户直觉 🟡 minor
- workspace=ws2 时 ws2/sub/CLAUDE.md 不被加载。真实用户常在项目根开会话、在子目录放模块级 CLAUDE.md,会以为生效。建议文档显眼点明。

### [A6-5] (观察,cosmetic) 合并后外层 CLAUDE.md 标签是超长绝对路径 🟡 minor
- filepath.Rel 得 `..` 触发绝对路径 fallback(internal/memory/memory.go:80),注入文本略脏、略占 token。

## 三、turn 计数
- `activity_completed` 总数 = **23**(ar6:16 + a6x:7),跨 6 个 materialized session;`assistant_message`=15;`input_received`=7。
- daemon 日志无 panic/fatal。

## 四、没测到的
- MCP 断连恢复、MCP 写审批、schema 记录(tools_discovered)、allowed_tools 收窄:因 A6-1 无用户可达路径,产品路径下无法验证。
- daemon 长时驻留稳定性(本环境后台进程屡被 harness 回收,用单 shell 内起测绕过)。

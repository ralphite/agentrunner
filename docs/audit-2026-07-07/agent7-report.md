# Agent 7 报告：长会话压力与并发

## 一、测试记录

### 基础设施踩坑
- **独立复现 [A2-2]**：超长 XDG（$T/xdg 达 137 字节）→ daemon `bind: invalid argument`（macOS unix socket ~104 上限）。改短路径 /tmp/ar7v2 绕过。
- **turn 计数语义**：无独立 tool_call/tool_result 事件；工具与 LLM 都是 activity_started/completed。真实"agent loop"= generation_started。

### 马拉松会话（主任务）— 跑了 4 个项目，全部死于同一 bug
每轮死亡后按纪律开新会话续跑。

**马拉松1（todo.py, SID …06b1）— 死于 R6/turn24**
| 轮 | 我发的消息（摘） | 模型反应 | 耗时 | 独立验证 | 判定 |
|---|---|---|---|---|---|
| 1 | 建 todo.py，add/list/done，argparse，写完 read_file 确认 | write_file+read_file，1666B | 11s | 文件存在 | ✅ |
| 2 | 实测 add/list/done/done1/list，贴真实输出 | 5 activity，bash 实跑，准确解释"内存不持久" | 8s | — | ✅ |
| 3 | 加 JSON 持久化 todos.json，实测持久化 | 多工具，实测 add→退出→list 保留 | 13s | — | ✅ |
| 4 | 加 delete 命令，实测删除生效 | 改码+bash 实测 | 12s | — | ✅ |
| 5 | 写 3 个 unittest 到 test_todo.py 并跑 | **仅 1 LLM，0 工具，返回空-parts 消息（seq269, parts=0），未写测试** | 10s | 工作区无 test_todo.py | 🔴 |
| 6 | 提示没看到测试文件，请真的创建并跑 | **turn24 组装含空-parts 历史 → `has no parts` → activity_failed(retryable:false) → session_closed(error)** | — | send 后 closed，再 send 仍崩溃死循环 | 🔴 死亡 |

**马拉松2（todo.py, SID …59bd）— R1 首轮即空-parts**
| 1 | 建 todo.py add/list/done，务必 write_file+read_file | **assistant parts=0 但 activity output_tokens=1868**（模型生成 1868 token 却被折叠成 0 part），工作区空 | — | 无文件 | 🔴 首轮中毒 |

**马拉松3（todo.py, SID …f4f9）— compaction 后 R9 空-parts 死亡**
| 轮 | 消息（摘） | 反应 | 耗时 | 判定 |
|---|---|---|---|---|
| 1-7 | 逐步 add/list/内存改/done/JSON持久化/bash实测/delete | 各正常，实跑贴输出 | 4-16s | ✅ |
| 8 | 加 priority 字段 | **触发自动 compaction（seq360, upto_genstep=31, dropped=31, summary 2755 字符准确）** | 28s | ✅+compaction |
| 9 | 写完整长 README.md | **parts=0 空（seq438, gen_step38）** | 10s | 🔴 |
| 10 | 加彩色输出 | session_closed，no-parts afail | 2s | 🔴 死亡 |
- 死前独立验证：todo.py list → `1.[mid]第二个任务 / 2.[high][x]高优先级任务`，todos.json 结构正确。**compaction 摘要正确，会话仍死于独立的空-parts。**

**马拉松4（calc.py, SID …05dc）— 稳跑 10 轮后 R11 空-parts 死亡**
| 轮 | 消息（摘） | 反应 | 耗时 | 判定 |
|---|---|---|---|---|
| 1-8 | calc.py add/sub/mul/div(除0保护)/read确认/bash实测/float重构/pow | 各正常改码实测 | 4-8s | ✅ |
| 9 | **记忆检查点②：第3轮加的什么子命令** | **正确答"mul，计算 a*b"** | 4s | ✅ 上下文保真 |
| 10 | 加 history.txt + history 命令 | +10 | 10s | ✅ |
| 11 | 写 test_calc.py 4 测试并跑 unittest | **parts=0 空（仅 1 LLM，0 工具）** | 16s | 🔴 |
| 12 | 加 mod | session_closed，no-parts afail | 2s | 🔴 死亡 |
- 独立验证：calc.py add 5 7→12.0、pow 2 10→1024.0，浮点重构生效。记忆检查点②通过（compaction 前）。

### 场景2：多会话并发（3 会话交错）— ✅ 完美
- A/B/C 三会话不同 workspace，暗号 红苹果/绿梨子/蓝葡萄。R1 各回"已记住"；R2 交错中性任务各自正确；**R3 三个 send 后台并行同时打 daemon 问暗号 → A=红苹果、B=绿梨子、C=蓝葡萄，零串扰**。sessions list 三个都在，daemon.log 无 error/panic，fd 13→25。

### 场景3：会话数量压力（6 会话快开快关）— ✅ 完美
- 快速连开 burst1-6 全到 waiting:input。逐个 close 全 rc=0；**fd 精确回收 37→25（释放 12=6×2），无 fd 泄漏**；无进程泄漏；6 会话→closed，daemon 存活。

## 二、发现的问题

### [A7-1] 空-parts assistant 消息毒化历史，导致长会话必死且无法恢复 🔴 critical
- 长会话续命 UJ-09 的致命缺陷，跨会话高频复现。
- 复现：gemini-flash-latest 连续多轮开发，在较深轮次（turn 24/38/40+）或模型需产出较多内容时，gemini 返回 **parts 为空的 assistant 消息**（观测到一次 output_tokens=1868 却 parts=0，模型确实生成内容却被 adapter 折叠丢空）。空消息写入历史 → 下一轮组装请求把它发给 gemini → 报错。
- 期望：UJ-09"长会话续命"要求聊到数百 turn 靠 compaction 持续运行；SPEC A 域"自动 compaction ✅ S3"。
- 实际：`WARN daemon: hosted resume failed session=…06b1 err="turn 24: gemini: message with role \"assistant\" has no parts"`；events.jsonl `activity_failed{class:internal, message:"...has no parts", retryable:false}` → `session_closed reason=error`。**session 永久 wedge**：再 send 时 CLI 返回 rc=0"delivered"（误导），daemon 每次 resume 都重复崩溃再 close（seq 281-285 死循环）。
- 命中率：**4/4 马拉松会话全部命中**（…06b1/…59bd/…f4f9/…05dc），共 4 空-parts 消息、5 次 no-parts 崩溃、3 session 被 error-close。**与背景 bug 同源，独立拿到完整因果链（空-parts 产生→历史毒化→下轮崩溃→永久不可恢复）。**
- 证据：agent7/EVIDENCE-noparts-daemon.log、EVIDENCE-marathon-dead.jsonl、EVIDENCE-m3-compaction-then-death.jsonl、EVIDENCE-messages.txt

### [A7-2] no-parts 崩溃后 send 仍返回成功（rc=0 delivered），用户无从察觉会话已死 🟠 major
- 对 error-closed session `send "继续"` 返回 delivered rc=0，但 events.jsonl 只追加 input_received 后立即再崩溃再 close（seq 281-285）。（与 A1-3 同）
- 证据：agent7/EVIDENCE-marathon-dead.jsonl seq 281-285。

### [A7-3] daemon 崩溃/被杀后 send 仅提示"is the daemon running?"，无自动恢复 🟡（含环境干扰）
- 多次遇 daemon 静默退出（daemon.log **无 panic/stack**）。8 agent 共享物理机、`pkill -f "ar daemon"` 连坐误杀，最可能跨 agent 互杀；用 pidfile 精确管理后不再误杀。daemon 挂掉后已建会话无守护进程自愈，需人工重启（重启后 session 状态能正确 resume 保留，这点好）。

## 三、turn 计数
- 真实事件：generation_started=每个 LLM 决策轮；activity_completed=每次 activity（LLM+工具）完成；无独立 tool_call/tool_result。
- **全部会话总计（主 XDG /tmp/ar7v2，19 sessions + smoke 1）**：
  - **generation_started（真实 LLM agent loops）= 124**
  - **activity_completed（含工具 activity）= 194**
- 一人贡献 124 agent loop，**独立超过全局 100 loop 门槛**。

## 四、延迟数据
- 25 轮：min=2s，**中位 8s**，max=28s，mean=9.2s。最慢 3 轮 [16,16,28]s（28s 那轮含 compaction summarizer 额外 LLM，合理）。空-parts 死亡轮耗时反而短（2-10s，未真干活）。体验基线正常。

## 五、没测到的
- 手动 /compact（SPEC ❌/G7）。
- 记忆写回/CLAUDE.md 注入（UJ-09 未单独验证）。
- compaction **第二次**触发（会话都在首次 compaction 后不久死于空-parts，未活到第二次）。
- 记忆检查点①（马拉松3 因死亡未问到）；检查点②（马拉松4）已通过。

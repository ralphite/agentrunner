# Agent 1 报告：核心对话流 + 会话死亡 bug 复现

**环境**：`agentrunner dev (go1.26.1)`，真实 Gemini API（`gemini-flash-latest`），无 mock。因 `$T/xdg` 路径 137 字节超 macOS unix-socket 104 上限，改用短路径 `XDG_DATA_HOME=/tmp/ar1`（**独立复现 A2-2**，见 [A1-6]）。
**总计**：5 sessions / 21 turns。证据目录：scratchpad/agent1/。

---

## 一、测试记录（逐消息使用日志）

### 场景 1：复现用户会话死亡 bug（vibe-coding，blank workspace）
sid=`20260707-233150-write-a-number-guessing-game-i-4256`

1. `$ ar new --workspace ws1 base.yaml "write a number guessing game in python, single file, then run it once to verify"`
   → Round 1 完成（累计 3 turns）
   → 模型反应：`write_file(guess_game.py)`（llm-t1 in=730/out=664）→ `bash("echo -e \"50\n25\n75\nq\" | python3 guess_game.py")`（llm-t2）→ 回复（llm-t3）:「我已为您编写好了一个用 Python 实现的猜数字游戏…已写入 guess_game.py」
   → 验证：ws1/guess_game.py 在磁盘 1085 字节；has-no-parts=0

2. `$ ar send <sid> "add a scoring system and a play-again loop"`
   → Round 2 完成（累计 7 turns）
   → 模型反应：write_file（llm-t4）→ bash 跑一次（llm-t5）→ bash 用 `random.randint=lambda:42` 确定性测试（llm-t6）→ 回复:「已添加积分系统和再玩一局循环」
   → 验证：guess_game.py 增至 1992 字节；has-no-parts=0

3. `$ ar send <sid> "now convert it to a web app. use just js and html"`  ← **用户原文触发点**
   → 表面成功（status=waiting:input，不报错）
   → 模型反应：**llm-t8 in=4828/out=278，但落盘 `assistant_message = {"role":"assistant","parts":null}`（seq 90）**。278 个 output token 全被丢弃，web app 未落地（ws1 里始终只有 guess_game.py，无 index.html）
   → **历史已被污染，但用户毫无感知**

4. `$ ar send <sid> "open index.html in a headless check if possible, otherwise verify the js has no syntax errors by running node --check on it. report any issues."`
   → **活动立即失败，会话 error 关闭**（status=closed，has-no-parts=1）
   → llm-t9 `activity_failed`，`error.class=internal`，`message="gemini: message with role \"assistant\" has no parts"`，`retryable=false`（seq 99）→ `session_closed reason=error`（seq 101）

5. `$ ar send <sid> "hello, are you there?"`（第二层：重开是否立刻再死）
   → **立刻同错再关**（has-no-parts 1→2）：seq 102 input_received → seq 103 重跑同一失败 activity llm-t9 → seq 104 同错 → seq 106 再 closed

6. `$ ar resume <sid>` → 拒绝：`resume: task session already completed (error)`
   **判定**：🔴 **FAIL — 完整复现用户报告的会话永久死亡 [A1-1]**

### 场景 2：close → send 重开 + 上下文保持
1. `$ ar new --workspace ws3 base.yaml "remember the codeword: BANANA. just acknowledge."` → waiting:input
2. `$ ar close <sid>` → `closing`；status=closed
3. `$ ar send <sid> "what was the codeword? one word."` → `delivered`；status 回 waiting:input（**send 重开已 close 会话**）
   → 模型回复:`BANANA`——上下文跨 close/reopen 完整保留
   **判定**：✅ PASS

### 场景 3：idle 时 interrupt（等价 close）
1. waiting:input 时 `$ ar interrupt <sid>` → `interrupted`；status=closed
2. `$ ar send <sid> "still there? reply OK"` → `delivered`；waiting:input
   **判定**：✅ PASS

### 场景 4：长消息折叠（>10KB → file part）
1. `$ ar send <sid> "<21499 字节伪 config，中间埋 SECRET_MAGIC_TOKEN = ZEBRA_9317>…tell me ONLY the value of SECRET_MAGIC_TOKEN"`
   → `input_received`（seq 40）**带 files 键**，text 仅剩 624 字符——主体折叠为 file part
   → 模型回复:`ZEBRA_9317`（准确抽出埋在第 200 行的 token，内容无丢失）
   **判定**：✅ PASS

### 场景 5：图片输入
1. `$ ar send <sid> --image f.png "…"` → **usage 错误**（--image 必须在 sid 之前，见 [A1-5]）
2. `$ ar send --image qa/fixtures/build-error.png <sid> "what error message is shown in this screenshot? quote it exactly."`
   → 模型回复:「command.go:1234:15: undefined: EnableTraverseRunHooks2」——该字符串只存在于 PNG 像素里,**模型真的看到了图**
   **判定**：✅ PASS

### 场景 6：错误路径体验
1. `$ ar send does-not-exist-xyz "hi"` → `no live session … could not be resumed`，exit=1 ✅
2. `$ ar send <sid> ""` → `send needs session and text`，exit=1，不产生空 turn ✅
3. 特殊字符消息（反引号/$HOME/双引号/单引号/emoji 🎉🔥/换行）→ journal 逐字保留，模型回复 RECEIVED ✅
   **判定**：✅ PASS

### 场景 7：忙时投递排队（in-order）
1. `$ ar new … "count slowly from 1 to 5, one per line, then say DONE1"`
2. turn 进行中连发两条：`"then say APPLE then DONE2"`（delivery_seq=1）、`"then say BANANA then DONE3"`（delivery_seq=2）
   → 两条按序入队,**合并进同一个后续 turn** 消费,回复 `APPLE\nDONE2\nBANANA\nDONE3`
   → 不丢不乱序（合并为一个 turn 属行为非 bug）
   **判定**：✅ PASS

### 场景 8：daemon 日志卫生 / 密钥泄漏
1. 用 39 字符真实 GEMINI_API_KEY 精确 grep 所有日志与 journal → **未命中**（无明文泄漏）✅
2. grep panic|goroutine|fatal|nil pointer|SIGSEGV|data race → 零命中 ✅
3. 仅一条 WARN 确认 bug：`daemon: hosted resume failed … turn 9: gemini: message with role "assistant" has no parts`
   **判定**：✅ PASS

---

## 二、发现的问题

### [A1-1] 🔴 会话历史被空 assistant 消息永久污染，导致会话死亡且无法恢复
- **根因（journal 级证据）**：llm-t8（触发消息=「now convert it to a web app. use just js and html」）返回 output_tokens=278 却落盘 `assistant_message = {"role":"assistant","parts":null}`（rootcause-seq88-90.json）。几乎可确定是模型只返回 thought/thinking token 而无 text/tool_call，**写入前无空 parts 保护、下次请求前也不过滤**。下一 send 把污染历史发给 Gemini → `has no parts`（retryable=false）→ `session_closed reason=error`。
- **第二层（永久死亡）**：send 重开被接受（delivered）但重跑同一失败 activity、再送污染历史、再死（seq 102-106）。`ar resume` 拒绝。**无任何恢复路径**。
- **复现**：
  ```bash
  SID=$(ar new --workspace ws base.yaml "write a number guessing game in python, single file, then run it once to verify")
  ar send $SID "add a scoring system and a play-again loop"
  ar send $SID "now convert it to a web app. use just js and html"   # 注入空 assistant(表面无异常)
  ar send $SID "open index.html in a headless check ..."             # 立刻死
  ar send $SID "hello"                                               # 重开→立刻再死
  ar resume $SID                                                     # 拒绝恢复
  ```
- **修复方向建议**：(1) 写入侧：parts 为空时合成占位 part，绝不落 parts:null；(2) 请求侧：组装历史时过滤/修补空 parts；(3) 韧性：历史错误可修复而非钉死会话。
- 证据：agent1/repro-evidence/events-final.jsonl、rootcause-seq88-90.json

### [A1-2] 🟠 daemon 重启后，磁盘上 idle（waiting:input）会话无法 close/send
- 重启前创建、磁盘 waiting:input 的会话：`ar close` → `no live conversational session`；`ar send` → `no live session … could not be resumed`；但 sessions list 仍显示 waiting:input。**on-disk 状态与 daemon 内存注册表脱节，重启即孤儿化活会话**。（同一 daemon 内 close→send 重开完全正常。）
- 期望：SPEC「send 对 conversational 一律成立」应对重启后的磁盘会话生效（应能从 journal 恢复）。
- 备注：本环境 daemon 反复被并发 agent 的 pkill 误杀放大了暴露频率;但缺口本身真实。

### [A1-3] 🟡 error 关闭的会话，`ar send` 仍回 `delivered` 才失败（误导）
- delivered 只代表"入队"，用户以为成功，实则后台立刻同错再 closed。应即时拒绝或明示"会话处于错误态"。

### [A1-4] 🟡 多数子命令不支持 `--help`，把它当业务参数执行
- `ar resume --help` → 对名为 --help 的会话操作（no sessions found）;`ar close/interrupt --help` → 直接连 daemon。只有 new/send/events/inspect/sessions 有 usage。

### [A1-5] 🟡 `ar send --image` 要求 flag 在 sid 之前，放后面报 usage 静默不发
- 应位置无关，或报错点明正确顺序。

### [A1-6] 🔴 独立复现 A2-2（超长 XDG 路径 daemon bind 失败），不重复计。

---

## 三、没测到的
- 独立 8 轮纯压力长会话：场景 1 的 9-turn 已覆盖"第几轮出问题"（第 8 turn 注入污染、第 9 失败）。
- 全中文 3 轮对话：特殊字符场景已含中文/emoji 逐字往返;场景 1 模型全程中文正常。
- 429 的产品处理：本轮未触发。
- [A1-2] journal 自动恢复验证：受并发 agent pkill 干扰未完全隔离。

# Agent 5 报告:持久化恢复与观察面

## 总览
持久化的**存储层**(journal + 纯 fold + snapshot)经得起最严酷的拷打——3+ 次 kill -9、多次 fork 后,7 会话 events.jsonl **全部** JSON 合法、seq 严格连续(无缺号/无重号)、全 snapshot 可解析。问题全在**活跃编排层**:mid-turn crash 不自动 resume(卡死 running)、fork 后发消息永久丢失、sessions list running 状态撒谎。5 个问题含 2 个 critical。

环境: 独立撞到并绕过 [A2-2](超长 XDG socket bind 失败),改用 /tmp/a5x。另: `ar daemon` 二进制本身稳定(单 shell 内存活 60s+);早期"daemon 自退"是 run_in_background 的 shell 在 turn 间被拆除所致,**非产品 bug**,所有 crash 场景改"单 shell 内自带 daemon"重跑。

## 一、测试记录(逐场景日志)

### 场景 1: crash 矩阵 (a) — idle 会话 kill -9 — PASS ✅
1. `$ ar new --workspace ws1 base.yaml "记住一个暗号:蓝色风筝-42。先待命,不要做任何别的事,只回复'收到'。"`
   → 模型回复 `收到`(seq 9),waiting:input,末尾自动落 checkpoint_barrier,journal 10 行。
2. `$ kill -9 <daemon-pid>`(idle/waiting:input,无在飞活动)→ 快照 snapshots/3.json(891B)已在盘。
3. 重启 daemon;`$ ar send <sid> "接着刚才:那个暗号原样是什么?只说暗号本身。"` → 模型回复 `蓝色风筝-42`
   → 验证: pre-crash 10 行是 post-crash 精确前缀(append-only);全 22 事件 seq 1..22 连续。

### 场景 2: crash 矩阵 (b) — 在飞 bash kill -9 — PASS(伴 A5-1/A5-3)
1. `$ ar new --workspace ws2 base.yaml "记住暗号:红色气球-7。先待命,只回复'收到'。"` → 收到,idle
2. `$ ar send <sid> "运行 ./slow.sh 这一个命令,把它的输出告诉我。只运行这一个命令,不要做别的。"`(slow.sh=echo SLOW_START;sleep 60)
   → 确认在飞: ps 见 bash ./slow.sh(99137)+sleep 60(99141);ran.log 有 SLOW_START 无 SLOW_DONE
3. `$ kill -9 <daemon-pid>`(**turn 在飞,bash sleep 60 进行中**)
   → 孤儿检查: `bash ./slow.sh` PPID=1(重 parent 到 init),sleep 60 仍其子。**孤儿未被清扫**(与 SPEC "pgid 清扫是已知观察项"一致)
4. 重启 daemon → 自动 resume。seq 24=activity_failed,原文 `[interrupted by crash] the runtime died while this ran; the effect may or may not have happened and was NOT re-run` —— **in-doubt "执行不重跑"契约精确兑现** ✅
5. `$ ar send <sid> "停止一切命令。那个暗号是什么?只说暗号。"` → 最终 `红色气球-7`,settle 到 waiting:input。journal 165 事件 seq 连续、14 快照全合法。
   ⚠️ 伴生 [A5-1][A5-3]: 恢复后模型对 crash 回执**失控重跑**(post-crash 又跑 4× slow.sh + 6× sleep 15,billed 飙到 27123),且 mid-turn 不自动 resume。

### 场景 3: crash 矩阵 (c) — 在飞 LLM kill -9 — PASS(附 A5-3)
1. `$ ar new --workspace ws3 base.yaml "记住暗号:绿色灯塔-99。先待命,只回复'收到'。"` → 收到,idle
2. `$ ar send <sid> "用大约300字详细介绍 Go 的 goroutine 调度器 GMP 模型。"`,sleep 1.0,`$ kill -9 <daemon-pid>`(**LLM 生成在飞**,seq 17=activity_started llm-t2)
3. 重启 daemon(不 send,观察 20s)→ **卡死 running,0 进展**([A5-3])
4. `$ ar send <sid> "刚才我问你 goroutine 调度的问题,你回答了吗?如果没有,现在请回答;顺便说一下那个暗号。"` → 完整 GMP 长答 + `绿色灯塔-99`。events 显示 seq 19 用同一 llm-t2/attempt 1 **被重发**(idempotent 重发 = "LLM 重发"契约)✅
   补充干净复现: send→1s→kill→重启→20s 零进展 status=running;`ar resume <sid>` **成功重驱**被打断 turn(+5 事件,settle completed)。→ 恢复可行但**须用户显式 resume/send,无任何提示**。

### 场景 4: 观察面 events/inspect — PASS ✅
- `ar events`(人可读)、`--json`(与 events.jsonl 逐行相等)、`--state`(折叠状态清晰)。
- `ar inspect`: TIMELINE 按 gen-step 列 llm/tool、判定、token;USAGE 数字对得上(input 733+783=1516,output 62+1425=1487,billed 3003)。runaway 会话 inspect 出 14 gen-steps、billed 27123,失控一目了然。

### 场景 5: attach 补读 + live — PASS ✅
1. `$ ar attach --json <sid>`(--json 须在 sid 前)→ 先补读历史逐 generation message
2. attach 挂着时 `$ ar send <sid> "再写一段约150字,讲讲 channel 和 goroutine 的关系。"` → 新 message n=3 live 到达
   → 验证: 消息 n 序列 [1,2,3] 单调、无重复无缺失;补读段不作为 live 重发。首测"漏 live"实为我过早 kill attach,非产品 bug。

### 场景 6: barrier + fork + rewind 全链路 — workspace/history PASS ✅ / fork 续跑 FAIL ❌
1. git workspace,3 轮 write_file(fileA/B/C),每轮末自动落 checkpoint_barrier(bar-t1..t7)。
2. `$ ar barrier <sid>`(会话被 live daemon 持有时)→ `session locked: held by pid ... (a live session cannot be barriered externally)`——手动 barrier 只对非运行中会话(与 SPEC 一致)。
3. `$ ar fork -workspace <dir> <sid> bar-t5`(-workspace 须在位置参数**之前**;usage 写反,见 A5-5)→ fork 成功
   → 三不变量: **INV1** fork workspace=fileA+fileB 无 fileC ✅(工作区回滚);**INV2** 原 workspace 仍 A+B+C 不受影响 ✅;**INV3** fork 历史止于 barrier ✅
4. rewind: 无独立命令;SPEC 明确 rewind=fork+显式 resume,一致。
5. `$ ar send <fork-sid> "...创建 fileZ.txt..."` → **消息被静默丢弃**([A5-2] critical)。fork **无法续跑**。

### 场景 7: journal 完整性(被 kill -9 过的会话)— PASS ✅
- 全 8 会话: events.jsonl 逐行 JSON 合法、seq 连续、0 重号,snapshot 全可解析(哪怕经历 3+ 次 kill -9 与多次 fork)。

### 场景 8: sessions list 全景 — 大体 PASS ⚠️
- 列出 7~8 会话 + STATUS + TURNS,多数状态与 journal 末事件吻合。但卡死的 fork-bar-t6 持续报 running(末事件却是 checkpoint_barrier、12s 零进展)——状态撒谎([A5-3])。

### 场景 9: 错误路径 — PASS ✅(报错质量优秀)
- events|inspect|attach|fork|barrier <不存在 sid> → 一律 `no session matches "..."`,exit 2,无 stack ✅
- `ar fork <sid> bar-t999` → `no barrier "bar-t999" in <sid> (try --list)` ✅(教你怎么修)
- **daemon 没起时** sessions list/events/inspect **照常工作**(直接读盘)✅;仅 send 失败 `daemon dial: ... (is the daemon running?)`。小疵: send 报错却 exit 0。

### 场景 10: 孤儿与资源卫生 — PASS ✅
- 全测完 pkill 后: daemon 0 个、无泄漏 sleep/slow.sh。XDG 共 1.4M(合理)。fork 会话 snapshot=0(复用父随行库省空间)。

## 二、发现的问题

### [A5-2] fork 出的会话永久吞掉发给它的每一条消息(无法续跑)🔴 critical
- 招牌能力"续跑 fork"完全不可用,且违反"崩溃不丢输入"铁律(消息被 ack delivered、fsync 进 inbox,却永不消费)。
- 复现:
  ```
  ar fork -workspace <dir> <sid> <barrier-id>   # 得 fork-sid
  ar send <fork-sid> "创建 fileZ.txt 内容 ZZZ"   # 返回 delivered
  # 无任何 input_received、文件不写、对话状态没这条消息
  ar send <fork-sid> "再试一次"                   # 仍 delivered,仍被吞
  ar resume <fork-sid>                            # 也不消费 inbox
  ```
- 实际: 两个独立 fork(bar-t5、bar-t2)均 100% 复现。消息 durably 堆在 inbox.jsonl(delivery_seq 1,2…)却永不消费;waiting_resolved{resolution:input_received} 照发但对应 input_received 从不落盘。
- **根因(读源码定位)**: session_started 的 Conversational 标志——`internal/event/types.go:114` 注释"a send-driven revival wires the inbox **iff this is true**"。正常会话 Conversational=True(send 生效),fork **MISSING**(吞消息)。fork 创建时未置 Conversational=true。
- 证据: agent5/forkT2-inbox.jsonl、fork5-inbox.jsonl(盘上未消费消息)、forkT2-events.jsonl。

### [A5-3] mid-turn crash 后会话不自动 resume,卡死 running 且状态撒谎 🟠 major
- 直接对应用户"会话死后无法恢复"。存储没坏、恢复能力也在,但**默认不触发**,用户看到永久 running 假象。
- 复现: new→send 长 LLM turn→sleep 1 kill -9 daemon(打断在飞 LLM)→重启,不操作,观察 20s → status=running,journal 0 增长(卡死)。
- 恢复可行但须用户显式动作: `ar resume <sid>` 重驱被打断 turn(答完 settle completed);`ar send` 驱动新 turn。二者皆无"此会话需 poke"提示。sessions list 一直报 running(journal 末事件却非在飞活动)。
- 证据: agent5/s3-atcrash-events.jsonl、final-daemon.log。

### [A5-1] crash 回执触发模型失控重跑,runtime 无节流/无护栏 🟠 major
- 单次 mid-bash crash 后,billed 从正常几百飙到 **27123**,真实 API 花费失控(429 风险陡增)。QA-08 认可"模型看到 crash 后重跑属合法 agency",但**产品对此毫无护栏**。
- 实际: 恢复后模型对 [interrupted by crash] 回执连开 4× slow.sh(各 60s)+ 6× sleep 15 + 多次 ls/cat,inspect 见 14 gen-steps billed 27123。存储层始终无损。
- 证据: agent5/s2-final-events.jsonl。

### [A5-4] `ar close` 无法关闭已持久化但非 live 的会话 🟡 minor
- `ar close <idle-但当前未被 daemon 持有的 sid>` → `no live conversational session`(exit 1)。只有 daemon 正 host 的会话可 close;fresh daemon 启动后会话"未 live"关不掉。间接卡住手动 barrier。

### [A5-5] `ar fork` usage 文案把 --workspace 写在位置参数之后,实际须在之前 🟡 minor
- usage 显示 `fork <session> <barrier> [--workspace <dir>]`,但 Go flag 解析要求 flag 在前。同类: ar attach 的 --json 也须在 sid 前。

## 三、turn 计数
- 只用一个 XDG /tmp/a5x(因 A2-2 改短路径;指令给的 $T/xdg 从未创建、0 会话)。
- 共 **8 会话**(4 正常+1 parent+3 fork),**总 generation_started(gen-step)=42**,与 sessions list TURNS 之和(42)一致。

## 四、没测到的
- in-flight 子 agent kill -9(做了 LLM 版 in-doubt)。
- fork-of-fork(嵌套 fork)。
- 多模态 blob 在 fork/rewind 归属(SPEC 🟡 G1)。
- A5-2 修复验证(纪律: 不改仓库)。
- daemon 有卡死 running 会话时 startup 自动 resume 是否拖慢其他会话(疑似串行延迟 ~88s,未确证)。

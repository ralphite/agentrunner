# 黑盒2 报告 D:回顾 AI 做了什么与花费

结局:审得出来,但要拼三条命令(inspect 花费+用了哪些工具 / attach 改了哪些文件跑了哪些命令 / events -json 存档),还得会看。没有一条"给我一份人话报告"的命令,而最像总览的 inspect 恰恰把细节和失败都藏了。

## 问题
### [R2-D-3] inspect 对"被拒/被拦的操作"只字不提,给人"一切正常"的错觉 🔴
审那个被自动拒绝的会话:TIMELINE 只剩 3 行 `llm complete **allow [budget]**`,四个被 deny 的 tool 调用**全部消失**,status 还显示 `running`。看 inspect 会以为顺利跑完;真相 `decision:deny, auto-denied` 只埋在 events 的 approval_responded 里。审计门面命令静默抹掉失败、满屏 allow=误导性,用户据此得出错误结论。

### [R2-D-1] 没有任何一处显示"花了多少钱",只有 token 🟠
inspect/events -json grep `cost|dollar|usd|$` 全空。只报 token 数;`billed 6028` 用"billed(已计费)"却给 token,误导。负责任用户第一问"花了我多少钱"无解,得自己查 Gemini 单价手算。

### [R2-D-2] inspect 总览把"改了哪些文件、跑了哪些命令"全省掉了 🟠
tool 行只有名字 write_file/bash,没有 path、没有 command。想知道改了哪个文件必须换 attach 或去 events 扒。

### [R2-D-4] 人话历史只有 attach(还得 Ctrl-C 退);events 默认视图是内部黑话且截断 🟠
events 默认 69 行 checkpoint_barrier/effect_resolved/gate_results/timer_set/snapshot_ref,args(文件内容/命令)被终端宽度截断成 `…`;help 自述 "raw journal (debugging)"。审计者缺一个"人话时间线"命令,现只能由 attach 兼任但它是直播跟随、要 Ctrl-C。

### [R2-D-5] 无人值守默认"自动拒绝",且 new 遇 approval 直接断流退出(exit=1) 🟡
默认 spec `ar new` 第一步写文件即 `⏸ approval required` 然后 `stream ended before the reply`/exit=1;实际 AGENTRUNNER_APPROVE unset 导致全程自动 deny,会话零产出却看不出"因被拒没干成"。

### [R2-D-6] 导出物只有开发者格式 JSONL,没有"能发给同事看"的报告 🟡
events -json 完整不截断(好)但满是 seq/causation_id/snapshot_ref;无 --markdown/summary 人读导出。

## 正面(很稳的部分)
- **持久化+找得回最稳**:会话全在磁盘(events.jsonl+snapshots),daemon 反复崩 5 次重起后 sessions/inspect/attach 都还找得到看得到。
- **attach 回放人能看懂**,且显示了 inspect 缺的 tool 参数(哪个文件、python3 greet.py、stdout)。
- events -json 一条命令导出完整可回放 JSONL。

## 结局
持久化和可回放做得实在,导出 JSONL 完整;但面向审计/成本的人话总览缺位——inspect 藏细节又藏失败、成本只报 token 不报钱、events 给机器/开发者看。想"对 AI 的活心里有数"得当半个开发者来拼。
（注:daemon 在非交互 shell 反复自死、要双 fork 才活——多半沙箱 reap,与 round1 判定一致。)

# 黑盒2 报告 E:关掉再回来继续

结论:找回来了也接着聊上了,但路上两处能把普通人劝退的坑(daemon 隐形命脉 + 审批会话卡死)。

## 问题
### [R2-E-1] daemon 是隐形单点命脉:宿主进程一没(关终端/走开),整个对话运行时连不上,连 ar sessions 都报 no daemon 🔴
起 daemon 的 shell/终端结束后,new/send/sessions/attach/inspect 全部失效(`daemon dial: no such file or directory`)。README 那句"use & to background"严重低估后果。期待:daemon 可靠自我常驻(ar daemon start 真后台化),或 README 醒目警告"关终端=daemon 死=命令全连不上,回来先重启 daemon"。

### [R2-E-2] ar daemon & 照 README 做会立刻失败;要 nohup … & disown 才勉强留住且仍不稳 🟠
照做后紧接 `ar new` → `no daemon running? start one with: agentrunner daemon &`——它给的修复建议正是我刚做过却失败的那条,死循环式误导。(注:此环境进程回收激进,真实终端未必这么严重,但"照 README 起不来"本身是文档缺陷。)

### [R2-E-3] 会话被 daemon 复活后卡死在 waiting:approval,该审批再也无法应答,任务永久阻塞 🔴
动 bash 的会话触发审批、daemon 重启后:`ar approve <session> <正确 approval_id> approve` → `no pending approval`,而 sessions 始终 waiting:approval。会话活着列得出却永远推不动,resume 也没救回。notes.txt 永远建不成。（= 白盒审计 M2/A4-5 审批 broker 仅内存、重启丢 pending,本次修复轮我 defer 了它——现从新用户视角确认为"僵尸会话"。）期待:daemon 恢复会话时重建可应答的 pending-approval,或给"此审批已失效,请重发上条消息"的出路。

### [R2-E-4] 回来后想批准卡住的会话却找不到审批 id——sessions/inspect/approve -h 都不给,只能翻标为 debugging 的 events 🟠
inspect 不显示待审批项;approve -h 说需要 approval-id 但没说去哪查;唯一能挖到 id 的 events 被 help 注为 "raw journal (debugging)"。期待:inspect/sessions 在 waiting:approval 时直接显示待审批 id + 命令 + 可复制的 approve 命令。

### [R2-E-5] ar attach 回放完历史强制"跟随直播",没有"只看历史就退出"选项,回来瞄一眼会把终端占死 🟡
回放完一直挂等新事件,只能 Ctrl-C;非交互卡满 2 分钟超时。期待 --no-follow/--replay-only。

### [R2-E-6] ar send <模糊前缀> 报错误导:把"前缀歧义"说成"没这会话且无法恢复" 🟡
`ar send 2026 "hi"` → `no live session 2026 and it could not be resumed`(像"活丢了");对比 `ar inspect 20260708-072` 能报 `ambiguous:` 列候选。send 路径缺同样的歧义检测。

### [R2-E-7] new --detach 的 flag 位置不宽容:放末尾直接 usage 报错 🟡
`ar new spec.yaml "msg" --detach` → usage;必须 `ar new --detach spec.yaml "msg"`。

## 正面(靠谱的核心)
- **纯聊天 resume 彻底成功**:开头让它记 BLUE-OTTER-42/teal,中途 daemon 死好几次,回来只用 `ar sessions` 列出两会话状态清清楚楚,再用**前缀**(不用记全 ID)send,它准确记得暗号,轮次 1→2 无缝。
- 多会话并存能分清(状态列+id 带开头语);前缀寻址好用,歧义时 inspect 列候选;续聊命令结尾提示贴心;记忆恢复零损耗;落盘 journal + daemon 重启即恢复。

## 结局
只要不触发审批、且知道"回来先重启 daemon"这条暗规则,"开个头→离开→回来继续"这条路是通的、体验好。门槛全在那条没人明说的 daemon 常驻规则 + 审批恢复的 bug。

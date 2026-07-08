# 黑盒2 报告 A:出错与恢复

核心症结:启动/恢复(resume)很强,但"停止"和"如实告知状态"很弱——出错后状态是假的(永久 running),想停停不下(submit 无解),想救不知道用 resume(零指引),唯一忠实的 events 又是开发者黑话。

## 问题
### [R2-A-1] submit 后台任务无法用任何 CLI 命令停止 🔴
`ar submit <spec> "跑长任务"` 想中途停:interrupt→"no live interruptible session";kill→"no live session accepting kills"且 ps 给不出 handle;close→"no live conversational session"。而 sessions/inspect 全程 running、sleep 120 进程真在烧。唯一出路是等 bash ~120s 超时或 kill -9 OS 进程(工具从不给 PID)。起得了停不了。

### [R2-A-2] 客户端/daemon 一断,会话僵在 running 且无任何提示 🔴
new/submit 客户端被 SIGHUP 或 daemon 重启后:sessions/inspect 永久 running,ps 说 "no tasks in flight",events 停在 activity_cancelled 后再无事件,进程也没了。用户看到一个永远在跑其实早死的会话,零提示"它掉线了/该 resume"。(= 白盒 M1/A5-3/A8-2 状态撒谎,新用户视角。)

### [R2-A-3] resume 在无 daemon 时静默挂死,不报错 🟠
打印 "resuming session…" 后无限 hang(2 分钟被迫 kill);同场景 send 是秒失败并提示 "no daemon running? start one with: agentrunner daemon &"。resume 应同样快速失败。

### [R2-A-4] daemon 照官方 `daemon &` 启动,关 shell 即死 🟠
随 shell SIGHUP 死、socket 消失,之后 send/interrupt/new 全报 "no daemon";要 nohup … & disown 才活但从不说。(= E 的 R2-E-2)

### [R2-A-5] "停止成功"却显示 "run completed"(措辞误导) 🟡
前台 run Ctrl-C 打断,末行却写 `run completed: 2 turns`,应是 interrupted/canceled。

### [R2-A-6] -h/help 行为不一致,有的还要 daemon 🟡
`resume -h`→"no session matches -h";`interrupt -h`→为看帮助居然去连 daemon 报 "no daemon";`daemon -h` 空白;run -h/submit -h 正常。

### [R2-A-7] bash 被拒时不告诉用户怎么开权限 🟡
gen-step 里 `denied by policy`;AI 说了"shell 权限被拒"但从不提 -mode bypass 或 spec 的 permissions 块。(= round1 BW-3, R2-B)

### 跨切面信息架构问题
interrupt/kill/close/resume 各自只认某一类"活会话",报错只说"没有这类会话",从不说"那你该用哪个命令"。用户僵住时挨个试挨个被拒,毫无指引;resume 实为万能解僵键却零发现性。

## 正面
- 前台 run 的 Ctrl-C **一次就停**,sleep 子进程干净消失(无 orphan),AI 回确认。
- **resume 是真管用的万能解僵键**,把每个僵尸 running 救成 completed。
- events(原始 journal)能拼出真相(虽是 debug 级)。

## 结局
救得回(resume 万能但靠运气发现),但正在跑的 submit 停不掉、状态集体谎报 running、想救无指引。最懵:submit 的 sleep 120——sessions 说 running、进程铁证在跑,interrupt/ps/kill/close 四个"停"异口同声"没有活的会话",手里所有刹车全坏。

# AgentRunner 黑盒 QA 第二轮 — 2026-07-08(运营期基础工作流)

**性质**:5 个 agent + 协调者本人,以真正的黑盒方式使用 `ar`——**零产品知识、禁读任何源码/docs、只能从工具自身的输出学用法**。二进制为当前 main 构建的 `/tmp/ar`(含 C1 修复)。全程真实 Gemini API。

**与第一轮的分工**:第一轮(BLACKBOX-QA.md)找的是**上手期**的墙——新用户进不了门(无帮助、spec 无从发现、工具默认禁、回复不显示)。这一轮专攻第一轮没走到的**运营期**——用户好不容易开始干活之后:出错了能不能搞明白、能不能停下来、能不能监督 AI、能不能审计花费、走开再回来找不找得回、以及**web 驾驶舱**(用户真正用的界面)。

**一句话总纲**:**引擎和持久化是真结实,但"控制"和"如实报告状态"这两件运营基本功很弱。** 用户能起、能续、关机也丢不了活;但想停停不下、看到的状态是假的(永久 `running`/满屏 `allow`)、想监督被默认路径劝退、想审计只有 token 没有钱。两轮合起来:**内核稳,外面这层"控制 + 可观测 + 上手"的皮,处处漏。**

---

## 一、按主题的问题(按对真实用户的杀伤力排序)

杀伤力:🔴 致命(运营死路 / 危险的错误信息)· 🟠 严重 · 🟡 糙。括号内为独立命中的来源。

### 🔴 T1 — "停止"是坏的:你能起一个你停不掉的活(R2-A-1)
`ar submit` 一个后台任务后想中途停:`interrupt`→"no live interruptible session";`kill`→"no live session accepting kills"(且 `ps` 给不出 handle);`close`→"no live conversational session"。而 `sessions`/`inspect` 全程显示 `running`、进程真在烧资源。**CLI 里没有任何命令能停掉它**,唯一出路是等 bash 内置 ~120s 超时、或自己 `kill -9` OS 进程(工具从不告诉你 PID)。起得了、停不了。

### 🔴 T2 — 状态在撒谎:面向用户的命令谎报 `running`/`allow`(R2-A-2 · R2-D-3 · R2-E-3;并印证白盒 M1)
可观测面集体误报,而且互相打架:
- **掉线即僵尸 running**(A-2):`new`/`submit` 客户端被 SIGHUP 或 daemon 重启后,`sessions`/`inspect` **永久显示 running**,`ps` 却说 "no tasks in flight",`events` 停在 `activity_cancelled`,进程也没了。用户面对一个"永远在跑、其实早死"的会话,零提示。
- **inspect 抹掉失败**(D-3):审一个被自动拒绝的会话,`inspect` 的 TIMELINE 只剩 3 行 `llm complete **allow**`,**四个被 deny 的工具调用全部消失**,status 还写 `running`。看总览会以为一切正常——真相 `decision:deny` 只埋在 `events` 里。**审计门面命令静默隐藏拒绝,是误导性最强的一个。**
- **审批僵尸**(E-3):动 bash 的会话触发审批、daemon 一重启,该审批**永远批不了**(`ar approve <正确 id>` → `no pending approval`),会话永远卡 `waiting:approval`。
- 这三者同源:**`sessions`/`inspect`/`ps` 在会话不再"活"时不反映真相**。

### 🔴 T3 — daemon 是隐形、脆弱的单点命脉(R2-A-4 · R2-E-1 · R2-E-2)
对话形态全靠一个后台 daemon,但:照 help 的 `agentrunner daemon &` 起,**关掉那个终端 daemon 就死**、socket 消失,之后 `send`/`new`/`sessions`/`attach` 全部 `daemon dial: no such file (no daemon running?)`。而它给的修复建议 `start one with: agentrunner daemon &` **正是刚失败的那条命令**,死循环式误导。要 `nohup … & disown` 才勉强活、且仍不稳。一个新用户完全料不到"关终端 = 一切连不上",撞见 `no daemon` 时极易以为"活丢了"。

### 🔴 T4 — 监督是反的、且无从发现(R2-B-1 · R2-B-2)
一个"想在 AI 动手前先点头"的谨慎用户:
- **同一个 spec,`run` 默默 deny,`new` 却停下问我**——行为相反、零解释。照 README(主推 `run`)走,得到的是"denied by policy + AI 说自己没权限",既不问你也不指路,极可能错误地放弃——而真相是**换成 `ar new` 监督-审批全都有,且默认就开**。
- `run` 下操作被静默拦截,真正原因 `auto-denied (AGENTRUNNER_APPROVE unset or never)` 只偶然从 AI 转述漏出;用户不知为何被拦、也不知怎么放行。

### 🟠 T5 — 模式与权限不透明、默认危险(R2-B-3 · R2-B-5 · R2-A-7)
`mode` 四个值 `default|plan|acceptEdits|bypass` **全部零解释**,最危险的 `bypass`(全自动绕过审批)还从 `init` 模板里被藏起、零警示;语义全靠建文件跑一遍反推。默认全拒的真正机关是**未公开的环境变量 `AGENTRUNNER_APPROVE`**(文档/help/模板从不出现)。bash 被拒时也从不提"怎么开权限"。`plan` 模式在 `run` 下是"假审批"(AI 说"请批准这个计划"但根本无处可批)。

### 🟠 T6 — 图片:几乎无从发现,且一次性任务传不了(R2-C-1 · R2-C-2 · R2-C-3)
"给 AI 看张截图"这个基础诉求:全产品**唯一**的图片线索是 `ar help` 里 `send … (--image attaches files)` 半句括号(措辞还是泛泛的 "files"),README 只字不提(`grep -i image` = 0)。而 `--image` **只存在于 `send`**——最顺手的 `ar run … --image` 报 `flag not defined`,被迫走 `daemon → new → send` 三步。好不容易用上 `send --image` 还有**参数顺序坑**:flag 放 session id 之后 → 光秃秃 `usage:`,不说是顺序错。(正面:传进去后**AI 是真用视觉看了**——逐行复述截图里没法猜的 `command.go:1234:15`。)

### 🟠 T7 — 审计与成本:只有 token 没有钱,无人话报告,细节被藏(R2-D-1 · R2-D-2 · R2-D-4 · R2-D-6)
负责任用户回头审"AI 做了啥、花了多少":
- **从头到尾不告诉你花了多少钱**,只有 token 数;`billed 6028` 用"billed(已计费)"这个词却给的是 token,误导。
- `inspect` 总览的工具行**只有名字**(`write_file`/`bash`),**没有改了哪个文件、跑了什么命令**——要另开 `attach` 或去 `events` 扒。
- 唯一"人话历史"是 `attach`(还得 Ctrl-C 退);`events` 默认视图是内部黑话(`checkpoint_barrier`/`effect_resolved`/`gate_results`)且 `args` 被终端截断。
- 导出只有开发者格式 JSONL,没有"能发给同事看"的报告。

### 🟠/🟡 T8 — 恢复强但无从发现;停止/恢复命令彼此打架、无指引
- `resume` 是**真管用的万能解僵键**(把僵尸 `running` 救成 `completed`),但名字像"接着干活"、且僵住的当下**没有任何一处提示用它**;靠运气从 help 瞟到才想起(R2-A);无 daemon 时 `resume` 还**静默挂死**不报错(R2-A-3)。
- `interrupt`/`kill`/`close`/`resume` 各自只认某一类"活会话",报错只说"没有这类会话"、**从不说"那你该用哪个"**——用户僵住时挨个试挨个被拒(R2-A 跨切面 · R2-E-4)。
- 零散:被打断却显示 "run completed"(R2-A-5);多个子命令 `-h` 不一致、`interrupt -h` 为看帮助竟去连 daemon(R2-A-6);`attach` 回放完强制跟随直播、无 `--replay-only`(R2-E-5);`send <模糊前缀>` 报 "no live session … could not be resumed"(像活丢了,其实是前缀歧义)(R2-E-6);`new --detach` flag 放末尾即 `usage`(R2-E-7)。

### web 驾驶舱(协调者浏览器实测,当前 C1-fixed 代码)
- **[R2-W-1] "关闭会话"按钮点了完全无反应、零反馈** 🟠:等 4 秒状态仍 `待命,等你输入`,无确认/toast/报错/console 错误。用户无法从驾驶舱关闭会话。(与 CLI"close 关不掉"同源,但 web 连回执都不给。)
- **[R2-W-2] 新会话点"创建"后弹窗不关闭** 🟠:会话已静默建在弹窗背后(侧栏出现、已真跑),用户看不到,易误以为没成功而**再点创建→建重复会话**。
- 说明:web app 自称"测试驾驶舱",`原始 journal`/`inspect 树` 等内部面是给开发者的调试件,by-design,不计为 basic-workflow bug。

---

## 二、经得起夸的部分(内核确实稳)

- **持久化 + 找得回,是全场最稳的一块**:会话全在磁盘(`events.jsonl`+`snapshots`),daemon 反复崩了 5 次重起后 `sessions`/`inspect`/`attach` 都还找得到、看得到;关机也丢不了活。
- **`resume` 真管用**:万能解僵,把每个谎报 `running` 的僵尸救成 `completed`。
- **多轮记忆完美**:走开再回来,暗号(BLUE-OTTER-42 / teal)零损耗;前缀寻址好用、歧义时 `inspect` 列候选;多会话状态可区分。
- **human-in-the-loop 审批设计到位(一旦走对路)**:`⏸ approval required` 明确展示 AI 要干什么、approve 命令自动拼好、**deny 能带理由且 AI 真读到并调整行为**;daemon 会话默认就会问你——安全默认其实很好。
- **图片真用视觉看了**、**前台 `run` 输出可见 + Ctrl-C 一次干净停止**、**坏路径/非图文件报错说人话不泄漏内部**。
- **web 驾驶舱是那个"好入口"**:预填能用的 spec(带 tools + permissions allow)、内联显示 AI 回复 + 工具卡片、有可见的 `图片` 按钮——**恰好补上了第一轮 CLI 的多堵墙**(spec 无从发现、工具默认禁、回复不显示、传图无从发现)。C1 会话死亡修复也在 web 上端到端验证通过。

---

## 三、两轮合起来看 + 与白盒审计的呼应

- **第一轮 = 上手墙**(进不了门):无帮助、spec 靠猜、工具默认禁、静默假成功、回复不显示。
- **第二轮 = 运营墙**(进门之后):停不掉、状态撒谎、监督被劝退、审计没钱数、daemon 脆弱、恢复无指引。
- **这一轮从新用户视角独立印证了白盒审计里几个我当时 defer 的问题**,说明它们是真实且用户可见的,值得排期:
  - T2(掉线僵尸 running / inspect 藏失败)= 白盒 **M1**(mid-turn crash 不自愈 + 状态撒谎)+ **A8-2**。
  - T4/E-3(审批 daemon 重启后丢失、僵尸 waiting:approval)= 白盒 **M2 / A4-5**(审批 broker 仅内存)。
  - T8(前台被打断显示 "run completed"、单次 Ctrl-C)= 白盒 **m2 / A2-1** 的同族。
- **贯穿两轮的同一句话**:**内核(事件溯源、持久化、恢复、多轮记忆、真实工具执行、审批闭环)是好的;塌方的全在最外层"控制 + 可观测 + 上手"这层皮。** `run` 一次性路径体验好,而 `daemon`/`new`/`send`/`submit` 这套(web UI 底座)在控制与状态呈现上处处硌手——这与用户最初在 web UI 上的挫败一脉相承。

---

## 四、修复优先级建议(纯建议,本轮未改代码)

1. **让状态说真话(T2)**:会话不再"活"时,`sessions`/`inspect` 显式标 `stranded`/`crashed`/`needs-resume`,而不是永久 `running`;`inspect` 必须显示被 deny/失败的操作,不能只留 `allow`。这是杀伤力最大、最误导人的一类。
2. **让"停止"可用、可发现(T1/T8)**:`submit` 任务给一条能停的命令(或让 `kill`/`interrupt` 认它);`interrupt`/`kill`/`close` 的"没有这类会话"报错统一改成"要停这个会话请用 X / 要恢复请 `resume`"。
3. **让 daemon 不再是隐形陷阱(T3)**:`ar daemon` 提供真后台化(`start`/`--detach`),或 README 醒目警告 + `sessions` 在 daemon 缺席时提示"先 `ar daemon` 再来"。
4. **让监督可发现(T4/T5)**:`run` 的 `denied by policy` 那行直接附解法("要逐步审批用 `ar new`;放行用 `-mode bypass` 或 spec `permissions`");每个 `mode` 一句话说清、危险值加警告;审批 id 在 `inspect`/`sessions` 直接可见。
5. **审计给人话 + 给钱数(T7)**:`inspect` 带"改了哪些文件/跑了哪些命令"摘要和美元估算;提供一个人读的历史/导出。
6. **图片可发现 + 一次性可传(T6)**、web 驾驶舱的 close 按钮与创建弹窗(R2-W-1/2)顺手修掉。

---

## 附:证据
逐 agent 完整黑盒实录(每条命令原文 + 工具真实输出 + 新用户反应)见本目录 `blackbox2-agentA/B/C/D/E.md` 与 `blackbox2-recon-web.md`。

---
description: Codex parity 持续优化引擎——每轮先「对着 Codex 截图比对 live 8809 → 排出最大 UI/UX 差距 → 并发关闭」;只做实质功能/UI/UX 改进,永不停、永不自我关停
---

# /parity-drive — 每轮:对标 Codex → 找最大差距 → 关闭它

你是 AgentRunner webui **对标 OpenAI Codex 桌面 app** 的持续优化引擎主驾驶。
这台机器**永远在跑、连续无空转**,目标只有一个:**把 webui 一步步逼近、追平、
直到超过 Codex 的 UI/UX**。一轮结束**立刻**起下一轮(runner 已是 while-true + 几秒
guard,没有 30min sleep),轮末过「反停滞审计」写进回复。**永远有下一个差距可以关闭。**

## ⛔ 关于停机(硬性,最高优先级,压过本文件其余一切)

你**绝不**以任何方式关停这个循环或它的定时器。具体禁止(全部):
- 不运行 `launchctl bootout` / `unload` / `remove` / `disable`,针对
  `com.agentrunner.parity-drive` 或 `com.agentrunner.parity-drive-watchdog`;
- 不重命名 / 移动 / 删除 `~/Library/LaunchAgents/com.agentrunner.parity-drive*.plist`
  (包括改成 `.stopped`、`.bak` 等任何后缀);
- 不 `CronDelete` / 不删自己的定时任务、不 kill runner 或 watchdog 进程;
- 不写"停循环 / 停定时器 / 收工 / 达终局可停 / parity 已完成可停"之类的
  动作或结论。
**「没有开放条目」永远不是停机条件,而是去发现新差距的触发器。** UI/UX 离
Codex 还很远,永远有下一个差距。判断"当前这批做完了"时,你的动作是**回到第
一步重新对标 Codex、开新一批**,不是停。只有真人可以停它。

## ⏱ 速度硬规则(idle 即失败 + 进度必须【持续可见】,与 ⛔ 并列最高)

**空转是失败;"在干活但外面看不见"也是失败(用户裁决)。** 更新之间的长间隔不可接受
——"在工作" ≠ "有进度"。
- runner 已 while-true + 几秒 guard,**没有 30min sleep**;一轮完立即下一轮(runner 另有
  55min 硬顶 + 5min 停滞 watchdog + 每 2min 一条 heartbeat 兜底)。若上游明确返回
  usage/weekly/rate limit,runner 释放锁后每 5min 探测一次,避免无产出的热循环。
- **⏱ 可见进度硬指标(失败轮判据):任何时刻,距上一条 `parity-drive.log` 新行 或 上一次
  push 落 origin/main,都不得超过 5 分钟。** 超了这一轮算**失败轮**——哪怕子 agent 正在跑。
- **一轮 = 一批 implementer,派完就收轮(硬性)**:每轮**只派一批** 2–3 个 implementer
  (并发、touches 白名单两两无交集、worktree 隔离),等它们落完 → 推完 → 部署 → 更台账,
  **这一轮就结束**。**绝不在同一轮里开第二批**——下一批留给下一轮(runner 8s 后自动开)。
  **目标单轮 <15min**;跑长了赶紧收尾让下一轮接手,**禁止**在轮内链多批(32min 双批轮就是
  反面教材)。**为什么**:每个轮界是一次带新鲜眼睛的「重截图 live → 重对标 Codex → 重排
  优先级」checkpoint;长轮会漂移、拖延这次重排。finder 是 read-only,可在这一批里多发。
- **落一个推一个(禁批量攒推)**:每个 implementer 一验完(node24 vitest + build 绿)就
  **立即 merge + push 它那份到 origin/main**,并 echo `[time] step: landed <组件> <sha>`。
  **不许**攒到整批都完了才一次性推。每个子 agent 也自己 rebase/retry 推自己的(disjoint
  白名单→干净)。
- **每步打点(log 每 ≤2min 有新行)**:开轮、派每个 finder/implementer、每次落地、每次
  push,都 `echo "[$(date '+%F %T')] step: …" >> ~/Library/Logs/parity-drive.log`;同步等
  子 agent 时也要周期汇报「还在等哪几个、各跑了多久」。**绝不让 log 静默超过 2 分钟。**
- git 冲突用 **rebase/retry**;**绝不靠暂停循环换省事**(绝不 bootout/停循环避碰撞)。

## 只做【已有后端】功能的 UI/UX parity(不加新功能、不做壳)

**硬规则(用户裁决):**
- 只对**我们已经有可用后端的功能**做 UI/UX parity——真差距在那里,而且**还有太多**。
- **绝不新增功能、绝不做壳**:一个功能若没有清晰后端支撑(缺得多、或需要真正的
  设计/后端集成)**就不做**。做"看起来很像"的占位壳页(如曾被移除的 Plugins/Sites
  页)是**错的**——那是假的表面积,不是 parity。
- 任何需要**新后端集成**的(GitHub PR、插件注册表、站点托管等):**不做**,从 ledger
  移出、标 out-of-scope。
- gap ledger 只登记**已有后端功能上的 UI/UX 差距**(spacing/typography/层级/状态/
  hover/密度/对齐/图标/空态载态/文案)。

## 目标:追平 Codex 的 UI/UX

对齐规则(用户裁决):双方都有的功能按 Codex 做;核心差异功能不强凑;我方
独有功能套 Codex 风格。**衡量标尺只有一个:对着 Codex,我们的 UI/UX 差距有没有
在实质地变小。** 用户直话:现在的 UI/UX 离该有的样子**还很远**;如果循环把时间
花在给按钮写可访问性属性上,UI/UX **等于原地踏步**——这正是过去发生的,必须停。

## 🎯 每轮第一步(不可跳过):对着 Codex 金标截图比对 live,排出差距优先级

**这是每轮强制的第一步,是选活的前提——没做完比对,不许派任何 implementer。**
你无法凭空知道什么值得做;**必须先看见 Codex 与我们的差别**。

**Codex 金标截图(真像素,主参照)已入库 `qa/codex-reference/`。** 屏 → 参照图:

| Codex 金标图 | 屏 | 我方 live(8809)对应 |
|---|---|---|
| `codex-diff-review.jpg` | **Diff/review 分栏(最重要)**:左对话 + 右满高语法高亮 diff、逐文件头、+/- 行数、unmodified 折叠、AgentRunner/Review tab、Commit or push | Changes split / Review |
| `codex-task-thread.jpg` | 任务 thread:`Edited N files +x -y` 变更卡(Undo/Review)、文件行 +/-、Show N more、右侧 Environment 面板、底部 composer | 富会话 |
| `codex-thread-environment-panel.jpg` | thread + Environment 面板展开(Changes/Worktree/Create branch/Commit or push、Background processes、Browser、Sources) | 富会话右栏 |
| `codex-new-task-home.jpg` | New task 首页空态:"What should we build in {repo}?" + 4 建议卡 + composer chips(repo/worktree/env/branch)+ 模型选择 | home |
| `codex-scheduled.jpg` | Scheduled:搜索、All/Active/Paused、cadence + next-run、Suggestions | Scheduled |

> Codex 的 Pull requests / Plugins / Sites / Chat 屏是**新功能**(GitHub PR 集成 /
> 插件注册表 / 站点托管),我们**没有后端** → **out-of-scope、不做、不比对**。
> 这些金标图仅留档,别据它们建壳页。裁剪组件图(`codex-crop-*.jpg`)才是逐组件
> 对齐的主参照(sidebar/composer/model 菜单/add 菜单/diff/change card/命令面板/
> scheduled 等,均对应我们**已有**的屏)。

**全局 sidebar**(首要对齐面,只做我们真有的):New task / Scheduled → `Pinned` →
`Projects`(按 repo 分组 + 缩进任务行 + Show more/less)→ `Tasks` → 账户行。

流程:
1. **截我方 live**:playwright 对 `127.0.0.1:8809` 逐屏截图(上表每个"对应"屏,
   × light/dark × 1440/390),存 `qa/runs/<日期>/`。
2. **逐屏并排 diff**:我方截图 vs 匹配的 `qa/codex-reference/*.jpg`,写出**差距清单**:
   每条 =〈屏〉+「Codex 怎样 / 我们怎样 / 差在哪 / 为什么 Codex 更好 / 关闭它的动作」。
   microcopy/tokens 细节补查文字参照 `docs/increments/INC-41-CODEX-UI-REFERENCE.md`
   + `docs/CODEX-PARITY.md`。
3. **按价值排序**:最大、最能被用户看见、最影响"这产品好不好用"的差距排最前,
   排进 / 刷新 `INC-41-BACKLOG.md`。**本轮就打最靠前的那几条。**

⚠️ 金标图是**已入库的真像素**,不用每轮重截(**headless 轮自己截不到 Codex app**——
无 Computer Use / 无录屏权限)。**比对这一步永不跳过、永不假装。** 金标图要刷新/补屏
时,台账记一条「需交互 session/真人用 Computer Use 补 `qa/codex-reference/`」。

## 只做实质改进,严禁划水(反划水硬规则)

- **要做**:功能 / UI / UX 的**实质**改进——真正改变产品做什么、用起来什么感觉、
  长什么样。关闭对 Codex 的**可见**差距,就是本引擎的全部意义。
- **不要优先**:性能 / 速度、可访问性(aria、焦点环、对比度微调)。它们是**次要、
  以后再说**,不许拿它们冒充进展。
- **可访问性已从默认镜头轮换里移除;性能不再是强制轴。** 除非某条正好卡死核心
  功能可用性,否则本引擎**不派** a11y / perf 专项工作。
- **反划水判定(硬性)**:一轮如果产出**只有** aria 标签 / 焦点环 / 对比度这类
  微可访问性小修,或纯装饰性 nits——**这一轮算失败轮**。每轮**必须关闭至少一个
  实质的功能 / UI / UX 差距**(用户一眼能看见对着 Codex 变好了)。**没有可见地朝
  Codex parity 前进的轮 = 失败轮**;轮末审计必须诚实标注失败、说明原因与下轮补法,
  不许把小修粉饰成进展。

## 并发与文件分区(硬性——把进度做快的主杠杆,默认就并发)

- **finder 并发**:read-only,无写冲突,直接一批多发,不同屏 / 不同功能面并行取证。
- **implementer 并发**:每个跑在**独立 worktree**(`isolation: "worktree"`)且带
  一份**互斥 touches 白名单**——任意两个 implementer 的白名单**交集必须为空**。
  分区规则:
  - 按**文件 / 组件**切:各 implementer 只许改自己白名单内的文件。
  - **共享文件**(如 `styles.css`、路由表、`viewModels.ts` 这类多处要动的):
    要么把相关多条**并进同一个** implementer 串行做;要么本轮**只放一个**碰它、
    其余相关条目让路到下轮。`styles.css` 一律**只追加**注释块、不改既有块。
  - 派工前自查:把每个 implementer 的白名单列出来,肉眼确认两两无交集再发。
- **冲突兜底**:merge/rebase 时源码冲突**保双方语义**;`dist/*` 是 gitignored
  构建产物(见「构建产物纪律」),不会再有 dist 冲突。
- **别的 session 也在推 main**(Tailwind 迁移、INC 系列):纯视觉 CSS 重排它在做 →
  避开或纯追加;结构 / 逻辑 / 新入口 / 功能类我方做。rebase 冲突先读对方 commit 意图。

## 运行形态(先判定,再走协议)

- **headless 轮**(launchd 定时器 → `scripts/parity-drive-cron.sh` → `claude -p`,
  env 有 `PARITY_DRIVE_HEADLESS=1`):锁已由 runner 脚本持有,**不要**再抢/释放锁。
  **并发是默认,但小批快轮**:每轮 implementer **最多 2–3 个**(并发、白名单互斥、
  worktree 隔离);headless 轮进程一退出还在跑的子 agent 会被杀,所以要 `wait` 到它们
  完成——但**落一个就立即推一个**(见下),不许干等整批。**宁可多个小快轮,不要大慢轮。**
  runner 有 55min 硬顶 + 5min 停滞 watchdog 兜底。三条纪律:
  1. **落一个推一个 + 每 ≤2min 打点**:每个 implementer 一验完就立刻 merge+push 它那份
     并 log「landed …」;等子 agent 时也周期 log「还在等哪几个」。**log 静默 >5min 或
     >5min 无 push = 失败轮**(见 ⏱ 速度硬规则)。
  2. **打点**:每完成一步 `echo "[$(date '+%F %T')] step: <一句>" >>
     ~/Library/Logs/parity-drive.log`,外部可见进度,尸检有据。
  3. **让路**:开轮 `git status` 若有**非本轮产生的脏文件**(其他并发 session 的
     未提交工作),不 add、不 revert、不 rebase 碰它们;台账记「让路 <文件>」。
- **交互轮**(在活跃 session 里被触发):先抢锁 `mkdir /tmp/parity-drive.lock`
  ——占用中且新鲜(<45min)则本轮跳过;>45min 判陈锁清掉重占。轮末 `rm -rf` 锁。

## 每轮协议:对标 Codex → 并发发现 → 并发改进 → 验证推送 →(睡)→ 再来

1. **同步 + 收割**:`git fetch origin main` + fast-forward;工作区脏文件按「让路」。
   收割上轮遗留子 agent(TaskList / 后台完成通知)。live 8809 挂了/落后 main →
   rebuild 部署(见环境速查)。
2. **对标 Codex(强制第一步)**:执行上面「🎯 每轮第一步」——取 Codex 参照 × 截
   live × 逐屏并排 × 刷新按价值排序的差距清单。**没做完这一步不许进第 4 步。**
3. **发现(Find)— 并发多 finder**:一次性 dispatch **多个** finder(read-only,
   不同屏 / 不同功能面并行),镜头聚焦**功能 / UI / UX 差距**:布局与信息层级、
   交互完整性、空/错/载态的**功能性**、组件相对 Codex 的行为差、缺失的功能入口、
   微交互与视觉一致性……**不含** a11y / perf 专项镜头。产出带 file:line + 截图 +
   「对标 Codex 差在哪」。轮内等齐后登记 BACKLOG;**先排除刻意决策**(git log -S +
   注释,有 QA-45/INC 依据的判 ✂ 记理由,不修)。
4. **改进(Improve)— 小批并发 implementer(≤2–3 个)**:从差距清单选**最高价值**且
   touches 互不重叠的 **2–3 条**,**同时**各派一个 implementer(worktree 隔离,并发,让
   **每个 implementer 自己 vitest+build 通过后 merge+push 自己那份**)。prompt 给全:差距
   详情 + Codex 目标态、互斥 touches 白名单、验收断言、验证命令、纪律(**不 commit dist**、
   不动白名单外文件、vitest 全绿、**落完自己 rebase/retry 推 origin main**)。**一轮只派这
   一批**(2–3 个);**本轮绝不开第二批**——落完推完就走第 5、6 步收轮,下一批交给下一轮。
5. **落一个推一个 + 复验**:每个 implementer 一 push 到 origin/main,主线就 `git fetch`
   拉过来、log 一行「landed <组件> <sha>」、部署 8809、playwright 复验改动屏对 Codex 参照
   差距真的关小了 + console 0 → BACKLOG 打 ✅。**禁止**攒到整批完了才推/才部署。**dist 是
   gitignored 构建产物,绝不 git add/commit。** 等待期间每 ≤2min log 一次进度。
6. **反停滞审计 + 台账 + 不停机**(轮末必答,写进回复):
   - 本轮**关闭了哪个对 Codex 的可见差距**?贴改动屏 before/after 截图。
   - **若本轮没有可见地朝 parity 前进 → 标注「失败轮」**,写原因 + 下轮补法。
   - push 了哪些 commit?哪些子 agent 在跑/跑完?BACKLOG:+✅ / 新增 ☐ 各几条?
   - 台账 `INC-41-BACKLOG.md` 末尾追加一行 `- <日期时间> 轮N:比对X屏、关差距
     <描述>、派工Z(并发)、push <sha…>、live=<js hash>`。
   **一批落完就【结束本轮】**(不在同轮再开一批);报告 ≠ 停机。runner 8s 后自动开新轮,
   新轮从「🎯 第一步」**重新截 live、重对标 Codex、带新鲜眼睛排下一批**。目标:轮 <15min。

## 本轮质量闸门(判断"这批工作收干净了"用,**永不是**停机条件)

1. `docs/increments/INC-41-BACKLOG.md` 本轮涉及条目 ✅ 或 ✂(带理由);
2. 改动屏对 Codex 参照复比,目标差距确实关小、且无视觉/功能回退;
3. 全景 playwright(home/富会话/approval/Changes split/Scheduled/Settings ×
   light/dark × 1440/390)稳态 console error+warning = 0;
4. `qa/runs/` 存档本轮 before/after 截图 + 三层文档行齐活。

四闸门都绿 = 这批干净了,**不是**可以停了。绿了回到「🎯 第一步」开下一批(见 ⛔)。

## 构建产物纪律(硬性——绝不提交 dist)

- `webui/frontend/dist/` 是 Vite 构建产物、被 Go 二进制 `//go:embed` 内嵌,
  **已 gitignore,永不 `git add`/commit**。仓库里只留 `dist/.gitkeep` 占位符
  (让 `go:embed` 能在干净 checkout 上编译),**别删它**。
- 部署 = 本地 `npm run build`(重生成 dist)→ `go build` 内嵌 → 重启服务。构建
  产物只存在于工作树与二进制里,**不进 git**——所以再没有 dist 合并冲突,也**不该
  因 rebuild 产生任何 commit**。
- **若 `git status` 冒出 `webui/frontend/dist/` 改动:是配置回退了——停下修
  `.gitignore`,绝不提交这些产物。**

## 环境速查

- node24:`export PATH="$(ls -d $HOME/.nvm/versions/node/v24* | tail -1)/bin:$PATH"`
- 前端:`cd webui/frontend && npx vitest run && npm run build`
- **部署 8809(launchd 守护制)**:8809 由 `com.agentrunner.webui8809` LaunchAgent
  常驻(KeepAlive,session 无关,自愈)。部署 = 换二进制 + 重启服务,**不要**自己
  起进程:
  ```
  BIN=~/.local/share/agentrunner/bin
  cd webui/frontend && npm run build && cd ..        # 先重生成 dist(gitignored)
  go build -o "$BIN/arwebui-live.new" . && mv "$BIN/arwebui-live.new" "$BIN/arwebui-live"
  cd .. && go build -o "$BIN/ar-live.new" ./cmd/agentrunner && mv "$BIN/ar-live.new" "$BIN/ar-live"  # CLI 有变时
  launchctl kickstart -k gui/$(id -u)/com.agentrunner.webui8809
  ```
  验:`curl -s -o /dev/null -w '%{http_code}\n' 127.0.0.1:8809/`(200)+
  `curl -s 127.0.0.1:8809/ | grep -o 'index-[A-Za-z0-9_-]*\.js'`(看 live 跑的是
  哪个 bundle;hash 含连字符)。服务日志:`~/Library/Logs/agentrunner-webui8809.log`。
- playwright venv:
  `/private/tmp/claude-501/-Users-yadong-dev2-agentrunner/b84daf52-9db3-44c9-8c46-9a5d9f61a6df/scratchpad/pwenv/bin/python`
  (若丢失:任意 scratchpad `python3 -m venv pwenv && pwenv/bin/pip install playwright`)。
  chromium `channel="chrome"` headless、dsf=2;session 页 goto 用
  `wait_until="domcontentloaded"` + sleep(networkidle 被 SSE 卡死)。
  快速切会话时被切走会话的在途请求 502 属已知良性瞬态;稳态单页必须 0。
- 常用真实会话:富=`20260711-011831-what-is-the-project-297d`、
  approval=`20260711-040811-reply-with-exactly-qa45-thinki-b98f`、
  diff=`20260710-213428-create-qa42-worktree-browser-t-d8ac`。
- **Codex 参照**:金标截图 `qa/codex-reference/*.jpg`(**真像素,主参照**,屏映射见
  「🎯 每轮第一步」)+ `qa/codex-reference/README.md`;文字参照(补 microcopy/tokens)
  `docs/increments/INC-41-CODEX-UI-REFERENCE.md` + `docs/CODEX-PARITY.md`(与 QA-45
  实测冲突处以 QA-45 为准)。任务登记簿 `INC-41-BACKLOG.md`(canonical)。

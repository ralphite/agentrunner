---
description: UI 持续优化引擎——每轮「巡检 live → 并发发现 → 并发改进 → 验证推送」,双轴(UI 质量 + 速度/性能)齐头并进;永不停、永不自我关停,达标即找下一批
---

# /parity-drive — 每次触发执行一轮「巡检→发现→改进」优化循环

你是 AgentRunner webui 的**持续 UI 优化引擎**的主驾驶。这不是一次性的
parity 验收任务,而是一台**永远在跑、永远在把 UI 变得更好更快**的机器:
每次被唤醒执行下面一整轮,轮末通过「反停滞审计」并把结果写进回复,然后
launchd 睡 ~30min 再叫你跑下一轮。**永远有下一项可优化。**

## ⛔ 关于停机(硬性,最高优先级,压过本文件其余一切)

你**绝不**以任何方式关停这个循环或它的定时器。具体禁止(全部):
- 不运行 `launchctl bootout` / `unload` / `remove` / `disable`,针对
  `com.agentrunner.parity-drive` 或 `com.agentrunner.parity-drive-watchdog`;
- 不重命名 / 移动 / 删除 `~/Library/LaunchAgents/com.agentrunner.parity-drive*.plist`
  (包括改成 `.stopped`、`.bak` 等任何后缀);
- 不 `CronDelete` / 不删自己的定时任务、不 kill runner 或 watchdog 进程;
- 不写"停循环 / 停定时器 / 收工 / 达终局可停 / parity 已完成可停"之类的
  动作或结论。
**「没有开放条目」永远不是停机条件,而是去发现新改进的触发器。** 任何界面
都能更好:更易用、更快、更精致、更一致。判断"当前这批做完了"时,你的动作
是**开新一批**,不是停。只有真人可以停它;你的职责是让它一直有价值地跑下去。

## 目标与「达标即续航」(这个循环没有终点)

对齐规则(用户裁决):双方都有的功能按 Codex 做;核心差异功能不强凑;
我方独有功能套 Codex 风格。达到 parity 只是**地板不是天花板**——达成后继续
朝"比 Codex 更好、更快"推进。

## 双轴:每轮都要推进两条线(谁都不许饿死)

每一轮**必须同时**在这两条轴上都有动作,不允许只做一轴:

- **轴 A — UI 质量 / 最终视觉与体验结果**:可访问性(键盘可达、焦点可见、
  对比度、触控目标)、空态 / 错误态、加载 / 骨架态、动效与过渡、响应式断点、
  文案与 i18n、视觉打磨、信息层级与密度。
- **轴 B — 速度 / 性能**:首屏加载、渲染时间、bundle 体积、API 延迟、
  感知响应性、掉帧 / jank。**能量就量,别猜**——改前抓 baseline 数字、改后
  复测,把 before/after 写进 BACKLOG / 台账,让"变快了"是**可证的**,不是感觉。

轮内的 finder 批次**至少各含一个 A 镜头和一个 B 镜头**;implementer 批次里
只要 BACKLOG 有性能条目就必须带一个性能改进上路。哪轮发现某轴暂时没料,
当场派一个该轴的 finder 去找——不许用"这轮先只做另一轴"打发过去。

## 本轮质量闸门(四闸门:判断"这批工作收干净了"用,**永不是**停机条件)

1. `docs/increments/INC-41-BACKLOG.md` 本轮涉及条目 ✅ 或 ✂(带理由);
2. 最近一轮 finder 复查(双镜头)无新 P1/P2 发现;
3. 全景 playwright 扫描(home/富会话/approval/Changes split/Scheduled/
   Settings × light/dark × 1440/390)稳态 console error+warning = 0;
4. 全景 QA-43 验收归档(`qa/runs/` + 三层文档收口);
   性能类改动额外附 before/after 实测数字方算"收干净"。

四闸门都绿 = 本轮这批干净了,**不是**可以停了。绿了就去开下一批(见 ⛔)。

## 运行形态(先判定,再走协议)

- **headless 轮**(launchd 定时器 → `scripts/parity-drive-cron.sh` → `claude -p`,
  env 有 `PARITY_DRIVE_HEADLESS=1`):锁已由 runner 脚本持有,**不要**再抢/释放锁。
  **并发是默认**:一批里同时派多个子 agent(见「并发与文件分区」),但**必须在
  本轮内 `wait` 到它们全部完成再收割/推送/收轮**——headless 轮进程一退出,还
  在跑的子 agent 会被杀,所以绝不能让轮在子 agent 未完成时结束。**55min
  watchdog** 会杀超时轮,故单轮并发数要能在窗口内跑完(经验:2–4 个 finder
  或 2–3 个 implementer 一批),另遵三条(2026-07-11 被杀教训):
  1. **早发布**:每落定一个单元立即 commit+push,不许攒到轮末——被杀=全丢。
  2. **打点**:每完成一步 `echo "[$(date '+%F %T')] step: <一句>" >>
     ~/Library/Logs/parity-drive.log`,外部可见进度,尸检有据。
  3. **让路**:开轮 `git status` 若有**非本轮产生的脏文件**(共享 checkout 里
     常是其他并发 session 的未提交工作),不 add、不 revert、不 rebase 碰它们;
     只做不涉这些文件的工作,台账记「让路 <文件>」。
- **交互轮**(在活跃 session 里被触发):先抢锁 `mkdir /tmp/parity-drive.lock`
  ——占用中且新鲜(<45min)则本轮跳过(headless 正在跑);>45min 判陈锁清掉
  重占。轮末 `rm -rf /tmp/parity-drive.lock`。可用后台子 agent 与完整协议。

## 每轮协议:巡检 → 发现 → 改进 → 验证推送 →(睡)→ 再来

1. **同步 + 收割**:`git fetch origin main` + fast-forward;确认工作区干净
   (脏文件按「让路」处理)。收割上一轮遗留的子 agent(TaskList / 后台完成
   通知)。若 live 8809 挂了或落后 main:rebuild + 部署(见环境速查)。

2. **巡检 live(Inspect)**:对**真实运行的 webui `127.0.0.1:8809`**实测,两轴
   各下探针,记 baseline:
   - 轴 A:playwright 截图全景(见闸门 3 的页面矩阵 × light/dark × 1440/390)
     + axe 可访问性扫描;肉眼过空态 / 错误态 / 加载态 / 动效 / 层级。
   - 轴 B:量 bundle(gzip 后字节)、导航时序(FCP/DCL/load)、关键 API
     `time_total`、交互 jank(见「性能实测速查」)。数字落台账做 baseline。

3. **发现(Find)— 并发多镜头 finder**:一次性 dispatch **多个** finder
   (read-only,`isolation` 无所谓,天然不冲突),**各自不同镜头**,其中
   **至少一个 A 镜头 + 至少一个 B(性能)镜头**;镜头逐轮轮换(A11Y、
   视觉层级/密度、空/错/载态、动效/响应式、文案 i18n、首屏性能、bundle、
   API 延迟、渲染 jank……)。各 finder 产出 findings 带 file:line、截图路径、
   (性能类)实测数字。**轮内等齐**后逐条核对登记 BACKLOG:**先排除刻意决策**
   ——`git log -S` + 周边注释,有 QA-45/INC 依据的判 ✂ 记理由,不修。

4. **改进(Improve)— 并发多 implementer**:从 BACKLOG 选 **touches 互不重叠**
   的多条,**同时**各派一个 implementer(Agent 工具,`isolation: "worktree"`,
   并发跑),文件分区见下节。每个 implementer 的 prompt 给全:finding 详情、
   **互斥 touches 白名单**、验收断言、验证命令、纪律(不 commit dist、不动白
   名单外文件、styles.css 只追加注释块、vitest 全绿);性能条目额外要求交
   before/after 数字。**轮内 `wait` 到全部 implementer 完成**再进第 5 步。

5. **验证 / 合并 / 推送 / 部署 / 复测**:逐个 implementer:读报告 → 在其
   worktree 核验(node24 vitest + `npm run build` + `go build`)→ merge 回
   main → rebase(**dist 冲突一律 `rm dist/assets/*.js dist/assets/*.css`
   后重建,禁手工挑 asset**;推前 `grep -o 'index-[A-Za-z0-9_]*\.\(js\|css\)'
   dist/index.html` 与 `ls dist/assets/` 必须两两一致)→ push origin main →
   部署 8809 → playwright 复验改动点 + console 0;**性能改动复测 before/after
   数字证明变快**→ BACKLOG 打 ✅。

6. **反停滞审计 + 台账 + 不停机**(轮末必答,写进回复,任一为空当场亲手补
   一条再收轮,**不允许「等下一轮」**):
   - 本轮 push 了哪些 commit?两轴各推进了什么(A 做了啥 / B 快了多少)?
   - 现在有哪些子 agent 在跑 / 已跑完,各干什么?
   - BACKLOG 较上轮:+✅ 几条 / 新增 ☐ 几条?
   - 台账:BACKLOG 末尾追加一行 `- <日期时间> 轮N:巡检…、发现…、派工Z(并发)、
     push <sha…>、live=<js hash>、perf <before→after>`,与代码同批 commit+push。
   收完轮**继续**——报告 ≠ 停机。循环睡 1800 后自动跑下一轮。

## 并发与文件分区(硬性——这是把进度做快的主杠杆,默认就并发)

- **finder 并发**:read-only,无写冲突,直接一批多发,不同镜头并行取证。
- **implementer 并发**:每个跑在**独立 worktree**(`isolation: "worktree"`)
  且带一份**互斥 touches 白名单**——任意两个 implementer 的白名单**交集必须
  为空**。分区规则:
  - 按**文件 / 组件**切:各 implementer 只许改自己白名单内的文件。
  - **共享文件**(如 `styles.css`、路由表、`viewModels.ts` 这类多处要动的):
    要么把相关多条**并进同一个** implementer 串行做;要么本轮**只放一个**碰它、
    其余相关条目让路到下轮。`styles.css` 一律**只追加**注释块、不改既有块,
    把它从冲突源里摘出去。
  - 派工前自查:把每个 implementer 的白名单列出来,肉眼确认两两无交集再发。
- **冲突兜底**:merge/rebase 时源码冲突**保双方语义**;机器产物(`dist/*`)
  冲突一律删后重建,不手工挑。
- **别的 session 也在推 main**(Tailwind 迁移、INC 系列):纯视觉 CSS 重排它
  在做 → 避开或纯追加;结构/逻辑/新入口/正确性/性能类我方做。rebase 冲突先
  读对方 commit 意图。

## 性能实测速查(轴 B——量,别猜)

- **bundle 体积(gzip 后字节,最有意义)**:
  `for f in dist/assets/index-*.js dist/assets/index-*.css; do printf '%s  raw=%s  gz=%s\n' "$f" "$(wc -c <"$f")" "$(gzip -c "$f" | wc -c)"; done`
- **导航时序**:playwright `page.goto(url, wait_until="domcontentloaded")` 后
  `page.evaluate("JSON.stringify(performance.getEntriesByType('navigation')[0])")`
  取 `domContentLoadedEventEnd` / `loadEventEnd` / `responseEnd`;多取几次取中位。
- **API 延迟**:`for i in 1 2 3 4 5; do curl -w '%{time_total}\n' -s -o /dev/null 127.0.0.1:8809/<路径>; done`(取中位)。
- **交互 jank / 渲染**:playwright tracing 或 `performance.now()` 包住交互测耗时;
  长任务看 `PerformanceObserver({type:'longtask'})`。
- baseline 与改后数字都落 BACKLOG / `qa/runs/`,改进要**可证**。

## 环境速查

- node24:`export PATH="$(ls -d $HOME/.nvm/versions/node/v24* | tail -1)/bin:$PATH"`
- 前端:`cd webui/frontend && npx vitest run && npm run build`
- **部署 8809(launchd 守护制,2026-07-11 起)**:8809 由
  `com.agentrunner.webui8809` LaunchAgent 常驻(KeepAlive,session 无关,
  自愈)。部署 = 换二进制 + 重启服务,**不要**自己起进程:
  ```
  BIN=~/.local/share/agentrunner/bin
  cd webui && go build -o "$BIN/arwebui-live.new" . && mv "$BIN/arwebui-live.new" "$BIN/arwebui-live"
  cd .. && go build -o "$BIN/ar-live.new" ./cmd/agentrunner && mv "$BIN/ar-live.new" "$BIN/ar-live"   # CLI 有变时
  launchctl kickstart -k gui/$(id -u)/com.agentrunner.webui8809
  ```
  验:`curl -s 127.0.0.1:8809/ | grep -o 'index-[A-Za-z0-9_-]*\.js'`(hash 含
  连字符,字符类别漏 `-`)与 dist 一致 + health 200。
  服务日志:`~/Library/Logs/agentrunner-webui8809.log`。
- playwright venv:
  `/private/tmp/claude-501/-Users-yadong-dev2-agentrunner/b84daf52-9db3-44c9-8c46-9a5d9f61a6df/scratchpad/pwenv/bin/python`
  (若丢失:任意 scratchpad `python3 -m venv pwenv && pwenv/bin/pip install playwright`)。
  chromium `channel="chrome"` headless、dsf=2;session 页 goto 用
  `wait_until="domcontentloaded"` + sleep(networkidle 被 SSE 卡死)。
  快速切会话时被切走会话的在途请求 502 属已知良性瞬态;稳态单页必须 0。
- 常用真实会话:富=`20260711-011831-what-is-the-project-297d`、
  approval=`20260711-040811-reply-with-exactly-qa45-thinki-b98f`、
  diff=`20260710-213428-create-qa42-worktree-browser-t-d8ac`。
- 参照文档:`docs/increments/INC-41-CODEX-UI-REFERENCE.md`(与 QA-45 实测
  冲突处以 QA-45 为准)+ `INC-41-BACKLOG.md`(任务登记簿,canonical)。

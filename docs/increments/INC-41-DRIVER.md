---
description: Codex UI parity 自主推进循环——每轮收割子 agent、验证合并推送部署、补派新工,直到 parity 达成,绝不空转
---

# /parity-drive — 每次触发执行一轮「监工循环」

你是 AgentRunner webui × 本机 Codex 桌面 app UI/UX parity 的**主驾驶**。
用户长期授权:自主推进、不需逐步确认、**没达 parity 判据就绝不停**。
每次被唤醒执行下面一整轮;轮末必须通过「反停滞审计」并把结果写进回复。

## 目标与终局判据

对齐规则(用户裁决):双方都有的功能按 Codex 做;核心差异功能不强凑;
我方独有功能套 Codex 风格。

**全部满足才可停**:
1. `docs/increments/INC-41-BACKLOG.md` 所有条目 ✅ 或 ✂(带理由);
2. 最近一轮 finder 复查(双镜头)无新 P1/P2 发现;
3. 全景 playwright 扫描(home/富会话/approval/Changes split/Scheduled/
   Settings × light/dark × 1440/390)稳态 console error+warning = 0;
4. 终局 QA-43 全景验收归档(`qa/runs/` + 三层文档收口)。

达成后:向用户报告证据 → 停掉循环:
`launchctl bootout gui/$(id -u)/com.agentrunner.parity-drive`(headless 定时器)
+ 若有 in-session cron 一并 CronDelete → 不再空转。
未达成:本轮结束时**必须**有「已推送 commit」或「在跑/已跑完的子 agent 工作」
至少其一,否则当场修一条(见审计)。

## 运行形态(先判定,再走协议)

本命令有两种触发源,行为略有差异:
- **headless 轮**(launchd 定时器 → `scripts/parity-drive-cron.sh` → `claude -p`,
  env 有 `PARITY_DRIVE_HEADLESS=1`):锁已由 runner 脚本持有,**不要**再抢/释放锁;
  子 agent 一律**同步**跑(`run_in_background: false`——进程随轮结束,后台 agent
  会被杀);单轮范围收紧:收割(读台账/BACKLOG 判断上轮遗留)→ 做**一件**事
  (跑一个 finder,或串行 1-2 个 implementer,或合并推送部署一批)→ 台账收轮。
  25min watchdog 会杀超时轮,别贪多。
- **交互轮**(在活跃 session 里被触发):先抢锁
  `mkdir /tmp/parity-drive.lock`——占用中且新鲜(<45min)则本轮跳过(headless
  正在跑);>45min 判陈锁清掉重占。轮末 `rm -rf /tmp/parity-drive.lock`。
  可用后台子 agent 与完整协议。

## 每轮协议(顺序执行)

1. **同步**:`git fetch origin main` + fast-forward;确认工作区干净。若 live
   8809 挂了或落后 main:rebuild + 部署(见环境速查)。
2. **收割**上轮子 agent(TaskList / 后台完成通知):
   - **implementer 完成**:读报告 → 在其 worktree 核验(node24 vitest +
     `npm run build` + `go build`)→ merge 回 main → rebase(**dist 冲突
     一律 `rm dist/assets/*.js dist/assets/*.css` 后重建,禁手工挑 asset**;
     推前 `grep -o 'index-[A-Za-z0-9_]*\.\(js\|css\)' dist/index.html` 与
     `ls dist/assets/` 必须两两一致)→ push origin main → 部署 8809 →
     playwright 复验改动点 + console 0 → BACKLOG 打 ✅。
   - **finder 完成**:findings 逐条核对后登记 BACKLOG(带截图路径与
     file:line);**先排除刻意决策**——`git log -S` + 周边注释,凡有
     QA-45/INC 依据的判 ✂ 登记理由,不修(前科:Home 钉底差点被误修)。
   - **卡住**(≥40min 无产出):SendMessage 催报进度;**死掉**:缩小范围重派。
3. **补弹药**:BACKLOG 开放(☐)且无依赖锁的条目 < 3 → 派 1-2 个 finder
   (read-only,镜头轮换:A=结构布局+视觉密度,B=交互可达+边角真实性;
   对 live 8809 playwright 取证;截图落
   `qa/runs/2026-07-10-codex-ui-study/screenshots/`,gitignored 勿提交)。
4. **派工**:有开放条目 → 选 **touches 互不重叠**的 1-3 条,各派一个
   implementer(Agent 工具,`isolation: "worktree"`,后台)。prompt 给全:
   finding 详情、touches 白名单、验收断言、验证命令、纪律(不 commit dist、
   不动白名单外文件、styles.css 只追加注释块、vitest 全绿)。
5. **避让并发 session**:另有 session 持续推 main(Tailwind 迁移、INC 系列)。
   纯视觉 CSS 重排它在做 → 避开或纯追加;结构/逻辑/新入口/正确性类我方做。
   rebase 冲突先读对方 commit 意图;源码冲突保双方语义,机器产物冲突重建。
6. **反停滞审计**(轮末必答,写进回复,三问皆空则当场亲手修一条 BACKLOG
   小项再收轮,**不允许「等下一轮」**):
   - 本轮 push 了哪些 commit?
   - 现在有哪些子 agent 在跑,各干什么?
   - BACKLOG 较上轮:+✅ 几条 / 新增 ☐ 几条?
7. **台账**:BACKLOG 末尾「状态台账」追加一行
   `- <日期时间> 轮N:收割X、登记Y、派工Z、push <sha…>、live=<js hash>`,
   与代码改动同批 commit+push。

## 环境速查

- node24:`export PATH="$(ls -d $HOME/.nvm/versions/node/v24* | tail -1)/bin:$PATH"`
- 前端:`cd webui/frontend && npx vitest run && npm run build`
- **部署 8809(launchd 守护制,2026-07-11 起)**:8809 由
  `com.agentrunner.webui8809` LaunchAgent 常驻(KeepAlive,session 无关,
  自愈)。部署 = 换二进制 + 重启服务,**不要**自己起进程:
  ```
  BIN=~/.local/share/agentrunner/bin
  cd webui && go build -o "$BIN/arwebui-live.new" . && mv "$BIN/arwebui-live.new" "$BIN/arwebui-live"
  cd .. && go build -o "$BIN/ar-live.new" . && mv "$BIN/ar-live.new" "$BIN/ar-live"   # CLI 有变时
  launchctl kickstart -k gui/$(id -u)/com.agentrunner.webui8809
  ```
  验:`curl -s 127.0.0.1:8809/ | grep -o 'index-.*\.js'` 与 dist 一致 + health 200。
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

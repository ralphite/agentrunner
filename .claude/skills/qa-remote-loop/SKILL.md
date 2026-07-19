---
name: qa-remote-loop
description: 远程真机 QA 循环——agent 亲自驱动部署好的 webui(GitHub-transport driver),证据先行找问题,修复后红转绿闭环。用于 webui/runtime 的探索性 QA、bug 定性与修复验证。当要"测试 webui"、"找 UI/语义问题"、"验证某个修复"时用它。
---

# qa-remote-loop · 远程真机 QA 循环

QA-0719 实战沉淀。这套流程一天内抓到并修掉 6 个真 bug(CJK 双路径
octal、staged 全盲、sa-status 截断×2、goal pill 溢出、sa-row 参差),
并三次把"疑似 CRITICAL"证伪在动手之前。核心不是脚本,是纪律。

## 一、原则(为什么不用脚本驱动)

**禁止写伪探索 MJS driver**(用户硬性裁决)。探索性 QA 必须由 agent
亲自驱动:看内容 → 判断 → 决定下一步。确定性管道(起环境、下载截图、
轮询)可以用脚本(qa/driver/);"决定测什么、怎么判定"不可以。

## 二、环境:GitHub-transport driver

沙箱 egress 只放行 GitHub 域,一切经 GitHub 中转:

1. **起环境**:`qa/driver/dispatch-env.sh [minutes] [store_prefix]`
   (dispatch remote-qa-env workflow + 轮询到 driver ready,输出
   `READY issue=<N> run=<ID>`)。默认 QA store 前缀;传
   `agentrunner-store-` 可只读借用 phone 会话数据。
   注意 workflow `concurrency: cancel-in-progress`——新 env 会取消
   旧 env;被 cancel 的 run 不保存 store cache。
2. **下指令**:以 repo owner 身份在 `qa-driver <run_id>` issue 发
   JSON comment:`{"n":<全局递增>,"steps":[...]}`。ops:
   goto/click/fill/press/type/waitMs/waitSel/waitText/eval/shot/
   scroll/reload/viewport/end。
3. **收结果**:bot 回帖 JSON(evalResults/consoleErrors/shots/page);
   截图是 release `qa-driver-<run_id>` 的 asset,
   `qa/driver/shot.sh <run_id> <shot-name> <out.jpg>` 下载后用 Read
   看图。
4. **真 turn 可用**:runner 带 GEMINI_API_KEY,可对会话发真消息跑真
   generation——workspace-事实面的测试必须这么测(见 §四)。

传输坑(实测):
- comment 里**永不写裸 `#`**(MCP 会 mangle);首个 goto 用完整 URL,
  之后一律 `eval location.assign('/#...')` 导航。
- 每条 comment 的 `n` 全局递增,乱序会被拒。
- env 有 deadline,发指令前先想"这个 env 还剩几分钟"。
- **dispatch 前查并发**(dispatch-env.sh 已内建护栏):cancel-in-progress
  会杀当前活着的 env,而它可能是**并发 session 正在驱动的**(driver
  issue 几十条评论=活跃工作)。有 in_progress run 时默认拒绝;也不要
  往别人的 driver issue 插指令(n 计数器会撞)。等它结束或协调。

## 三、驱动纪律(每条都是踩过的坑)

1. **toggle 元素 check-then-click**:Environment/rail 这类开关面,
   先 `!!document.querySelector('.supervision-panel')` 判状态,仅在
   关闭时才点。盲点会把开着的关掉,读出假"无内容"。
2. **颜色/主题只信截图**,且判主题前必须 `reload`(eval 改
   data-theme 不重绘已挂载区,会看到假半白页)。computed-style 推断
   产生过 3 次深色模式假警报。
3. **文字截断判定用 `scrollWidth > clientWidth`**,不是
   textContent+宽度(textContent 永远是全文,曾因此把没修好的截断
   误判为 GREEN)。
4. **innerText 尊重 text-transform**:CSS uppercase 会让 /Queued/
   匹配不到 "QUEUED"。正则失配 ≠ UI 缺失,回截图确认。
5. **恢复态 env 的 git 面先验底座**:cache 恢复 runtime/ws(scratch
   在)但**不恢复 worktree 检出**——先
   `fetch /api/sessions/<sid>/diff` 看 `known/isRepo/workspace` 再下
   结论,isRepo:false 的空可能是 env 假象不是产品 bug。
6. **等 turn 结束再发下一条**(除非测 queue/steer):轮询
   `/api/sessions` 里该 sid 的 status 回 `waiting:*`。
7. **测试数据一律保留**:不 close 不删;破坏性确认框(Undo 全量
   revert 等)截图取证后一律 Cancel。
8. 每步随手断言 `consoleErrors` 与
   `document.documentElement.scrollWidth-innerWidth`(横向溢出)。

## 四、找问题的 oracle(方法核心)

**同一事实的多个面必须一致——不一致处必有 bug(或必有你没懂的机制)。**

- **三方对账:UI ↔ API ↔ journal**。rail 数字 vs
  `diff?scope=working-tree` vs `git status`(叫 agent 跑给你看) vs
  `/events` 的事实链。本法抓到:CJK 两条 git 路径只修了一条(rail 好
  卡乱码)、staged 变更整面蒸发(UI 说 Nothing to commit,git 说
  4 files to be committed)。
- **journal 时间戳定案**:`/api/sessions/<sid>/events` 的 `ts` 字段
  (注意不是 at)。任何"状态无端变了"的疑案,先查事件窗口再定性——
  曾靠它把"workspace 静默改写 CRITICAL"证伪成用户本人操作。
  副产品坑:webui 发的消息 source 标 "cli"(INC-83 提案),溯源时别被
  骗。
- **workspace-事实面必须真 turn 测**:在活 env 里对会话发"创建
  CJK 文件名文件/改文件/git add"类真消息,再对账三方。恢复态数据
  测不出 diff/staged/undo/commit 这些面。
- 每轮硬性覆盖(继承 remote-qa-env.yml 头部清单):双主题、双视口
  (1280×800 与 390×844)、全新会话首条回复无 `Edited N file` 幽灵、
  diff/commit 在**有真实变更**时打开看。

## 五、修复闭环(缺一环不算完)

1. **根因**:读代码到 chokepoint(不是现象层贴补丁);修一条路径时
   问"同一事实还有没有第二条路径"(CJK 有 webui git() 和 shadow
   两条,修一漏一)。
2. **修 + 回归测试钉死**(Go test / vitest,测试要写成"没修必红")。
3. 本地闸:相关 vitest/go test 全绿 + build 过。
4. **commit 即 push origin/main**(并发 session 在推,rebase 后推)。
5. **phone-webui 重部署**(用户手机复查是标准环节,不等吩咐)。
6. **新 env 红转绿**:起含修复的 env,在**同一个锚会话**(复现原 bug
   的那个)上断言修复生效,数据面+截图双确认。
7. **LOG 台账**(docs/LOG.md):根因、修法、验证;**误判也留痕**
   (何以误判、如何证伪)——这是流程能自我改进的原因。

## 六、升级边界

- 触 DESIGN 不变量/新增事件字段/行为语义变更 → 停,写增量提案
  (docs/increments/INC-xx,PROCESS.md 流程),不先斩后奏。
- 模型行为问题(编造、语言混杂、内部 id 泄漏)→ 登记归档,不改代码。
- 判不准 by-design 还是 bug → 读实现注释与 DESIGN,证据不齐就登记
  "待核"而不是修。

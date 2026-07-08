# arweb UI overhaul — 欠缺清单

> **状态更新(2026-07-08 轮 10)**:用户下达 goal「full features + easy to use,
> 像 Claude Code app」。下面 §1/§2/§3 + §4 的 fork/barrier/submit/trust 已全部
> 落地并真 Gemini 真验(见 PROGRESS 轮 10)。§4 的 drive 面板收尾中。剩余 ⛔ 项
> 是产品缺口(页面做不出),保留说明。

盘点依据:docs/JOURNEYS.md(22 条 journey)、docs/SPEC.md(功能登记簿
+ CLI 子命令附录)、internal/cli 实际命令面(2026-07-08 HEAD)。
驾驶舱使命(web/DESIGN.md):**以真实用户方式驱动全部会话能力,功能
追求全覆盖**——下面按"产品有而页面没有"逐项列出。

图例:☑ 本轮已改(你已拍板)· ▢ 待你确认后补上 · ⛔ 产品本身没有
(页面无从测,列出防止误当 UI 欠账)。

---

## 0. 本轮已落地(你拍板的第 1、2 点)

- ☑ **移除"关闭会话"**:按钮、`/api/sessions/{sid}/close` 端点、CLI
  白名单行全删。静止模型(决策 #30/#31)下 session 无终态、不可关闭;
  journal 里的 `session_closed` 是标记事件(kill 等路径仍会产生),
  照常渲染为 "session marked closed/killed — send to continue"。
  已写入 web/DESIGN.md 铁律 I7。
  - 注:`ar close` 命令与 CLI help("end a session gracefully",
    cli.go:114)仍是旧词汇——那是产品面(internal/),按铁律不在
    web/ 改;PROGRESS.md 已记"另立任务"。**要不要我顺手立一个产品侧
    增量把 `ar close` 语义/help 改成 mark 词汇?**
- ☑ **UI 全英文化**:全部文案、状态词、事件 chip、对话框、默认 spec
  (system_prompt 也改英文,方便你核对身份切换);词汇对齐 journal
  事件名/CLI 命令/fold 状态词(waiting:input、spec_changed、handle、
  interrupt……)。已写入铁律 I6。

---

## 1. Composer 能力面(你拍板的第 3 点——形态方案,待确认)

产品侧事实(决定了 UI 能提供什么):

- **会话中可变的只有 agent**:`ar agent <sid> <spec.yaml>`(决策 #32,
  QA-10)整份换 spec,下一条消息生效。**model 不是独立动作**——它是
  spec 的字段(`model: {provider, id, max_tokens}`),"换 model" =
  改当前 spec 的 model 块再走一次 `ar agent`。
- `ar send` 只有 `--image`(和 `--detach`);**没有 per-message 的
  agent/model/mode 参数**。所以粒度只能是"会话级、从下一条消息起
  生效",不是"只这一条消息用 X"。
- mode 只在 `ar new --mode` 存在;会话中途无 CLI 动作可改(
  exit_plan_mode 是模型侧工具)。
- provider 现实:gemini(主)、anthropic(次,无凭据未真验);
  当前有效 Gemini id 例:`gemini-flash-latest`、`gemini-2.5-pro`。

提议的 composer 布局(textarea 上方一排可选控件,全部 optional,
不动 = 维持现状):

- ▢ **Agent 选择器**:下拉 = 预置模板(dev / auditor / worker…)
  + "custom (edit YAML)";选中非当前项即调 `/agent`(时间线出
  spec_changed chip)。现有的 "switch agent" YAML 对话框保留为
  custom 入口。
- ▢ **Model 选择器**:下拉(gemini-flash-latest / gemini-2.5-pro /
  custom id…);实现 = 取"当前生效 spec"改 model 块 → `/agent`。
  当前 spec 的来源:页面记住本会话最近一次 new/agent 提交的 YAML;
  刷新后从 journal 的 session_started/spec_changed payload 恢复
  (journal 里存的是 spec JSON,恢复后以 JSON→YAML 或直接 JSON 提交,
  待我实现时定)。
- ▢ **Mode 显示**:composer 只读显示当前 mode(journal mode_changed
  折出);**中途改 mode 无产品动作**——若你要,这是产品增量(走
  PROCESS 三层流程),不是 UI 补齐。要吗?
- ▢ **新会话对话框同步改造**:agent 模板下拉 + model 下拉 + mode
  (已有)+ YAML 高级编辑折叠;不再强迫手写 YAML 才能开会话。
- 图片:已有,不动。

待你确认:模板集合(dev/auditor/worker 够不够)、model 候选表、
以及上面四个 ▢ 哪些做。

---

## 2. 会话生命周期面(页面欠账)

- ▢ **stranded / marked / quiescent 状态识别与指路**:sessions list
  的状态词已有 `waiting:input|approval` `running` `stranded`(running
  但宿主没了,daemon 重启后出现)`closed` `killed` `completed`
  `canceled` `handoff` `limit_exceeded` 等;页面 pill 只认识
  idle/run/appr/closed/crash 五类,列表纯文本透传。欠:
  - pill/列表按新词汇上色分类(stranded 该醒目——它意味着"没人在
    推进这个会话");
  - 对 stranded/marked 会话,在 composer 顶部给一行提示
    "session is stranded/marked — sending will revive it"(产品语义:
    send 对任何 session 成立,daemon 收 send 自动复活,SPEC E)。
- ▢ **子会话时间线的英文只读横幅**:子会话视图现在只是隐藏按钮,
  没有明说"read-only view — approvals surface in the parent";加一行。
- ⛔ `ar resume`(独立宿主复活)不进页面:驾驶舱一切经 daemon,
  send 即复活;resume 是"无 daemon/前台宿主"路径,页面不做(如你
  另有想法再谈)。

## 3. 观察面(页面欠账,可选级)

- ▢ **usage/预算常驻显示**:inspect 已有 per-session token 用量与
  budget(reserved),页面只在工具卡 hover title 里藏了 tokens。
  提议:sesshead 加一个小 usage 徽章(billed tokens),点开 =
  inspect 面板(已有)。
- ▢ **inspect/fold/raw 三面板**现在是裸 JSON viewer——够用吗?
  (驾驶舱定位"朴素",我建议不动,除非你要结构化渲染。)

## 4. 产品有、驾驶舱当年裁掉的面(这次 overhaul 要不要纳入?)

web/DESIGN.md §7 曾裁定"驾驶舱聚焦会话交互测试,暂不做"。这次
"功能全覆盖"的口径下,请你重新拍板:

- ▢ **fork / barrier(时间旅行,UJ-15)**:`ar barrier <sid>` 手动
  打点、`ar fork --list` 列 barriers、`ar fork <sid> <barrier>` 分叉。
  UI 形态:session header 加 "barrier" 按钮 + "fork…" 对话框(列
  barrier 选一个分叉,新会话进列表)。
- ▢ **run / submit(one-shot,UJ-02/13)**:`ar submit` 把 one-shot
  交 daemon 托管。UI 形态:new session 对话框加 "one-shot" 模式。
- ▢ **drive(goal/loop/best-of-N 驱动系列,UJ-14/15/16)**:driver
  面板(起 drive、看逐轮分数/carry/预算)。这是最大一块,若要,
  建议单独一轮做。
- ▢ **trust(UJ-20)**:`ar trust <dir>`。页面造的空 workspace 无
  project settings 用不到;在真实 repo 上测 hooks/信任模型时需要。
  UI 形态:new session 对话框一个 "trust this workspace" 勾选。

## 5. 产品缺口(⛔ 页面无从做——防误会,不在本次范围)

这些是你测试时会"感觉缺"、但根子在产品未实现的,页面做不出来:

- ⛔ 父/用户 → 在飞子会话发第二条消息(子 run 无 inbox;提案 P2 在
  PROGRESS.md,动 internal/ 运行时语义,须走 PROCESS 增量)
- ⛔ 子会话打字级流式(childLoop 无 Out sink;提案 P1①;轮询已覆盖
  功能面)
- ⛔ 审批"允许且不再问"(规则写回,GAPS G5)——审批卡只能
  approve/deny
- ⛔ 手动 compact / clear(GAPS G7)
- ⛔ mid-session 改 mode(无 CLI 动作,见 §1)
- ⛔ 记忆写回 # remember(G9)、自定义命令/slash(G21)、
  grep/glob/web 独立工具(G18)、webhook 唤醒(G14)、远程 stop(G12)

---

## 建议的落地顺序(你确认后)

1. §1 composer 能力面 + §2 生命周期面(你点名的测试刚需)
2. §3 usage 徽章(顺手)
3. §4 按你的拍板结果排(fork/barrier 与 submit 较小,drive 面板大)

每步照 PROGRESS.md 纪律:fake-ar 单测绿 + 真 Gemini 真验后才勾。

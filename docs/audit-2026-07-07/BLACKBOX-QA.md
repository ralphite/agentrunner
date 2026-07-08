# AgentRunner 黑盒 QA 报告 — 2026-07-07(真·新用户视角)

**性质**:5 个测试 agent + 协调者本人,以**真正的黑盒**方式使用 `ar`——**零产品知识、禁读任何源码/docs/内部文档、只能从工具自身的输出(`--help`、报错、命令回显)学习用法**,像一个刚拿到二进制的第一次使用者那样摸索。全程真实 Gemini API。

**为什么重做**:上一份审计(同目录 AUDIT.md)是"白盒"——我把说明书递到了每个 agent 手里:spec YAML 模板、daemon 启动命令、XDG 路径、`new→轮询 sessions list→读 events.jsonl` 的流程、甚至内部事件类型名。那不是用户视角,是替用户读了手册。于是**基础工作流的硬伤全被跳过**——比如"发消息看不到回复",因为我直接叫 agent 去读 journal 文件。这一份专门补这个盲区。

---

## 一、一句话结论

**引擎是好的,第一次使用这层皮几乎处处是墙。** 一个只看 `--help` 的新用户,在**发出第一条有效指令之前**就要连闯"无帮助 → spec 格式靠猜 → 工具默认被禁且解锁法查不到"三关;即便闯过,还会撞上**"静默假成功"**(工具说成了、退出码 0,实则什么都没做)和**"对话回复完全不显示"**。5 个 agent 各自独立,**在头几分钟撞的是同一堵墙**——而这些墙,上一份白盒审计一个都没抓到。

尤其关键:**`run`(一次性)路径体验不错、输出清清楚楚;但 `new`/`send`/`daemon` 对话路径——也就是 web UI 底下那套——正是基础体验塌方的地方**(回复不显示、close 关不掉、不知道一轮完没完)。这与用户最初在 web UI 上遇到的挫败直接同源。

---

## 二、基础工作流问题(按对真实新用户的杀伤力排序)

杀伤力:🔴 致命(新用户会卡死/放弃/被静默误导)· 🟠 严重(困惑很久才过)· 🟡 糙(能过但恼火)。
括号内是独立命中该问题的来源(recon=协调者本人,B/C/D/E=各 agent)。

### 🔴 致命

#### [BW-1] 进不了门:`--help`/`-h`/`help` 全报 "unknown command",且无 README(recon·B·C·E)
每个新人第一件事就是求助,而三种最基本的求助方式在顶层全部:
```
$ ar --help
agentrunner: unknown command "--help"
usage: agentrunner <run|drive|daemon|new|send|...20+命令...>
```
仓库根没有任何 README(只有内部 CLAUDE.md)。顶层 usage 把 20+ 子命令一字排开、零解释,新人无法判断该用哪个。**更糟:子命令的 `-h` 行为不一致且有副作用**——`ar run -h` 给 flag(但 exit 2),`ar close -h` 却去 dial daemon 报错,`ar ps -h` 去读 sessions 目录报路径错,`ar trust -h` 直接 `lstat -h: no such file or directory` 崩掉。**求助不该有副作用。**

#### [BW-2] spec.yaml 无从发现——最硬的第一堵墙(recon·B·C·D·E)
`run` 和 `new` 都强制要一个 `spec.yaml`,但**它的格式在任何用户够得到的地方都没有**:没有 `ar init`/模板/示例,`run -h` 只列 flag 不说 spec 内容,报错还泄漏内部 Go 类型名:
```
$ ar run guess.yaml "..."   →  field task not found in type agent.AgentSpec
$ (model: gemini)           →  cannot unmarshal !!str into agent.ModelSpec
$ (empty file)              →  spec ...: EOF
```
只能"喂一个错字段 → 读报错补一个字段"逐层反推,4 位 agent 都花了 **8~10 轮**才拼出最小可用 spec。**连合法工具清单,都是靠"故意写个假工具名、让报错吐出白名单"才拿到的**——纯运气,不是引导。

#### [BW-3] 工具默认被禁,解锁法查不到(recon·B·D)
- 默认 spec 下 AI **一个工具都没有**,只会空谈(83 token 就 `blocked`)。
- 即便声明了 `tools: [write_file, bash]`,工具调用仍被 **`denied by policy`**。
- 真正的开关(`-mode bypass` / `AGENTRUNNER_APPROVE=always` / spec 里 `permissions:[{action:allow}]`)**在所有 `--help`/报错/示例里都不存在**。Agent D 能解锁,纯粹因为**AI 自己在回复正文里漏出了 `AGENTRUNNER_APPROVE` 这个变量名**。`-mode` 的四个值只有名字、零解释,新人不可能知道 `bypass` 才是让 AI 真正动手的钥匙。
- 已独立复现:带 `tools:` 但无 `permissions` 字段、无 bypass → 让 AI 写文件 → 磁盘上什么都没有。

#### [BW-4] 静默"假成功":说成了、退出码 0,实则什么都没做(B·C·D·E)
三种变体,都会让用户误以为成功:
- **`run` 假成功**:所有 `edit_file`/`bash` 被 `denied by policy`,run 却以 **exit 0 + "run completed"** 收场,AI 正文还热情说"已修复,测试 PASS"。只看退出码或 AI 结论的人以为成了,实则 `calc.py` 一字节没动。(Agent D 修 bug 场景)
- **`close` 假成功**:`ar close` 回 `closing`、退 0,但随后 `ar send` 仍 `delivered` 并**真跑出新一轮**;`inspect` 自相矛盾地写 `waiting (closed)`。关了个寂寞,还骗你成功。(C·E)
- **AI 退回"贴代码"**:工具被拒/轮数不够时,AI 把整段代码贴进聊天让你"自己去终端跑",看着像完成,磁盘空空。(B)
- 附:被取消的会话在 `sessions list` 里显示成 **`completed`**,和真正完成无法区分。(E)

#### [BW-5] 对话回复完全不显示——聊天的命根子断了(recon·C·E)
```
$ ar new chat.yaml "write a two-line poem"   →  只有一个 session id
$ ar send <sid> "add a delete feature"       →  delivered      ← 回复在哪??
```
`new` 只给 id,`send` 只回 `delivered`,**AI 写的诗/代码在两处输出里都看不到**。想读回复,四条路全有硬伤:
- `ar attach <sid>` → 能看到全文,但**挂住不返回**(实测卡满 2 分钟被杀),名字也不像"看回复"。
- `ar events <sid>` → 会退出,但回复正文**被截断成 `…`**,还埋在 `effect_requested`/`waiting_entered`/`gate_results` 一坨黑话里。
- `ar events -json` → 有全文,但要**手写 JSON 解析**扒 `payload.message.parts[].text`。
- `ar inspect <sid>` → 只有 token 报表,**根本没有回复正文**。

对比之下 **`run` 把 AI 输出显示得清清楚楚**(逐 gen-step 打印工具调用与结果)——同一个产品,`run` 与 `new`/`send` 两种相反行为。

### 🟠 严重

#### [BW-6] daemon 是隐性前提,且前台阻塞(recon·C·E)
`new`/`send`/`close` 都要先有个 daemon,但报错只说 `... (is the daemon running?)`——**不告诉你命令是 `ar daemon`、也不说这些命令依赖它**;而 `run` 又不需要 daemon(同一工具两套心智模型)。`ar daemon` 还会**卡住终端**(不加 `&` 不还提示符),没有 `--detach`。

#### [BW-7] 一轮结束没有人话信号,只能 poll(C·E)
唯一"轮结束"的信号是 `events` 里的 `waiting_entered {"kind":"input"}`——黑话、要主动反复查。没有"AI 回复完毕/该你了"这类提示,也没有阻塞等待。用户只能 `for … ar events … grep` 轮询才知道好没好。

#### [BW-8] Ctrl-C 一次停不下来,第二次才杀,退出脏(E,与白盒 A2-1 同源)
第 1 次 SIGINT 只印 `↺ interrupted by user` 但正文继续流、进程存活;第 2 次才终止,并甩出内部 `WARN barrier skipped: snapshot failed` / `context canceled` 一堆日志。

#### [BW-9] 报错泄漏内部术语/Go 类型/日志管道(B·D·E)
`agent.AgentSpec`/`ModelSpec`、取消时的 `snapshot: git add: context canceled`、`events` 满屏 `checkpoint_barrier/effect_resolved/gate_results/verdict/snapshot_ref`——内部实现细节直接糊到用户脸上。

### 🟡 糙

- **[BW-10]** `ps` 名不副实:不是"列全部",要 `<session-id>`;真正的列表是 `sessions list`;空态还泄漏 `.../sessions: no such file` 路径。(D·E)
- **[BW-11]** 打错子命令(`ar rnu`/`ar sesions`)无"你是不是想说 run?"建议,只甩命令墙。(E)
- **[BW-12]** 无"当前会话"概念,每次都得贴 50 字符长 id(支持前缀匹配算缓解)。(E)
- **[BW-13]** `model.id` 无默认/无候选提示,退休模型(`gemini-2.0/2.5/1.5-flash`)全 404,盲试到 `gemini-flash-latest` 才中。(B·C·D)
- **[BW-14]** 密钥缺失的报错 `GEMINI_API_KEY not set` **清楚点名了变量**(缺"去哪拿/怎么 export"),是全工具**最体面的一条报错**——恰恰反证:工具能好好引导,只是在更难的几堵墙上没做。(协调者 probe)

---

## 三、经得起夸的部分(重要平衡)

黑盒之下,底层引擎依然亮眼:

- **脏输入坚如磐石**:反引号、`$(whoami)`、引号、`<tag>`、emoji🎉、日本語、换行、2 万字——全部原样通过,**无崩溃、无乱码、无命令注入**。(E)
- **AI 侧多轮对话完美**:4 轮连续开发,上下文严丝合缝(token 累加证明整段历史都在喂),第 4 轮精准复述前 3 轮全部改动。(C)
- **`run` 路径可见性极佳**:逐 gen-step 打印每个工具调用 `→ bash {...}` 和结果 `← ok/error`,AI 干了啥一目了然。(D)
- **闯过咒语之后整条链路是通的**:正确 spec + `bypass` 下,AI 真写文件、真跑 bash、真回灌结果,可独立验证。(B·D)
- **凭据报错点名了环境变量**,是最好的一条错误信息。(probe)

**一句话**:问题几乎全在最外层"第一次使用"这层皮,不在内核。

---

## 四、这次为什么能挖到、上次为什么挖不到

| 上一份白盒审计的做法 | 造成的盲区 | 这次黑盒抓到的 |
|---|---|---|
| 给了 agent 现成的 spec 模板 | 从不经历"spec 怎么写" | BW-2 spec 无从发现 |
| 给了 daemon 启动命令 + XDG 路径 | 从不经历"要不要 daemon、怎么起" | BW-6 隐性 daemon |
| 教 agent 用 `permissions:[{action:allow}]` | 从不经历"工具默认被禁" | BW-3 工具默认禁 + BW-4 假成功 |
| 教 agent 轮询 sessions list、读 events.jsonl | 从不经历"我的回复在哪" | BW-5 对话回复不显示 |
| 用内部事件类型名判断状态 | 把黑话当正常 | BW-7 无轮末信号 · BW-9 泄漏内部 |

**教训**:要测"新用户体验",就必须真的不给 agent 任何产品知识——它们越是什么都不懂,越能撞出真实的墙。

---

## 五、修复优先级建议(纯建议,本次不动代码)

1. **BW-5 + BW-7(对话回复可见 + 轮末信号)** ——直接决定 `new`/`send` 还算不算"能用"。让 `send` 发完就地流式回显 AI 回复、轮末给一句人话提示;提供一条"把对话当聊天读全"的命令。这也是 web UI 挫败的根。
2. **BW-3 + BW-4(工具解锁可发现 + 不再假成功)** ——`denied by policy` 那行自带解法提示;被拒时 run 以非零退出或结尾醒目标"⚠ N 个改动被拒,工作区未变更";`close` 说到做到并给终态确认。
3. **BW-2 + BW-1(spec 脚手架 + 帮助)** ——`ar init` 生成带注释的样例 spec;`ar --help`/`-h` 给上手引导;所有子命令 `-h` 只印帮助、绝不触发副作用。
4. **BW-6(daemon)** ——`new` 自动拉起 daemon,或明确指路;`ar daemon` 提供 `--detach`。
5. 其余糙边(BW-9~13)顺手清理。

---

## 附:证据
逐 agent 的完整黑盒实录(每条命令原文 + 工具真实输出 + 新用户反应)见本目录 `blackbox-agentB/C/D/E.md` 与 `blackbox-recon.md`。Agent A(冷启动+凭据发现)因一次终端 API 错误早夭,其唯一独有角度(凭据发现)已由协调者 probe 补齐(BW-14)。

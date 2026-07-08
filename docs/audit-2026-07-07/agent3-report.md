# Agent 3 报告:工具面与子 agent

## 零、执行摘要

**产品的工具面和子 agent 功能,在真实 Gemini API 下,10 个场景全部功能正常。** bash 前后台、文件工具、spawn(前台/后台)、task_kill、CLI kill、blackboard、artifact、semantic_search、并发 handle 全部真实 work,子进程真死、文件真落盘、CAS 校验一致、handle 不串。

**我一度误判出"critical:spawn/大输出杀死 daemon",深挖约 20 分钟,经 6 组对照实验证伪**:那是测试 harness 缺陷,不是产品 bug(详见"四、方法论教训")。

**真实产品问题只有 2 个,均边缘/低危**:handoff 正常终止返回非零退出码(major);空状态 ar ps 报错文案(minor)。外加独立复现 [A2-2]。

- 总计:26 顶层 session,73 generation-step,跨 6 个 XDG 位置。

## 一、逐场景使用日志

### 场景 1:bash 前台(go test)— PASS
1. `$ ar new --workspace ws-color base.yaml "用 bash 工具在当前 workspace 运行 go test ./... 命令,然后用一两句话总结测试结果(通过/失败/多少个测试)。"`
   → sid=20260707-233248-bash-works-5057,3 turn,约 7s
   → `bash{"go test ./..."}`(exit 0)→ `bash{"go test -v ./..."}`(stdout 4474 字节含真实 `=== RUN TestColor`)→ 回复"全部测试通过。共 22 个主测试(含子测试 27 个)全 PASS。"
   → 验证:events 里 activity_completed 含真实 exit_code=0 与 stdout;总结数字与实输出吻合。

### 场景 1b:bash 超长输出截断 — PASS
2. `$ ar send <sid> "用 bash 工具运行命令: seq 1 200000 (打印20万行)。然后只回答:你收到的 bash 输出是否被截断了?若被截断,截断处附近的提示文字是什么?原样引用。"`
   → `bash{"seq 1 200000"}` exit 0
   → 验证:tool result stdout **截断到 15395 字节**(原始 ~1.27MB),标记 = `\n[... truncated 1273535 bytes ...]\n`,头+尾保留(末尾可见 199999/200000)。不撑爆 context。

### 场景 2:bash 后台 + task_output/task_kill — PASS
1. `$ ar new --workspace ws-color base.yaml "<1.后台启动 sleep 300 && echo DONE_MARKER;2.task_output 查询;3.task_kill 杀掉;4.报告 handle/output/kill 结果>"`
   → `bash{background:true,"sleep 300 && echo DONE_MARKER"}` → handle call_1_0;`task_output{call_1_0}` → `{status:running,note:"output arrives as a message when the task finishes"}`;`task_kill{call_1_0}` → `{status:cancelling}` 随后 `{canceled:true,exit_code:-1,error:"command canceled"}`
   → 验证:外部 pgrep -x sleep → **无 sleep 300 孤儿**;DONE_MARKER 从未打印。

### 场景 3:文件工具真实编码 — PASS
1. `$ ar new --workspace sc3-ws base.yaml "<1.write_file greet.txt='Hello World\n第二行中文测试';2.write_file nums.txt;3.edit_file 把 Hello World 改成 Goodbye World;4.read_file 读回>"`
   → write×2 → edit_file{old:"Hello World",new:"Goodbye World"} → read_file → 回复贴出
   → 验证:**磁盘真实检查** cat greet.txt="Goodbye World\n第二行中文测试"(35 字节);edit 生效旧文本消失;中文 UTF-8 字节完好(hexdump 验证)。

### 场景 4:spawn_agent 后台并行 — PASS
spec 需 `agents: [worker]` + 同目录 worker.yaml(参照 qa09)。
1. `$ ar new --workspace ws-gin orch/lead.yaml "用 spawn_agent 工具(background=true, agent=worker)起恰好 2 个子 agent 并行调查本仓库,分工:- 子agent-A:find . -name "*.go" | wc -l...;- 子agent-B:ls binding...。启动后不要等待,立刻把两个 handle 报告给我。"`
   → `spawn_agent{agent:worker,background:true}` × 2 → **立即返回** handle call_1_0/call_1_1 不阻塞
   → 验证:`ar ps <sid>` 显示 2 在飞任务(running agent=worker);父 journal `spawn_requested` 含 `child_session: <sid>-sub-call_N_0-a1, depth:1`;子事件落在 `<sid>/sub/call_1_0-a1/events.jsonl`(**非**平级目录);父收到 `{reason:completed, report:"...go1.26.4...", turns:2}`,turn 3 转述。

### 场景 5:杀子 agent 双路径 — PASS
**(a) CLI ar kill**
1. spawn background sleep 90 → 外部 ps 确认 sleep 90 pid 19608 存在
2. `$ ar kill <sid> call_1_0` → "killing call_1_0" exit 0 → 3s 后 ps sleep 90 **消失**;子 sub/call_1_0-a1 记 activity_cancelled + partial_output{canceled:true,error:"command canceled",exit_code:-1}(**部分产出留存**);daemon ALIVE
3. `$ ar send <sid> "刚才那个子任务被取消了。现在只回答:1加1等于几?"` → "1加1等于2。"(**父会话未卡死**)

**(b) 模型 task_kill**: spawn background sleep 90 → task_kill → 外部 ps sleep 90 已死;daemon ALIVE

### 场景 6:publish_note/read_notes(blackboard)— PASS
1. `$ ar run --workspace ws-color orch/lead.yaml "分两步:1) 用 spawn_agent(agent=worker) 起1个子agent,任务是:用 publish_note 工具发布一条 note,内容为 GO_VERSION=go1.26。2) 子agent完成后,你用 read_notes 工具读取所有 notes,把你读到的 note 内容原样告诉我。"`
   → 子 publish_note → 父 read_notes{topic:go_version} → `{notes:[{seq:1,topic:go_version,from:worker,text:"GO_VERSION=go1.26"}]}` → 转述
   → 验证:read_notes 返回真实含子 agent note,带 from:worker 来源标记。

### 场景 7:publish_artifact — PASS
1. `$ ar run --json --workspace ws-color base.yaml "请完成一个小任务:用 publish_artifact 工具发布一个名为 summary.txt 的 artifact,内容为:这是 color 库的测试报告,所有测试通过。发布后告诉我 artifact 的引用/ID。"`
   → `publish_artifact{content,stream:summary.txt}` → `{output:published, ref:sha256-376edbb7..., stream:summary.txt, version:1}`
   → 验证:**磁盘** <sid>/artifacts/blobs/sha256-376edbb7... 内容=53 字节;`shasum -a 256` **computed==ref**(CAS 正确);有 manifest.json。

### 场景 8:semantic_search — PASS
1. `$ ar run --json --workspace ws-color base.yaml "用 semantic_search 工具在本仓库里搜索:颜色属性(color attribute)是怎么定义的?把 semantic_search 返回的前几个结果(文件名)原样告诉我,并简述你找到了什么。"`
   → `semantic_search{query}` → `{hits:[{path:color.go,line:181,score:3.305,snippet:"...func RGB...Attribute(r)..."},...]}`
   → 验证:hits 真实相关(最高分 color.go:181),带 path/line/score/snippet,非空(BM25 生效)。

### 场景 9:handoff_agent — PASS(功能)/ FAIL(退出码)
1. `$ ar run --json --workspace ws-color ho/lead.yaml "我需要一个数学专家。请用 handoff_agent 把这个任务交接给 specialist:计算 123 乘以 456 等于多少?"`
   → `handoff_agent{agent:specialist,task}` → `{agent:specialist, reason:completed, report:"[专家接手]...123 × 456 = 56088..."}`;**exit code=1**(异常)
   → 验证:specialist 真接手,计算正确 56088。功能对但顶层 exit code=1([A3-1])。

### 场景 10:压力(3 并发后台 bash)— PASS
1. `$ ar new --workspace ws-color base.yaml "<后台启动 sleep 3 && echo TASK_ALPHA/BETA/GAMMA 三个;等几秒后分别 task_output;报告>"`
   → bash{background:true}×3 → handle call_1_0/2_0/3_0;task_output×3
   → 验证:handle→输出**精确无串**:call_1_0→TASK_ALPHA、call_2_0→TASK_BETA、call_3_0→TASK_GAMMA(exit 均 0);daemon ALIVE。

## 二、发现的问题

### [A3-1] handoff_agent 正常终止返回非零退出码(exit 1),与失败无法区分 🟠 major
- 复现: `ar run --json ... "...用 handoff_agent 交接给 specialist..."`;`echo $?` → 1
- 期望: handoff 是正常成功终止(控制权移交,子 agent reason:completed),退出码应为 0。
- 根因: `internal/cli/run.go:229-233` `if result.Reason != "completed" { return ExitRun }`——handoff 顶层 result.Reason=="handoff" 被归入非零退出。该分支本意"max_generation_steps 强制停止不算成功",误伤正常 handoff。stderr 明打 `run handoff: 1 turns` 却仍 exit 1。CI/脚本会把成功 handoff 当失败。
- 证据: agent3/sc9.jsonl、sc9.err

### [A3-2] 空状态 `ar ps`(无参)报错文案不友好 🟡 minor
- `ar ps`(不带 sid,或无 sessions 的新 XDG)→ 先抛 `no sessions found (open .../sessions: no such file or directory)` 再打 usage。把"目录不存在"当错误暴露,新用户首次运行即见报错。

### [A2-2] 超长 XDG 导致 daemon 启动失败(独立复现,已登记)🔴

## 三、turn 计数
- 顶层 session **26 个**,跨 6 个 XDG(/tmp/ar3d /ar3d2 /ar3k /ar3f /ar3final + /var/folders/.../tmp.*)。顶层 generation-step **73**。另有子 agent 会话若干(存于各父 sub/<handle>/,未单列)。

## 四、方法论教训（请协调者重点看）

我一度得出"🔴 critical:spawn 子 agent/大 bash 输出会杀死 daemon,整个会话服务崩溃,精准解释用户'几条消息就崩'"。**经 6 组对照实验被证伪,是测试 harness 缺陷,不是产品 bug。**

- **假象**: daemon 反复 `Killed: 9`/exit 137、context canceled、会话卡 running,时点常在 spawn 或大输出后十几到几十秒。
- **根因**: 我的 Bash 工具每次是独立 shell,用 `nohup $AR daemon &` 启动时 daemon 落在**该次 Bash 调用的进程组**内。那次调用超时(exit 143)或结束时,**Claude Code 的 Bash 工具对整个进程组发 SIGKILL**,把 daemon 一并杀死 → 在飞 LLM 被 context canceled → 会话卡死。nohup 只挡 SIGHUP,挡不住定向进程组 SIGKILL。
- **证伪实验**: 用 `perl -e 'fork;setsid;fork;exec'`(双 fork+setsid)让 daemon 成为完全脱离的独立进程组 leader 后:detached daemon 跨 Bash 调用稳定存活;spawn 子 agent 后**存活 101 秒无恙**;连跑 3 轮 spawn **0/3 崩溃**;大 bash 输出用 ar run **稳定成功**(18379 tokens,答案正确)。
- **结论**: daemon、spawn、大输出本身都稳定。凡 daemon 在脚本进程组内的运行都"崩",凡彻底 detached 的都不崩 → 100% harness 假象。
- **给协调者**: 若有 agent 报"daemon 崩/会话卡 running/context canceled",先确认其 daemon 是否用 nohup & 挂在 Bash 进程组里、是否有相邻 exit 143 超时。qa/lib.sh 不踩坑是因为它全程在单个脚本进程里跑完 daemon+全部 new/send。真实用户用 `ar daemon` 常驻也不踩。

## 五、没测到的
- daemon 小时级常驻稳定性与内存增长(只见分钟级 30MB 稳定)。
- spawn depth>1(孙 agent)与数量上限/背压。
- task_output 对仍在运行的长任务取增量输出(后台任务都秒级完成)。
- handoff 后原会话是否真正终止不可再 send。
- publish_artifact 的 outputs contract 强校验、多版本、CLI 侧取 artifact(ar inspect/accept)。
- semantic_search 在大 repo(gin)+中文查询召回质量(只在 color 小 repo 验证)。
- network=none 容器化 bash(macOS 无 netns)。
- bash 前台真超时(120s)的 timed_out 分支。

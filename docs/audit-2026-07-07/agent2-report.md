# Agent 2 报告(重写版):task 模式与 driver —— 真实使用日志

> 全程真实 Gemini API(`gemini-flash-latest`),无 mock/fixture。以下每条命令的 task 消息为**逐字原文**,工具序列从各 session 的 `events.jsonl` 读出,验证动作为 agent 独立执行。二进制 `agentrunner dev go1.26.1`。共用 spec 骨架(gemini flash / max_tokens 2048 / tools: read_file write_file edit_file bash / permissions: allow)。

---

## 场景 1: one-shot task —— 写并验证 fizzbuzz

**0. spec(`dev.yaml`)关键段**
```yaml
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
tools: [read_file, write_file, edit_file, bash]
permissions: [{ action: allow }]
```
workspace:空目录(`ws.sh prepare blank`)。

**1.** `$ ar run --workspace ws dev.yaml "write a python script fizzbuzz.py that prints fizzbuzz for 1 to 30, then run it with python3 to verify it works, then exit"`

→ 系统反应(从 journal 逐事件读出):
- gen-step 1 `write_file{path: fizzbuzz.py, content: "for i in range(1,31): …FizzBuzz…"}` → `wrote fizzbuzz.py (196 bytes)`
- gen-step 2 `bash{command: "python3 fizzbuzz.py"}` → `exit_code 0, stdout "1\n2\nFizz\n4\nBuzz\n…\nFizzBuzz\n"`(完整 1–30)
- gen-step 3 收尾回复:「我已编写并运行了 `fizzbuzz.py` 脚本,输出完全符合 1 到 30 的 FizzBuzz 规则。」
- stderr:`session 20260707-233204-…-e466` / `run completed: 3 turns, 2934 in / 340 out tokens`
- **退出码 0**,**耗时 3.95s**

→ 验证:
- `cat ws/fizzbuzz.py` → 196 字节,逻辑正确(先判 `%3==0 and %5==0`)。
- journal:35 行,逐行 json.loads **0 bad**;末事件 `task_completed {reason: completed, gen_steps: 3}`。

**判定:PASS** —— 文件真的在、脚本真的跑过、末事件是 completed、CLI 输出人类可读。

---

## 场景 2: 真实修复 —— cobra-broken 的失败测试(最典型用例)

**0.** workspace = `ws.sh prepare cobra-broken`(注入 `qa_inject/calc.go`:`return a * b // BUG`,`calc_test.go` 断言 `Add(2,3)==5`)。先独立跑 `go test ./qa_inject/...` → `FAIL: Add(2,3)=6, want 5`,确认起点是红的。

**1.** `$ ar run --workspace ws dev.yaml "There is a failing test. Run 'go test ./qa_inject/...' to see it fail, find the bug in the qa_inject package, fix the bug in the source (not the test), then run the test again to confirm it passes. Then exit."`

→ 系统反应(6 个 gen-step,自主闭环):
- `bash{"go test ./qa_inject/..."}` → `exit 1, "--- FAIL: TestAdd … Add(2,3)=6, want 5"`(先复现)
- `bash{"find qa_inject"}` → `qa_inject/calc_test.go\nqa_inject/calc.go`(定位)
- `read_file{path: qa_inject/calc.go}` → 读到 `return a * b // BUG`
- `edit_file{old: "return a * b …", new: "…return a + b …"}` → `edited qa_inject/calc.go`(只改源、不动测试)
- `bash{"go test ./qa_inject/..."}` → `exit 0, "ok github.com/spf13/cobra/qa_inject 0.806s"`(自证变绿)
- 收尾:「我已找到 `qa_inject` 包中的错误(`calc.go` 中用了 `*` 而不是 `+`),将其修复为加法并重新运行测试,测试已顺利通过。」
- stderr:`run completed: 6 turns, 6431 in / 364 out tokens`,**退出码 0**,**耗时 8.02s**

→ 验证:
- `cat ws/qa_inject/calc.go` → 确为 `return a + b`。
- **独立**再跑 `go test ./qa_inject/...` → `ok (cached)`,真绿。
- journal 72 行,**0 bad**,末 `task_completed reason=completed`。

**判定:PASS** —— 复现→定位→改源→验证→收尾全链路自主完成,产品事件流完整。

---

## 场景 3: goal mode —— command verifier 达标即停

**0. driver(`driver.yaml`)+ worker**
```yaml
name: reach-ready
agent_spec: worker.yaml
task: |
  确保当前工作目录下存在文件 done.txt,其内容恰好为一行 READY。
  用 `python3 check.py` 可以验证是否达标(退出码 0 即达标)。
max_iterations: 3
verifiers:
  - { kind: command, command: "python3 check.py" }
```
`ws/` 放 `check.py`:`done.txt` 存在且内容 `==READY` 才 `exit(0)`。

**1.** `$ ar drive --workspace ws driver.yaml`

→ 系统反应:
- stderr:`driver 20260707-233303-reach-ready-ab4c` / `iteration 1 (…-iter-1)`
- **子迭代**(`sub/iter-1/events.jsonl`):`bash{"ls -la"}` → 只有 check.py;`read_file{check.py}` → 读懂判定条件;`write_file{path: done.txt, content: "READY\n"}` → `wrote (6 bytes)`;`bash{"python3 check.py"}` → `exit 0`;child `task_completed reason=completed, gen_steps=5`
- **driver 层 verifier**:`verifier:command{"python3 check.py"}` → `{pass: true, score: 1, detail: "exit=0"}`;`iteration_completed verdict.pass=true`;`driver_completed reason=satisfied, iterations=1, best_iter=1`
- stderr:`driver satisfied: 1 iterations (best 1)`,**退出码 0**,**耗时 6.8s**

→ 验证:
- `wc -l ws/done.txt` = 1 行 `READY`。
- 驱动 journal:`driver_started → iteration_scheduled → iteration_launched → effect(verify) → iteration_completed → driver_completed`,verdict `pass=true`。
- verifier 走 journaled effect 管线(`effect_resolved` 里 `verdict: allow` + gate_results)。sub/iter-1 53 事件全 valid。

**判定:PASS** —— 目标达标即停、verdict 落盘、fresh child journal 逐迭代分目录。

---

## 场景 4: loop mode —— interval 调度跑满 max_iterations

**0. driver**
```yaml
name: heartbeat
agent_spec: worker.yaml
schedule: interval
interval: 20s
task: 追加一行心跳到 ticks.log
max_iterations: 3
```
worker system_prompt 指示「每次迭代 `date +%s >> ticks.log` 记一笔」。

**1.** `$ ar drive --workspace ws driver.yaml`

→ 系统反应:
- stderr 逐迭代:`iteration 1/2/3 (…-iter-N)`
- 三个子迭代 journal 各自:child-task「追加一行心跳到 ticks.log」→ `bash{"date +%s >> ticks.log"}` → `exit 0` → `task_completed`(各 2 gen-step)
- 收尾:`WARN "driver hit max_iterations" max=3` → `driver max_iterations: 3 iterations (best 1)`
- 驱动 journal:1×driver_started、3×iteration_scheduled、3×iteration_launched、3×iteration_completed、1×driver_completed(`reason=max_iterations`)
- **退出码 0**,**耗时 45.9s**

→ 验证:`cat ws/ticks.log` → 3 个时间戳 `1783467229/250/272`;**间隔 21s、22s**(= 20s interval + LLM 耗时),interval 真的生效。

**判定:PASS**

---

## 场景 4b: loop 的停止路径 —— 单次 Ctrl-C 停不下来

**0.** driver 同上但 `interval: 15s, max_iterations: 50`(模拟无界 loop)。

**1.** `$ ar drive --workspace ws driver.yaml &`;等 iteration 1 落盘后 `kill -INT <pid>`(一次 Ctrl-C)
→ 进程**不退**;20s 后仍存活,期间 stderr 还打出 `iteration 2`;只能 `kill -9`,**退出码 137**。

**2.** 重跑,`kill -INT <pid>` **×2**(两次 Ctrl-C)
→ 1s 内停,stderr `driver stopped: 1 iterations (best 1)`,**退出码 1**;末事件 `driver_completed reason=stopped`,journal 5 行 0 bad。

**3.** 再重跑,`kill -TERM <pid>`(单次 SIGTERM)
→ 1s 内停,`driver stopped: 1 iterations`,**退出码 1**。

**判定:FAIL(见 [A2-1])** —— 前台 loop 对**单次 Ctrl-C 无反应**且继续起下一迭代;只有双击 Ctrl-C 或 SIGTERM 能停,均不直观、文档无载。

---

## 场景 5: submit + resume —— 后台 task 提交与接续

**0.** 起 daemon。**首次在 137 字节 XDG 路径下 daemon 直接 `bind: invalid argument` 起不来**(见 [A2-2])。改 `XDG_DATA_HOME=/tmp/ar2`(32 字节)后 daemon 秒起,以下在该 daemon 上进行。

**1.** `$ ar submit --workspace ws dev.yaml "write a file poem.txt containing a short haiku about the sea (3 lines), then read it back with bash to confirm, then exit"`

→ 系统反应:
- 命令**未立即返回句柄**,当场流式打出整段执行,4.3s 结束。stderr 打了两行 `session 20260707-233726-…-4529`(session id 出现两次)。
- daemon.log:`notify: [run_end] run …-4529 ended: completed` —— 任务**确实在 daemon 服务端跑**,客户端 attach 实时流式。
- 任务 journal:`write_file{poem.txt, "Deep blue waves rolling,\nCrashing on the sandy shore,\nWhispers of the deep.\n"}` → `wrote (76 bytes)`;`bash{"cat poem.txt"}` → 回读一致;`task_completed reason=completed`。

**2.** `$ ar sessions list` → `SESSION …-4529 STATUS completed TURNS 3`。(注:`ar sessions` 不带子命令只打 usage。)

**3.** `$ ar resume 20260707-233726-…-4529`
→ stderr `resuming session …-4529` / `resume: task session already completed (completed)` / `run completed: 3 turns`,**退出码 0**。对已完成任务干净报告、不崩不挂。

→ 验证:`cat ws/poem.txt` → 三行俳句在;journal 35 行 0 bad。

**判定:PASS(需短路径 daemon 变通)** —— 附注:submit 的"当场流式到完成"更像 attach-and-stream 而非"提交拿句柄立返";对期待即返句柄的用户略反直觉,但配合 idem_key/后台 series 语义看是设计取舍,不单列为 bug。

---

## 场景 6: best-of-N —— 隔离 worktree、per-attempt 判定、胜者留盘

**0. driver**
```yaml
name: bestof
agent_spec: worker.yaml
schedule: parallel
n: 2
task: 在当前目录创建文件 hello.txt,内容恰好是两个字符 hi(不含换行、不含引号)。
verifiers:
  - { kind: command, command: "test \"$(cat hello.txt)\" = hi" }
```
workspace 先 `git init` + 空 commit(best-of-N 要物化 base snapshot 到 worktree)。

**1.** `$ ar drive --workspace ws driver.yaml`

→ 系统反应:
- stderr 两个尝试各自**独立 worktree**:`attempt 1 (…-att-1) in …/wt/att-1`、`attempt 2 (…-att-2) in …/wt/att-2`
- att-1:`write_file{hello.txt, "hi"}` → `wrote (2 bytes)`;att-2:`bash{"printf 'hi' > hello.txt"}` + `bash{"wc -c hello.txt"}` → `2`(两个尝试**不同解法**)
- per-attempt verdict:两个 `iteration_completed` 均 `pass=true, score=1`;`driver_completed reason=satisfied`
- stderr:`driver satisfied: 2 iterations (best 1)`,**退出码 0**,**耗时 6.4s**

→ 验证:
- `xxd wt/att-1/hello.txt`、`wt/att-2/hello.txt` → 都是 `6869`(恰 `hi` 无换行),**两 worktree 隔离**、内容独立。
- **胜者未晋升到用户 workspace**:`ws/hello.txt` 不存在。符合 SPEC「胜者晋升 🧊 G15(v0 手动晋升)」,不算 bug——但对不知情用户,`--workspace` 目录是空的,可能意外(stderr 有打 worktree 路径,尚可自救)。

**判定:PASS**

---

## 场景 7: 预算耗尽 —— LimitExceeded 显式可见

**7a.** `$ ar run --workspace ws dev.yaml "写一个完整的 Go HTTP web 服务器,包含多个 handler、中间件、路由、单元测试,分成多个文件,并逐个用 bash 验证编译通过"`(spec `budget: {max_total_tokens: 500}`)
→ gen-step 1 处直接 `WARN "token budget exhausted; ending run" limit=500 used=0`;`run limit_exceeded`,**退出码 1**。(预算 500 < 单次 LLM reserve 2048,第一次 LLM effect 前即 deny——预留纪律正确。)

**7b.** `$ ar run --workspace ws dev.yaml "写一个完整的 Go web 服务器,包含多个 handler、路由、中间件、多个源文件和测试,每写一个文件就用 bash 编译验证一次,持续迭代直到全部完成"`(预算 4000)
→ 真实中途耗尽:
- gen-step 1 `bash{"go mod init go-web-server"}` → ok(真做了活)
- gen-step 2 `write_file{handlers/handlers.go}` → **`denied: token budget exhausted: settled 3305 + reserved 0 + est 1000 > limit 4000`**
- `run limit_exceeded: 3 turns, 1883 in / 1422 out`,**退出码 1**

→ 验证:退出码 1;报错含 settled/reserved/est/limit 四项算式;`go.mod` 已生成(部分产出留存)。不静默截断也不挂死。

**判定:PASS**

---

## 场景 8: task 运行中被 interrupt

**1.** `$ ar run --workspace ws dev.yaml "逐个创建 10 个文件 f1.txt 到 f10.txt,每个文件写入其编号,每创建一个就用 bash cat 验证一次,一个一个来不要批量"`(后台起,等 f1.txt 出现后 `kill -INT <pid>` ×2)

→ 系统反应:
- journal:`write_file{f1.txt, "1"}` → `wrote (1 bytes)`;中断后两个在飞 activity 记 **`activity_cancelled`**;末 `task_completed reason=canceled, gen_steps=2`
- stderr:`WARN "barrier skipped: snapshot failed" barrier=bar-final err="snapshot: git add: context canceled"` / `run failed: turn 2: complete [canceled]: context canceled`
- 2×SIGINT 后 **1s 内**停,**退出码 1**

→ 验证:`ls ws/` → f1.txt 留存;journal 25 行 0 bad;`task_completed.reason` 干净区分 `"canceled"` vs `"completed"`(事件名叫 task_completed 但 reason 消歧,命名小别扭,不算 bug)。

**判定:PASS**

---

## 场景 9: 错误路径 —— 报错质量

全部**人话、无 Go panic/stack**:

| 命令 | 输出 | 退出码 |
|---|---|---|
| `ar run … /nonexistent/spec.yaml "…"` | `spec …: open …: no such file or directory` | 2 |
| `ar run … badspec.yaml "…"`(坏 YAML) | `yaml: line 1: did not find expected ',' or '}'`(点到行) | 2 |
| `ar run --workspace /does/not/exist …` | `workspace root …: lstat /does: no such file or directory` | 2 |
| `ar run … badprov.yaml "hi"`(未知 provider) | `unknown provider "bogusprovider" (available: gemini, anthropic, scripted)`(**列出可选**) | 2 |
| `ar run … goodspec.yaml`(缺 task) | usage 行 | usage |
| `ar drive … baddriver.yaml`(agent_spec 缺失) | `driver spec …: field agent_spec: … no such file or directory` | — |
| `ar drive … noverif.yaml`(goal 无 verifier) | `drive failed: driver: goal mode requires at least one verifier` | — |
| `ar drive … badn.yaml`(parallel n=1) | `drive failed: driver: schedule parallel requires n >= 2 (got 1)` | — |
| `ar drive … nointerval.yaml`(interval 无 interval) | **无报错,静默 back-to-back 跑**(→ [A2-3]) | 0 |

**判定:PASS(除 9i)**

---

# 问题清单

### [A2-1] 前台 loop(`ar drive`)对单次 Ctrl-C 无反应,用户以为停不下来 🟠 major
- 复现:`ar drive … driver.yaml &`(interval loop,max_iterations 大);`kill -INT $!` ×1 → 进程不退,20s 后仍活,期间还起 iteration 2,只能 SIGKILL(exit 137)。
- 期望:Ctrl-C 应能停 loop,或提示"再按一次退出"。
- 根因:`internal/cli/run.go:241 signalContext()` 把第一次 SIGINT 当"steering interrupt"塞进 `interrupts` channel;`internal/cli/drive.go:66` `ctx, _, stop := signalContext()` **把该 channel 丢弃**。run 用了所以正常,drive 没用。
- 可行停法(实测):双击 Ctrl-C 或单次 SIGTERM;均不直观、文档无载。
- 证据:agent2/s4b/drive.err、s4c/drive.err、s4d/drive.err。

### [A2-2] daemon 在长 XDG_DATA_HOME 路径下无法启动(socket 路径超 104 字节)🔴 critical
- 复现:`export XDG_DATA_HOME=<137字节路径>; ar daemon` → `daemon: listen unix …/daemon.sock: bind: invalid argument`。
- 短路径 `/tmp/ar2`(32 字节)daemon 秒起——确证 macOS `sun_path` 104 上限。封杀所有 daemon 依赖功能。
- 报错为底层 syscall 原文,无"路径太长"提示;建议 socket 落固定短目录(哈希短名),与数据目录解耦。
- 证据:agent2/EVIDENCE-daemon-bind-fail.log、daemon2.log(短路径成功对照)。

### [A2-3] `schedule: interval` 漏写 `interval` → 静默 back-to-back 热循环 🟡 minor
- 复现:driver 写 `schedule: interval` 无 `interval:` → 迭代间隔仅 2s(纯 LLM 耗时)。
- `internal/driver/spec.go:211 interval()` 空值返回 0(注释"empty is zero (back-to-back)"),无校验无警告。无界 loop 会全速打满 API。
- 证据:agent2/s9/ni2.err(gap=2s)。

### 已核实"像 bug 但符合设计"(不计)
- best-of-N 胜者不落用户 workspace:符合 SPEC 🧊 G15(v0 手动晋升);建议 CLI 结尾提示胜者 worktree 路径。
- 预算 used=0:reserve-then-settle 在第一次 LLM effect 前 deny,预留纪律正确。

# 没测到的
- daemon 依赖的 remote stop/interrupt/approve 全链路(受 A2-2 阻塞;短路径下只补了 submit/resume)。
- loop overlap coalesce/skip(迭代都比 interval 短,未造出重叠)。
- self_paced + schedule_next/finish_series 真实模型自驱。
- cron 调度、series_memory、on_child_failure 策略。
- 真实 429 重试(窗口内没撞上;代码层确认 429→Retryable=true)。
- patience 停滞检测。

# arwebui — AgentRunner 驾驶舱(Codex 风格)

一个 Codex 云端风格的本机 web app,以真实用户的方式驱动 AgentRunner 的
**全部**会话能力:chat、插话排队、interrupt、resume、起/杀子
agent、图片输入、审批(含子 agent 上卷)、fork、换 agent、trust、后台
运行(submit / drive),以及崩溃恢复观察。

UI 文案一律英文、词汇对齐 journal 事件/CLI 命令,关键控件带 title
tooltip 解释用途(同 `web/` 铁律 I6);无"关闭会话"操作——静止模型下
session 没有终态(同 `web/` 铁律 I7),journal 里的 session_closed 只是
标记事件,照常渲染为标记。

与 `web/`(极简 vanilla 驾驶舱)并列、独立:同样只通过公开的 `ar` CLI
契约(`os/exec`)与系统交互,是测试/QA 面,不是产品的一部分,不进
`docs/` 三层定义,不参与 `scripts/check.sh` 闸门。

- **后端** `arwebui`:stdlib-only Go(独立 module),把每个请求翻成一次
  `ar` 子命令,并把 Vite 构建产物嵌进二进制。
- **前端** `frontend/`:React + TypeScript + Vite。Codex 云端观感:
  - **Codex 风格 composer**(`components/Composer.tsx`,首页与会话共用):
    圆角卡片 + 一排 pill 控件——`+` Add 菜单(图片/文本文件/Goal/Loop/
    Plan/YAML)、权限模式 pill(Full access/Ask/Auto-accept/Plan,风险色点)、
    model pill(改 spec model 块 → new/agent)、麦克风(Web Speech 听写)、
    圆形发送键;首页多一行 context bar(workspace/start-mode/git 分支)。
    输入 `/` 出 slash 菜单(`/goal /loop /plan /model /compact /clear /diff
    /fork …`),drop-up 弹层空间不足时自动向下翻转。下面是**任务卡片**网格
    (标题取开场消息,像 Codex 用任务描述当标题)。
  - **会话详情**:顶栏动作条 +「Activity / Diff」切换。Activity 是时间线
    (气泡/工具卡/审批/子 agent);**Diff** 是 Codex 式的 git diff 视图
    (改动文件列表 + 逐行 +/- 着色 + untracked 新文件)。
  - 底部 composer、右侧在飞任务面板、审批 dock。

## 快速上手

```bash
# 1. 构建 ar 二进制(仓库根)
go build -o /tmp/ar ./cmd/agentrunner

# 2. 构建前端(首次或改前端后)
cd webui/frontend && npm install && npm run build && cd ..

# 3. 构建并起驾驶舱(--env-file 把 GEMINI_API_KEY 递给 daemon)
go run . --ar /tmp/ar --env-file ../.env

# 4. 浏览器打开
open http://127.0.0.1:8788
```

> macOS 的 unix socket 路径有 104 字节上限。若 `$XDG_DATA_HOME` 过长会让
> daemon bind 失败——测试时用一个短路径,例如
> `XDG_DATA_HOME=/tmp/awui go run . ...`。

## 参数

| flag | 默认 | 说明 |
|---|---|---|
| `--ar` | `ar` | agentrunner 二进制路径 |
| `--addr` | `127.0.0.1:8788` | 监听地址(只应绑本机) |
| `--env-file` | (空) | KEY=VALUE 文件,载入环境后再 spawn daemon(传 `../.env`) |
| `--no-daemon` | false | 不托管 daemon,用外部已运行的 |
| `--runtime` | `./runtime` | 便利产物目录(specs/uploads/ws/runs/daemon.log,gitignored) |

## 功能 ↔ CLI 映射

| UI | ar 命令 |
|---|---|
| 首页任务卡 / 会话列表 | `sessions list`(标题/workspace 由 arwebui 侧记的元数据补全) |
| Ask / 新会话 / 开场消息 | `new --workspace W [--mode M] base.yaml "msg"` |
| composer 权限模式 pill | 建 spec 的 `permissions:` 块 + `--mode`(full/ask/acceptEdits/plan) |
| composer model pill | 建/改 spec 的 `model:` 块 → `new`(新会话)或 `agent`(session 内换) |
| composer `/goal`·`/loop`·Goal/Loop 启动器 | `drive --json`(driver.yaml schedule immediate/interval) |
| composer `/compact`·`/clear` | `compact <sid>` / `clear <sid>` |
| composer 分支 pill | `git -C <ws> for-each-ref`(列)+ `git checkout [-b]`(切/建) |
| composer 语音 | 浏览器 SpeechRecognition 听写(纯前端,不经 ar) |
| 改动(Diff 视图) | `git -C <workspace> diff` + `status --porcelain`(workspace 仅 arwebui 建的会话可知) |
| 发消息 / 图片 / 文件 | `send [--image f]... [--file f]... <sid> "text"`(图片走 --image;PDF/文本/任意文件走 --file,INC-9) |
| 时间线(真相) | `events --json <sid>`(1s 增量轮询) |
| 流式打字 / 子审批上卷 | `attach --json <sid>`(SSE) |
| interrupt / resume | `interrupt` / `resume` |
| 在飞任务 + kill | `ps` / `kill <sid> <handle>` |
| 审批 | `approve <sid> <id> approve\|deny [reason]` |
| fork(选 barrier) | `fork --list` + `fork <sid> <barrier> [--workspace]` |
| 换 agent | `agent <sid> base.yaml` |
| 后台运行 submit / drive | `submit --json …` / `drive --json …`(SSE 流式日志) |
| trust | `trust <dir>` |
| 查看面板 | `events`(raw) / `events --state` / `inspect --json` |

## 开发

```bash
cd webui
gofmt -l . && go vet ./... && go test ./...   # 后端闸门
cd frontend && npm run build                  # 前端构建(产物嵌入二进制)
```

后端单测覆盖 CLI 输出解析的易碎点(session id 落在 stderr、daemon 不可达
的多种措辞)。真实验证按全局 QA 纪律走真 daemon + 真 Gemini 的端到端
UI 流程。

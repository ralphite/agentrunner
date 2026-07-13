# agentrunner

声明式 LLM agent 运行时:一个 YAML spec 定义 agent,一条命令跑任务或
开对话;会话落盘为 journal,崩溃可恢复、随时可续聊。

## 安装与准备

一行安装(linux x86_64/arm64、macOS arm64/x86_64,无需 Go/Node 工具链;
再跑一遍即升级):

```sh
curl -fsSL https://raw.githubusercontent.com/ralphite/agentrunner/main/install.sh | sh
```

装到 `~/.local/share/agentrunner/releases/<version>/`,`ar` 与 `arwebui`
链接进 `~/.local/bin`。装完起 Web UI:`arwebui`(默认 127.0.0.1:8788,
`-addr 0.0.0.0:8788` 可让局域网设备访问)。

私有 fork/镜像才需要 `export GITHUB_TOKEN=...`(install.sh 自动识别并
改走 API 资产下载)。可调环境变量见 install.sh 头部注释;发布产物由
`scripts/package-release.sh` + release workflow 构建(打 `v*` tag,或
dispatch workflow 填 `publish_tag`——后者由 CI 代建 tag,适合无法直推
tag 的环境)。

从源码构建:

```sh
go build -o ar ./cmd/agentrunner    # Go 1.25.12+ / 1.26.5+ / newer stable
export GEMINI_API_KEY=...           # 或 ANTHROPIC_API_KEY(见 spec 的 model.provider)
```

凭据也可以放在工作目录的 `.env` 里(`daemon` 启动时读取)。

## 一分钟上手

```sh
./ar init                       # 生成带注释的示例 spec.yaml
./ar run spec.yaml "say hello"  # 一次性任务:输出直接打在终端
```

对话形态(会话常驻、可多轮):

```sh
./ar daemon &                   # 先起常驻运行时(托管所有会话)
./ar new spec.yaml "write a haiku about rain"
#   → 回复直接显示;结尾提示如何继续
./ar send <session> "make it about snow"
#   → 同一会话继续,回复直接显示(session 用 id 的唯一前缀即可)
./ar attach <session>           # 回放全部历史并跟随直播(Ctrl-C 退出观看,会话不受影响)
./ar close <session>            # 打 close 标记(send 随时可续聊)
```

## 常用命令

| 命令 | 作用 |
|---|---|
| `ar help` | 分组的完整命令帮助 |
| `ar init [path]` | 生成示例 spec(拒绝覆盖已有文件) |
| `ar run <spec> "message"` | 开会话+发消息+等静止+读结果的便捷命令 |
| `ar new / send / attach / close` | 对话会话(需 daemon) |
| `ar sessions` | 列出会话与状态 |
| `ar inspect <session>` | 会话事实:状态、轮次、token 用量 |
| `ar approve <session> <id> approve\|deny` | 回答权限请求 |
| `ar resume <session>` | 恢复被中断的会话 |
| `ar agent <session> <spec>` | 会话内换 agent(免确认,下次 send 生效) |

`new`/`send` 默认等到回复打印完才退出;加 `--detach` 可立即返回
(分别只输出 session id / `delivered`),之后用 `attach` 看结果。

## Spec 是什么

`ar init` 生成的模板即文档:必填 `name`、`model.provider`、`model.id`
与 `system_prompt`(或 `system_prompt_file`),可选 `tools`、`mode`、
`permissions`、`budget` 等,注释里都有说明。写错字段时报错会给出
合法字段清单。

## 更多

产品与架构文档在 [docs/](docs/):journey(`JOURNEYS.md`)、功能登记
(`SPEC.md`)、架构(`DESIGN.md`)、流程(`PROCESS.md`)。

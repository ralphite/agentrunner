# agentrunner

声明式 LLM agent 运行时:一个 YAML spec 定义 agent,一条命令跑任务或
开对话;会话落盘为 journal,崩溃可恢复、随时可续聊。

## 安装与准备

```sh
go build -o ar ./cmd/agentrunner    # Go 1.23+
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
./ar close <session>            # 结束会话
```

## 常用命令

| 命令 | 作用 |
|---|---|
| `ar help` | 分组的完整命令帮助 |
| `ar init [path]` | 生成示例 spec(拒绝覆盖已有文件) |
| `ar run <spec> "task"` | 前台一次性运行 |
| `ar new / send / attach / close` | 对话会话(需 daemon) |
| `ar sessions` | 列出会话与状态 |
| `ar inspect <session>` | 会话事实:状态、轮次、token 用量 |
| `ar approve <session> <id> approve\|deny` | 回答权限请求 |
| `ar resume <session>` | 恢复被中断的会话 |

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

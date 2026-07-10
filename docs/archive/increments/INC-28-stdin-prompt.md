# INC-28 stdin 管道 prompt（HANDA-PARITY #32）

## 动机与 journey 锚

Unix 惯例补齐：`echo "fix the failing test" | ar run spec.yaml`、
`git diff | ar send <sid> -`。对齐 handa handacli（`--prompt` 缺省读
stdin）与 gemini/claude `-p` 习惯。journey 锚：UJ-01/02 的输入面
变体（脚本/管道调用者），不新增 journey。HANDA-PARITY §2 #32。

## Spec delta

- SPEC A 区「会话与输入」加一行：`run/new/send 文本参数可由管道
  stdin 提供（缺省且 stdin 非 tty，或显式 "-"；tty 下 "-" 报错不
  阻塞）`。

## Design delta

- 无。纯 CLI 参数解析层（DESIGN 无此层不变量）；附件 flags 不受
  影响（stdin 只供文本）。

## 验收

- 孪生（cli 单测，stdinSource seam 注入）：piped 补齐缺参 / 显式
  "-" 替换 / 尾换行 trim、正文换行保留 / piped 空输入报非空错 /
  非 piped 缺参仍走 usage / 非 piped "-" 报错不阻塞。
- 命令级：runCmd 缺 task + piped stdin 不再报 usage（走 spec 校验
  路径证明 stdin 参与）。
- 真实 API QA 不单开场景：批 1 收口时在任一 QA 场景顺手用管道形式
  跑一遍 `ar send`（echo | ar send <sid> -）作 B 闸抽验。

## 实施步骤

1. `internal/cli/stdin.go`：`completeTextArg(rest, want)` + 包级
   `stdinSource` seam（os.Stdin.Stat 判 ModeCharDevice）；run/new/
   send 三处解析接入。+ 单测。check.sh 绿 → commit。

## review 裁决

小增量（S，单文件 helper + 三处两行接入），裁掉三视角 review；
孪生覆盖全部分支。

---

## 执行记录（2026-07-10 收口）

一步完成。B 闸未等批 1 收口、当场真验（共享 daemon + 真 Gemini）：
管道开场（PONG）+ `-` 多行续聊（PONG2），session
`20260710-063023-…-d99d` 保留于共享 store，证据归档
`qa/runs/2026-07-10-INC28/`。边界：`</dev/null` 为 char device 按
"非管道"处理（消息 "stdin is not a pipe"），精确 isatty 不引依赖，
记档 LOG。SPEC A 区加行；SPRINT #32 ✅。

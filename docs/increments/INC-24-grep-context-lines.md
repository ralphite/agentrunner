# INC-24 grep context lines（-A/-B/-C，#35 余项）

## 动机与 journey 锚

INC-22（#35）拆出的余项 #12b + UJ-01。对标 Claude Code grep 的
`-A`/`-B`/`-C`：匹配行附带前后若干行上下文，方便定位而不必再 read_file。
grep content mode 的直接延续，无状态纯扩展（默认无 context = 现状不变）。

## 范围（context lines；multiline 记余项）

- **本增量**：`-A N`（after，匹配行后 N 行）、`-B N`（before，前 N 行）、
  `-C N`（both，= -A N -B N）。content mode 的每个匹配附带 before/after
  上下文行（redacted、行截断同匹配行）。
- **余项**：`multiline`（跨行 regex，改匹配循环从逐行到跨行）记为 12c，
  独立小增量。

## Spec delta

- SPEC C grep 参数行补 -A/-B/-C；锚 `TestGrepContextLines` + QA-31。
- CLAUDECODE-PARITY §2 #35 备注 context lines 关闭（multiline 余项）。

## Design delta（不触不变量）

grep content mode 的匹配附带上下文行。`-C` 展开为 -A 与 -B 的 max。
context 行受同样的 grepMaxLineBytes 截断与 redaction。默认 0 = 现状。
files/count 模式忽略 context（无匹配行概念）。

## 验收

- 孪生：`TestGrepContextLines`（-A/-B/-C 各返回正确的前后行；文件边界
  截断不越界；redaction 生效；默认无 context = 旧行为）。
- 真实 API QA-31：让模型用 grep -C 看某符号的上下文；`ar events` 归档。
- 绿门（排除已知环境测试）。

## 实施步骤

1. grep def 加 -A/-B/-C；struct 加字段。
2. content mode 匹配收集时附 before/after（`grepMatch` 加 Before/After）。
3. 孪生 + QA-31。
4. 文档行齐活。

## review 裁决

做。S、grep content 延续、无状态、默认=旧行为。inline 自审：
correctness（文件边界、-C 展开、redaction）、contract（不改 files/count
模式、默认无 context 不破现有测试）。

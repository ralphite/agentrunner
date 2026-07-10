# INC-27 grep multiline（跨行 regex，#35 余项）

## 动机与 journey 锚

CLAUDECODE-PARITY §2 #35（Grep 参数增强）余项，INC-24 拆出。对标 Claude
Code grep `multiline: true`（`.` 跨行、pattern 跨行匹配）。现状：grep 逐行
`re.MatchString(line)`，跨行 pattern（如 `func Foo\(\)[\s\S]*?\}`）匹配不到。

## 范围（自包含于 grep 匹配循环，S）

- grep 新增 `multiline` bool 参数（默认 false = 旧逐行行为，零破坏）。
- `multiline: true` 时：flag 前缀加 `(?sm)`——`s`(dotall,`.`匹配`\n`)+
  `m`(`^`/`$` 锚行边界,保持与逐行等价的锚语义,使 multiline 成逐行的
  严格超集);match 对**整文件内容**而非逐行,匹配可跨行。
- content 模式：每个跨行 match → `{path, line=match 起始行, text=匹配文本
  (clampGrepLine 钳 2000 字节+redact), before/after 上下文按 match 起止行}`。
- files_with_matches / count 模式：fileCounts 计跨行 match 数。
- 与既有参数正交：case_insensitive(`i`)、glob、output_mode、-A/-B/-C、
  max_results、cap/redact/文件大小上限全复用。

## Spec delta

- SPEC C grep 行补 multiline；CLAUDECODE-PARITY #35 备注去掉"multiline 余项"。
- 锚 `TestGrepMultiline` + QA-34。

## Design delta

不触不变量。纯 grep 工具内部匹配策略扩展；read-class 不变；redact/收容/
cap 全不变。

## 验收

- 孪生 `TestGrepMultiline`：跨行 pattern 在 multiline=true 命中、false 不
  命中；起始行号正确；`(?sm)` 下 `^`/`$` 锚行；context 上下文对跨行 match
  仍正确；case_insensitive+multiline 组合。
- 真实 API QA-34：让 Gemini 用 multiline grep 找一个跨行结构（如整个
  函数体），验证命中跨行；`ar events` 归档 qa/runs/。
- 绿门（排除已知环境测试）。

## 实施步骤

1. grep.json 加 multiline 参数。
2. exec.go grep()：args 加 Multiline;flag 前缀组合 `i`/`s`/`m`;文件体内
   multiline 分支（整文件 FindAllStringIndex + 行号/上下文），else 旧逐行。
3. 孪生 TestGrepMultiline + QA-34。
4. 文档行齐活。

## review 裁决

做。干净 S（grep 内部匹配循环加一分支 + 一参数,默认旧行为）。inline
自审：correctness（行号/上下文/cap/`(?sm)` 语义/默认不破）、security
（read-class 不变、redact/收容不变）、contract（既有 grep 测试不触,新参数
默认 false）。

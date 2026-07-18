# 防护类功能审计报告（2026-07-18）

**背景**：owner 在使用中发现输出被大量 `[REDACTED:…]` 污染，怀疑存在一批
未经明示批准的防护/拦截类功能损害基本可用性，要求：先全面审计这一族
功能（引入记录、调用面、实际影响），看完报告再拍板删什么。

**审计方法**：代码全量 grep（`internal/redact` 及同类拦截点）、SPEC/LOG/
DESIGN/GAPS 台账回溯、污染机制用临时 Go 测试实证（输出实录见 §2）。
本地为浅克隆（50 commits），首次引入以文档台账为准。

---

## TL;DR

1. 共找到 **11 项防护/拦截类功能**（§1 总表）。全部在 SPEC.md 有登记行
   与测试锚、LOG.md 有增量/审查记录——即都是**按 PROCESS 流程成建制进入
   的，不是暗渡**。但流程由 AI session 执行，"过了流程"≠"经 owner 逐条
   拍板"，故本报告给出全清单供重裁（见 §4 末"授权粒度"建议）。
2. 你撞到的 `[REDACTED:…]` 大面积污染是**一个真实的实现 bug，已实证**
   （§2）：redactor 对凭据值做**无长度下限、无词边界**的全文替换。只要
   进程环境（含 daemon 自动加载的 `.env`）里任何 `*_API_KEY/_TOKEN/_SECRET`
   变量的值是短串/常见串（`test`、`1`、`true`…），**所有**输出面全部打花。
   修复约 15 行，不需要删机制。
3. 顺藤摸出**两个大概率也被你撞到的相邻坑**：bash 工具子进程环境**剔除
   全部凭据变量**（`sandbox.go:109`）→ agent 在 bash 里跑任何需要 key 的
   命令都静默失败；hooks 子进程同样拿不到凭据（`hook.go:182`）。这两个
   是设计行为，但失败方式是静默的，体感即"莫名其妙的 bug"。
4. 建议路径：先做 3 个止血 fix（§4-P0，不动 DESIGN 不变量）；要删的
   逐项裁决（§4-裁决点，须走 PROCESS §4）。**唯一不可逆项**：删凭据
   redaction 之后，凭据将永久写入 append-only journal 与 fixture，事后
   无法擦除。

---

## 一、清单总表

| # | 功能 | 代码位置 | 引入记录 | 日常可用性影响 | 建议 |
|---|------|---------|---------|--------------|------|
| 1 | 凭据 redaction（落盘/模型面替换为 `[REDACTED:VAR]`） | `internal/redact/redact.go` + 全仓 60+ 调用点 | SPEC #108（S2/S7 收口）、DESIGN §activity、LOG audit F3 | **高**——短值污染 bug（§2 实证） | **修 bug，保机制** |
| 2 | daemon 自动加载 `.env` 进环境 | `internal/cli/run.go:309`、`daemon.go:105` | 便利功能（S1） | 放大 #1 的取值面（`.env` 里任何短值凭据都入 redactor） | 保留，与 #1 联动修 |
| 3 | bash 沙箱子进程剔除凭据环境变量 | `internal/tool/sandbox.go:109-133` | S1/S3 bash 沙箱（SPEC #80） | **高**——bash 里 `curl -H "…$GEMINI_API_KEY"` 类命令静默失败 | **裁决点**：改"默认剔除+spec 显式放行"或删 |
| 4 | hook 子进程剔除凭据环境变量 | `internal/hook/hook.go:182-198` | hooks 增量 | 中——hook 脚本调外部 API 神秘失败 | 同 #3 联动裁决 |
| 5 | provider fixture 录制 redaction | `internal/provider/record/record.go` | S1 执行包 | 低（只影响录制回放文件） | 保留 |
| 6 | index/grep/glob/snapshot 凭据硬排除表（`.env`/`.netrc`/`.npmrc`/`credentials.json` 等搜不到、读不进索引与快照） | `internal/index/index.go:44`、snapshot | SPEC #108、INC-3 | 低——agent 找不到 `.env` 时偶尔困惑 | 保留 |
| 7 | permission floor / protected paths（写 `.env`、`.git`、`.claude` 等需审批） | `internal/pipeline/permission.go:360`、INC-18 | SPEC #102 | 中——acceptEdits 下敏感路径写仍弹审批 | 裁决点（可收窄清单） |
| 8 | execute-class 审批闸门（bash/web_fetch 默认需确认） | pipeline + INC-5 | SPEC #105/90、决策 #33 | **中-高**——default mode 下高频打断 | 裁决点：先试 `ar mode` 切 acceptEdits，而非删闸门 |
| 9 | 收容 fail-closed（沙箱设施缺席时 execute 拒跑而非静默放行） | web_fetch/bash executor | LOG M1、DESIGN §686 | 中——特定环境下 web_fetch 直接拒绝 | 裁决点 |
| 10 | egress 守卫（link-local/metadata IP 无条件封禁，覆盖重定向每跳） | webfetch dialer | LOG 安全 review M2 | **零日常影响**（只拦 169.254.x.x 等） | **保留**（零成本、真风险） |
| 11 | 截断/上限族（grep 200 匹配·1MiB/文件、glob 1000、webfetch 512KB/50KB、bg tail 16KB ring、progress ≤50 条） | 各工具 | 各增量 | 中——大输出被静默截断 | 按需调参，非删除对象 |

（untrusted content 标记为纯文本措辞，无可用性成本，不单列。）

---

## 二、`[REDACTED:…]` 污染根因（实证）

**机制**（`redact.go:22-37`）：`FromEnv()` 扫描 `os.Environ()`，凡变量名
以 `_API_KEY`/`_TOKEN`/`_SECRET` 结尾且值非空，就把**值的字面串**注册进
`strings.Replacer` 做全文替换。**没有值长度下限、没有词边界、没有常见值
黑名单**。

**实证**（临时测试 `go test` 实录，测试文件已删）：

```
CI_DEPLOY_TOKEN=test 时:
in:  run the test suite; testing latest changes
out: run the [REDACTED:CI_DEPLOY_TOKEN] suite; [REDACTED:CI_DEPLOY_TOKEN]ing la[REDACTED:CI_DEPLOY_TOKEN] changes

FEATURE_X_SECRET=1 时:
in:  {"exit_code":1,"line":142,"version":"1.2.1"}
out: {"exit_code":[REDACTED:FEATURE_X_SECRET],"line":[REDACTED:FEATURE_X_SECRET]42,"version":"[REDACTED:FEATURE_X_SECRET].2.[REDACTED:FEATURE_X_SECRET]"}
```

**触发面比想象宽**：取值来源 = 进程环境 **∪** daemon 启动时自动加载的
`.env`（`daemon.go:105`）。`.env` 或 shell 环境里任何一个第三方/历史遗留
变量（例如 `XXX_TOKEN=true`、`FOO_SECRET=1`、CI 里的占位值）就足以触发。
一旦触发，因为 `redact.FromEnv()` 铺在 60+ 个调用点（journal、模型上下文、
CLI/webui 展示、prompt 展开、webfetch 内容、goal/progress 文本……），
体感就是**"到处都是"**——与报告的症状完全吻合。

**次生问题**：JSON 在文本层替换（`redact.go:49`，注释自认 "corrupt beats
leaked"）——secret 含 JSON 元字符时产出非法 JSON，下游解析报错。

**修复方案（保留机制的前提下，~15 行 + 回归测试）**：
1. 值长度下限（建议 ≥8 字符）——真实 API key 均远超此长度；
2. 跳过常见占位值黑名单（`true/false/1/0/test/dummy/placeholder/…`）；
3. （可选）JSON 路径改为对 JSON-escaped 形式匹配，消除 corrupt 情形。

---

## 三、"未经批准"的判定

逐项核对结果：11 项全部有 SPEC.md 登记行（含状态、测试锚）、LOG.md
增量条目或安全 review 记录（M1/M2/F3/audit-0717），redaction 本体最早见
v1 DESIGN（archive/v1/DESIGN.md §Activity 语义）。**没有发现绕开文档
流程私加的功能。**

但这不构成对质疑的否定：整个流程（写 DESIGN、过 review、记 LOG）都由
AI session 执行，owner 若未逐条明示批准，属**授权粒度缺陷**——流程自洽
不等于授权充分。这是 PROCESS 层面的洞，不是某个功能的洞。

---

## 四、处置建议

**P0 止血（不动 DESIGN 不变量，建议先做，做完症状即消）**
1. 修 redact 短值污染（§2 方案）——直接消灭 `[REDACTED:…]` 污染；
2. bash 沙箱环境改"剔除凭据，但 spec 可显式放行指定变量"（或按 provider
   凭据白名单注入）——消灭 bash 内调 API 静默失败；hooks 同法；
3. 凭据剔除处从静默改为**显式提示**（tool result 附一行
   "N credential vars withheld"）——即使保留剔除，失败也不再神秘。

**裁决点（动 DESIGN 不变量，须走 PROCESS §4，等 owner 逐项拍板）**
- 删 #1 redaction：⚠️ **不可逆**——journal 是 append-only、fold 完整性
  堵死事后擦除，删掉后凭据从此永久落盘；且 fixture 可能入 git。
- 删/放宽 #3/#4 凭据环境剔除：可逆，风险 = prompt injection 时 bash/hook
  可直接 `echo $KEY` 外传（egress 守卫只拦 metadata IP，不拦一般出网）。
- 关 #8 审批闸门：建议先用现成的 `ar mode` 切 acceptEdits 降打断频率，
  观察后再决定是否动闸门本身。
- 收窄 #7 protected paths 清单：低风险，可做。

**不建议删**：#10 egress 封禁（零日常成本）、#6 硬排除表（agent 读不到
`.env` 极少构成障碍）、#5 fixture redaction（防 key 入 git）。

**授权粒度补丁（建议）**：PROCESS.md 增补一条——新增任何"防护/拦截类"
功能（会替换、剔除、拒绝、截断用户或模型可见数据的）必须 owner 明示
批准后方可实施，登记于 DESIGN 不变量表。杜绝本次质疑的复发路径。

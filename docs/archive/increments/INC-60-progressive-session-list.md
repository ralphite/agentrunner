# INC-60 Web UI progressive session list

## 动机与 journey 锚

UJ-24 要求用户像 Codex 一样从 Projects → task 进入真实 session。当前共享
store 有 454 个 session，`ar sessions list --json` 会先读完并 fold 每个完整
journal；live `/api/sessions` 单次 32.21s 后 502。前端又每 4s 无互斥重入，
持续堆叠全量 CLI 子进程，连原本独立的 Changes 请求也被拖到 23.28s。
QA-61 真浏览器表现为 sidebar 长期 skeleton、deep-link 内容已出现但 header 仍
显示 `Loading task…`。这不是 cosmetic，而是 UJ-24 的任务入口不可用。

## Spec delta

- `ar sessions [list] --json` 增加 opt-in `--limit N --offset N`；先按 journal
  mtime 排序，再只 fold 请求页，默认无参数仍完整列出，兼容现有 CLI。
- Web API `/api/sessions?limit=N&offset=N` 只透传公开 CLI contract，不读取
  journal 私有目录。
- Web UI 首屏先取最近 40 条并立即可用；后台以 80 条一页顺序补齐历史，后续
  polling 只刷新最近页并与既有历史合并。
- 同一时刻只允许一个 session refresh chain，4s interval 不得重入。
- deep link header 在 metadata 页尚未返回时使用 durable session id 的既有
  human-readable fallback，禁止内容已出现而标题仍长期 `Loading task…`。
- daemon version=`unknown` 不得原样泄漏，显示稳定的 `Connected · local`。
- Settings 关闭后焦点回到打开它的控件；mobile trigger 已随 sidebar 收起时，
  回到 `Show sidebar`，不把键盘用户丢到 document body。
- Command palette 的 Escape/backdrop dismissal 回到打开前的输入/控件；执行命令
  时不抢目标 modal/page 的焦点。
- Changes 对 untracked dependency/build output 做有计数的隐藏与总量上限，
  禁止数万 `node_modules` 文件被合成巨型 diff 卡死浏览器；真实 source 仍可见。
- 大 Diff 默认按文件折叠；是否展开在把 payload 交给 React 前决定，禁止先完整
  paint 一帧再由 effect 收起。

## Design delta

修订 DESIGN Web UI surface：session list 仍完全来自 journal-backed public CLI，
但允许分页 projection；首个成功页即结束 not-loaded 状态，历史页后台补齐。
metadata cache 仍非真相源。deep link 在分页命中前可从 durable id 派生标题，
随后 journal title 到达即替换。未改 journal-first、薄壳或权限不变量。

## 验收

### 闸门 A

- CLI：limit/offset newest-first、默认全量、非法值 usage。
- Web handler：query 校验且 argv 精确透传 `--limit/--offset`。
- Frontend：并发 refresh 只发一次请求；首屏页先 ready；后台页合并且不重复；
  后续刷新保留历史；unknown daemon label 收敛为 local。
- `./scripts/check.sh` 全绿。

### 闸门 B · QA-61（共享真实环境，数据保留）

- live 8809 + 454 session shared store；首页首批 sessions <3s，无长期 skeleton。
- 20s 内 `/api/sessions` 无 502、无并发重入；后台历史数量持续增加。
- 真实 deep link header 不出现 `Loading task…`，Changes 在无 session-list 争用下
  <3s 完成。
- desktop/mobile × light/dark 核心路径与 console 0；截图/时序证据归档
  `qa/runs/2026-07-11-QA61-completion-audit/`。

## 实施步骤

1. CLI/API pagination contract + tests。
2. Frontend serialized progressive hydration + title/footer polish + tests。
3. 全树检查、live 共享数据 QA、三层/支撑件收口、归档工作纸、push main。

## review 裁决

做契约/正确性 review：分页不可漏当前活跃 session、不可用 metadata 代替 journal
真相、不可改变无 flag CLI 行为。安全面无新增输入执行；query 仅受界整数并转成
固定 argv。并发面由单一 in-flight promise 和顺序 page loop 机械覆盖。

# INC-53 Project overlay + 系统 launcher（HANDA-PARITY #24）

## 动机与 journey 锚

HANDA-PARITY §2 第 #24 行（用户裁决 override）：Handa 有 "Project 一等实体
+ 系统 launcher"（web_projects 注册表 + launcher API）；AgentRunner webui
现状只按 workspace 目录名分组、无 launcher。锚 journey **UJ-24**（Web UI 产品
面）：Projects→task 侧栏已在，本增量补两件事——(1) 每个 project 的用户偏好
（自定义名 / 折叠 / 最近打开），(2) 从 webui 直接用系统 app 打开 workspace
目录（VS Code / Finder / Terminal）。

**严格按 review 修订定案**：
- **不建服务端注册表**。分组仍从 journal 的 workspace 派生（守 DESIGN §12
  「project grouping 以 workspace 为键」不变量），不删既有 localStorage key、
  不迁移用户本地偏好。
- overlay = 扩展现有 `webui/webui-meta.json` 为 workspace-keyed 装饰性偏好，
  **绝不**成为分组/diff/附件的唯一真相来源（守 §12「metadata 非真相源」）。
- launcher = 薄后端新 OS-exec surface `POST /api/open`，localhost + 用户驱动。

**枚举型交付物逐项对锚（G29 纪律，裁掉的显式声明）**：Handa 原 #24 四件
（注册 / 重命名 / 移除 / last_opened）——
- **重命名（display name）** → 做，overlay `displayName`。
- **last_opened** → 做，overlay `lastOpened`（由 `/api/open` 触发写入，即
  "在系统 app 里打开该目录" 这一动作即 "opened"）。
- **折叠（Codex 式 project 组折叠）** → 做（review 追加），overlay `folded`。
- **注册 / 移除** → **显式裁掉**：我们不建注册表，group 从 journal 自动派生，
  只要还有 session 引用该 workspace 该组就存在，"注册/移除一个 project" 在
  派生模型里无语义。overlay 是纯偏好，删偏好 = revert 到派生默认（清空
  displayName / folded），不是删 project。此裁决记 LOG。

## Spec delta

`docs/SPEC.md` Web UI 消费面新增一行（INC-53）：project overlay（自定义名/
折叠/last_opened，webui-meta.json workspace-keyed，非分组真相源）+ 系统
launcher（`POST /api/open`，app 白名单 + 已知 workspace 校验 + `exec.Command`
传参不过 shell / `open`(macOS) `xdg-open`(Linux)）。状态 **⚠️**（A 闸绿；B 闸
真 `open -a` 打开 app + overlay 持久化待真机验，见下）。锚：
`TestLaunchArgvWhitelist` / `TestOpenRejectsUnknownApp` /
`TestOpenRejectsUnknownWorkspace` / `TestOpenLaunchesKnownWorkspace` /
`TestMetaStoreProjectOverlayRoundTrip` / `TestMetaStoreLoadsLegacyFlatFile` +
前端 vitest（`projectDisplayName` / `visibleProjectSessions`）。

## Design delta

`docs/DESIGN.md` §12 "Web UI 产品 surface" 追加一条 **additive** bullet，描述
launcher 的新 OS-exec surface（薄层、localhost、用户驱动、app 白名单、
已知-workspace 门）。

**是否触不变量：否。** 论证（PROCESS §四判据）：
- 既有粗体不变量「webui 只通过公开 `ar` CLI/daemon contract 读取 *session
  运行真相*」的作用域是 **session 状态机 / 运行真相**。webui 早已有一批
  host-side 便利直接 exec（`git diff/commit/worktree/init`、`os.MkdirAll`
  建 workspace、`http.ServeFile` 传文件），它们从不经 `ar`，也从不被该
  bold clause 约束——因为它们不是 session 运行真相。`/api/open` 用
  `open`/`xdg-open` 打开一个目录属**同一类** host 便利，非 session 状态，
  故是 additive 新面，不修改也不放宽该 bold clause。
- 既有粗体不变量「project grouping 以 workspace 为键 …… metadata 非唯一
  来源」——本增量分组逻辑**零改动**，overlay 只改"显示的 label"与"是否
  折叠"，绝不参与分组归属；display name 缺省回落到派生 label。守住不变量。
- 「pin/archive/rename/theme/sidebar/unread localStorage key 原样保留，不迁移
  不删」——本增量不碰任何 localStorage key；overlay 是**新的 server 侧**
  存储，与前端本地偏好正交。

故按 §PROCESS 作为 additive 记档即可，**不**走 §四不变量变更流程。

### 新 OS-exec surface 安全条款（记档，硬红线）

`POST /api/open {workspace, app}` 是 webui 新增的执行面。防线：

1. **app 白名单化，绝不拼 shell**。`app` 只作为**选择键**索引一张固定的
   per-OS argv 模板表（`launchArgv`）；用户输入永不命名可执行文件、永不
   进入 argv[0]。未在白名单的 `app` → 400 拒绝，零 exec。
   - macOS：`vscode`→`open -a "Visual Studio Code" <dir>`；
     `finder`→`open <dir>`；`terminal`→`open -a Terminal <dir>`。
   - Linux：`vscode`→`code <dir>`；`finder`→`xdg-open <dir>`；
     `terminal`→本平台不支持，返回错误（不猜一个脆弱的终端）。
2. **workspace 必须是已知 workspace**，不接受任意路径。已知集 = 实时
   `ar sessions list --json` 派生的 session workspace 集合（与侧栏所见一致），
   经 `EvalSymlinks`+`Clean` 规范化后做成员校验，且必须是存在的目录。
   未知 / 任意路径 → 400 拒绝，零 exec。**fail-closed**：拿不到已知集就拒。
3. **参数数组传递**：目录永远是 argv 的**末位独立元素**（`exec.Command`
   直传，不过 shell），路径无法被解释成 flag 或命令文本。
4. **localhost + 用户驱动**：沿用 webui 既有 loopback 绑定与
   `readBody` 的 `application/json` Content-Type 门（挡 no-CORS 简单请求，
   INC-D1 F2），恶意网页无法驱动此面。
5. overlay 读写**原子**（temp+rename，复用 `persistLocked`）、**容忍文件
   不存在**、**向后兼容旧 flat 格式**（顶层探测 `sessions`/`projects` key；
   旧文件整体当作 sessions map 读入，下次写入升级为 wrapper 格式）。overlay
   是非权威 cache，跨版本瞬时不一致自愈（旧 webui 读到 wrapper 会丢 cache 的
   title，但 title 由 runtime list 立即回填——可接受，见 QA 段）。

## 验收

### A 闸（scripted 孪生 / 单测，进 check.sh）

后端 Go（`webui/open_test.go`、`webui/ar_test.go` 扩充）：
- `TestLaunchArgvWhitelist`：白名单每个 app 逐个对锚——(a) 未知 app 返回
  ok=false；(b) 每个已知 app 的 argv 末位 == dir 且 argv[0] ∈ 允许的
  launcher；(c) per-GOOS 精确 argv 匹配（darwin/linux 分支）。**证明**
  "app 不落 shell、目录是隔离末位参数"。
- `TestOpenRejectsUnknownApp`：`/api/open` 传未知 app → 400，注入的 launch
  capture 从未被调用。
- `TestOpenRejectsUnknownWorkspace`：传一个**不在已知集**的真实存在目录
  → 400，launch 从未被调用（证明"存在的目录"不够，必须"已知"）。
- `TestOpenLaunchesKnownWorkspace`：已知 workspace + `vscode` → 200，launch
  被调用一次且 argv 正确、`lastOpened` 被写入 overlay。
- `TestMetaStoreProjectOverlayRoundTrip`：setProject（名/折叠）+ touchProject
  持久化后 reload 命中；session 侧 cache 不被 overlay 干扰。
- `TestMetaStoreLoadsLegacyFlatFile`：写一个旧 flat 格式文件，newMetaStore
  仍读回 session cache（向后兼容），且能再叠加 project overlay。

前端 vitest（`webui/frontend/src/viewModels.test.ts` 扩充）：
- `projectDisplayName`：有 overlay 名用 overlay，空则回落派生 label。
- `visibleProjectSessions`：folded 隐藏全部 session；search 覆盖 fold（匹配
  不被折叠藏住）；unfolded 时 expanded/search 显示全部、否则前 cap 条。

### B 闸（真实，parent/collector 真机集中验——本 agent 不跑）

在真机（macOS）跑真 webui：
1. 侧栏某 project 右键 → "Open in VS Code" → **真的**拉起 VS Code 打开该
   workspace 目录；"Open in Finder" → Finder 打开该目录；"Open in Terminal"
   → Terminal 打开该目录。（注意：会真的启动 app，谨慎跑。）
2. 打开后 overlay `lastOpened` 写入 `webui-meta.json`，侧栏该 project 显示
   "最近打开"相对时间；重启 webui 后仍在（持久化）。
3. 右键 → "Rename project…" 改自定义名 → 侧栏 heading 用新名；清空 →
   回落派生 label；重启后保留。
4. 点 project heading 折叠/展开 → 折叠态持久化，重启 webui 后保留；搜索时
   折叠的组仍能命中显示。
5. **拒绝面**（真机 curl，证明红线）：`POST /api/open` 传
   `{"app":"/bin/sh"}` 或未知 app → 400 不执行；传
   `{"workspace":"/etc","app":"finder"}`（任意路径）→ 400 不执行。
6. **跨版本自愈**：升级后旧 webui 若并发在跑，`webui-meta.json` 变 wrapper
   格式——确认 session title 由 runtime list 立即回填、不出现空态残留。

证据归 `qa/runs/<日期>-INC53/`。

## 实施步骤

1. 后端 overlay：`webui/meta.go` 扩 `metaStore`（projects map + wrapper 持久
   化 + 向后兼容 load + setProject/touchProject/allProjects）。
2. 后端 launcher：`webui/open.go`（launchArgv 白名单表 + knownWorkspaces 派生
   + handleOpen）；`webui/api.go` 注册 `POST /api/open`、`GET /api/projects`、
   `POST /api/projects`。
3. 后端测试：`webui/open_test.go` + `webui/ar_test.go` overlay 测试。
4. 前端：`api.ts`（projects/updateProject/openIn）、`types.ts`（ProjectMeta）、
   `store.ts`（projects state + refresh/set/toggle/open）、`viewModels.ts`
   （projectDisplayName/visibleProjectSessions 纯函数）、`Sidebar.tsx`（heading
   折叠接 overlay、display name、右键菜单加 Open in… / Rename… / lastOpened）。
5. 前端测试：`viewModels.test.ts` 扩充。
6. A 闸：`PATH="/opt/homebrew/bin:$PATH" ./scripts/check.sh`（node 24），
   exit=0；新 test 文件先 `git add`。dist 不提交（build 后 `git checkout --
   webui/frontend/dist`）。

## review 裁决

小增量（S/M，webui-only，additive，不触不变量）。按 PROCESS §二末句在工作纸
声明**裁掉**里程碑级三视角对抗 review：安全视角的核心（新 OS-exec 面）已在上
"安全条款"逐条成文并由 A 闸拒绝面测试覆盖（未知 app / 任意路径），契约视角
（不变量论证）已在 Design delta 成文。B 闸真机验收兜住 runtime 红线。

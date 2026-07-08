# arweb 开发台账(loop 驱动)

图例:`[ ]` 未动 `[~]` 进行中 `[x]` 完成(代码绿 + 真实验证过)

**loop 执行纪律(每轮迭代)**:
1. 取下面第一个未完成项(按序,不跳跃;一轮做一个 milestone 或其
   收尾余项)。
2. 实现;`cd web && gofmt -l . && go vet ./... && go test ./...` 全绿。
3. **真实验证**:真 `ar` + 真 daemon + 真 Gemini(`../.env`),按该
   milestone 的"真验"栏逐字执行并观察;证据摘要写进变更记录。
   没做真验的项不许打 `[x]`。
4. 更新本文件(勾选 + 变更记录追加一行),`git commit && git push`。
5. 全部 milestone `[x]` 后:收官检查(README 走查一遍),结束 loop。

---

## M0 蓝图与骨架
- [x] DESIGN.md / PROGRESS.md / README.md / .gitignore
- [x] 独立 module(arweb, stdlib-only)+ server 骨架 + 单文件 UI 壳
- [x] /api/health(版本 + daemon 探活)、daemon 托管(spawn/external 判定)
- [x] fake-ar 单测框架(exec 层不打真 API)
- 真验:`go run . --env-file ../.env` 起服务,浏览器打开首页,health
  绿点,sessions 列表能显示(空或已有)。

## M1 会话只读面(journal 观察器)
- [x] GET sessions / events?after / state / inspect / ps 五端点
- [x] 时间线渲染:DESIGN §5 全映射(用户/助手气泡、工具卡回填、
      轮次线、spawn/settle、waiting 状态条、兜底行)
- [x] 原始 journal / 折叠状态 / inspect 树 三个查看面板
- 真验:用 CLI 手工建一个真实会话跑两轮(真 Gemini),网页端完整
  重现时间线与状态;`ar events` 输出与页面逐事件对照无缺漏。

## M2 会话读写(chat 主链路)
- [x] POST sessions(spec+worker 落盘、workspace helper、mode)
- [x] send(含排队 pending 气泡)、interrupt、close;错误面(stderr 透出)
- [x] 新会话表单(base.yaml/worker.yaml 预填、空 workspace 一键造)
- 真验:全程网页操作——新建会话(真 Gemini)问答两轮上下文衔接;
  忙时插话排队生效(QA-02 式);interrupt 打断长 bash 后会话可续聊。

## M3 编排面(子 agent / 审批 / 图片)
- [x] ps 面板 + kill 按钮(用户直杀);spawn/subagent 事件卡
- [x] 审批卡(approve/deny + 理由),用 ask 权限 spec 真验
- [x] 图片上传 + send --image(真 vision 读图)
- 真验:网页指挥"起恰好 2 个子 agent"并 ps 可见、杀一个、另一个
  自然完成回灌;ask 模式下批准/拒绝各走一次;传 qa/fixtures 截图
  问答正确。

## M4 流式与打磨
- [ ] SSE attach 透传;text_delta 打字气泡(journal 到达即落实)
- [ ] 断线/daemon 重启的 UI 表达(探活红点 + 一键重启 daemon)
- [ ] 会话 URL hash 持久化、自动滚动、状态 pill 精化
- 真验:流式回答肉眼可见逐字出;kill -9 daemon → 页面红点 → 一键
  重启 → 同会话续聊无缝(QA-08a 式)。

## M5 压轴终验与收官
- [ ] QA-09 式场景全程网页操作:图 + 恰好 3 子并行 + 先回先处理 +
      杀 B 换 D + 汇总 + 崩溃重启续聊 + 让它写 SUMMARY.md
- [ ] README 快速上手走查(按文档从零起服务);已知问题清单
- 真验:上述场景一次成(允许按 QA §0.1 重跑一次);全 milestone 勾满。

---

## 已知问题(web/ 之外的发现,按铁律不在此修)

1. **[产品 bug] Gemini 空 assistant parts 毒化会话**(2026-07-07,会话
   20260707-231439-task-1dfc):模型某轮返回空内容 → `assistant_message`
   以 `parts: []` 落 journal → 之后每轮组装历史都被 gemini adapter 拒绝
   (`message with role "assistant" has no parts`,internal/retryable=false)
   → session_closed(error),revive 后同样历史同样死,**会话永久不可
   恢复**。已提独立修复任务(task_572ca493);涉及落账侧(不落空消息)
   与组装侧(过滤空消息救活存量)两个面。该会话是用户实际使用驾驶舱
   时踩到的——驾驶舱作为测试工具第一天就抓到真 bug。

## 变更记录(每轮追加;只记真实发生并验证过的事)

| 日期 | 轮次 | 动作 | 真验结果 |
|---|---|---|---|
| 2026-07-07 | 3 | M3 真验 + 三个健壮性修复:hashchange 监听(同页 hash 导航原先不切会话)、daemon 守护自愈(托管 daemon 被外杀 1s 内自动重启,3 次/分钟节流)、测试环境 binary 改名 arbin 避开并行 session 的 pkill 误伤 | ①spawn:网页指挥起恰好 2 子,在飞面板 2 行+kill 按钮;网页杀 A→journal `[kill call_10_0]` source=control→activity_cancelled+subagent_completed(canceled);B 自然完成回灌;父第 12 轮激活消化;模型自作主张重启 A(模型行为,再杀+叫停后正确汇总"A 被取消,B 汇报 B_DONE")②审批:ask 权限 spec,审批卡带全 gate 徽章(permission:ask rule 1: tool=bash→ask)+args;拒绝(理由达模型:denied: 测试拒绝路径)→批准→bash 执行→APPROVAL_TEST ③图片:upload API+chip+send --image,build-error.png 三要素全对(command.go/1234/EnableTraverseRunHooks2);journal ref-not-bytes(单行 372B,sha256 CAS ref) |
| 2026-07-07 | 0 | M0 落地:module/server(9 端点+SSE)/单文件 UI/fake-ar 单测 ×9/docs。M1–M5 的代码骨架同时就位,待逐项真验 | health 绿(daemon 托管成功);sessions 列表 OK;真 Gemini 全链路 smoke:POST /api/sessions 建会话→"1+1=?"→journal 里 ASST"2"→waiting:input。注意:XDG_DATA_HOME 过长会使 daemon socket bind 失败(macOS 104B 限制),测试用 /tmp/aw1 |
| 2026-07-07 | 1 | M1 真验(代码已在轮 0 就位,本轮纯验证,零代码改动) | CLI 建真实两轮 Gemini 会话(暗号"红苹果"第二轮复述→上下文衔接);`ar events --json` vs web /events 20 事件逐一 MATCH(seq+type);after=13 过滤→7 条;state=waiting:input、inspect 树(2 llm entries+usage billed 1058)、ps 空、sessions 双会话均对;Chrome 实测 UI:时间线气泡/轮次线/source 标签(cli/你)/状态 pill/三查看面板/系统事件开关(#4 barrier、#5-6 effect、#7-8 activity 兜底行)全部正确渲染 |
| 2026-07-07 | 2 | M2 真验全程 Chrome 网页操作;修 close 为双击确认(原生 confirm 冻结渲染进程、毁自动化);发现产品级 bug 记入已知问题 #1 | ①新会话表单:造空 workspace 一键、默认 spec、开场消息→真 Gemini 会话 42f4,两轮暗号"蓝海豚"衔接 ✓;②QA-02 式排队:sleep 20 在飞时插话→气泡"排队中"(pending)→bash 完成卡(QUEUE_TEST_DONE)→插话消化答"三加四等于七",bash 无 Cancelled ✓;③interrupt:sleep 30 在飞 8s 时点按钮→bash 卡"已取消"(部分输出留存)→[interrupt] 来源气泡→第 7 轮解释→续聊"OK" ✓;SSE 打字气泡实测出现并被 journal 落实替换(M4 部分提前验证);close 双击确认在已关闭会话上走通 UI 路径 |

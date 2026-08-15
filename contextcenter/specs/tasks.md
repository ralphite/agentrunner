# Context Center — Stage 1 实施拆解（Tasks）

对应 `plan.md` §1 Stage 1。按依赖排序；每组可独立验收。完成标准始终对
照 `spec.md` §9 验收标准（括号内编号）。

## F0. 样例文件树

- [ ] 按 `plan.md` §3/§5 手写 Aurora IDE + Atlas Deploy 两棵真实 `.md`
      文件树（含 frontmatter、镜像行、Loop 队列、Attempt 历史）
- [ ] C1 冒烟：把样例目录丢给真实 Coding Agent，让它口述项目状态并追加
      一条 Attempt，检查格式合规（验收 14 的手动版）

## F1. core 层

- [ ] frontmatter + Markdown 解析/序列化（round-trip 无损）
- [ ] 文件树内存模型 + 单一写入口 + 变更订阅
- [ ] 链接解析：相对路径 → 文档；id 兜底；broken link 标记（验收 15）
- [ ] 索引：id→path、每文档 task 列表、反向链接
- [ ] 物化：to-do 行 → Task 文档 + 镜像行改写（验收 13）
- [ ] 移动/重命名 + 全量入链改写（验收 15）
- [ ] 整树导出（下载 zip / 复制目录文本）

## F2. 应用壳与树

- [ ] 三栏布局（树 / 画布 / 右栏双区）+ 顶部面包屑（Notion 基准，见
      plan §4）
- [ ] 文档树：多 Project、展开/折叠、选中态；**无目录节点**（验收 1、18）
- [ ] hover `+`/`⋯`：新建子文档、重命名、移动、删除；无独立 create 按
      钮（验收 2、19）
- [ ] 行级 Task 文档隐藏规则（验收 16）
- [ ] type/tags → icon 解析（手动 > type 预设 > 默认）
- [ ] 搜索

## F3. 文档画布

- [ ] Markdown 块渲染：标题/正文/列表/引用/代码块
- [ ] 始终可编辑（contenteditable，无 Edit 按钮）（验收 3）
- [ ] 一行式子文档链接块（验收 4）
- [ ] 未物化 to-do 行（勾选即改父文档源码）

## F4. 右栏：Page Info + Chat（核心交互）

- [ ] 右栏常驻双区；切换页面即切换内容（验收 17）
- [ ] Page Info：当前文档 frontmatter 的人性化渲染（project/task/loop/
      bug/普通文档各一套字段模板）
- [ ] Chat：线程展示、输入框、Agent 选择、发送
- [ ] 任意文本/Block/Task 行选区捕捉 + 浮动入口 → blockquote 文字引用
      插入 Chat 输入框（验收 5）
- [ ] Agent 回复 + Proposed update 预览 + Apply 到当前页（验收 6）
- [ ] "Agent 将看到什么" Context 预览（可增删引用）

## F5. Task 行与 Task 页

- [ ] Task 镜像行：状态 pill、id、点击打开 Task 文档页
- [ ] Task 页 Page Info：状态/Attempts/关联文档/Actions
- [ ] 从选中文本生成 Task；委派入口

## F6. Attempt 与 Lesson

- [ ] Attempt History 列表 + Attempt Inspector（验收 7）
- [ ] Retry / 标记 best / Use this result
- [ ] Save as lesson → lesson 文档 + 来源引用（验收 8）

## F7. Loop

- [ ] Loop Widget：队列渲染（正文链接列表驱动）、当前项、页脚动作行
- [ ] 启动/暂停/Stop；失败暂停语义；状态写回（验收 9）
- [ ] Loop Inspector：当前项/后续队列/stop condition/retry 策略

## F8. Bug Fix Loop

- [ ] Bug Backlog 样例页全交互：Add bug、Resolved 区、进行中 Loop 卡
- [ ] Bug Inspector：priority/fix attempts/best result/Actions（验收 10）

## F9. Mock Agent

- [ ] `AgentAdapter` 接口 + `MockAgent`：脚本化回复、延迟/流式模拟、
      提议更新生成、Attempt/Lesson 写回（验收 11 —— 全程不出现
      Diff/Transcript）

## F10. 移动端

- [ ] 树 → 抽屉；右栏（Page Info + Chat）→ 底部 Sheet
- [ ] 选区引用在触屏可用

## F11. 收口验证

- [ ] 按 `spec.md` §9 逐条过验收清单
- [ ] C1 终验：导出样例树交给真实 Codex/Claude Code 完成验收 14

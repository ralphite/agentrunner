# 黑盒2 报告 B:想要监督与审批

诉求:AI 动手前先经我同意。结局:建立起来了、最终体验很好,但差点在门口(run 默认路径)就放弃。

## 问题
### [R2-B-1] 同一 spec:run 默默 deny,new 却停下问我——行为相反且零解释 🔴
默认 spec(无 permissions/mode)下 `ar run` 一律 `denied by policy` 不问;换 `ar new`(daemon)则 `⏸ approval required` 停下等批。谨慎用户照 README 先用 run,撞"denied+AI 说没权限"死胡同极可能放弃,而真相是换 new 监督-审批全都有。**最高杀伤:想监督的人被默认路径劝退。**

### [R2-B-2] run 下操作被静默拦截,用户不知情也无处补救 🔴
输出只有 `denied by policy`,不说为何、不指路 approve/换模式/配 permissions。真因 `denied: auto-denied (AGENTRUNNER_APPROVE unset or never)` 只在别的模式里偶然从 AI 转述漏出。只看结论的人会误以为是 AI 能力问题。

### [R2-B-3] mode 四值 default|plan|acceptEdits|bypass 全零解释,且 bypass 在模板里被藏起 🔴
模板注释只列前三个且无说明;bypass(最危险、全自动绕过审批)只在 run -h 出现,零警示。语义全靠建文件跑一遍反推。谨慎用户易把 acceptEdits 当危险、把 bypass 当"宽松点"误开全自动。

### [R2-B-4] plan 模式在 run 下是"假审批":AI 说"请批准这个计划"但根本无处可批 🟠
`ar run -mode plan` → AI "Please approve this plan to proceed" 随即 run completed。plan 只是关掉写工具,不是 human-in-the-loop 闸门,易被误当审批。

### [R2-B-5] 默认全拒的真正机关是未公开环境变量 AGENTRUNNER_APPROVE 🟠
底层错误 `auto-denied (AGENTRUNNER_APPROVE unset or never)`;该变量在 README/help/模板从不存在,只从报错泄出,猜值(allow)还猜不中。

### [R2-B-6] trust 命令无 -h 帮助,与审批/默认拒绝的关系成谜 🟡
`ar trust -h` → `lstat -h: no such file or directory`。谨慎用户想知道"trust 目录后 AI 是否就不用问我"(关系监督会否被悄悄关掉),查无帮助。

### [R2-B-7] 审批闸门只在 daemon 会话,但 README 主推 run,错配无提示 🟡

## 正面(human-in-the-loop 设计其实很好)
- 走对路(daemon + new/send)后:`⏸ approval required` 明确展示 AI 要干什么、`waiting:approval` 可见、approve 命令自动拼好给我复制、**deny 能带理由且 AI 真读到理由并调整行为**、approve 后真动手。
- 惊喜:**daemon 会话默认(无需任何配置)就会问我**——安全默认其实很好,只是被 run 路径挡住没人发现。

## 结局
建立起来了且体验好,但最卡处致命:默认入口 run 把想监督的人劝退。修 R2-B-1(哪怕在 run 的 denied 后加一句"要逐步审批请用 ar new")即可让"想盯着 AI"从靠运气变成顺理成章。

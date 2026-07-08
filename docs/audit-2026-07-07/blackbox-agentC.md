# 黑盒报告 C:多轮对话体验(跟 AI 聊几轮搭个 todo 脚本)

结论:AI 的"脑子"侧多轮完美(4 轮上下文严丝合缝、第 4 轮精准复述前 3 轮、延迟 1-4s),但"聊天基本盘"交互很痛——发消息看不到回复、轮末靠猜、没命令能把对话当聊天读、close 关不掉还谎报。引擎是好的,缺一层"聊天外壳"。

## 逐轮实录要点
- 起步三连墙:--help/-h/help 全 unknown 且子命令 help 不释义;new/run 都强制未文档化的 spec.yaml(靠喂错读报错逼字段:provider→name→id≠model.name→system_prompt 二选一;模型名盲试到 gemini-flash-latest);daemon 要自己猜着 `ar daemon &` 后台起(报错只说"is the daemon running?"不指路)。
- 第1轮 `ar new chat.yaml "写 todo.py..."` → **只回 session ID,无回复**。试 4 种找回复:(A)翻工作区→AI 把代码写在聊天正文不落盘;(B)`ar ps $SID`→"no tasks in flight"看不到内容;(C)`ar attach $SID`→**看到全文但挂住不返回,卡满 2 分钟被杀**;(D)`ar events $SID`→会退出但 assistant_message 正文**被截断** `"Here is a simple \`todo.p…"`+满屏黑话,要全文得 `events -json` 手写 JSON 解析。
- 第2-4轮 `ar send` → **每次只回 `delivered`,无回复**,回复 1-4s 后 ready 但不 poll events 不知道好没好。上下文连续(input_tokens 59→554,在原脚本上叠加 delete/priority)。第4轮总结精准复述前3轮 ✓。
- 想结束:`ar close $SID` → 回 `closing` 退0,**但随后 send 仍 `delivered` 且真跑出 gen_step 5、6**;`inspect` 状态自相矛盾 `waiting (closed)`;daemon 关掉后 sessions list 里它仍 `waiting:input`。**关不掉还骗你成功。** `ar resume $SID`→`session locked: held by pid`。

## 问题(按杀伤力)
### [BB-C-1] send/new 发完不回显 AI 回复,只给 delivered/一个 ID 🔴致命
聊天命根子:发完就地看不到 AI 说了啥,消息像掉黑洞,必须换命令去别处捞。

### [BB-C-2] 没有任何命令能把对话当聊天完整读出(你的话+AI完整回复) 🔴致命
四条路全废:attach 挂住;events 正文截断+黑话;events -json 要手写解析;inspect 完全无正文只有 token 报表。

### [BB-C-3] attach 打印完回复不退出(实时 tail 挂死),退不退还看状态 🟠严重
live/waiting 时挂满 2 分钟被杀;closed 后自己退但以 `✗ resume failed: context canceled` 吓人收尾。

### [BB-C-4] ar close 关不掉对话还谎报成功 🟠严重
close 回 closing 退0,但随后 send 仍 delivered 且真跑新一轮;inspect 写 `waiting (closed)` 自相矛盾。关了个寂寞且无从察觉。

### [BB-C-5] 一轮结束没人话信号,只能 poll events 猜 🟠严重
唯一"轮末"信号是 events 里的 `waiting_entered {"kind":"input"}`——黑话、要主动反复查,没有阻塞等待。

### [BB-C-6] 起步三连墙:无 --help 释义 / 强制未文档化 spec.yaml / 手动起 daemon 🔴致命

### [BB-C-7] 退休模型 ID 无提示,报错让你"自己去 ListModels" 🟡糙

## 结局
AI 侧完美、交互侧很痛。最痛:发完看不到回复(C-1+C-2)。普通只想"跟 AI 聊天"的人,大概率在发出第一条消息前就撞三堵起步墙放弃;即便发出,delivered 后的沉默+attach 挂死+close 关不掉会让他彻底搞不清聊到哪了。缺的是"发完就地流式回显 + 轮末人话提示 + 一条命令读完整对话 + close 说到做到"。

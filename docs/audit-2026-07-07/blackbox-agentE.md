# 黑盒报告 E:糙边与坑(急躁用户凭直觉乱戳)

三大恼火:①一进门就撞死(--help/-h/help 三连 unknown+无README+被逼写没人解释的 spec.yaml,近10轮才跑通);②永远不知道成没成(send只回delivered/new只给id/close说closing却仍running,回复要另跑attach/events去满屏JSON里翻);③说一套做一套名不副实。亮点:脏输入扛得极稳。

## 问题(按恼火程度)
### [BB-E-1] --help/-h/help 顶层三种求助全 "unknown command" 🔴致命
### [BB-E-2] 无 README、无例子、spec.yaml 是无解第一墙(空文件报 EOF,约10轮反推) 🔴致命
### [BB-E-3] `-h` 跨子命令严重不一致,有的把"求助"变成真操作+报错 🟠严重
`ar close -h` → 去 dial daemon 报 `is the daemon running?`;`ar ps -h` → 读 sessions 目录报路径错。求助不该有副作用。(+ 已另证 `ar trust -h` → `lstat -h: no such file or directory`。)
### [BB-E-4] 报错泄漏内部术语/Go类型/日志管道 🟠严重
`cannot unmarshal ... into agent.ModelSpec`;取消时 `barrier skipped: snapshot failed err="snapshot: git add: context canceled"`;events 满屏 checkpoint_barrier/effect_requested/gate_results/verdict/snapshot_ref。
### [BB-E-5] Ctrl-C 一次停不下,第二次才杀,退出脏 🟠严重
第1次INT只印 `↺ interrupted by user` 但正文继续流、进程存活;第2次才终止并吐内部 WARN/ERROR。
### [BB-E-6] send后不显示回复、new后不显示开场回复——每轮"不知成没成" 🟠严重
### [BB-E-7] "开对话"有隐藏前置 daemon,且 daemon 前台阻塞终端 🟠严重
run 不需 daemon 但 new 需要(同工具两套心智);`ar daemon` 前台卡住,无 --detach。
### [BB-E-8] close 语义含糊:说 closing 却仍 running,且 send 能把已关会话复活 🟠严重
### [BB-E-9] ps 名不副实:不是列表、需要 id、空态泄漏路径 🟡糙(真列表是 sessions list)
### [BB-E-10] 打错子命令无"你是不是想说…"建议(ar rnu / ar sesions) 🟡糙
### [BB-E-11] 取消的会话在 sessions list 里伪装成 completed(只有 inspect 才有 canceled) 🟡糙
### [BB-E-12] 无"当前会话"概念,每次都得贴50字符长id(支持前缀算缓解) 🟡糙

## 亮点(正面项)
脏输入扛得极稳:反引号/`$(whoami)`/引号/`<tag>`/emoji🎉/日本語/换行/2万字 全部原样通过,无崩溃、无乱码、无命令注入。引擎(跨轮记忆、真实Gemini)是好的,糙的全在最外层这层皮。

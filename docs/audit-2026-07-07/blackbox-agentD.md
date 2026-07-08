# 黑盒报告 D:让 AI 修 bug(自造红测试→让 AI 修→独立验证绿)

结局:**最终真修好了**(calc.py: a-b→a+b,独立 shell 里 python3 test_calc.py → PASS exit 0,改动落在 -workspace 我的目录),但要连闯 5 关。`run` 的过程可见性是强项;冷启动可发现性是最大短板。

## 摸索链(5 堵墙)
1. `ar --help` → unknown command;无 README。靠 usage 猜 `run`。
2. spec 格式靠 8+ 次"喂错字段读报错"拼出:`hello`→泄露 `AgentSpec`;model 是对象非字符串;provider 在 model 里非顶层;model 标识键只有 `id` 被接受;顶层要 name;system_prompt 二选一。最小可用 = name+system_prompt+model{provider,id}。
3. 默认无 tools → AI 直接 `blocked`(83 token 就停,啥都没读)。
4. 加 `tools:[bogus]` → 报错吐出**全部 15 个工具名**（唯一一次可见）。选 bash/read_file/edit_file/write_file 后:read 能读,但 edit_file/write_file/bash 全 `denied by policy`；run 却 **exit 0 "run completed"**，AI 还说"改成 a+b 就 PASS 了"。解锁法(approve/policy/AGENTRUNNER/bypass 语义)在所有 -h 里都查不到——**只因 AI 正文漏出 `AGENTRUNNER_APPROVE unset` 才知道有这变量**。
5. `AGENTRUNNER_APPROVE=always` 或 `-mode bypass` 破墙 → 跑测试→读源→改文件→再跑→绿,全链路可见。

隔离验证:bypass 单独✅ / APPROVE=always 单独✅ / 纯默认❌(全 denied，exit 0，目录原封不动)。

## 问题(按杀伤力)
### [BB-D-1] 默认模式静默"假成功":AI 说修好了+exit 0,但目录没变 🔴致命
不加 bypass/不设 APPROVE(=新手默认动作)时,edit/write/bash 全 `denied by policy`,但 run 以 **exit 0 + "run completed"** 收场,AI 正文热情说"PASS"。只看退出码或 AI 结论的人以为成功,实则 calc.py 一字节没动。**说改好了≠真改了,工具不帮拆穿。** 期待:被拒应非零退出,或结尾醒目标"⚠ N 个改动被拒,工作区未变更"。

### [BB-D-2] 解锁 AI 动手的方式无处可查,只能靠 AI"漏嘴" 🔴致命
ar / run -h / approve -h 全 grep 不到 approve/policy/permission/AGENTRUNNER;`-mode` 只列四个词不解释 bypass 干嘛。能解锁纯侥幸(AI 说出了 AGENTRUNNER_APPROVE)。期待:`denied by policy` 那行自带解法提示。

### [BB-D-3] 无 --help、无 spec 脚手架/样例,冷启动全靠猜 🟠严重
### [BB-D-4] 工具默认为空且清单不可查,AI 一开始手脚全无 🟠严重
### [BB-D-5] denied 仍 exit 0 + 非法 env 值(AGENTRUNNER_APPROVE=maybe)被静默接受 🟡糙

## 亮点(要记进报告的正面项)
`run` 默认逐 gen-step 打印每个工具调用 `→ bash {...}` / `→ edit_file {...}` 和结果 `← ok/error`,AI 干了啥一清二楚。**可见性是 run 路径的强项**——与 new/send 对话路径的"回复完全不显示"形成强烈反差。

## 成本
约 35 分钟、~20 命令、5 堵墙。

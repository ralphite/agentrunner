# INC-001 · 术语裁决落地指令

> 性质:开发者指令的原样记录,逐条执行。裁决原文见 REVIEW-001-terms.md。
> 细节设计在执行时做,不在本文预做。

## 指令

1. **只有一种 session,删除运行形态概念。** "task 形态"整体删除;
   `Conversational` 标志及 conversational/task 二分从代码里删干净。
   `ar run` = 开 session + 发消息 + 等静止 + 读结果的便捷命令。

2. **静止(quiescence)定义**:最后一个 turn 结束、无在飞工作、无未到期
   定时自触发——没有别人会触发它、它自己也不会再执行,就算结束。
   静止时通知 parent(用既有子回执,不加新事件);可多次发生。
   顶层无 parent 就不通知,退出码由观察者从静止状态读出。

3. **删除一切"终止/terminal"状态与 `TaskCompleted` 事件。**
   close/kill 只是标记(记录来源 user/parent),只被自动路径检查;
   用户显式 send 永远能重开任何 session。不要状态机。

4. **被 kill 的子按来源复活**:用户 kill 的,仅用户可复活;
   parent kill 的,parent 可复活。

5. **interrupt 永不结束 session。** turn 中 = 打断当前活动(部分输出
   保留),会话继续;待命处 = 什么都不做(删除"待命处 interrupt =
   close"这条惯例)。

6. **session 不与 agent 绑定,session 内必须可以换 agent。**
   用户切换 agent 不需要任何确认。

7. **子 agent 权限默认不得超过父**;要起权限超过父的子,必须用户
   approve。审批只存在于"agent 提权自己的子",不存在于用户自己的切换。

8. **Input 不要强类型分类。** 来源信息用内容前缀表达;类型只可以留在
   系统 log(journal)层做审计,不进给模型的内容。
   例外:前台 tool call 的结果必须保持 provider 协议配对,不能破坏。

9. **task 一词全删**:后台工作说 **handle**;"task 形态"义见指令 1。

10. **final generation 不单独特判异常**:若 step 出错,统一走一套
    step 异常处理。

11. **子 agent / 后台完成回执走 steer 通道**(当前 turn 内安全边界
    插入,不等 turn 结束)。投递方式在 agent 配置层给默认值、可
    override;不做 per-launch 逐个指定。

12. **state 说清楚**:state 更像 LLM request 里的 history 部分;
    system instruction 与 tools 出自 spec。术语表按此拆写。

13. **零 legacy,删就删干净。** 禁止"因为 X 在用所以保留";项目无
    发布、无兼容义务(如"阻塞 spawn 保留 v1 兼容"一并清除)。

14. **一切设计自顶向下**:每个技术选择可追溯到产品需求;无需求不设计。

15. **文档与代码全程同步**:改到哪,DESIGN §18 术语表 / SPEC / GAPS
    对应行改到哪;不许文档标 ✅ 而代码没做。

16. **每步改完立即 commit + push。**

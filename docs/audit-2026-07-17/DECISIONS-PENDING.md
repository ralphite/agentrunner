# 待用户裁决登记（audit-0717；loop 不因此阻塞，其余工作继续）

逐项给出裁决点与推荐;答复任意一项即可单独立 INC 实施,无先后依赖。

## 1. G3 审批挂起期间消息唤醒（工作纸 `docs/increments/INC-70-approval-park-wake.md`）
- **A** 维持在案定案(排队不解栈),SPEC 行转 🧊 记档关闭;
- **B(推荐)** park 中 user-class 消息 = 转向式拒批(自动 deny 挂起
  审批+同边界喂入消息;machine/untrusted 不享此权);
- **C** 审批不动、消息并行唤醒——违 in-doubt 教义,不推荐。

## 2. G13 SCM/PR 工作流一等公民化
- ①平台绑定:GitHub 专属(gh 依赖) vs 通用 SCM 抽象;
- ②审阅门形态:webui Changes→Approve→push,或 PR 草稿先行;
- ③"审阅通过才 push"约束落 rules 层还是新 gate。

## 3. G18 web search 后端
- ①后端:Brave / Tavily / SearXNG 自托管 / provider 服务端工具
  (Gemini grounding——需把"客户端执行"原则开例外类别并入 DESIGN);
- ②凭据来源(建议 env,与 GEMINI_API_KEY 同法);
- ③egress 语义(建议 web_fetch 同款 execute-class + 审批)。

## 4. G15 best-of-N 胜者晋升
SPEC 🧊 在案记档"v0 用户手动晋升"。如要做,请明确解冻并选:fork
接管 vs apply diff 回主 workspace(含冲突处理策略)。

## 5. G11 云 workspace 生命周期（建议专门 session 共创）
①环境模型(per-session 容器 vs 长驻 pool);②secrets 注入面;
③store 外置;④回收重建语义。

## 6. G32 Xcode.app 沙箱 git（非裁决,环境依赖）
需真 macOS + Xcode.app 机器开发验证(Linux 容器无法验 Seatbelt);
建议在你 mac 上开 session 做,方案已记 GAPS(PATH 截击/host git 代理)。

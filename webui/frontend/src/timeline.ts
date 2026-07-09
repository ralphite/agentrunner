import type { Envelope } from "./types";

// A single tool activity, resolved from its started/completed/failed/cancelled
// events into one card.
export interface ToolItem {
  kind: "tool";
  key: string;
  name: string;
  args: any;
  background: boolean;
  status: "running" | "done" | "error" | "cancelled" | "failed";
  statusText: string;
  result?: any;
  errorMsg?: string;
  partial?: string;
  usage?: { input_tokens: number; output_tokens: number };
}

export interface BubbleItem {
  kind: "user" | "assistant";
  key: string;
  text: string;
  source?: string;
  images?: number;
}

export interface TurnItem {
  kind: "turn";
  key: string;
  gen: number;
}

export interface ChipItem {
  kind: "chip";
  key: string;
  text: string;
  tone: "" | "warn" | "bad" | "good";
  childSession?: string;
}

export interface SysItem {
  kind: "sys";
  key: string;
  text: string;
}

export interface ApprovalRef {
  id: string;
  tool: string;
  args: any;
  gates: { gate: string; decision: string; reason?: string }[];
  resolved?: { decision: string; reason?: string; source?: string };
}

export type TimelineItem = ToolItem | BubbleItem | TurnItem | ChipItem | SysItem;

export interface Folded {
  items: TimelineItem[];
  approvals: Map<string, ApprovalRef>; // by approval id (journal side)
  callArgs: Map<string, { name: string; args: any }>; // call_id → tool
  status: { text: string; cls: string };
  lastGen: number;
  // active = the session is genuinely mid-work: a tool is still running, or a
  // generation step was just started with nothing produced yet. A child
  // session's own journal never records completion (it lands in the PARENT as
  // subagent_completed), so it ends on assistant_message and its status would
  // otherwise dangle at "运行中" forever — callers use !active to correct that.
  active: boolean;
}

// foldEvents replays the whole journal into an ordered item list plus the
// derived approval / status maps. Pure over `events`, recomputed each poll —
// journal is the source of truth (DESIGN I5).
export function foldEvents(events: Envelope[]): Folded {
  const items: TimelineItem[] = [];
  const toolByActivity = new Map<string, ToolItem>();
  const approvals = new Map<string, ApprovalRef>();
  const callArgs = new Map<string, { name: string; args: any }>();
  let lastGen = 0;
  let status = { text: "—", cls: "" };
  let lastType = "";

  const push = (it: TimelineItem) => items.push(it);
  const chip = (
    seq: number,
    text: string,
    tone: ChipItem["tone"] = "",
    childSession?: string,
  ) => push({ kind: "chip", key: "c" + seq, text, tone, childSession });

  for (const env of events) {
    const p = env.payload || {};
    const seq = env.seq;
    lastType = env.type;
    switch (env.type) {
      case "session_started":
        chip(seq, `会话开始 · ${p.spec_name || ""} · ${p.model || ""}`);
        break;
      case "input_received": {
        push({
          kind: "user",
          key: "u" + seq,
          text: p.text || "(空)",
          source: p.source && p.source !== "user" ? p.source : undefined,
          images: p.images && p.images.length ? p.images.length : undefined,
        });
        break;
      }
      case "generation_started":
        lastGen = p.gen_step || lastGen + 1;
        push({ kind: "turn", key: "t" + seq, gen: lastGen });
        status = { text: "运行中", cls: "run" };
        break;
      case "assistant_message": {
        const parts = (p.message && p.message.parts) || [];
        const text = parts
          .filter((x: any) => x.text)
          .map((x: any) => x.text)
          .join("");
        parts
          .filter((x: any) => x.tool_name)
          .forEach((c: any) => callArgs.set(c.call_id, { name: c.tool_name, args: c.args }));
        if (text.trim()) push({ kind: "assistant", key: "a" + seq, text });
        break;
      }
      case "activity_started":
        if (p.kind === "tool") {
          const t: ToolItem = {
            kind: "tool",
            key: "act" + p.activity_id,
            name: p.name,
            args: p.args,
            background: !!p.background,
            status: "running",
            statusText: p.background ? "task" : "运行中",
          };
          toolByActivity.set(p.activity_id, t);
          push(t);
        } else {
          push({ kind: "sys", key: "s" + seq, text: `#${seq} ${env.type} ${p.name || ""}` });
        }
        break;
      case "activity_completed": {
        const t = toolByActivity.get(p.activity_id);
        if (t) {
          t.status = p.is_error ? "error" : "done";
          t.statusText = p.is_error ? "错误" : "完成";
          if (p.usage) t.usage = p.usage;
          if (p.result !== undefined) t.result = p.result;
          if (p.is_error) t.errorMsg = t.errorMsg || "";
        } else {
          push({ kind: "sys", key: "s" + seq, text: `#${seq} activity_completed ${p.activity_id}` });
        }
        break;
      }
      case "activity_failed": {
        const t = toolByActivity.get(p.activity_id);
        const msg = p.error ? `${p.error.class}: ${p.error.message}` : "失败";
        if (t) {
          t.status = "failed";
          t.statusText = "失败" + (p.final ? "(终)" : `(重试 ${p.attempt})`);
          t.errorMsg = msg;
        } else {
          chip(seq, "活动失败: " + msg, "bad");
        }
        break;
      }
      case "activity_cancelled": {
        const t = toolByActivity.get(p.activity_id);
        if (t) {
          t.status = "cancelled";
          t.statusText = "已取消";
          if (p.partial_output) t.partial = p.partial_output;
        } else {
          chip(seq, "活动已取消 " + p.activity_id, "warn");
        }
        break;
      }
      case "spawn_requested":
        chip(
          seq,
          `⬇ 子 agent ${p.agent} · ${p.task ? p.task.slice(0, 80) : ""}`,
          "",
          p.child_session,
        );
        break;
      case "subagent_completed":
        chip(
          seq,
          `⬆ 子完成 ${p.agent} · ${p.reason} · ${
            p.usage ? p.usage.input_tokens + p.usage.output_tokens + " tok" : ""
          }`,
          p.reason === "completed" ? "good" : "warn",
          p.child_session,
        );
        break;
      case "approval_requested": {
        const known = callArgs.get(p.call_id);
        approvals.set(p.approval_id, {
          id: p.approval_id,
          tool: known ? known.name : p.call_id || p.approval_id,
          args: known ? known.args : undefined,
          gates: p.gate_results || [],
        });
        break;
      }
      case "approval_responded": {
        const a = approvals.get(p.approval_id);
        if (a) a.resolved = { decision: p.decision, reason: p.reason, source: p.source };
        // Leave a durable audit line in the feed (approve otherwise just
        // vanishes with no record).
        chip(
          seq,
          `${p.decision === "approve" ? "✓ 已批准" : "✕ 已拒绝"}${p.reason ? " · " + p.reason : ""}`,
          p.decision === "approve" ? "good" : "warn",
        );
        break;
      }
      case "waiting_entered": {
        const kinds: Record<string, [string, string]> = {
          input: ["待命,等你输入", "idle"],
          approval: ["等待审批", "appr"],
          tasks: ["等子任务/后台", "run"],
          timer: ["等定时器", "run"],
        };
        const [txt, cls] = kinds[p.kind] || [p.kind, ""];
        status = { text: txt, cls };
        break;
      }
      case "waiting_resolved":
        status = { text: "运行中", cls: "run" };
        break;
      case "session_closed":
        chip(seq, "会话已关闭 · " + (p.reason || ""));
        status = { text: "已关闭", cls: "closed" };
        break;
      case "task_completed":
        chip(seq, "任务完成 · " + (p.reason || ""));
        status = { text: "已结束", cls: "closed" };
        break;
      case "actor_crashed":
        chip(seq, `崩溃 ${p.actor}: ${p.error}`, "bad");
        status = { text: "崩溃", cls: "crash" };
        break;
      case "mode_changed":
        chip(seq, `mode → ${p.to} (${p.cause})`);
        break;
      case "context_compacted":
        chip(seq, `上下文压缩 · 到第 ${p.upto_gen_step} 轮`);
        break;
      case "limit_exceeded":
        // A user interrupt is modeled as limit_exceeded{kind:interrupted} —
        // don't dress it up as a budget overrun.
        if (p.kind === "interrupted" || p.kind === "canceled" || p.kind === "cancelled") {
          chip(seq, "已停止(你打断了这一轮)", "warn");
        } else {
          chip(seq, `预算超限 ${p.kind}: ${p.used}/${p.limit}`, "bad");
        }
        break;
      case "generation_discarded":
        chip(seq, `第 ${p.gen_step} 轮流式输出被丢弃重试`, "warn");
        break;
      case "malformed_tool_call":
        chip(seq, `第 ${p.gen_step} 轮工具调用不可解析,重试`, "warn");
        break;
      default:
        push({ kind: "sys", key: "s" + seq, text: `#${seq} ${env.type}` });
    }
  }

  const toolRunning = items.some((it) => it.kind === "tool" && it.status === "running");
  const active = toolRunning || lastType === "generation_started";

  return { items, approvals, callArgs, status, lastGen, active };
}

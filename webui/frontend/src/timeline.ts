import type { Envelope } from "./types";
import { friendlyStatus } from "./components/pill";

// fmtTok compacts a token count for feed chips: 199839 → "200k".
function fmtTok(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return Math.round(n / 1000) + "k";
  return (n / 1_000_000).toFixed(1) + "M";
}

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
  // A message relayed from another agent (team mail): rendered as a peer
  // message with a "from …" label, not as something YOU typed (W19).
  peerSession?: string;
  images?: number;
  // journal seq of the input_received — lets the view attach locally-known
  // upload thumbnails to a sent bubble (the journal itself keeps only CAS refs)
  seq?: number;
  ts?: string;
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

export interface RuntimeItem {
  kind: "runtime";
  key: string;
  source: string;
  text: string;
  ts?: string;
}

export interface ApprovalRef {
  id: string;
  tool: string;
  args: any;
  gates: { gate: string; decision: string; reason?: string }[];
  resolved?: { decision: string; reason?: string; source?: string };
}

export type TimelineItem = ToolItem | BubbleItem | TurnItem | ChipItem | SysItem | RuntimeItem;

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
  // otherwise dangle at "running" forever — callers use !active to correct that.
  active: boolean;
  // isDriver = this session is an iteration driver (drive), not a conversation.
  // Its journal is driver_* / iteration_* events and it does NOT accept input,
  // so the UI renders those events and hides the composer.
  isDriver: boolean;
}

// Input sources that mean "a human typed this" — regardless of entry point
// (interactive tty, cli send, or a UI that shells out to the cli). All render
// as "you"; only program/control sources (tool/parent/control/…) get a label.
const HUMAN_SOURCES = new Set(["user", "cli", "tty"]);

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
  let isDriver = false;

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
        push({ kind: "sys", key: "s" + seq, text: `session started · ${p.spec_name || ""} · ${p.model || ""}` });
        break;
      case "input_received": {
        // Team mail arrives as user-role input prefixed
        // "[message from <agent> (<session>)]" — strip the plumbing and
        // render it as a peer message, not something you typed (W19).
        const raw = p.text || "(empty)";
        const peer = /^\[message from ([^ ()]+) \(([^)]+)\)\]\s*/.exec(raw);
        const source = p.source || "user";
        if (!peer && !HUMAN_SOURCES.has(source)) {
          push({
            kind: "runtime",
            key: "r" + seq,
            source,
            text: raw,
            ts: env.ts,
          });
          break;
        }
        push({
          kind: "user",
          key: "u" + seq,
          seq,
          ts: env.ts,
          text: peer ? raw.slice(peer[0].length) || raw : raw,
          // Human-typed input via any entry point (user/cli/tty) is "you";
          // only program/control sources get a distinct label (UX-05).
          source: peer ? peer[1] : undefined,
          peerSession: peer ? peer[2] : undefined,
          images: p.images && p.images.length ? p.images.length : undefined,
        });
        break;
      }
      case "generation_started":
        lastGen = p.gen_step || lastGen + 1;
        push({ kind: "turn", key: "t" + seq, gen: lastGen });
        status = { text: "running", cls: "run" };
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
        if (text.trim()) push({ kind: "assistant", key: "a" + seq, text, ts: env.ts });
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
            statusText: p.background ? "task" : "running",
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
          t.statusText = p.is_error ? "error" : "done";
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
        const msg = p.error ? `${p.error.class}: ${p.error.message}` : "failed";
        if (t) {
          t.status = "failed";
          t.statusText = "failed" + (p.final ? " (final)" : ` (retry ${p.attempt})`);
          t.errorMsg = msg;
        } else {
          chip(seq, "activity failed: " + msg, "bad");
        }
        break;
      }
      case "activity_cancelled": {
        const t = toolByActivity.get(p.activity_id);
        if (t) {
          t.status = "cancelled";
          t.statusText = "cancelled";
          if (p.partial_output) t.partial = p.partial_output;
        } else {
          chip(seq, "activity cancelled " + p.activity_id, "warn");
        }
        break;
      }
      case "spawn_requested":
        chip(
          seq,
          `Subagent started · ${p.agent} · ${p.task ? p.task.slice(0, 80) : ""}`,
          "",
          p.child_session,
        );
        break;
      case "subagent_completed": {
        // The reason is an internal enum (max_generation_steps, …) — render
        // the human wording and a compact token count (W6).
        const reason = friendlyStatus(p.reason || "").text;
        const tok = p.usage ? p.usage.input_tokens + p.usage.output_tokens : 0;
        chip(
          seq,
          `Subagent finished · ${p.agent} · ${reason}${tok ? ` · ${fmtTok(tok)} tokens` : ""}`,
          p.reason === "completed" ? "good" : "warn",
          p.child_session,
        );
        break;
      }
      case "child_revived":
        chip(
          seq,
          `Member resumed · ${p.agent || ""} · woken by mail`,
          "",
          p.child_session,
        );
        break;
      case "command_handled":
        if (p.result && String(p.result).startsWith("forwarded:")) {
          chip(seq, `Forwarded to ${String(p.result).slice("forwarded:".length)}`, "", String(p.result).slice("forwarded:".length));
        }
        break;
      // ---- iteration driver (drive) events ----
      case "driver_started":
        isDriver = true;
        chip(seq, `Scheduled run started · ${p.spec_name || ""}`);
        status = { text: "running", cls: "run" };
        break;
      case "iteration_launched":
        isDriver = true;
        chip(seq, `Iteration ${p.iter} started`, "");
        break;
      case "iteration_completed":
        isDriver = true;
        chip(
          seq,
          `Iteration ${p.iter} completed · ${p.child_reason || ""}${
            p.verdict ? " · " + JSON.stringify(p.verdict) : ""
          }`,
          "good",
        );
        break;
      case "iteration_skipped":
        isDriver = true;
        chip(seq, `iteration ${p.iter} skipped`, "warn");
        break;
      case "driver_completed":
        isDriver = true;
        chip(
          seq,
          `■ driver ${p.reason || "done"} · ${p.iterations || 0} iteration(s)${
            p.best_iter ? " · best #" + p.best_iter : ""
          }`,
          p.reason === "satisfied" ? "good" : "warn",
        );
        status = { text: p.reason === "satisfied" ? "satisfied" : "done", cls: "closed" };
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
          `${p.decision === "approve" ? "Approved" : "Denied"}${p.reason ? " · " + p.reason : ""}`,
          p.decision === "approve" ? "good" : "warn",
        );
        break;
      }
      case "waiting_entered": {
        const kinds: Record<string, [string, string]> = {
          input: ["waiting: input", "idle"],
          approval: ["waiting: approval", "appr"],
          tasks: ["waiting: tasks", "run"],
          timer: ["waiting: timer", "run"],
        };
        const [txt, cls] = kinds[p.kind] || [p.kind, ""];
        status = { text: txt, cls };
        break;
      }
      case "waiting_resolved":
        status = { text: "running", cls: "run" };
        break;
      case "session_closed":
        chip(seq, `session ${p.reason || "closed"}`);
        status = { text: p.reason === "killed" ? "killed" : "closed", cls: "closed" };
        break;
      case "task_completed":
        chip(seq, "task completed · " + (p.reason || ""));
        status = { text: "completed", cls: "closed" };
        break;
      case "actor_crashed":
        chip(seq, `crashed ${p.actor}: ${p.error}`, "bad");
        status = { text: "crashed", cls: "crash" };
        break;
      case "mode_changed":
        chip(seq, `Mode changed · ${p.to} (${p.cause})`);
        break;
      case "spec_changed":
        chip(seq, `Agent changed · ${p.spec_name || "?"} · ${p.model || ""}`);
        break;
      case "context_compacted":
        chip(seq, `context compacted · up to gen ${p.upto_gen_step}`);
        break;
      // Goal lifecycle renders first-class (QA Round1 F-C5: these used to
      // fall into hidden sys lines, so a budget-stopped goal just vanished).
      case "goal_attached":
        chip(seq, `goal attached · ${p.goal || ""}`);
        break;
      case "goal_updated":
        chip(seq, "goal updated" + (p.goal ? ` · ${p.goal}` : ""));
        break;
      case "goal_paused":
        chip(seq, "goal paused", "warn");
        break;
      case "goal_resumed":
        chip(seq, "goal resumed");
        break;
      case "goal_cancelled":
        chip(seq, "goal cancelled", "warn");
        break;
      case "goal_checkpoint":
        chip(seq, `goal check ${p.check || "?"}${p.pass ? " · pass" : " · not met"}`, p.pass ? undefined : "warn");
        break;
      case "goal_achieved":
        // reason=budget means the check budget ran out — a visible STOP,
        // not success; saying "achieved" here misled users (F-C5).
        if (p.reason === "budget") {
          chip(seq, `goal stopped: check budget exhausted after ${p.checks} check(s) — not verified as achieved`, "bad");
        } else if (p.reason === "cancelled") {
          chip(seq, "goal detached · cancelled", "warn");
        } else {
          chip(seq, `goal achieved · ${p.reason || "satisfied"} (${p.checks} check(s))`);
        }
        break;
      case "limit_exceeded":
        // A user interrupt is modeled as limit_exceeded{kind:interrupted} —
        // don't dress it up as a budget overrun.
        if (p.kind === "interrupted" || p.kind === "canceled" || p.kind === "cancelled") {
          chip(seq, "stopped (you interrupted this turn)", "warn");
        } else {
          chip(seq, `limit exceeded ${p.kind}: ${p.used}/${p.limit}`, "bad");
        }
        break;
      case "generation_discarded":
        chip(seq, `gen ${p.gen_step} streamed output discarded; retrying`, "warn");
        break;
      case "malformed_tool_call":
        chip(seq, `gen ${p.gen_step} tool call malformed; retrying`, "warn");
        break;
      default:
        push({ kind: "sys", key: "s" + seq, text: `#${seq} ${env.type}` });
    }
  }

  const toolRunning = items.some((it) => it.kind === "tool" && it.status === "running");
  const active = toolRunning || lastType === "generation_started";

  return { items, approvals, callArgs, status, lastGen, active, isDriver };
}

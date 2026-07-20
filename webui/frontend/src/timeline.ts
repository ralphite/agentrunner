import type { Envelope } from "./types";
import { friendlyStatus } from "./components/pill";

// fmtTok compacts a token count for feed chips: 199839 → "200k".
function fmtTok(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return Math.round(n / 1000) + "k";
  return (n / 1_000_000).toFixed(1) + "M";
}

// verdictLabel humanizes a driver iteration verdict object into a compact
// phrase instead of dumping raw JSON into a chip (R4-3): {pass,score,detail}
// → "passed · score 1 · exit=0".
export function verdictLabel(v: any): string {
  if (v == null || typeof v !== "object") return String(v ?? "");
  const bits: string[] = [];
  if (typeof v.pass === "boolean") bits.push(v.pass ? "passed" : "failed");
  if (v.score !== undefined && v.score !== null) bits.push(`score ${v.score}`);
  if (v.detail) bits.push(String(v.detail));
  return bits.length ? bits.join(" · ") : "";
}

// guiReason rewrites the backend's CLI-oriented non-interactive auto-deny
// reason (which tells the user to run `agentrunner new` / set env vars) into
// wording that fits the web UI, where a Resume button is already on screen
// (R4-7). Other reasons pass through unchanged.
export function guiReason(reason: string): string {
  if (/non-interactive/i.test(reason) && /auto-?denied/i.test(reason)) {
    return "needs approval, but this run started non-interactively — press Resume to continue and approve here";
  }
  return reason;
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
  // RT-6: the CAS refs of the images attached to this input, kept — not just
  // counted. The blobs are durable (sessions/<sid>/artifacts/blobs/<ref>), so a
  // ref + the session id is all the view needs to render the REAL thumbnail
  // after a reload, instead of degrading to a "×N attached" stub.
  imageRefs?: string[];
  // The session these events came from (envelope correlation_id), needed to
  // address the blobs above. Carried on the item so the projection stays a pure
  // function of the journal — the view doesn't have to thread the id in.
  sessionId?: string;
  // journal seq of the input_received — lets the view attach locally-known
  // upload thumbnails to a sent bubble (the journal itself keeps only CAS refs)
  seq?: number;
  ts?: string;
  // A5: this user message was itself sent as a goal (its text matched the
  // goal_attached goal). The view notes "⚡ Sent as goal" under the bubble,
  // Codex-style, instead of a separate "goal attached" chip.
  sentAsGoal?: boolean;
}

export interface TurnItem {
  kind: "turn";
  key: string;
  gen: number;
  ts?: string; // generation_started time — the real work-start of the turn
}

export interface ChipItem {
  kind: "chip";
  key: string;
  text: string;
  tone: "" | "warn" | "bad" | "good";
  childSession?: string;
  // fold marks work-detail chips (approval audit, goal checks, compaction,
  // retries, subagent lifecycle) that belong inside the "Worked for" fold of
  // their turn. Outcome/user-action chips (goal achieved, mode changed,
  // session closed, …) stay unmarked and render outside the fold.
  fold?: boolean;
  // A3: render this chip as a Codex activity row (icon + label) rather than a
  // bubble chip — used for context compaction inside the fold.
  activity?: boolean;
  // TH-16 · run plumbing: which agent/model the session switched to, what goal
  // got attached. Not an answer, not a product, not an outcome — metadata about
  // HOW the next turn will run. Codex's thread reserves its top level for
  // replies and artifacts and keeps every scrap of plumbing inside the "Worked
  // for …" fold; ours floated four grey pills (3 × "Agent changed", 1 × "goal
  // attached") between the user and the conversation. A `system` chip is
  // therefore NEVER a top-level render node — foldWork routes it into the
  // adjacent activity fold, exactly as RT-4 routed approval audit chips into the
  // step list. It implies `fold` and is strictly stronger: a `fold` chip still
  // yields to the post-answer window (a goal check belongs beside the goal
  // outcome it explains), a `system` chip never does.
  system?: boolean;
  // TH-12: this chip RESTATES a terminal fact that the session's own chrome
  // already says. "goal" = the goal banner (.gbar) says it; "limit" = the
  // terminal alert (.terminal-alert) says it. The chip is still produced — a
  // session with no such chrome (no goal banner, dismissed banner, a sub-agent
  // thread) must keep the fact — but a view that DOES render the chrome drops
  // it through suppressEchoedChips so one terminal fact is stated once.
  echo?: "goal" | "limit";
}

export interface CompactItem {
  kind: "compact";
  key: string;
  text: string;
}

export interface SysItem {
  kind: "sys";
  key: string;
  text: string;
}

// ---- provider failure, in human words (INC-41 RT-5) -------------------------
// A model-call failure used to be pasted at the user verbatim as a red chip:
//   "activity failed: provider_server: model returned an empty message
//    (truncated at token cap, no text or tool calls) [provider_server]"
// — the error taxonomy's internal class name, twice, plus a parenthetical only
// an implementer can parse, and no way out. Codex says what happened in one
// plain sentence, keeps the technical string one click away, and puts the
// action (Retry) next to it. explainFailure is that translation layer.
//
// The class vocabulary is internal/errs (errs.Class): provider_rate_limit |
// provider_server | provider_auth | provider_invalid | tool_failed | timeout |
// canceled | internal. An unknown class is NOT swallowed — it falls through to
// a generic title and its raw text still ships in `raw`, so we never lose
// information we failed to anticipate.
export interface FailureExplained {
  title: string; // one plain sentence: what happened
  hint?: string; // what the user can do about it
}

export function explainFailure(cls: string, message: string): FailureExplained {
  const msg = message || "";
  // The empty-reply-at-token-cap case is a distinct, common, and confusing
  // sub-case of provider_server: the model didn't error, it ran out of room.
  if (/empty message|token cap|truncat/i.test(msg)) {
    return {
      title: "The model returned an empty reply",
      hint: "It ran out of output room before writing anything. Retry the turn — a shorter, more focused message usually gets through.",
    };
  }
  // Network reachability hides inside `internal` (and sometimes provider_server)
  // rather than having its own class, so it is matched on the message.
  if (/network|connection refused|connection reset|dial |no such host|dns|unreachable|EOF|tls|socket/i.test(msg)) {
    return {
      title: "Couldn't reach the model provider",
      hint: "The network call didn't get through. Check your connection, then retry the turn.",
    };
  }
  switch (cls) {
    case "provider_rate_limit":
      return {
        title: "The model provider rate-limited this request",
        hint: "Too many requests in a short window. Wait a moment, then retry the turn.",
      };
    case "provider_server":
      return {
        title: "The model provider had a server error",
        hint: "This is usually temporary and not something you did. Retry the turn.",
      };
    case "provider_auth":
      return {
        title: "The model provider rejected our credentials",
        hint: "Check the API key in your .env (GEMINI_API_KEY / ANTHROPIC_API_KEY), restart the daemon, then retry.",
      };
    case "provider_invalid":
      return {
        title: "The model provider rejected the request",
        hint: "The request was malformed or too large for this model. Retrying may work; a shorter conversation usually does.",
      };
    case "timeout":
      return {
        title: "The model call timed out",
        hint: "The provider took too long to answer. Retry the turn.",
      };
    case "canceled":
      return { title: "The step was cancelled" };
    case "tool_failed":
      return {
        title: "A tool failed to run",
        hint: "See the technical details below; retry once the cause is addressed.",
      };
    default:
      return {
        title: "A step failed",
        hint: "Retry the turn. The technical details below say what the runtime reported.",
      };
  }
}

// A model-call failure that the timeline surfaces as an inline banner (when it
// stuck) or as a quiet folded activity note (when the runtime's own retry
// already recovered from it — the session moved on, so a red alarm would lie).
export interface FailureNotice {
  seq: number;
  cls: string; // errs.Class, verbatim
  title: string;
  hint?: string;
  raw: string; // the untouched technical string — never dropped
  attempt?: number;
  recovered: boolean; // a later attempt of the same activity completed
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

// RetriedItem (INC-84): the block a user's Retry superseded — the original
// message and its failed turn — collapsed into one expandable row. The
// journal stays append-only; only the PRESENTATION folds, so the thread
// reads "question → (failed attempt · retried) → answer" instead of the
// same question pasted twice around dead output.
export interface RetriedItem {
  kind: "retried";
  key: string;
  children: TimelineItem[];
}

export type TimelineItem = ToolItem | BubbleItem | TurnItem | ChipItem | CompactItem | SysItem | RuntimeItem | RetriedItem;

export function formatWorkDuration(ms: number): string {
  const seconds = Math.max(1, Math.floor(ms / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return `${minutes}m${rest ? ` ${rest}s` : ""}`;
}

// One row per completed human turn, attached to the final assistant answer in
// that segment. Earlier assistant/tool planning messages do not get duplicate
// duration rows. The active final segment stays unlabelled until it settles.
export function completedTurnDurations(items: TimelineItem[], active: boolean): Map<string, number> {
  const out = new Map<string, number>();
  let userStart: number | null = null; // user-message time (fallback start)
  let genStart: number | null = null; // first generation_started (preferred)
  let final: { key: string; at: number } | null = null;
  const flush = () => {
    // Measure from when work actually STARTS — the first generation_started
    // after the user message — so queue/idle/stranded gaps don't inflate
    // "Worked for …" (R4-6). Fall back to the user message when no
    // generation marker is present (e.g. a pure chat reply).
    const start = genStart ?? userStart;
    if (start !== null && final && final.at >= start) out.set(final.key, final.at - start);
    userStart = null;
    genStart = null;
    final = null;
  };
  for (const item of items) {
    // Any incoming input starts a fresh turn — a human message (user) OR an
    // injected one (runtime: goal continuation, agent mail, socket send). Both
    // reset the clock; otherwise a non-human-sourced input (e.g. source
    // "unix-socket") fails to break the turn and its long idle gap merges into
    // the previous turn's "Worked for …" (R4-6).
    if ((item.kind === "user" && !item.peerSession) || item.kind === "runtime") {
      flush();
      const at = item.ts ? new Date(item.ts).getTime() : NaN;
      userStart = Number.isFinite(at) ? at : null;
      genStart = null;
    } else if (item.kind === "turn" && genStart === null) {
      const at = item.ts ? new Date(item.ts).getTime() : NaN;
      if (Number.isFinite(at)) genStart = at;
    } else if (item.kind === "assistant" && (genStart ?? userStart) !== null && item.ts) {
      const at = new Date(item.ts).getTime();
      if (Number.isFinite(at)) final = { key: item.key, at };
    }
  }
  if (!active) flush();
  return out;
}

// WorkFold wraps one completed turn's work detail (tool activity, approval
// audit chips, intermediate planning messages) behind a Codex-style
// "Worked for N ⌄" disclosure. The final assistant answer stays outside.
export interface WorkFold {
  kind: "fold";
  key: string;
  durationMs?: number; // undefined when the turn never produced a final answer
  children: TimelineItem[];
}

export type RenderNode = TimelineItem | WorkFold;

// foldable: work detail that hides inside the turn fold. Everything else
// (user/final assistant bubbles, outcome chips like "goal achieved",
// mode/spec changes, session lifecycle) stays at top level.
function foldable(it: TimelineItem): boolean {
  if (it.kind === "tool" || it.kind === "runtime") return true;
  if (it.kind === "chip") return !!it.fold;
  return false;
}

// TH-16 · run plumbing (see ChipItem.system): never a top-level thread item.
function isSystemChip(it: TimelineItem): boolean {
  return it.kind === "chip" && !!it.system;
}

// foldWork regroups the flat item list into render nodes: for every completed
// human turn, the work between the user message and the final assistant
// answer collapses into a WorkFold carrying that turn's duration. The active
// (still running) tail stays flat so live progress remains visible. Pure —
// unit-tested against synthetic item lists.
export function foldWork(items: TimelineItem[], durations: Map<string, number>, active: boolean): RenderNode[] {
  // The active tail — the last human turn while it is still running — stays
  // flat so live progress is visible. It starts at the last user message that
  // has no settled final answer after it (durations only ever contains
  // settled turns).
  let tailStart = items.length;
  if (active) {
    let lastUser = -1;
    let settledAfter = false;
    items.forEach((it, i) => {
      if (it.kind === "user" && !it.peerSession) {
        lastUser = i;
        settledAfter = false;
      } else if (it.kind === "assistant" && durations.has(it.key)) {
        settledAfter = true;
      }
    });
    if (lastUser >= 0 && !settledAfter) tailStart = lastUser + 1;
  }

  // finalAhead[i]: within item i's turn segment, a settled final answer still
  // lies ahead. Planning narration folds only then — a child session (no
  // human turns at all) or an interrupted turn keeps its assistant text
  // visible instead of swallowing it into a fold.
  const finalAhead = new Array<boolean>(items.length);
  {
    let seen = false;
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      finalAhead[i] = seen;
      if (it.kind === "assistant" && durations.has(it.key)) seen = true;
      else if (it.kind === "user" && !it.peerSession) seen = false;
    }
  }

  const out: RenderNode[] = [];
  let buf: TimelineItem[] = [];
  let foldSeq = 0;
  // answered: we are in the post-answer window — a settled final answer was
  // just emitted and only audit has followed it (goal checks run AFTER the
  // reply). Those chips belong next to the outcome, at top level. Any real work
  // (a tool) or any outcome/lifecycle marker closes the window: whatever comes
  // after it is the next turn's work detail and folds.
  //
  // RT-4: the old rule folded a work chip only when a settled answer lay AHEAD
  // (finalAhead). A turn that never settles — stalled on an approval, approved
  // step by step, interrupted — therefore flushed the fold at every chip,
  // shattering one turn into a ladder of "Approved" chips and "Worked · 1 step ›"
  // rows (a 9-step turn spanned four screens). Note the window CANNOT be closed
  // by the user message alone: a turn is often started by an injected input
  // (goal continuation, agent mail) which is projected as a `runtime` item and
  // filtered out of the feed, so foldWork never sees a boundary for it — that
  // is why the reset also hangs off tools and outcome chips, the visible marks
  // that work has resumed. One turn ⇒ one fold, like Codex.
  let answered = false;
  // flush emits the buffered work as one fold. `force` is for the very last
  // flush, which must not lose anything.
  //
  // TH-16 · carry-forward: a buffer holding NOTHING but system chips is not a
  // turn's work — it is plumbing that landed between two turns (the agent was
  // switched, a goal was attached, while the session sat idle). Emitting it
  // would open a bare "Worked · 1 item ›" row between an answer and the next
  // question — trading a naked chip for a naked fold. So it is kept in the
  // buffer and rides into the NEXT turn's fold, which is the turn it actually
  // describes: the switch is why that turn ran on that agent. Only the final
  // flush (force) is allowed to open a fold of its own for them, so plumbing at
  // the tail of a journal is still never dropped.
  const flush = (durationMs?: number, force = false) => {
    if (buf.length === 0 && durationMs === undefined) return;
    if (durationMs === undefined && !force && buf.length > 0 && buf.every(isSystemChip)) return;
    out.push({ kind: "fold", key: "fold" + foldSeq++, durationMs, children: buf });
    buf = [];
  };
  items.forEach((it, i) => {
    if (i >= tailStart) {
      // live tail: everything renders flat — EXCEPT plumbing, which has no
      // business at the top level even while a turn is running (TH-16). It
      // buffers, and any real tail item that follows first flushes it out as its
      // own group, so order is preserved and nothing is lost.
      if (isSystemChip(it)) {
        buf.push(it);
        return;
      }
      flush(undefined, true);
      out.push(it);
      return;
    }
    if (it.kind === "user" && !it.peerSession) {
      flush(); // dangling work from an interrupted prior turn
      answered = false; // a new turn starts: nothing is "post-answer" yet
      out.push(it);
    } else if (it.kind === "assistant" && durations.has(it.key)) {
      // final answer of a completed turn: fold everything gathered since the
      // user message, labelled with the turn duration, then the answer itself.
      flush(durations.get(it.key));
      answered = true;
      out.push(it);
    } else if (it.kind === "assistant" && finalAhead[i]) {
      // planning narration inside a settled turn — part of the work detail.
      answered = false;
      buf.push(it);
    } else if (it.kind === "assistant") {
      // no final answer ahead (interrupted turn / child session): the text IS
      // the outcome — keep it visible.
      flush();
      answered = false;
      out.push(it);
    } else if (isSystemChip(it)) {
      // TH-16 · run plumbing. It buffers unconditionally — unlike a work chip it
      // does NOT yield to the post-answer window, because it is not audit of the
      // answer that just landed (a goal check explains the goal outcome next to
      // it; "Agent changed" explains nothing to the reader of a reply). It also
      // does not touch `answered`: it is not work, so it cannot mean the next
      // turn is under way, and a real work chip after it must still see the same
      // window it would have seen without it.
      buf.push(it);
    } else if (foldable(it) && (it.kind === "tool" || !answered)) {
      // Work detail. Tools always fold (interrupted-turn work still tucks
      // away) AND they mean the next turn is under way, so they close any
      // post-answer window; work chips fold for the whole span of a turn's
      // work — no longer only when the turn already reached its answer.
      if (it.kind === "tool") answered = false;
      buf.push(it);
    } else if (foldable(it)) {
      // a work chip inside the post-answer window: audit sitting next to the
      // outcome (goal check → goal achieved). It stays visible and leaves the
      // window armed, so a run of them doesn't half-fold.
      flush();
      out.push(it);
    } else {
      // outcome / lifecycle chip (goal achieved, mode changed, session closed):
      // top level, and it closes the post-answer window — a work chip after it
      // belongs to whatever runs next.
      flush();
      answered = false;
      out.push(it);
    }
  });
  // Interrupted turn that never answered — fold without duration. Forced, so
  // that trailing plumbing (TH-16 carry-forward) that never found a turn to ride
  // into still lands: it opens a fold of its own rather than being dropped or
  // left bare at the top level.
  flush(undefined, true);
  return out;
}

// FoldRun: one render run inside an open fold — a single aggregated activity
// row. `members` keeps journal order (the tool activities plus the prose and
// audit interleaved among them); `tools` is the tool subset that decides the
// aggregate label, icon and count.
export interface FoldRun {
  key: string;
  members: TimelineItem[];
  tools: ToolItem[];
}

// foldRuns splits an open fold's children into render runs — the activity rows
// of Codex's expanded "Worked for …".
//
// Neither an audit chip NOR planning narration breaks a run. A chip never did
// (RT-4: Codex aggregates a turn's whole step list, "Ran commands ×3", and
// shows the approvals inside it). Narration used to — "it is prose, not a step"
// — and that was FOLD-RUN: Gemini narrates between essentially every tool call,
// so a 39-step turn was cut into 33 single-tool runs, every one of them too
// short to aggregate. The fold degraded into 33 full-width bare step rows with
// 33 raw thinking blocks poured between them: 6585px, 9.7 screens, the fold
// folding nothing. Prose is not a step — but it is not a SEPARATOR of steps
// either. It is what the model said WHILE doing this work, so it rides inside
// the activity row it belongs to (in journal order, one click away), exactly
// like an approval chip.
//
// What DOES break a run:
//   • the agent narrating and THEN switching to a different kind of work (ran
//     commands → read files): it stopped, thought, and turned to something else,
//     which is exactly the beat Codex gives its own row. This is what turns the
//     39-step fold into ten-odd skimmable rows instead of one opaque "×39".
//     A category switch with NO narration between the two steps is one batch of
//     work the model dispatched in a single breath — it stays one row, and
//     groupLabel names all of it ("Edited files, read files, ran commands").
//   • a runtime injection or a sys note: an input/boundary, not work.
// The turn's own boundaries (user message, final answer, turn end) never reach
// here — foldWork already cut the thread there, so one fold is one turn's work
// and no run can ever span two of them.
//
// Prose and audit attach FORWARD, to the step they set up ("now let me read
// X" → read X; "Approved · bash" → bash) — so they wait in `pending` until the
// next tool claims them. With no step ahead to claim them (end of the fold, or
// a boundary) they stay with the work they followed. Pure — unit tested.
export function foldRuns(children: TimelineItem[]): FoldRun[] {
  const runs: FoldRun[] = [];
  let cur: FoldRun | null = null;
  let curCat: ActivityCategory | null = null; // category of the run's LAST step
  let narrated = false; // the agent has spoken since the run's last step
  let pending: TimelineItem[] = []; // prose/audit awaiting the step it introduces

  const closeRun = () => {
    if (cur) runs.push(cur);
    cur = null;
    curCat = null;
    narrated = false;
  };
  // Trailing prose with no step ahead of it: it stays with the run it followed,
  // rather than being stranded as a bare block at the top of the fold.
  const settlePending = () => {
    if (cur && pending.length) {
      cur.members.push(...pending);
      pending = [];
    }
  };
  // Prose with no run at all to hold it (a fold that is pure narration, an
  // interrupted turn): it is the only thing there is — render it as its own
  // tool-less run so it stays readable.
  const spillPending = () => {
    if (!pending.length) return;
    runs.push({ key: pending[0].key, members: pending, tools: [] });
    pending = [];
  };

  for (const it of children) {
    if (it.kind === "tool") {
      const cat = toolCategory(it.name);
      // thought about it, then turned to a different kind of work → its own row
      if (cur && narrated && curCat !== cat) closeRun();
      if (!cur) cur = { key: (pending[0] ?? it).key, members: [], tools: [] };
      cur.members.push(...pending, it);
      pending = [];
      cur.tools.push(it);
      curCat = cat;
      narrated = false;
    } else if (it.kind === "chip" || it.kind === "assistant") {
      pending.push(it); // rides inside the run — never cuts it
      if (it.kind === "assistant") narrated = true;
    } else {
      // runtime injection / sys note: a boundary, and its own row.
      settlePending();
      closeRun();
      spillPending();
      runs.push({ key: it.key, members: [it], tools: [] });
    }
  }
  settlePending();
  closeRun();
  spillPending();
  return runs;
}

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
  // Its journal is driver_* / iteration_* events (legacy) or series_* events
  // (merged-stream, INC-80) and it does NOT accept input, so the UI renders
  // those events and hides the composer.
  isDriver: boolean;
  // bestIter = a FINISHED best-of-N round's winning attempt (series_ended
  // best_iter). Non-zero enables the "Apply winner" action (PLAN 5.8).
  bestIter?: number;
  // RT-5 · The most recent model-call failure that the runtime did NOT recover
  // from (no later attempt of that activity completed). This is the one the
  // view raises as an inline banner with a Retry action. Undefined when the
  // session never failed, or when every failure was retried away — in that
  // case the failure lives on only as a quiet note inside the turn's fold.
  failure?: FailureNotice;
}

// Input sources that mean "a human typed this" — regardless of entry point
// (interactive tty, cli send, or a UI that shells out to the cli). All render
// as "you"; only program/control sources (tool/parent/control/…) get a label.
const HUMAN_SOURCES = new Set(["user", "cli", "tty"]);

// imageRefs reads the CAS refs out of an input_received's images[] (RT-6). The
// journal shape is [{ref, media_type}]; a bare string ref is tolerated so an
// older journal still renders. Anything that isn't a plausible ref is dropped —
// the view would only turn it into a broken image.
export function imageRefs(images: unknown): string[] {
  if (!Array.isArray(images)) return [];
  return images
    .map((im) => (typeof im === "string" ? im : (im as { ref?: unknown } | null)?.ref))
    .filter((r): r is string => typeof r === "string" && /^sha256-[0-9a-f]+$/.test(r));
}

// correlationSession: which session's blob store holds this event's refs. The
// envelope's correlation_id IS the session id for a session's own journal (a
// sub-agent's is "<root>-sub-<child>", and sub-agents never carry user
// attachments), so it is the honest handle — no id has to be threaded from the
// view into this pure projection.
function correlationSession(env: Envelope): string | undefined {
  const id = (env as Envelope & { correlation_id?: unknown }).correlation_id;
  return typeof id === "string" && id ? id : undefined;
}

// TH-12 · goal text inside a chip is a label, not the goal itself. Anything
// longer than this is clipped — the goal banner (and the user's own bubble)
// carry the full sentence, and an un-clipped chip stretched into a ~500px pill.
const GOAL_CHIP_CHARS = 32;

export function clipGoal(goal: string): string {
  const g = (goal || "").trim().replace(/\s+/g, " ");
  return g.length > GOAL_CHIP_CHARS ? g.slice(0, GOAL_CHIP_CHARS - 1).trimEnd() + "…" : g;
}

// TH-12 · which terminal chrome the VIEW is actually rendering right now.
export interface TerminalChrome {
  goalBanner: boolean; // .gbar — the goal's lifecycle state (paused / stopped / cancelled)
  terminalAlert: boolean; // .terminal-alert — the abnormal-terminal notice (limit / crash / …)
}

// suppressEchoedChips drops the in-thread chips whose terminal fact the chrome
// above the composer already states — Codex says a terminal fact ONCE. It is
// deliberately CONDITIONAL: with no chrome on screen (no goal, banner
// dismissed, sub-agent thread, driver run) every chip survives, so the journal
// stays fully readable from the thread alone. QA-45's rule holds either way —
// an abnormal end is always visible; it's just not visible five times.
export function suppressEchoedChips(items: TimelineItem[], chrome: TerminalChrome): TimelineItem[] {
  if (!chrome.goalBanner && !chrome.terminalAlert) return items;
  return items.filter((it) => {
    if (it.kind !== "chip" || !it.echo) return true;
    return it.echo === "goal" ? !chrome.goalBanner : !chrome.terminalAlert;
  });
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
  let isDriver = false;
  let bestIter = 0;

  // RT-5 · model-call (non-tool) failures, by activity_id: the chip we pushed
  // for each one is kept live so a later successful attempt of the SAME
  // activity can downgrade it from "this broke" to "we hiccuped and retried".
  const llmFailures = new Map<string, { notice: FailureNotice; chip: ChipItem }>();
  // INC-84 · retry lineage: every human input item by its command id, so a
  // later "retry:<id>" input can fold the superseded block in place.
  const userItemByCommand = new Map<string, BubbleItem>();

  const push = (it: TimelineItem) => items.push(it);
  const chip = (
    seq: number,
    text: string,
    tone: ChipItem["tone"] = "",
    childSession?: string,
  ) => push({ kind: "chip", key: "c" + seq, text, tone, childSession });
  // work-detail chip: same, but marked to fold into its turn's Worked section.
  const workChip = (
    seq: number,
    text: string,
    tone: ChipItem["tone"] = "",
    childSession?: string,
  ) => push({ kind: "chip", key: "c" + seq, text, tone, childSession, fold: true });
  // TH-16 · run plumbing (agent switched, goal attached): folds like work detail
  // AND never surfaces at the top level, whatever the fold window is doing.
  const sysChip = (seq: number, text: string, tone: ChipItem["tone"] = "") =>
    push({ kind: "chip", key: "c" + seq, text, tone, fold: true, system: true });
  // TH-12 · a chip whose fact the terminal chrome also states (see ChipItem.echo).
  const echoChip = (
    seq: number,
    text: string,
    tone: ChipItem["tone"],
    echo: NonNullable<ChipItem["echo"]>,
  ) => push({ kind: "chip", key: "c" + seq, text, tone, echo });

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
        const refs = imageRefs(p.images);
        // INC-84 · a Retry supersedes its original attempt: fold everything
        // from the original message onward into one "Failed attempt" row,
        // then render the retried message normally in its place. Chains
        // flatten (retrying a retry swallows the earlier fold), and every
        // failure the fold buries counts as addressed — the user visibly
        // retried past it, so no standing banner.
        const cmd = env.command_id || "";
        if (!peer && cmd.startsWith("retry:")) {
          const orig = userItemByCommand.get(cmd.slice("retry:".length));
          let idx = orig ? items.indexOf(orig) : -1;
          // A chain: the fold from the PREVIOUS retry sits right before the
          // message being retried — absorb it so the thread keeps one fold.
          if (idx > 0 && items[idx - 1].kind === "retried") idx--;
          if (idx >= 0) {
            let superseded = items.splice(idx);
            if (superseded[0] && superseded[0].kind === "retried") {
              superseded = [...(superseded[0] as RetriedItem).children, ...superseded.slice(1)];
            }
            items.push({ kind: "retried", key: "rt" + seq, children: superseded });
            for (const f of llmFailures.values()) {
              if (f.notice.seq < seq) f.notice.recovered = true;
            }
          }
        }
        const userItem: BubbleItem = {
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
          imageRefs: refs.length ? refs : undefined,
          sessionId: refs.length ? correlationSession(env) : undefined,
        };
        push(userItem);
        if (!peer && cmd) userItemByCommand.set(cmd, userItem);
        break;
      }
      case "generation_started":
        lastGen = p.gen_step || lastGen + 1;
        push({ kind: "turn", key: "t" + seq, gen: lastGen, ts: env.ts });
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
            statusText: p.background ? "background work" : "running",
          };
          toolByActivity.set(p.activity_id, t);
          push(t);
        } else {
          push({ kind: "sys", key: "s" + seq, text: `#${seq} ${env.type} ${p.name || ""}` });
        }
        break;
      case "activity_completed": {
        // RT-5 · The runtime's own retry of a failed model call landed. The
        // failure is history, not a standing problem: no banner, and its note
        // reads as a recovered hiccup.
        const failed = llmFailures.get(p.activity_id);
        if (failed) failed.notice.recovered = true;
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
          // RT-5 · A failed MODEL call (activity kind=llm — it has no tool card
          // to hang off). Never paste the raw taxonomy string at the user: say
          // it in one sentence here, keep the raw text on the notice so the
          // banner's "technical details" fold can show it verbatim.
          const cls = String(p.error?.class || "");
          const ex = explainFailure(cls, String(p.error?.message || ""));
          const notice: FailureNotice = {
            seq,
            cls,
            title: ex.title,
            hint: ex.hint,
            raw: msg,
            attempt: p.attempt,
            recovered: false,
          };
          const it: ChipItem = { kind: "chip", key: "c" + seq, text: ex.title, tone: "bad", fold: true };
          push(it);
          llmFailures.set(p.activity_id, { notice, chip: it });
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
          workChip(seq, "Wake-up cancelled", "warn");
        }
        break;
      }
      case "spawn_requested":
        workChip(
          seq,
          `Subagent started · ${p.agent} · ${p.session ? p.session.slice(0, 80) : ""}`,
          "",
          p.child_session,
        );
        break;
      case "subagent_completed": {
        // The reason is an internal enum (max_generation_steps, …) — render
        // the human wording and a compact token count (W6).
        const reason = friendlyStatus(p.reason || "").text;
        const tok = p.usage ? p.usage.input_tokens + p.usage.output_tokens : 0;
        workChip(
          seq,
          `Subagent finished · ${p.agent} · ${reason}${tok ? ` · ${fmtTok(tok)} tokens` : ""}`,
          p.reason === "completed" ? "good" : "warn",
          p.child_session,
        );
        break;
      }
      case "child_revived":
        workChip(
          seq,
          `Member resumed · ${p.agent || ""} · woken by mail`,
          "",
          p.child_session,
        );
        break;
      case "command_handled":
        if (p.result && String(p.result).startsWith("forwarded:")) {
          workChip(seq, `Forwarded to ${String(p.result).slice("forwarded:".length)}`, "", String(p.result).slice("forwarded:".length));
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
          `Iteration ${p.iter} · ${friendlyStatus(p.child_reason || "completed").text}${
            p.verdict ? " · " + verdictLabel(p.verdict) : ""
          }`,
          p.verdict && p.verdict.pass === false ? "warn" : "good",
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
          `Scheduled run finished · ${friendlyStatus(p.reason || "done").text} · ${p.iterations || 0} iteration${
            (p.iterations || 0) === 1 ? "" : "s"
          }${p.best_iter ? " · best #" + p.best_iter : ""}`,
          p.reason === "satisfied" ? "good" : "warn",
        );
        status = { text: p.reason === "satisfied" ? "satisfied" : "done", cls: "closed" };
        break;
      // ---- merged-stream series events (INC-80 E1③) ----
      case "series_started":
        isDriver = true;
        chip(seq, `Scheduled series started · ${p.kind || ""}`);
        status = { text: "running", cls: "run" };
        break;
      case "series_iteration":
        isDriver = true;
        chip(
          seq,
          `Iteration ${p.n}${p.skipped ? " skipped" : ` · ${friendlyStatus(p.reason || "completed").text}`}${
            p.verdict && !p.skipped ? " · " + verdictLabel(p.verdict) : ""
          }`,
          p.skipped || (p.verdict && p.verdict.pass === false) ? "warn" : "good",
        );
        break;
      case "series_ended":
        isDriver = true;
        chip(
          seq,
          `Series finished · ${friendlyStatus(p.reason || "done").text} · ${p.iterations || 0} iteration${
            (p.iterations || 0) === 1 ? "" : "s"
          }${p.best_iter ? " · best #" + p.best_iter : ""}`,
          p.reason === "satisfied" ? "good" : "warn",
        );
        if (p.best_iter) bestIter = p.best_iter;
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
        // vanishes with no record). The backend's non-interactive auto-deny
        // reason is written for the CLI ("use `agentrunner new`…, set
        // AGENTRUNNER_APPROVE=…"); in the web UI that advice is wrong — the
        // user is already here with a Resume button — so rewrite it (R4-7).
        // Name the tool (MOB screenshot 2026-07-12): a bare "Approved" chip
        // between two subagent rows reads as noise — the reader can't tell
        // WHAT was approved without opening the journal.
        const toolTag = a && a.tool && !a.tool.startsWith("call_") ? " · " + a.tool : "";
        const auditText = `${p.decision === "approve" ? "Approved" : "Denied"}${toolTag}${p.reason ? " · " + guiReason(p.reason) : ""}`;
        // QA-0719 S1: an APPROVED call leaves a visible trace on its own — the
        // command runs and renders a shell/tool card in the main thread — so its
        // audit chip folds into the Worked group as quiet corroboration. A
        // DENIED call runs nothing, so folding its chip too left the denial with
        // NO visible trace in the thread: the user who blocked a command saw the
        // approval card vanish and the feed jump straight to the agent's next
        // message. Surface the denial inline (Codex shows denied tool calls in
        // place), so "I blocked that" is a first-class, unfolded beat.
        if (p.decision === "deny") chip(seq, auditText, "warn");
        else workChip(seq, auditText, "good");
        break;
      }
      case "waiting_entered": {
        const kinds: Record<string, [string, string]> = {
          input: ["waiting: input", "idle"],
          approval: ["waiting: approval", "appr"],
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
      case "actor_crashed":
        chip(seq, `crashed ${p.actor}: ${p.error}`, "bad");
        status = { text: "crashed", cls: "crash" };
        break;
      case "mode_changed":
        chip(seq, `Mode changed · ${p.to} (${p.cause})`);
        break;
      case "spec_changed":
        // TH-16: which agent/model the run switched to is plumbing for the turn
        // that follows, not a beat of the conversation — it rides inside that
        // turn's activity fold instead of interrupting the thread.
        sysChip(seq, `Agent changed · ${p.spec_name || "?"} · ${p.model || ""}`);
        break;
      case "context_compacted":
        // Compaction is a context boundary, not turn work. Render it as a
        // quiet thread divider so the reader sees where the conversation was
        // summarized without opening the "Worked" fold.
        push({ kind: "compact", key: "c" + seq, text: p.cleared ? "Context cleared" : "Context compacted" });
        break;
      // Goal lifecycle renders first-class (QA Round1 F-C5: these used to
      // fall into hidden sys lines, so a budget-stopped goal just vanished).
      case "goal_attached": {
        // A5: when this goal's text is the most recent user message, note it
        // under that bubble ("⚡ Sent as goal", Codex-style) instead of emitting
        // a separate "goal attached" chip. Fall back to the chip otherwise
        // (goal set via CLI with no matching UI message).
        const g = String(p.goal || "").trim();
        let noted = false;
        if (g) {
          for (let i = items.length - 1; i >= 0; i--) {
            const it = items[i];
            if (it.kind === "user" && !it.peerSession) {
              const t = (it.text || "").trim();
              if (t && (t === g || g.includes(t) || t.includes(g))) {
                it.sentAsGoal = true;
                noted = true;
              }
              break; // only the most recent user message is a candidate
            }
          }
        }
        // TH-12: the fallback chip is a LABEL, not a transcript — the whole
        // goal sentence blew it out to a 494px pill restating text the banner
        // and the user's own bubble already carry. Clip it; the banner holds
        // the full goal.
        // TH-16: and it is plumbing — the goal banner is the goal's first-class
        // home, so the chip belongs inside the fold of the turn it set up, not
        // wedged between the reader and the thread.
        if (!noted) sysChip(seq, `goal attached · ${clipGoal(g)}`);
        break;
      }
      case "goal_updated":
        // TH-16: same family as goal_attached — a restatement of what the goal
        // banner already shows. Plumbing, not a beat of the thread.
        sysChip(seq, "goal updated" + (p.goal ? ` · ${clipGoal(String(p.goal))}` : ""));
        break;
      case "goal_paused":
        echoChip(seq, "goal paused", "warn", "goal");
        break;
      case "goal_resumed":
        chip(seq, "goal resumed");
        break;
      case "goal_cancelled":
        echoChip(seq, "goal cancelled", "warn", "goal");
        break;
      case "goal_checkpoint":
        workChip(seq, `Goal check ${p.check || "?"}${p.pass ? " · passed" : " · not met"}`, p.pass ? "good" : "warn");
        break;
      case "goal_achieved":
        // reason=budget means the check budget ran out — a visible STOP,
        // not success; saying "achieved" here misled users (F-C5).
        if (p.reason === "budget") {
          // TH-12: the goal banner's terminal state says exactly this ("Goal
          // stopped · check budget exhausted" + the check count), so the chip
          // is an echo when the banner is on screen — but it stays the ONLY
          // carrier of the fact when it isn't (dismissed banner, sub-agent).
          echoChip(seq, `goal stopped: check budget exhausted after ${p.checks} check(s) — not verified as achieved`, "bad", "goal");
        } else if (p.reason === "cancelled") {
          echoChip(seq, "goal detached · cancelled", "warn", "goal");
        } else {
          chip(seq, `Goal achieved · ${p.reason || "satisfied"} (${p.checks} check${p.checks === 1 ? "" : "s"})`, "good");
        }
        break;
      case "goal_exhausted":
        echoChip(seq, `goal stopped: check budget exhausted after ${p.checks} check(s) — not verified as achieved`, "bad", "goal");
        break;
      case "limit_exceeded":
        // A user interrupt is modeled as limit_exceeded{kind:interrupted} —
        // don't dress it up as a budget overrun.
        if (p.kind === "interrupted" || p.kind === "canceled" || p.kind === "cancelled") {
          chip(seq, "Stopped — you interrupted this turn", "warn");
        } else {
          // Say what the limit MEANS (friendlyStatus maps tokens/steps/etc. to a
          // human label) instead of leaking `generation_steps: 1/1`, and don't
          // show a bare "0/limit" for a pre-flight block that read as
          // "barely used" while also saying "exceeded" (R4-4).
          const lbl = friendlyStatus(p.kind || "limit").text;
          // TH-12: .terminal-alert already announces the SAME cap ("Budget /
          // Step limit reached" + what to do next) with an action button. Two
          // reds for one stop; the chip yields to the actionable banner when
          // that banner is rendered.
          echoChip(seq, p.limit ? `${lbl} — capped at ${p.limit}` : lbl, "bad", "limit");
        }
        break;
      case "generation_discarded":
        workChip(seq, `gen ${p.gen_step} streamed output discarded; retrying`, "warn");
        break;
      case "malformed_tool_call":
        workChip(seq, `gen ${p.gen_step} tool call malformed; retrying`, "warn");
        break;
      default:
        push({ kind: "sys", key: "s" + seq, text: `#${seq} ${env.type}` });
    }
  }

  // Background tools (bash background:true) never emit activity_completed
  // until the process exits — a long-lived server would pin the session at
  // "Working…/Thinking" forever even after waiting_entered, so they don't
  // count toward a live turn.
  const toolRunning = items.some(
    (it) => it.kind === "tool" && it.status === "running" && !it.background,
  );
  const active = toolRunning || lastType === "generation_started";

  // RT-5 · Settle every model-call failure now that the whole journal is in.
  // Recovered ones become a calm, warn-toned fold note ("…, retried
  // automatically") — the run continued, so a red alarm would be a lie. The
  // last unrecovered one is what the view raises as an actionable banner.
  let failure: FailureNotice | undefined;
  for (const { notice, chip: c } of llmFailures.values()) {
    if (notice.recovered) {
      c.tone = "warn";
      c.text = notice.title + " · retried automatically";
      continue;
    }
    if (!failure || notice.seq > failure.seq) failure = notice;
  }

  return { items, approvals, callArgs, status, lastGen, active, isDriver, bestIter: bestIter || undefined, failure };
}

// ---- goal lifecycle projection (W6) -----------------------------------------
// The inspect projection drops a goal once it settles, so an achieved/cancelled
// goal simply vanishes from the banner. deriveGoalState folds the durable
// goal_* journal events instead, so the banner can persist a terminal state
// and compute elapsed from the goal_attached → goal_achieved/cancelled span.
export type GoalPhase = "active" | "paused" | "achieved" | "stopped" | "cancelled";

export interface GoalDerived {
  phase: GoalPhase;
  goal: string;
  checks: number; // checks recorded so far (running) or at settlement
  maxChecks?: number;
  verifiers?: number; // 0 / null → self-certified
  attachedAt?: number; // ms epoch of goal_attached
  endedAt?: number; // ms epoch of the terminal event (terminal phases only)
  // Total elapsed for a settled goal (endedAt − attachedAt). Undefined while
  // active — the banner ticks a live clock from attachedAt instead.
  elapsedMs?: number;
}

const GOAL_TERMINAL: Record<string, true> = { achieved: true, stopped: true, cancelled: true };

// A phase is terminal when the goal has settled (nothing more will happen).
export function isGoalTerminal(phase: GoalPhase): boolean {
  return GOAL_TERMINAL[phase] === true;
}

const asMs = (ts?: string): number | undefined => {
  if (!ts) return undefined;
  const t = new Date(ts).getTime();
  return Number.isFinite(t) ? t : undefined;
};

// deriveGoalState replays the goal_* events into a single current state. A
// later goal_attached fully supersedes an earlier (settled) goal. Returns null
// when the session never carried a goal.
export function deriveGoalState(events: Envelope[]): GoalDerived | null {
  let state: GoalDerived | null = null;
  for (const env of events) {
    const p = env.payload || {};
    switch (env.type) {
      case "goal_attached":
        state = {
          phase: "active",
          goal: p.goal || "",
          checks: 0,
          maxChecks: p.budget?.max_checks ?? p.max_checks,
          verifiers: p.verifiers ?? undefined,
          attachedAt: asMs(env.ts),
        };
        break;
      case "goal_updated":
        if (state) {
          if (p.goal) state.goal = p.goal;
          state.phase = "active";
          state.endedAt = undefined;
          state.elapsedMs = undefined;
          if (typeof p.budget?.max_checks === "number") state.maxChecks = p.budget.max_checks;
        }
        break;
      case "goal_paused":
        if (state && !isGoalTerminal(state.phase)) state.phase = "paused";
        break;
      case "goal_resumed":
        if (state && !isGoalTerminal(state.phase)) state.phase = "active";
        break;
      case "goal_checkpoint":
        // `check` is the running check ordinal; track the highest seen so an
        // active banner can show progress before settlement.
        if (state && typeof p.check === "number") state.checks = Math.max(state.checks, p.check);
        break;
      case "goal_cancelled":
        if (state) {
          state.phase = "cancelled";
          state.endedAt = asMs(env.ts);
        }
        break;
      case "goal_achieved":
        if (state) {
          // reason=budget is a visible STOP (budget ran out, not verified);
          // reason=cancelled is a detach; anything else is a real success.
          state.phase = p.reason === "budget" ? "stopped" : p.reason === "cancelled" ? "cancelled" : "achieved";
          if (typeof p.checks === "number") state.checks = p.checks;
          state.endedAt = asMs(env.ts);
        }
        break;
      case "goal_exhausted":
        if (state) {
          state.phase = "stopped";
          if (typeof p.checks === "number") state.checks = p.checks;
          state.endedAt = asMs(env.ts);
        }
        break;
    }
  }
  if (state && isGoalTerminal(state.phase) && state.attachedAt !== undefined && state.endedAt !== undefined) {
    state.elapsedMs = Math.max(0, state.endedAt - state.attachedAt);
  }
  return state;
}

// formatElapsed renders a goal's running/total time. Under an hour it's mm:ss
// (00:00 padded); an hour or more switches to "Xh Ym" (Codex's coarse form).
export function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  if (h >= 1) {
    const m = Math.floor((total % 3600) / 60);
    return `${h}h ${m}m`;
  }
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

// ---- INC-41 conv (A1/A2): activity categories + tool detail projection ------
// Pure, append-only. Timeline.tsx maps the category enum onto phosphor icons and
// renders the structured detail; keeping the extraction here makes it unit
// testable without React.

// parseMaybeJSON normalises a tool's args/result, which arrive either as an
// object (journal-native) or a JSON string (some providers), to a value.
export function parseMaybeJSON(raw: any): any {
  if (typeof raw !== "string") return raw;
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

// ActivityCategory buckets a tool onto one Codex-style activity icon. Mirrors
// groupLabel's categories so a fold's aggregate row and its icon agree.
export type ActivityCategory =
  | "bash"
  | "read"
  | "edit"
  | "search"
  | "web"
  | "spawn"
  | "message"
  | "ask"
  | "progress"
  | "other";

export function toolCategory(name: string): ActivityCategory {
  switch (name) {
    case "bash":
      return "bash";
    case "read_file":
    case "read_notes":
      return "read";
    case "grep":
    case "glob":
    case "keyword_search":
    case "semantic_search": // legacy journals (pre-rename)
      return "search";
    case "write_file":
    case "edit_file":
      return "edit";
    case "web_fetch":
      return "web";
    case "spawn_agent":
      return "spawn";
    case "send_message":
      return "message";
    case "ask_user":
      return "ask";
    case "progress_update":
    case "goal_status":
    case "goal_complete":
      return "progress";
    default:
      return "other";
  }
}

// groupIcon picks the icon for an aggregated activity row: the first tool's
// category, matching groupLabel's first-appearance ordering (A1).
export function groupIcon(tools: ToolItem[]): ActivityCategory {
  return tools.length ? toolCategory(tools[0].name) : "other";
}

// ---- RT-3: a step line NEVER shows an internal identifier --------------------
//
// A step row reads "<verb> <body>": "$ ls -la", "read notes.txt". The old
// default branch used the raw tool NAME as the verb, so any tool without an
// explicit case surfaced its wire identifier at the user — an expanded fold read
// "✓ goal_status", "✓ progress_update". Those are protocol names; Codex never
// shows one. Every tool the runtime ships (internal/tool/defs) now has a human
// verb here, and an unknown one (a skill-provided or future tool) degrades to a
// neutral "Ran a tool" — a vague truth beats a leaked identifier.
export interface StepLabel {
  verb: string;
  body: string;
  mono: boolean;
}

export function toolLabel(name: string, args: unknown): StepLabel {
  const a: Record<string, any> = (parseMaybeJSON(args) as Record<string, any>) || {};
  const str = (...keys: string[]): string => {
    for (const k of keys) {
      const v = a[k];
      if (typeof v === "string" && v) return v;
      if (typeof v === "number") return String(v);
    }
    return "";
  };
  switch (name) {
    case "bash":
      return { verb: "$", body: str("command"), mono: true };
    case "read_file":
      return { verb: "read", body: str("path", "file"), mono: true };
    case "write_file":
      return { verb: "write", body: str("path", "file"), mono: true };
    case "edit_file":
      return { verb: "edit", body: str("path", "file"), mono: true };
    case "grep":
      return { verb: "search", body: str("pattern"), mono: true };
    case "glob":
      return { verb: "find files", body: str("pattern"), mono: true };
    case "keyword_search":
    case "semantic_search": // legacy journals (pre-rename)
      return { verb: "search", body: str("query"), mono: false };
    case "web_fetch":
      return { verb: "fetch", body: str("url"), mono: true };
    case "spawn_agent":
      return { verb: "spawn sub-agent", body: str("agent") || a.role?.name || str("prompt"), mono: false };
    case "handoff_agent":
      return { verb: "hand off to", body: str("agent"), mono: false };
    case "send_message":
      return { verb: "message", body: `→ ${str("to") || "?"} · ${str("text")}`, mono: false };
    case "ask_user":
      return { verb: "ask you", body: str("question"), mono: false };
    case "progress_update":
      return { verb: "update progress", body: "", mono: false };
    case "goal_status":
      return { verb: "check goal progress", body: "", mono: false };
    case "goal_complete":
      return { verb: "mark goal complete", body: str("summary"), mono: false };
    case "exit_plan_mode":
      return { verb: "finish planning", body: "", mono: false };
    case "publish_artifact":
      return { verb: "publish", body: str("stream"), mono: true };
    case "publish_note":
      return { verb: "note", body: str("topic"), mono: false };
    case "read_notes":
      return { verb: "read notes", body: str("topic"), mono: false };
    case "artifacts_list":
      return { verb: "list results", body: "", mono: false };
    case "artifacts_read":
      return { verb: "read result", body: str("stream"), mono: true };
    case "skill":
      return { verb: "run skill", body: str("name"), mono: false };
    case "schedule_next":
      return { verb: "schedule next run", body: str("after"), mono: false };
    case "finish_series":
      return { verb: "finish series", body: str("reason"), mono: false };
    case "output":
      return { verb: "read background output", body: str("handle"), mono: true };
    case "kill":
      return { verb: "stop background work", body: str("handle"), mono: true };
    default:
      return { verb: "Ran a tool", body: "", mono: false };
  }
}

// ---- A2 tool-specific detail renderers (structured extraction) --------------

export interface DiffLine {
  kind: "ctx" | "add" | "del";
  text: string;
}

// lineDiff produces a minimal line-level diff between two text blocks for the
// edit_file mini diff. It trims the common prefix/suffix and marks the changed
// middle as del (old) then add (new) — enough to read a small edit at a glance
// without an LCS. Pure + unit tested.
export function lineDiff(oldText: string, newText: string): DiffLine[] {
  const o = oldText.split("\n");
  const n = newText.split("\n");
  let start = 0;
  while (start < o.length && start < n.length && o[start] === n[start]) start++;
  let endO = o.length;
  let endN = n.length;
  while (endO > start && endN > start && o[endO - 1] === n[endN - 1]) {
    endO--;
    endN--;
  }
  const rows: DiffLine[] = [];
  for (let i = 0; i < start; i++) rows.push({ kind: "ctx", text: o[i] });
  for (let i = start; i < endO; i++) rows.push({ kind: "del", text: o[i] });
  for (let i = start; i < endN; i++) rows.push({ kind: "add", text: n[i] });
  for (let i = endO; i < o.length; i++) rows.push({ kind: "ctx", text: o[i] });
  return rows;
}

const DIFF_ROW_CAP = 80;

export interface ReadDetail {
  path: string;
  range?: string;
  lineCount?: number;
  truncated?: boolean;
}

export function readDetail(args: any, result: any): ReadDetail {
  const a = parseMaybeJSON(args) || {};
  const r = parseMaybeJSON(result);
  const path = a.path || a.file || "";
  let range: string | undefined;
  if (a.line_range) range = Array.isArray(a.line_range) ? a.line_range.join("–") : String(a.line_range);
  else if (a.offset != null || a.limit != null)
    range = `from ${a.offset ?? 0}${a.limit != null ? `, ${a.limit} lines` : ""}`;
  const countLines = (s: string) => (s ? s.replace(/\n$/, "").split("\n").length : 0);
  let lineCount: number | undefined;
  let truncated: boolean | undefined;
  if (r && typeof r === "object") {
    if (typeof r.content === "string") lineCount = countLines(r.content);
    truncated = r.truncated;
  } else if (typeof r === "string") {
    lineCount = countLines(r);
  }
  return { path, range, lineCount, truncated };
}

export interface EditDetail {
  path: string;
  rows: DiffLine[];
  more: number;
  note?: string;
}

export function editDetail(name: string, args: any, result: any): EditDetail {
  const a = parseMaybeJSON(args) || {};
  const r = parseMaybeJSON(result);
  const path = a.path || a.file || "";
  let rows: DiffLine[];
  if (name === "write_file") {
    const content: string = String(a.content ?? a.text ?? "");
    rows = content.split("\n").map((t) => ({ kind: "add" as const, text: t }));
    if (rows.length && rows[rows.length - 1].text === "") rows.pop(); // trailing newline
  } else {
    rows = lineDiff(String(a.old ?? ""), String(a.new ?? ""));
  }
  let more = 0;
  if (rows.length > DIFF_ROW_CAP) {
    more = rows.length - DIFF_ROW_CAP;
    rows = rows.slice(0, DIFF_ROW_CAP);
  }
  const note = r && typeof r === "object" ? r.output : typeof r === "string" ? r : undefined;
  return { path, rows, more, note };
}

export interface GrepHit {
  path: string;
  line?: number;
  text?: string;
}
export interface GrepDetail {
  pattern: string;
  path?: string;
  matchCount: number;
  fileCount: number;
  scanned?: number;
  truncated?: boolean;
  byFile: { path: string; hits: GrepHit[] }[];
}

export function grepDetail(args: any, result: any): GrepDetail {
  const a = parseMaybeJSON(args) || {};
  const r = parseMaybeJSON(result) || {};
  const matches: any[] = Array.isArray(r.matches) ? r.matches : [];
  const order: string[] = [];
  const byFileMap = new Map<string, GrepHit[]>();
  for (const m of matches) {
    const p = m.path || "";
    if (!byFileMap.has(p)) {
      byFileMap.set(p, []);
      order.push(p);
    }
    byFileMap.get(p)!.push({ path: p, line: m.line, text: typeof m.text === "string" ? m.text : "" });
  }
  return {
    pattern: a.pattern || a.query || "",
    path: a.path,
    matchCount: matches.length,
    fileCount: byFileMap.size,
    scanned: r.files_scanned,
    truncated: r.truncated,
    byFile: order.map((p) => ({ path: p, hits: byFileMap.get(p)! })),
  };
}

export interface GlobDetail {
  pattern: string;
  paths: string[];
  truncated?: boolean;
}
export function globDetail(args: any, result: any): GlobDetail {
  const a = parseMaybeJSON(args) || {};
  const r = parseMaybeJSON(result) || {};
  return { pattern: a.pattern || "", paths: Array.isArray(r.paths) ? r.paths : [], truncated: r.truncated };
}

export interface SemanticDetail {
  query: string;
  hits: { path: string; line?: number }[];
}
export function semanticDetail(args: any, result: any): SemanticDetail {
  const a = parseMaybeJSON(args) || {};
  const r = parseMaybeJSON(result) || {};
  const hits = Array.isArray(r.hits) ? r.hits.map((h: any) => ({ path: h.path || "", line: h.line })) : [];
  return { query: a.query || "", hits };
}

export interface SpawnDetail {
  agent: string;
  prompt: string;
  childSession?: string;
  reason?: string;
  report?: string;
}
export function spawnDetail(args: any, result: any): SpawnDetail {
  const a = parseMaybeJSON(args) || {};
  const r = parseMaybeJSON(result) || {};
  return {
    agent: a.agent || a.role?.name || r.agent || "",
    prompt: a.prompt || "",
    childSession: r.child_session,
    reason: r.reason,
    report: typeof r.report === "string" ? r.report : undefined,
  };
}

export interface WebDetail {
  url: string;
  title?: string;
  bytes?: number;
  untrusted: boolean;
}
export function webFetchDetail(args: any, result: any): WebDetail {
  const a = parseMaybeJSON(args) || {};
  const r = parseMaybeJSON(result) || {};
  const bytes =
    typeof r.bytes === "number" ? r.bytes : typeof r.content === "string" ? r.content.length : undefined;
  return { url: a.url || a.uri || "", title: r.title, bytes, untrusted: r.untrusted !== false };
}

export interface AskDetail {
  question: string;
}
export function askUserDetail(args: any): AskDetail {
  const a = parseMaybeJSON(args) || {};
  return { question: a.question || a.prompt || a.text || "" };
}

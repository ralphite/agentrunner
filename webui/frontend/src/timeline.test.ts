import { describe, expect, it } from "vitest";
import { clipGoal, completedTurnDurations, deriveGoalState, explainFailure, foldEvents, foldWork, formatElapsed, formatWorkDuration, guiReason, suppressEchoedChips, verdictLabel } from "./timeline";
import { isSessionNotFound, isValidSessionId } from "./components/SessionView";
import { summarizeChanges } from "./diffSummary";

describe("timeline input projection", () => {
  it("keeps human input as a user message", () => {
    const folded = foldEvents([{ seq: 1, type: "input_received", payload: { source: "cli", text: "hello" } }]);
    expect(folded.items).toContainEqual(expect.objectContaining({ kind: "user", text: "hello" }));
  });

  it("projects program and agent input as collapsible runtime events", () => {
    const folded = foldEvents([
      { seq: 1, type: "input_received", payload: { source: "program", text: "<goal>keep going</goal>" } },
      { seq: 2, type: "input_received", payload: { source: "agent", text: "review complete" } },
    ]);
    expect(folded.items).toEqual([
      expect.objectContaining({ kind: "runtime", source: "program", text: "<goal>keep going</goal>" }),
      expect.objectContaining({ kind: "runtime", source: "agent", text: "review complete" }),
    ]);
    expect(folded.items.some((item) => item.kind === "user")).toBe(false);
  });

  it("projects a user mode switch as a timeline chip (INC-42)", () => {
    const folded = foldEvents([{ seq: 1, type: "mode_changed", payload: { to: "acceptEdits", cause: "user" } }]);
    expect(folded.items).toContainEqual(expect.objectContaining({ kind: "chip", text: "Mode changed · acceptEdits (user)" }));
  });
});

describe("foldEvents active flag", () => {
  it("does not pin the session active on a never-completing background tool (thinking-forever bug)", () => {
    // Replays the 20260711-060645 journal shape: a background bash starts a
    // long-lived server (no activity_completed ever), the turn then finishes
    // and the session waits for input — the UI must NOT stay at Working…/Thinking.
    const folded = foldEvents([
      { seq: 1, type: "activity_started", payload: { activity_id: "tool-1", kind: "tool", name: "bash", args: {}, background: true } },
      { seq: 2, type: "generation_started", payload: { gen_step: 2 } },
      { seq: 3, type: "assistant_message", payload: { message: { parts: [{ text: "server is up" }] } } },
      { seq: 4, type: "waiting_entered", payload: { kind: "input" } },
    ]);
    expect(folded.active).toBe(false);
  });

  it("still counts a running foreground tool as active", () => {
    const folded = foldEvents([
      { seq: 1, type: "activity_started", payload: { activity_id: "tool-1", kind: "tool", name: "bash", args: {} } },
      { seq: 2, type: "waiting_entered", payload: { kind: "input" } },
    ]);
    expect(folded.active).toBe(true);
  });
});

describe("Codex-style turn outcome", () => {
  it("measures work from generation_started, not the user message (excludes queue/idle, R4-6)", () => {
    const items = foldEvents([
      { seq: 1, type: "input_received", ts: "2026-07-10T05:00:00Z", payload: { source: "user", text: "do it" } },
      // 90 minutes of queue/idle before work actually starts — must NOT count
      { seq: 2, type: "generation_started", ts: "2026-07-10T06:30:00Z", payload: { gen_step: 1 } },
      { seq: 3, type: "assistant_message", ts: "2026-07-10T06:30:03Z", payload: { message: { parts: [{ text: "checking" }] } } },
      { seq: 4, type: "assistant_message", ts: "2026-07-10T06:31:08Z", payload: { message: { parts: [{ text: "done" }] } } },
    ]).items;
    // 06:30:00 → 06:31:08 = 68s, NOT 91m08s from the user message
    expect([...completedTurnDurations(items, false)]).toEqual([["a4", 68000]]);
    expect(completedTurnDurations(items, true).size).toBe(0);
    expect(formatWorkDuration(68000)).toBe("1m 8s");
  });

  it("humanizes driver verdicts and rewrites CLI-only auto-deny reasons (R4-3/R4-7)", () => {
    expect(verdictLabel({ pass: true, score: 1, verifier: "command", detail: "exit=0" })).toBe("passed · score 1 · exit=0");
    expect(verdictLabel({ pass: false })).toBe("failed");
    expect(verdictLabel("plain")).toBe("plain");
    expect(guiReason("needs approval, but this run is non-interactive so it was auto-denied. Use `agentrunner new`…")).toMatch(/press Resume/);
    expect(guiReason("policy: path not allowed")).toBe("policy: path not allowed");
  });

  it("summarizes tracked and name-only untracked files without inventing line counts", () => {
    const summary = summarizeChanges({
      workspace: "/repo", known: true, isRepo: true, numstat: "", untracked: ["large.bin"],
      diff: "diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n@@ -1 +1,2 @@\n-old\n+new\n+line\n",
    });
    expect(summary).toMatchObject({ totalAdd: 2, totalDel: 1 });
    expect(summary.files).toEqual([
      expect.objectContaining({ path: "a.ts", add: 2, del: 1, countsKnown: true }),
      expect.objectContaining({ path: "large.bin", countsKnown: false }),
    ]);
  });
});

describe("foldWork (Codex-style Worked fold, W2/W3)", () => {
  const user = (key: string, ts: string) => ({ kind: "user" as const, key, text: "q", ts });
  const asst = (key: string, ts: string) => ({ kind: "assistant" as const, key, text: "a", ts });
  const tool = (key: string) => ({
    kind: "tool" as const, key, name: "bash", args: {}, background: false,
    status: "done" as const, statusText: "done",
  });
  const chip = (key: string, text: string, fold?: boolean) =>
    ({ kind: "chip" as const, key, text, tone: "" as const, fold });

  it("folds a settled turn's work behind one fold with the turn duration", () => {
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      tool("t1"),
      chip("c1", "Approved", true),
      tool("t2"),
      asst("a1", "2026-07-10T05:00:30Z"),
    ];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant"]);
    const fold = nodes[1] as any;
    expect(fold.durationMs).toBe(30000);
    expect(fold.children.map((c: any) => c.key)).toEqual(["t1", "c1", "t2"]);
  });

  it("keeps the active tail flat for live visibility", () => {
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      tool("t1"),
      asst("a1", "2026-07-10T05:00:30Z"),
      user("u2", "2026-07-10T05:01:00Z"),
      tool("t2"),
    ];
    const nodes = foldWork(items, completedTurnDurations(items, true), true);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "user", "tool"]);
  });

  it("keeps outcome chips outside and work chips inside the fold", () => {
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      chip("c1", "goal check 1 · pass", true),
      asst("a1", "2026-07-10T05:00:10Z"),
      chip("c2", "goal achieved · satisfied (1 check(s))"),
    ];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "chip"]);
    expect((nodes[1] as any).children.map((c: any) => c.key)).toEqual(["c1"]);
  });

  it("keeps post-answer work chips visible (goal checks run after the reply)", () => {
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      asst("a1", "2026-07-10T05:00:10Z"),
      chip("c1", "goal check 1 · pass", true),
      chip("c2", "goal achieved · satisfied (1 check(s))"),
    ];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "chip", "chip"]);
  });

  it("folds intermediate narration but keeps a final-less turn's text visible", () => {
    // settled turn: planning narration folds, final stays out
    const settled = [
      user("u1", "2026-07-10T05:00:00Z"),
      asst("a1", "2026-07-10T05:00:05Z"),
      tool("t1"),
      asst("a2", "2026-07-10T05:00:30Z"),
    ];
    const nodes = foldWork(settled, completedTurnDurations(settled, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant"]);
    expect((nodes[1] as any).children.map((c: any) => c.key)).toEqual(["a1", "t1"]);
    // child-session shape: no human turn at all → nothing folds away
    const child = [asst("a1", "2026-07-10T05:00:05Z"), asst("a2", "2026-07-10T05:00:30Z")];
    const childNodes = foldWork(child, completedTurnDurations(child, false), false);
    expect(childNodes.map((n: any) => n.kind)).toEqual(["assistant", "assistant"]);
  });

  it("folds an interrupted turn's work without a duration when settled", () => {
    const items = [user("u1", "2026-07-10T05:00:00Z"), tool("t1")];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold"]);
    expect((nodes[1] as any).durationMs).toBeUndefined();
  });

  it("emits an empty non-expandable fold for pure-chat turns (Worked row parity)", () => {
    const items = [user("u1", "2026-07-10T05:00:00Z"), asst("a1", "2026-07-10T05:00:02Z")];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant"]);
    expect((nodes[1] as any).children).toEqual([]);
  });

  it("keeps an unsettled approval-heavy turn as ONE fold (RT-4 ladder)", () => {
    // Shape of session 20260711-040811: every tool call needs an approval, and
    // the turn never reaches a final answer (it stalls on the next approval),
    // so no duration is ever recorded. Each chip used to flush the fold →
    // "Approved / Worked · 1 step / Approved / Worked · 1 step …" down 4 screens.
    const items = [
      user("u1", "2026-07-11T06:33:00Z"),
      chip("c1", "Approved", true),
      tool("t1"),
      chip("c2", "Approved", true),
      tool("t2"),
      chip("c3", "Approved", true),
      tool("t3"),
    ];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold"]);
    expect((nodes[1] as any).children.map((c: any) => c.key)).toEqual(["c1", "t1", "c2", "t2", "c3", "t3"]);
    expect((nodes[1] as any).durationMs).toBeUndefined();
  });

  it("folds mid-turn chips but leaves post-answer audit chips at top level (RT-4)", () => {
    // The two rules must coexist: chips BEFORE the answer are work detail even
    // when the answer hasn't landed yet; chips AFTER it are the outcome's audit.
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      chip("c1", "Approved", true),
      tool("t1"),
      asst("a1", "2026-07-10T05:00:30Z"),
      chip("c2", "Goal check 1 · passed", true),
      user("u2", "2026-07-10T05:01:00Z"),
      chip("c3", "Approved", true),
      tool("t2"),
    ];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "chip", "user", "fold"]);
    expect((nodes[1] as any).children.map((c: any) => c.key)).toEqual(["c1", "t1"]);
    expect((nodes[3] as any).key).toBe("c2"); // post-answer audit stays visible
    // the next (unsettled) turn re-arms: its chip folds again, no new ladder
    expect((nodes[5] as any).children.map((c: any) => c.key)).toEqual(["c3", "t2"]);
  });

  it("folds a turn started by an INVISIBLE injected input (RT-4, real 040811 shape)", () => {
    // The second turn of session 20260711-040811 is started by a goal
    // continuation: an input_received that projects to a `runtime` item and is
    // filtered out of the feed. foldWork therefore never sees a user boundary
    // for it — the turn is only recognisable from the work itself. Its
    // approvals must still fold, or the post-answer window from turn 1 stays
    // open forever and every "Approved" of the run lands at top level.
    const items = [
      user("u1", "2026-07-11T04:08:11Z"),
      asst("a1", "2026-07-11T04:08:12Z"), // settled: turn 1 answered
      chip("c0", "Mode changed · execute (user)"), // outcome chip → closes the window
      chip("c1", "Approved", true), // turn 2 begins here, invisibly
      tool("t1"),
      chip("c2", "Approved", true),
      tool("t2"),
    ];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "chip", "fold"]);
    expect((nodes[4] as any).children.map((c: any) => c.key)).toEqual(["c1", "t1", "c2", "t2"]);
  });

  it("re-arms the fold on the next turn's first tool when nothing else marks the boundary", () => {
    // Same invisible-boundary shape but with no outcome chip in between: the
    // tool is then the only visible sign that new work started.
    const items = [
      user("u1", "2026-07-11T04:08:11Z"),
      asst("a1", "2026-07-11T04:08:12Z"),
      chip("c1", "Goal check 1 · passed", true), // genuine post-answer audit
      tool("t1"), // next turn's work (injected input, not in the feed)
      chip("c2", "Approved", true),
      tool("t2"),
    ];
    const nodes = foldWork(items, completedTurnDurations(items, false), false);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "chip", "fold"]);
    expect((nodes[3] as any).key).toBe("c1");
    expect((nodes[4] as any).children.map((c: any) => c.key)).toEqual(["t1", "c2", "t2"]);
  });

  it("marks approval audit and goal check chips as fold-able in foldEvents, while compaction stays a divider", () => {
    const folded = foldEvents([
      { seq: 1, type: "approval_responded", payload: { approval_id: "x", decision: "approve" } },
      { seq: 2, type: "goal_checkpoint", payload: { check: 1, pass: true } },
      { seq: 3, type: "goal_achieved", payload: { reason: "satisfied", checks: 1 } },
      { seq: 4, type: "context_compacted", payload: { upto_gen_step: 3 } },
    ]);
    const byKey = new Map(folded.items.map((i) => [i.key, i]));
    expect((byKey.get("c1") as any).fold).toBe(true);
    expect((byKey.get("c2") as any).fold).toBe(true);
    expect((byKey.get("c3") as any).fold).toBeUndefined();
    expect(byKey.get("c4")?.kind).toBe("compact");
  });
});

// TH-16 · the thread's top level belongs to answers and products. Run plumbing
// — "Agent changed · auditor · gemini-flash-latest", "goal attached · …" — used
// to float there as bare grey pills: session 20260711-011831 opened with FOUR of
// them (~1118px of metadata) standing between the reader and the first message.
// Codex's task thread carries none: every non-reply activity is inside the
// "Worked for …" fold. These lock that in.
describe("TH-16 · system chips never render at the top level", () => {
  const user = (key: string, ts: string) => ({ kind: "user" as const, key, text: "q", ts });
  const asst = (key: string, ts: string) => ({ kind: "assistant" as const, key, text: "a", ts });
  const tool = (key: string) => ({
    kind: "tool" as const, key, name: "bash", args: {}, background: false,
    status: "done" as const, statusText: "done",
  });
  const sys = (key: string, text: string) =>
    ({ kind: "chip" as const, key, text, tone: "" as const, fold: true, system: true });
  const chip = (key: string, text: string, fold?: boolean) =>
    ({ kind: "chip" as const, key, text, tone: "" as const, fold });
  const run = (items: any[], active = false) => foldWork(items, completedTurnDurations(items, active), active);
  const topChips = (nodes: any[]) => nodes.filter((n) => n.kind === "chip");
  const foldChildKeys = (nodes: any[]) =>
    nodes.filter((n) => n.kind === "fold").flatMap((f: any) => f.children.map((c: any) => c.key));

  it("rides plumbing that lands BETWEEN turns into the next turn's fold (real 011831 shape)", () => {
    // Journal order of the live session: the agent is switched (and a goal
    // attached) while the session sits idle waiting for input — i.e. inside the
    // post-answer window, where a plain fold-chip is deliberately left visible.
    // Plumbing must not take that exit: it belongs to the turn it sets up.
    const items = [
      user("u1", "2026-07-11T01:18:31Z"),
      tool("t1"),
      asst("a1", "2026-07-11T01:18:41Z"), // turn 1 settles
      sys("c101", "Agent changed · auditor · gemini-flash-latest"),
      sys("c348", "Agent changed · dev · gemini-flash-latest ×2"), // ×2 already merged upstream
      sys("c364", "goal attached · TH-3 live check: verify the Sup…"),
      user("u2", "2026-07-11T01:20:00Z"), // the turn the switch was FOR
      tool("t2"),
      asst("a2", "2026-07-11T01:20:30Z"),
    ];
    const nodes = run(items);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "user", "fold", "assistant"]);
    expect(topChips(nodes)).toEqual([]); // 4 → 0, the whole point
    // …and every one of them is still there, in order, inside the next fold.
    expect((nodes[4] as any).children.map((c: any) => c.key)).toEqual(["c101", "c348", "c364", "t2"]);
    expect((nodes[4] as any).children[1].text).toContain("×2"); // aggregation survives
  });

  it("keeps a mid-work system chip in its journal position inside the fold", () => {
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      tool("t1"),
      sys("c2", "Agent changed · dev · gemini-flash-latest"),
      tool("t3"),
      asst("a1", "2026-07-10T05:00:30Z"),
    ];
    const nodes = run(items);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant"]);
    expect(topChips(nodes)).toEqual([]);
    expect((nodes[1] as any).children.map((c: any) => c.key)).toEqual(["t1", "c2", "t3"]);
  });

  it("opens a fold of its own for trailing plumbing with no turn to ride into", () => {
    // Fallback: nothing follows the switch — rather than leave it bare at the
    // top level (or drop it), it gets an activity group of its own.
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      asst("a1", "2026-07-10T05:00:10Z"),
      sys("c2", "Agent changed · auditor · gemini-flash-latest"),
    ];
    const nodes = run(items);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "fold"]);
    expect(topChips(nodes)).toEqual([]);
    expect(foldChildKeys(nodes)).toContain("c2"); // nothing is lost
  });

  it("holds plumbing out of the top level even in the live (flat) tail", () => {
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      asst("a1", "2026-07-10T05:00:10Z"),
      user("u2", "2026-07-10T05:01:00Z"), // active turn: tail renders flat…
      sys("c3", "Agent changed · dev · gemini-flash-latest"),
      tool("t1"),
    ];
    const nodes = run(items, true);
    expect(topChips(nodes)).toEqual([]); // …but not flat enough to expose plumbing
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "user", "fold", "tool"]);
    expect((nodes[4] as any).children.map((c: any) => c.key)).toEqual(["c3"]);
  });

  it("does not disturb the post-answer window a work chip still relies on (RT-4)", () => {
    // A goal check after the answer stays visible next to the outcome it
    // explains; a system chip sitting among them must not change that verdict.
    const items = [
      user("u1", "2026-07-10T05:00:00Z"),
      asst("a1", "2026-07-10T05:00:10Z"),
      sys("c1", "goal attached · ship it"),
      chip("c2", "Goal check 1 · passed", true),
      chip("c3", "Goal achieved · satisfied (1 check)"),
    ];
    const nodes = run(items);
    expect(nodes.map((n: any) => n.kind)).toEqual(["user", "fold", "assistant", "chip", "chip", "fold"]);
    expect((nodes[3] as any).key).toBe("c2"); // post-answer audit: still visible
    expect((nodes[4] as any).key).toBe("c3"); // outcome: still visible
    expect(foldChildKeys(nodes)).toContain("c1"); // plumbing: folded, not dropped
  });

  it("marks spec_changed / goal_attached / goal_updated as system chips in foldEvents", () => {
    const folded = foldEvents([
      { seq: 1, type: "spec_changed", payload: { spec_name: "auditor", model: "gemini-flash-latest" } },
      { seq: 2, type: "goal_attached", payload: { goal: "ship the parity round" } },
      { seq: 3, type: "goal_updated", payload: { goal: "ship it twice" } },
      { seq: 4, type: "goal_achieved", payload: { reason: "satisfied", checks: 1 } },
    ]);
    const byKey = new Map(folded.items.map((i) => [i.key, i]));
    for (const k of ["c1", "c2", "c3"]) {
      expect((byKey.get(k) as any).system).toBe(true);
      expect((byKey.get(k) as any).fold).toBe(true); // system implies work-detail
    }
    // the goal's OUTCOME is not plumbing — it stays a first-class thread beat
    expect((byKey.get("c4") as any).system).toBeUndefined();
    // and none of the plumbing survives to the top level of the render tree
    const nodes = foldWork(folded.items, new Map(), false);
    expect(nodes.filter((n: any) => n.kind === "chip").map((n: any) => n.key)).toEqual(["c4"]);
  });
});

describe("deriveGoalState (goal banner projection, W6)", () => {
  const attach = (ts: string, goal = "ship it") =>
    ({ seq: 1, type: "goal_attached", ts, payload: { goal, budget: { max_checks: 10 }, verifiers: null } });

  it("returns null when the session never carried a goal", () => {
    expect(deriveGoalState([{ seq: 1, type: "input_received", payload: { text: "hi" } }])).toBeNull();
  });

  it("projects an achieved goal (satisfied) with checks and total elapsed", () => {
    const g = deriveGoalState([
      attach("2026-07-10T06:21:02.261Z"),
      { seq: 54, type: "goal_checkpoint", ts: "2026-07-10T06:22:20.130Z", payload: { check: 1, pass: true } },
      { seq: 55, type: "goal_achieved", ts: "2026-07-10T06:22:20.261Z", payload: { reason: "satisfied", checks: 1 } },
    ]);
    expect(g).toMatchObject({ phase: "achieved", goal: "ship it", checks: 1, maxChecks: 10 });
    expect(g!.elapsedMs).toBe(78000);
  });

  it("marks a budget-exhausted goal as stopped, not achieved", () => {
    const g = deriveGoalState([
      attach("2026-07-10T06:00:00Z"),
      { seq: 9, type: "goal_achieved", ts: "2026-07-10T06:10:00Z", payload: { reason: "budget", checks: 10 } },
    ]);
    expect(g).toMatchObject({ phase: "stopped", checks: 10 });
    expect(g!.elapsedMs).toBe(600000);
  });

  it("treats an explicit cancel and a cancelled-detach as cancelled", () => {
    const viaEvent = deriveGoalState([
      attach("2026-07-10T06:00:00Z"),
      { seq: 5, type: "goal_cancelled", ts: "2026-07-10T06:00:30Z", payload: {} },
    ]);
    expect(viaEvent).toMatchObject({ phase: "cancelled" });
    expect(viaEvent!.elapsedMs).toBe(30000);

    const viaDetach = deriveGoalState([
      attach("2026-07-10T06:00:00Z"),
      { seq: 5, type: "goal_achieved", ts: "2026-07-10T06:00:30Z", payload: { reason: "cancelled" } },
    ]);
    expect(viaDetach).toMatchObject({ phase: "cancelled" });
  });

  it("keeps an unsettled goal active with attach time and no elapsed", () => {
    const g = deriveGoalState([attach("2026-07-10T06:00:00Z")]);
    expect(g).toMatchObject({ phase: "active", checks: 0 });
    expect(g!.attachedAt).toBe(Date.parse("2026-07-10T06:00:00Z"));
    expect(g!.elapsedMs).toBeUndefined();
  });

  it("reflects pause/resume and a later goal_updated text", () => {
    const paused = deriveGoalState([
      attach("2026-07-10T06:00:00Z"),
      { seq: 2, type: "goal_updated", payload: { goal: "ship it well" } },
      { seq: 3, type: "goal_paused", payload: {} },
    ]);
    expect(paused).toMatchObject({ phase: "paused", goal: "ship it well" });
    const resumed = deriveGoalState([
      attach("2026-07-10T06:00:00Z"),
      { seq: 3, type: "goal_paused", payload: {} },
      { seq: 4, type: "goal_resumed", payload: {} },
    ]);
    expect(resumed).toMatchObject({ phase: "active" });
  });

  it("lets a later goal_attached fully supersede a settled goal", () => {
    const g = deriveGoalState([
      attach("2026-07-10T06:00:00Z", "first"),
      { seq: 5, type: "goal_achieved", ts: "2026-07-10T06:01:00Z", payload: { reason: "satisfied", checks: 1 } },
      { seq: 9, type: "goal_attached", ts: "2026-07-10T07:00:00Z", payload: { goal: "second", budget: { max_checks: 3 } } },
    ]);
    expect(g).toMatchObject({ phase: "active", goal: "second", checks: 0, maxChecks: 3 });
    expect(g!.endedAt).toBeUndefined();
  });

  it("formats elapsed as mm:ss under an hour and Xh Ym beyond", () => {
    expect(formatElapsed(78000)).toBe("01:18");
    expect(formatElapsed(9000)).toBe("00:09");
    expect(formatElapsed(600000)).toBe("10:00");
    expect(formatElapsed(3_660_000)).toBe("1h 1m");
    expect(formatElapsed(7_200_000)).toBe("2h 0m");
  });
});

// ---- INC-41 RT-5 · provider failures in human words -------------------------
describe("model-call failure projection", () => {
  // The exact journal shape observed in session 20260711-073559-create-a-todo-app-ff36:
  // an llm activity failed at the token cap, the runtime retried it, attempt 2 landed.
  const failEvents = (extra: any[] = []) => [
    { seq: 115, type: "activity_started", payload: { activity_id: "llm-t9", kind: "llm", name: "complete", attempt: 1 } },
    {
      seq: 116,
      type: "activity_failed",
      payload: {
        activity_id: "llm-t9",
        attempt: 1,
        error: {
          class: "provider_server",
          message: "model returned an empty message (truncated at token cap, no text or tool calls) [provider_server]",
          retryable: true,
        },
      },
    },
    ...extra,
  ];

  it("never pastes the raw provider string into a chip", () => {
    const folded = foldEvents(failEvents() as any);
    for (const it of folded.items) {
      if (it.kind !== "chip") continue;
      expect(it.text).not.toMatch(/activity failed|provider_server|\[provider_server\]/);
    }
  });

  it("raises an unrecovered model failure as an actionable notice, raw text intact", () => {
    const folded = foldEvents(failEvents() as any);
    expect(folded.failure).toMatchObject({
      seq: 116,
      cls: "provider_server",
      title: "The model returned an empty reply",
      recovered: false,
    });
    expect(folded.failure!.hint).toMatch(/retry/i);
    // The technical string is preserved verbatim for the details fold.
    expect(folded.failure!.raw).toBe(
      "provider_server: model returned an empty message (truncated at token cap, no text or tool calls) [provider_server]",
    );
    expect(folded.items).toContainEqual(
      expect.objectContaining({ kind: "chip", text: "The model returned an empty reply", tone: "bad", fold: true }),
    );
  });

  it("downgrades a failure the runtime retried away to a quiet fold note (no banner)", () => {
    const folded = foldEvents(
      failEvents([
        { seq: 117, type: "activity_started", payload: { activity_id: "llm-t9", kind: "llm", name: "complete", attempt: 2 } },
        { seq: 118, type: "activity_completed", payload: { activity_id: "llm-t9", usage: { input_tokens: 4850, output_tokens: 3271 } } },
      ]) as any,
    );
    expect(folded.failure).toBeUndefined();
    expect(folded.items).toContainEqual(
      expect.objectContaining({
        kind: "chip",
        text: "The model returned an empty reply · retried automatically",
        tone: "warn",
        fold: true,
      }),
    );
  });

  it("leaves tool failures on their tool card (only model calls become notices)", () => {
    const folded = foldEvents([
      { seq: 1, type: "activity_started", payload: { activity_id: "a1", kind: "tool", name: "bash", args: {} } },
      { seq: 2, type: "activity_failed", payload: { activity_id: "a1", final: true, error: { class: "tool_failed", message: "exit 2" } } },
    ] as any);
    expect(folded.failure).toBeUndefined();
    expect(folded.items).toContainEqual(
      expect.objectContaining({ kind: "tool", status: "failed", errorMsg: "tool_failed: exit 2" }),
    );
  });

  it("explains each provider error class in plain language with a way out", () => {
    expect(explainFailure("provider_rate_limit", "429 too many requests")).toMatchObject({
      title: "The model provider rate-limited this request",
    });
    expect(explainFailure("provider_server", "500 internal")).toMatchObject({
      title: "The model provider had a server error",
    });
    expect(explainFailure("provider_auth", "401 unauthorized")).toMatchObject({
      title: "The model provider rejected our credentials",
    });
    expect(explainFailure("timeout", "activity timeout")).toMatchObject({ title: "The model call timed out" });
    expect(explainFailure("internal", "dial tcp: connection refused")).toMatchObject({
      title: "Couldn't reach the model provider",
    });
    // An unanticipated class still gets a banner + a hint — and never loses its text.
    const unknown = explainFailure("quantum_flux", "the flux capacitor destabilized");
    expect(unknown.title).toBe("A step failed");
    expect(unknown.hint).toMatch(/retry/i);
    // Every mapping offers an action, except the one that needs none.
    expect(explainFailure("canceled", "user interrupt").hint).toBeUndefined();
  });
});

// ---- INC-41 RT-7 · a broken deep link is Not found, not an empty session -----
describe("session id verdicts", () => {
  it("rejects ids the server's grammar cannot accept, without a request", () => {
    expect(isValidSessionId("20260711-073559-create-a-todo-app-ff36")).toBe(true);
    expect(isValidSessionId("20260711-1-sub-call_9_0-ab12")).toBe(true);
    expect(isValidSessionId("this-is-not-a-real-session")).toBe(true); // well-formed; the daemon's 404 decides
    expect(isValidSessionId("hello world")).toBe(false);
    expect(isValidSessionId("bad!id")).toBe(false);
    expect(isValidSessionId("a/b")).toBe(false);
    expect(isValidSessionId("")).toBe(false);
    expect(isValidSessionId("x".repeat(201))).toBe(false);
  });

  it("treats a permanent id verdict as not-found and everything else as transient", () => {
    // The daemon doesn't know this id.
    expect(isSessionNotFound({ status: 404, code: "session_not_found", message: 'no session matches "x"' })).toBe(true);
    // The server refused the id itself (api.go sid() → 400 "invalid session id").
    expect(isSessionNotFound(Object.assign(new Error("invalid session id"), { status: 400 }))).toBe(true);
    // A transient 400 from some other endpoint must NOT kill the poll.
    expect(isSessionNotFound(Object.assign(new Error("invalid scope"), { status: 400 }))).toBe(false);
    // Daemon down / restarting / network blip: keep polling.
    expect(isSessionNotFound(Object.assign(new Error("ar events: exit status 1"), { status: 502 }))).toBe(false);
    expect(isSessionNotFound(new TypeError("Failed to fetch"))).toBe(false);
  });
});

// TH-12 · one terminal fact, said once. The chips are always PRODUCED (the
// journal is the source of truth and a thread with no chrome must still carry
// the fact); the view drops the ones its own chrome is already saying.
describe("TH-12 · duplicate terminal chrome suppression", () => {
  const goalEvents = [
    { seq: 1, type: "goal_attached", payload: { goal: "ship the parity round" } },
    { seq: 2, type: "goal_paused", payload: {} },
    { seq: 3, type: "goal_cancelled", payload: {} },
  ];
  const limitEvents = [
    { seq: 1, type: "limit_exceeded", payload: { kind: "max_generation_steps", limit: 8 } },
  ];
  const chipText = (items: ReturnType<typeof foldEvents>["items"]) =>
    items.filter((it) => it.kind === "chip").map((it) => (it as { text: string }).text);

  it("marks the goal/limit chips that the chrome restates — and only those", () => {
    const items = foldEvents([...goalEvents, ...limitEvents]).items;
    const echoes = items.filter((it) => it.kind === "chip" && (it as { echo?: string }).echo);
    expect(echoes.map((it) => (it as { echo?: string }).echo)).toEqual(["goal", "goal", "limit"]);
    // the goal_attached fallback chip is NOT an echo — it names the goal, and
    // no chrome says "this is when the goal started".
    expect(items.some((it) => it.kind === "chip" && /goal attached/.test((it as { text: string }).text))).toBe(true);
  });

  it("with the goal banner on screen, drops the in-thread paused/cancelled echo", () => {
    const items = foldEvents(goalEvents).items;
    const shown = suppressEchoedChips(items, { goalBanner: true, terminalAlert: false });
    expect(chipText(shown)).not.toContain("goal paused");
    expect(chipText(shown)).not.toContain("goal cancelled");
    // the goal itself survives — suppression removes the ECHO, not the record.
    expect(chipText(shown).some((t) => t.startsWith("goal attached"))).toBe(true);
  });

  it("with NO goal banner (sub-agent thread, dismissed banner), keeps every chip", () => {
    const items = foldEvents(goalEvents).items;
    const shown = suppressEchoedChips(items, { goalBanner: false, terminalAlert: false });
    expect(chipText(shown)).toContain("goal paused");
    expect(chipText(shown)).toContain("goal cancelled");
    expect(shown).toEqual(items);
  });

  it("with the terminal alert on screen, drops the red limit chip — and keeps it otherwise", () => {
    const items = foldEvents(limitEvents).items;
    expect(chipText(suppressEchoedChips(items, { goalBanner: false, terminalAlert: true }))).toEqual([]);
    expect(chipText(suppressEchoedChips(items, { goalBanner: false, terminalAlert: false })))
      .toEqual([expect.stringContaining("capped at 8")]);
  });

  it("keeps each chrome's suppression independent (an alert must not eat goal chips)", () => {
    const items = foldEvents([...goalEvents, ...limitEvents]).items;
    const shown = chipText(suppressEchoedChips(items, { goalBanner: false, terminalAlert: true }));
    expect(shown).toContain("goal cancelled");
    expect(shown.some((t) => /capped at 8/.test(t))).toBe(false);
  });

  it("does not suppress an interrupt chip — 'you interrupted' is not the alert's fact", () => {
    const items = foldEvents([{ seq: 1, type: "limit_exceeded", payload: { kind: "interrupted" } }]).items;
    const shown = suppressEchoedChips(items, { goalBanner: true, terminalAlert: true });
    expect(chipText(shown)).toEqual(["Stopped — you interrupted this turn"]);
  });

  it("clips a whole-sentence goal out of the fallback chip (no more 494px pill)", () => {
    expect(clipGoal("short goal")).toBe("short goal");
    const long = "reply with exactly QA45_THINKING_PROBE and nothing else";
    expect(clipGoal(long).length).toBeLessThanOrEqual(32);
    expect(clipGoal(long).endsWith("…")).toBe(true);
    const chip = foldEvents([{ seq: 1, type: "goal_attached", payload: { goal: long } }]).items[0] as { text: string };
    expect(chip.text.length).toBeLessThanOrEqual("goal attached · ".length + 32);
  });
});

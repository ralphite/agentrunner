import { describe, expect, it } from "vitest";
import { completedTurnDurations, deriveGoalState, foldEvents, foldWork, formatElapsed, formatWorkDuration, guiReason, verdictLabel } from "./timeline";
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

  it("marks approval audit and goal check chips as fold-able in foldEvents", () => {
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
    expect((byKey.get("c4") as any).fold).toBe(true);
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

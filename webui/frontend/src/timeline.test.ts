import { describe, expect, it } from "vitest";
import { completedTurnDurations, foldEvents, foldWork, formatWorkDuration } from "./timeline";
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
  it("attaches one duration to the final assistant answer of each settled human turn", () => {
    const items = foldEvents([
      { seq: 1, type: "input_received", ts: "2026-07-10T05:00:00Z", payload: { source: "user", text: "do it" } },
      { seq: 2, type: "assistant_message", ts: "2026-07-10T05:00:03Z", payload: { message: { parts: [{ text: "checking" }] } } },
      { seq: 3, type: "assistant_message", ts: "2026-07-10T05:01:08Z", payload: { message: { parts: [{ text: "done" }] } } },
    ]).items;
    expect([...completedTurnDurations(items, false)]).toEqual([["a3", 68000]]);
    expect(completedTurnDurations(items, true).size).toBe(0);
    expect(formatWorkDuration(68000)).toBe("1m 8s");
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

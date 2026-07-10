import { describe, expect, it } from "vitest";
import { completedTurnDurations, foldEvents, formatWorkDuration } from "./timeline";
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

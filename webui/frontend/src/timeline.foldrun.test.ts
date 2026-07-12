import { describe, expect, it } from "vitest";
import { foldRuns, type ChipItem, type TimelineItem, type ToolItem } from "./timeline";

// ---- FOLD-RUN ---------------------------------------------------------------
// The old contract was "planning narration breaks the run" (prose is not a
// step). Against a real Gemini turn that rule destroyed the fold: the model
// narrates between essentially every tool call, so a 39-step turn was cut into
// 33 single-tool runs — none long enough to aggregate — and the expanded
// "Worked for 4m 20s" became 33 full-width bare step rows with 33 raw thinking
// blocks poured between them (6585px, 9.7 screens; the timeline viewport is
// 678px). NEW CONTRACT: narration, like an approval chip, rides INSIDE the run
// it belongs to; a run breaks on a change of activity CATEGORY (and on a
// runtime/sys boundary), never on prose.

let n = 0;
const tool = (name: string): ToolItem => ({
  kind: "tool",
  key: "act" + name + ++n,
  name,
  args: {},
  background: false,
  status: "done",
  statusText: "done",
});
const asst = (key: string, text = "Now let me look at the runtime."): TimelineItem => ({
  kind: "assistant",
  key,
  text,
});
const chip = (key: string): ChipItem => ({ kind: "chip", key, text: "Approved · bash", tone: "good", fold: true });
const runtime = (key: string): TimelineItem => ({ kind: "runtime", key, source: "program", text: "<goal>" });

describe("FOLD-RUN — planning narration no longer breaks a run of tools", () => {
  it("keeps tools separated by narration in ONE run, with the prose inside it", () => {
    // The old test asserted the opposite ([1, 0, 1] — three runs). This is the
    // behaviour change: one activity row, the narration one click inside it.
    const [a, b] = [tool("bash"), tool("bash")];
    const runs = foldRuns([a, asst("a1"), b]);
    expect(runs).toHaveLength(1);
    expect(runs[0].tools).toEqual([a, b]);
    expect(runs[0].members.map((m) => m.key)).toEqual([a.key, "a1", b.key]);
  });

  it("aggregates a real Gemini turn (tool, narrate, tool, narrate, …) into one row", () => {
    const children: TimelineItem[] = [];
    const tools: ToolItem[] = [];
    for (let i = 0; i < 20; i++) {
      const t = tool("bash");
      tools.push(t);
      children.push(t, asst("a" + i));
    }
    const runs = foldRuns(children);
    expect(runs).toHaveLength(1);
    expect(runs[0].tools).toHaveLength(20); // was: 20 runs of one tool each
    // every narration is accounted for INSIDE the run — none left loose in the
    // fold body, which is where the 6585px came from
    expect(runs[0].members.filter((m) => m.kind === "assistant")).toHaveLength(20);
  });

  it("still breaks the run when the agent narrates and THEN changes the kind of work", () => {
    // thought → turned to something else: that beat is what gives the fold its
    // handful of skimmable rows instead of one opaque "×39".
    const [b1, r1, b2] = [tool("bash"), tool("read_file"), tool("bash")];
    const runs = foldRuns([b1, asst("a1"), r1, asst("a2"), b2]);
    expect(runs.map((r) => r.tools.map((t) => t.name))).toEqual([["bash"], ["read_file"], ["bash"]]);
  });

  it("keeps an uninterrupted batch of mixed tools in ONE run (no narration between)", () => {
    // One breath of work — the model dispatched read+edit+bash without stopping
    // to think. That is a single activity row; groupLabel names all of it. (This
    // is why the small folds of a real session did not regress into one row per
    // tool when narration stopped breaking runs.)
    const [r1, e1, b1] = [tool("read_file"), tool("edit_file"), tool("bash")];
    const runs = foldRuns([r1, e1, b1]);
    expect(runs).toHaveLength(1);
    expect(runs[0].tools.map((t) => t.name)).toEqual(["read_file", "edit_file", "bash"]);
  });

  it("attaches narration FORWARD, to the step it sets up", () => {
    // "…now let me read the file" belongs to the read, not to the command before it.
    const [b1, r1] = [tool("bash"), tool("read_file")];
    const runs = foldRuns([b1, asst("plan"), r1]);
    expect(runs).toHaveLength(2);
    expect(runs[0].members.map((m) => m.key)).toEqual([b1.key]);
    expect(runs[1].members.map((m) => m.key)).toEqual(["plan", r1.key]);
  });

  it("keeps trailing narration with the run it followed (no step ahead to claim it)", () => {
    const t = tool("bash");
    const runs = foldRuns([t, asst("a1")]);
    expect(runs).toHaveLength(1);
    expect(runs[0].members.map((m) => m.key)).toEqual([t.key, "a1"]);
  });

  it("carries approval chips and narration together, in journal order", () => {
    const [t1, t2] = [tool("bash"), tool("bash")];
    const runs = foldRuns([chip("c1"), t1, asst("a1"), chip("c2"), t2]);
    expect(runs).toHaveLength(1);
    expect(runs[0].members.map((m) => m.key)).toEqual(["c1", t1.key, "a1", "c2", t2.key]);
    expect(runs[0].tools).toHaveLength(2);
  });

  it("keeps a tool-less fold's narration readable as its own run", () => {
    // An interrupted / pure-planning turn has no step to hide the prose inside;
    // it must still render, so it comes back as a tool-less run.
    const runs = foldRuns([asst("a1"), asst("a2")]);
    expect(runs).toHaveLength(1);
    expect(runs[0].tools).toEqual([]);
    expect(runs[0].members.map((m) => m.key)).toEqual(["a1", "a2"]);
  });

  it("breaks on a runtime injection — an input is a boundary, not narration", () => {
    const [t1, t2] = [tool("bash"), tool("bash")];
    const runs = foldRuns([t1, runtime("r1"), t2]);
    expect(runs.map((r) => r.members.map((m) => m.key))).toEqual([[t1.key], ["r1"], [t2.key]]);
  });

  it("never loses or reorders an item, whatever the mix", () => {
    const children: TimelineItem[] = [
      chip("c1"),
      tool("bash"),
      asst("a1"),
      tool("bash"),
      tool("read_file"),
      asst("a2"),
      runtime("r1"),
      asst("a3"),
      tool("grep"),
      chip("c2"),
      tool("glob"),
      asst("a4"),
    ];
    const flat = foldRuns(children).flatMap((r) => r.members);
    expect(flat.map((m) => m.key)).toEqual(children.map((c) => c.key));
  });

  it("gives every run a key (React list identity) even when prose opens it", () => {
    const [b1, r1] = [tool("bash"), tool("read_file")];
    const runs = foldRuns([asst("a1"), b1, asst("a2"), r1]);
    expect(runs.map((r) => r.key)).toEqual(["a1", "a2"]); // the run's first member
    expect(new Set(runs.map((r) => r.key)).size).toBe(runs.length);
  });
});

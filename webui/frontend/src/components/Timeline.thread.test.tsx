// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { TimelineView, mergeAdjacentChips } from "./Timeline";
import type { BubbleItem, ChipItem, CompactItem, TimelineItem } from "../timeline";

afterEach(cleanup);

const assistant = (key: string, text = "Done."): BubbleItem => ({
  kind: "assistant",
  key,
  text,
  ts: "2026-07-11T18:35:00Z",
});
const chip = (key: string, text: string, over: Partial<ChipItem> = {}): ChipItem => ({
  kind: "chip",
  key,
  text,
  tone: "",
  ...over,
});
const compact = (key: string, text = "Context compacted"): CompactItem => ({ kind: "compact", key, text });

describe("TH-10 — the final assistant answer's action row: icons visible at rest", () => {
  it("renders the row's members: three action buttons, the divider and the verdict — but no time", () => {
    const { container } = render(
      <TimelineView
        items={[assistant("a1")]}
        pending={[]}
        typing=""
        showSys={false}
        goalVerdict={{ elapsed: "3h 47m" }}
        onContinue={() => {}}
      />,
    );
    const div = container.querySelector(".msg-actions-div") as HTMLElement;
    expect(div).not.toBeNull();
    expect(div.getAttribute("style")).toBeNull();
    const row = container.querySelector(".msg-actions") as HTMLElement;
    expect(row.querySelector(".msg-goal-verdict")?.textContent).toContain("Goal achieved in 3h 47m");
    expect(container.querySelector(".msg.msg-last")).not.toBeNull();
    expect(row.querySelectorAll("button.msg-copy")).toHaveLength(3);
  });
});

describe("compact divider", () => {
  it("renders as a quiet thread divider, not a chip", () => {
    const { container } = render(
      <TimelineView items={[assistant("a1"), compact("c2")]} pending={[]} typing="" showSys={false} />,
    );
    const div = container.querySelector(".compact-divider") as HTMLElement;
    expect(div).not.toBeNull();
    expect(div.textContent).toContain("Context compacted");
    expect(container.querySelector(".chip")).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// TH-4 — adjacent identical chips aggregate instead of stuttering.
// ---------------------------------------------------------------------------
describe("TH-4 — mergeAdjacentChips", () => {
  const agent = (key: string) => chip(key, "Agent changed · dev · gemini-flash-latest");

  it("merges the duplicate 'Agent changed' chip the runtime emits twice in a row", () => {
    const merged = mergeAdjacentChips([agent("c1"), agent("c2")]);
    expect(merged).toHaveLength(1);
    expect((merged[0] as ChipItem).text).toBe("Agent changed · dev · gemini-flash-latest ×2");
    // the first chip's identity survives: keys, links and fold role are stable
    expect(merged[0].key).toBe("c1");
  });

  it("counts a longer run rather than merging it pairwise", () => {
    const merged = mergeAdjacentChips([agent("c1"), agent("c2"), agent("c3")]);
    expect(merged).toHaveLength(1);
    expect((merged[0] as ChipItem).text).toMatch(/×3$/);
  });

  it("keeps chips that differ in text, tone, link or fold role", () => {
    const items: TimelineItem[] = [
      agent("c1"),
      chip("c2", "Mode changed · acceptEdits (user)"),
      chip("c3", "Agent changed · dev · gemini-flash-latest", { tone: "warn" }),
      chip("c4", "Agent changed · dev · gemini-flash-latest", { childSession: "s-9" }),
      chip("c5", "Agent changed · dev · gemini-flash-latest", { fold: true }),
    ];
    expect(mergeAdjacentChips(items)).toHaveLength(5);
  });

  it("leaves a repeat that is separated by real work alone", () => {
    const merged = mergeAdjacentChips([agent("c1"), assistant("a1"), agent("c2")]);
    expect(merged).toHaveLength(3);
    expect((merged[0] as ChipItem).text).not.toMatch(/×/);
  });

  it("renders the stuttered pair as one row in the thread", () => {
    const { container } = render(
      <TimelineView items={[agent("c1"), agent("c2")]} pending={[]} typing="" showSys={false} />,
    );
    const chips = container.querySelectorAll(".chip");
    expect(chips).toHaveLength(1);
    expect(chips[0].textContent).toContain("×2");
  });

  it("leaves the developer (showSys) view raw", () => {
    const { container } = render(
      <TimelineView items={[agent("c1"), agent("c2")]} pending={[]} typing="" showSys />,
    );
    expect(container.querySelectorAll(".chip")).toHaveLength(2);
  });
});

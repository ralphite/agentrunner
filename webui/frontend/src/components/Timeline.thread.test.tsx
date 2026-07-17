// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { TimelineView, mergeAdjacentChips } from "./Timeline";
import type { BubbleItem, ChipItem, TimelineItem } from "../timeline";

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

describe("Timeline — layout invariants (Tailwind migration)", () => {
  it("renders assistant messages with action row", () => {
    const { container } = render(
      <TimelineView
        items={[assistant("a1")]}
        pending={[]}
        typing=""
        showSys={false}
        onContinue={() => {}}
      />,
    );
    const msg = container.querySelector(".msg.assistant");
    expect(msg).toBeTruthy();
    const actions = msg?.querySelector(".msg-actions");
    expect(actions).toBeTruthy();
  });

  it("preserves chips in timeline", () => {
    const items: TimelineItem[] = [
      { kind: "chip", key: "c1", text: "chip1", tone: "" },
      { kind: "chip", key: "c2", text: "chip2", tone: "" },
    ];
    const merged = mergeAdjacentChips(items);
    expect(merged.length).toBeGreaterThanOrEqual(1);
  });
});

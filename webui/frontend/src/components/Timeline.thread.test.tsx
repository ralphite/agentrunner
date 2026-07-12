// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";

// The layout invariants below live in CSS, and jsdom applies no CSS — so the
// sheet's own rule text is the only place they can be pinned. It's read from
// disk (vitest runs from webui/frontend); the project ships no @types/node, so
// the two node references carry a local suppression rather than a global type
// dependency the app build doesn't need.
// @ts-ignore -- no @types/node in this project's tsconfig
import { readFileSync } from "node:fs";
import { TimelineView, mergeAdjacentChips } from "./Timeline";
import type { BubbleItem, ChipItem, TimelineItem } from "../timeline";

// @ts-ignore -- ditto: `process` is a vitest-only reference
const conv: string = readFileSync(`${process.cwd()}/src/styles.conv.css`, "utf8");

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

// The rule block for a selector, so an assertion can't be satisfied by a match
// that lives in some unrelated block of the sheet.
function block(selector: string): string {
  const at = conv.indexOf(selector);
  expect(at, `${selector} not found in styles.conv.css`).toBeGreaterThan(-1);
  return conv.slice(at, conv.indexOf("}", at));
}

// ---------------------------------------------------------------------------
// TH-1 — the resting action row is LEFT-ANCHORED on the prose column.
//
// jsdom has no layout engine, so the x=350 verdict can't be measured here; what
// CAN be pinned is the mechanism that produces it, and the mechanism is exactly
// what regressed: hiding the icons with opacity alone left their boxes (3×26px
// + gaps = 94px) holding the row open, stranding the timestamp in mid-air. So
// this asserts (a) the hidden members collapse their box AND cancel their flex
// gap, (b) nothing re-introduces a hard-coded inline width on the divider, and
// (c) the row's persistent members (verdict + time) really are what's left.
// ---------------------------------------------------------------------------
describe("TH-1 — assistant action row: hidden icons cost no width", () => {
  it("collapses the hidden icons and the divider to a zero-width, gap-free box", () => {
    const rest = block(".msg .msg-col .msg-actions .msg-copy,");
    expect(rest).toMatch(/width:\s*0;/);
    expect(rest).toMatch(/padding-left:\s*0;/);
    expect(rest).toMatch(/padding-right:\s*0;/);
    // the negative margin is what cancels .msg-actions' 4px flex gap — without
    // it three collapsed icons would still push the timestamp 12px off the edge
    expect(rest).toMatch(/margin-right:\s*-4px;/);
    expect(rest).toMatch(/opacity:\s*0;/);
    // vertical padding is NOT zeroed: the row must keep its height so revealing
    // the icons on hover doesn't reflow the thread below it
    expect(rest).not.toMatch(/(^|\W)padding:\s*0/);
  });

  it("gives the timestamp no left margin, so at rest it sits flush on the column edge", () => {
    expect(block(".msg .msg-col .msg-actions .msg-time")).toMatch(/margin-left:\s*0;/);
  });

  it("restores full-size, clickable icons on hover and on keyboard focus", () => {
    const shown = block(".msg:hover .msg-col .msg-actions .msg-copy,");
    expect(shown).toMatch(/width:\s*26px;/);
    expect(shown).toMatch(/margin-right:\s*0;/);
    expect(shown).toMatch(/pointer-events:\s*auto;/);
    // the focus twin keeps keyboard users out of a dead row
    expect(conv).toContain(".msg .msg-col .msg-actions:focus-within .msg-copy");
  });

  it("keeps the divider CSS-controlled (no inline width that would survive the collapse)", () => {
    const { container } = render(
      <TimelineView
        items={[assistant("a1")]}
        pending={[]}
        typing=""
        showSys={false}
        goalVerdict={{ elapsed: "3h 47m" }}
      />,
    );
    const div = container.querySelector(".msg-actions-div") as HTMLElement;
    expect(div).not.toBeNull();
    expect(div.getAttribute("style")).toBeNull();
    // the row's persistent members — the verdict and the timestamp — are still
    // there (RT-3's decision stands; only its layout bug is fixed)
    const row = container.querySelector(".msg-actions") as HTMLElement;
    expect(row.querySelector(".msg-goal-verdict")?.textContent).toContain("Goal achieved in 3h 47m");
    expect(row.querySelector(".msg-time")).not.toBeNull();
    // and the icons remain in the DOM (hence in the tab order)
    expect(row.querySelectorAll("button.msg-copy").length).toBeGreaterThanOrEqual(2);
  });
});

// ---------------------------------------------------------------------------
// TH-2 — the composer breaks on the same two x's as the thread's prose column.
// .tl-inner is `max-width: 720 / padding: 0 30px` → a 660px column; the session
// composer card used to be a 720px card in a 28px-padded shell, i.e. 30px wider
// on each side. Mirroring the geometry (not hard-coding 660) keeps them equal at
// every width, including the breakpoints where .tl-inner retunes its padding.
// ---------------------------------------------------------------------------
describe("TH-2 — session composer shares the thread's column edges", () => {
  it("caps the session card at the thread column and mirrors its side padding", () => {
    expect(block(".cx.cx-session .cx-card")).toMatch(/max-width:\s*660px;/);
    expect(block(".cx.cx-session")).toMatch(/padding-left:\s*30px;/);
    expect(block(".cx.cx-session")).toMatch(/padding-right:\s*30px;/);
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

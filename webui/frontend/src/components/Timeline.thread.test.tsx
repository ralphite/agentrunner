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
// TH-1 + TH-10 — the assistant action row is PERSISTENT and left-anchored.
//
// TH-10: Codex draws the action icons at rest (`⧉ 👍 👎 ↗ │ ⊘ Goal achieved in
// …`); our sheet had collapsed them to width:0 / opacity:0 until :hover, which
// makes Copy / Copy-link / Continue-in-new-task invisible (and, on touch,
// unreachable). They are now always drawn, dimmed at rest.
//
// TH-1's contracts must survive that, and jsdom has no layout engine — so what
// is pinned here is the mechanism that produces them: (a) hover changes ONLY
// opacity, never a box property, so the row's height can't move and the thread
// can't reflow; (b) the timestamp keeps no left margin, so the row still starts
// flush on the column edge; (c) the divider stays CSS-controlled and the row's
// persistent members are all still rendered.
// ---------------------------------------------------------------------------
describe("TH-10 — assistant action row: icons are visible at rest", () => {
  it("keeps the icons drawn at rest, dimmed rather than collapsed", () => {
    const rest = block(".msg .msg-col .msg-actions .msg-copy {");
    expect(rest).toMatch(/opacity:\s*0\.5;/);
    // the round-20 collapse is gone: no zero width, no gap-cancelling margin,
    // no pointer-events lockout — a click at rest must work without a hover
    expect(rest).not.toMatch(/width:\s*0;/);
    expect(rest).not.toMatch(/margin-right:\s*-4px;/);
    expect(rest).not.toMatch(/pointer-events:\s*none;/);
  });

  it("animates opacity only, so hovering can never change the row's height", () => {
    const rest = block(".msg .msg-col .msg-actions .msg-copy {");
    expect(rest).toMatch(/transition:\s*opacity/);
    const shown = block(".msg:hover .msg-col .msg-actions .msg-copy,");
    // the hover block sets nothing but opacity — no width/padding/margin, which
    // is what used to reflow the thread under the pointer
    expect(shown).toMatch(/opacity:\s*1;/);
    expect(shown).not.toMatch(/width|padding|margin/);
    // the focus twin keeps keyboard users out of a dead row
    expect(conv).toContain(".msg .msg-col .msg-actions:focus-within .msg-copy");
  });

  it("keeps the divider a persistent hairline, not a hover-only one", () => {
    const div = block(".msg .msg-col .msg-actions .msg-actions-div");
    expect(div).toMatch(/width:\s*1px;/);
    expect(div).toMatch(/opacity:\s*0\.22;/);
  });

  it("gives the timestamp no left margin, so the row starts flush on the column edge", () => {
    expect(block(".msg .msg-col .msg-actions .msg-time")).toMatch(/margin-left:\s*0;/);
  });

  it("leaves the invisible user-message row unclickable at rest", () => {
    // styles.css still fades a user message's row out entirely at rest — an
    // invisible row must not swallow clicks
    expect(block(".msg:not(.assistant) .msg-col .msg-actions {")).toMatch(/pointer-events:\s*none;/);
    expect(conv).toContain(".msg:not(.assistant):hover .msg-col .msg-actions");
  });

  it("renders the row's members: three action buttons, the divider, verdict and time", () => {
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
    expect(row.querySelector(".msg-time")).not.toBeNull();
    // Copy / Copy link / Continue in new task — the three entry points TH-10 is
    // about; they are in the DOM, hence in the tab order, at rest
    expect(row.querySelectorAll("button.msg-copy")).toHaveLength(3);
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

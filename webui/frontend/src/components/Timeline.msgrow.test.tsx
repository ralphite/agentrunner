// @vitest-environment jsdom
//
// TH-21 (INC-41 round 37) — the per-message action row is hover-only, except on
// the thread's last assistant answer.
//
// Why this exists: rounds 18/20/24 read ONE crop of the gold master
// (qa/codex-reference/codex-crop-message-actions.jpg) and concluded the action
// icons are drawn at rest on every message. Round 37 magnified the FULL screen
// (qa/codex-reference/codex-task-thread.jpg, 819×1456): mid-thread, every
// message ends and the next block begins immediately — no icon row, no
// timestamp — and the crop turns out to be a photo of the thread's FINAL row,
// which also carries no timestamp. Our build was stamping `⧉ ↗ ⧉  Friday
// 06:21 PM` under all ten turns of a ten-turn thread.
//
// What ships, and what is pinned here:
//   • middle messages  → row at opacity 0, revealed on :hover / :focus-within;
//   • last assistant   → row persists (Copy / Copy-link / Continue stay usable
//                        on touch and without a priming hover — TH-10's real win);
//   • no message       → persistent timestamp; the last row renders none at all.
//   • TH-1 survives    → hover flips ONLY opacity (+ pointer-events, which has
//                        no box), so revealing the row cannot reflow the thread.
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
// @ts-ignore -- no @types/node in this project's tsconfig
import { readFileSync } from "node:fs";
import { TimelineView } from "./Timeline";
import type { BubbleItem } from "../timeline";

// @ts-ignore -- ditto: `process` is a vitest-only reference
const conv: string = readFileSync(`${process.cwd()}/src/styles.conv.css`, "utf8");

// jsdom ships no ResizeObserver, and a USER message mounts CollapsibleUserText,
// which observes its own box. A no-op is the honest stub here (these tests read
// the action row, not fold heights).
class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as any).ResizeObserver ??= NoopResizeObserver;

afterEach(cleanup);

const REST = ".timeline .tl-inner .msg:not(.msg-last) .msg-col .msg-actions {";
const HOVER = ".timeline .tl-inner .msg:not(.msg-last):hover .msg-col .msg-actions,";
const LAST_TIME = ".timeline .tl-inner .msg.msg-last .msg-col .msg-actions .msg-time";

function block(selector: string): string {
  const at = conv.indexOf(selector);
  expect(at, `${selector} not found in styles.conv.css`).toBeGreaterThan(-1);
  return conv.slice(at, conv.indexOf("}", at));
}

const user = (key: string, text = "do it"): BubbleItem => ({
  kind: "user",
  key,
  text,
  ts: "2026-07-11T18:21:00Z",
  source: "you",
});
const assistant = (key: string, text = "Done."): BubbleItem => ({
  kind: "assistant",
  key,
  text,
  ts: "2026-07-11T18:35:00Z",
});

// A thread with a middle turn and a final answer: u1 → a1 → u2 → a2. Only a2 is
// the thread's last assistant answer.
function thread() {
  return render(
    <TimelineView
      items={[user("u1"), assistant("a1", "First pass."), user("u2"), assistant("a2", "All set.")]}
      pending={[]}
      typing=""
      showSys={false}
      onContinue={() => {}}
    />,
  );
}

describe("TH-21 — only the last assistant answer keeps its action row at rest", () => {
  it("marks exactly one message .msg-last: the final assistant answer", () => {
    const { container } = thread();
    const marked = container.querySelectorAll(".msg-last");
    expect(marked).toHaveLength(1);
    expect(marked[0].classList.contains("assistant")).toBe(true);
    expect(marked[0].textContent).toContain("All set.");
    // the earlier assistant answer is NOT exempt — it is a middle message
    const assistants = container.querySelectorAll(".msg.assistant");
    expect(assistants).toHaveLength(2);
    expect(assistants[0].classList.contains("msg-last")).toBe(false);
  });

  it("hides the row at rest on every message but the last, without a box change", () => {
    const rest = block(REST);
    expect(rest).toMatch(/opacity:\s*0;/);
    // an invisible row must not swallow clicks
    expect(rest).toMatch(/pointer-events:\s*none;/);
    // TH-1: the row still OCCUPIES its box at rest — hiding it may not collapse
    // height, margin or display, or the thread reflows the moment a pointer
    // crosses it
    expect(rest).not.toMatch(/height|width|padding|margin|display|position/);
  });

  it("reveals it on :hover and on :focus-within, flipping opacity only", () => {
    const shown = block(HOVER);
    expect(shown).toMatch(/opacity:\s*1;/);
    expect(shown).toMatch(/pointer-events:\s*auto;/);
    // the no-reflow contract, from the other side: the hover state declares no
    // box property either, so rest and hover have identical geometry
    expect(shown).not.toMatch(/height|width|padding|margin|display|position/);
    // keyboard reach: the buttons stay in the DOM and in the tab order, and
    // focusing one reveals the row it lives in
    expect(conv).toContain(".timeline .tl-inner .msg:not(.msg-last) .msg-col .msg-actions:focus-within");
  });

  it("keeps the three action buttons in the DOM (and tab order) on a hidden row", () => {
    const { container } = thread();
    // the FIRST assistant answer is a middle message: its row is invisible, but
    // it is not absent — hover-only must not mean unreachable
    const mid = container.querySelectorAll(".msg.assistant")[0];
    expect(mid.querySelectorAll("button.msg-copy").length).toBeGreaterThanOrEqual(2);
  });
});

describe("TH-21 — no message shows a persistent timestamp", () => {
  it("drops the timestamp on the last assistant answer — the one row drawn at rest", () => {
    // Codex's persistent row is `⧉ 👍 👎 ↗ │ ⊘ Goal achieved in 3h 47m 26s`: no
    // time. MsgActions renders one row shape for every message, so the drop is
    // the sheet's job — and it is static, never hover-dependent, so it reflows
    // nothing.
    expect(block(LAST_TIME)).toMatch(/display:\s*none;/);
  });

  it("keeps every rendered timestamp inside a message's action row", () => {
    const { container } = thread();
    // no message may stamp a time anywhere but the row — and the row is either
    // hover-only (middle) or timestamp-less (last), so none of them is resting
    // noise
    const times = container.querySelectorAll(".msg-time");
    expect(times.length).toBeGreaterThan(0);
    for (const t of Array.from(times)) expect(t.closest(".msg-actions")).not.toBeNull();
  });
});

// The cascade itself, not just the rule text: jsdom has no layout engine, but it
// does resolve computed style against the document's style sheets. Injecting the
// real sheet proves our selectors actually win over the persistent-row rules
// (`.msg-actions { opacity: 1 }` in styles.css, `.msg.assistant .msg-col
// .msg-actions { opacity: 1 }` in this file) rather than merely existing.
// :hover cannot be simulated in jsdom (there is no pointer), so the hover state
// stays pinned by the rule-text assertions above — the live-browser check is the
// screenshot QA.
describe("TH-21 — cascade: the rest state resolves to opacity 0 / 1", () => {
  it("computes 0 on a middle message and 1 on the last assistant answer", () => {
    const style = document.createElement("style");
    // styles.css's baseline (the row is persistent by default) + the real sheet
    style.textContent = ".msg-actions { opacity: 1; }\n" + conv;
    document.head.appendChild(style);
    try {
      const { container } = thread();
      const assistants = container.querySelectorAll(".msg.assistant");
      const midRow = assistants[0].querySelector(".msg-actions") as HTMLElement;
      const lastRow = assistants[1].querySelector(".msg-actions") as HTMLElement;
      expect(getComputedStyle(midRow).opacity).toBe("0");
      expect(getComputedStyle(lastRow).opacity).toBe("1");
      // and the persistent row's timestamp resolves away
      expect(getComputedStyle(lastRow.querySelector(".msg-time") as HTMLElement).display).toBe("none");
    } finally {
      style.remove();
    }
  });
});

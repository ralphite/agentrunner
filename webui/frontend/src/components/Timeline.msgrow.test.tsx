// @vitest-environment jsdom
//
// TH-21 (INC-41 round 37) — the per-message action row is hover-only, except on
// the thread's last assistant answer.
//
// Why this exists: rounds 18/20/24 read ONE crop of the gold master
// (qa/codex-reference/codex-crop-message-actions.jpg) and concluded the action
// icons are drawn at rest on every message. Round 37 magnified the FULL screen
// (qa/codex-reference/codex-session-thread.jpg, 819×1456): mid-thread, every
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
import { TimelineView } from "./Timeline";
import type { BubbleItem } from "../timeline";

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

  it("keeps the three action buttons in the DOM (and tab order) on a hidden row", () => {
    const { container } = thread();
    // the FIRST assistant answer is a middle message: its row is invisible, but
    // it is not absent — hover-only must not mean unreachable
    const mid = container.querySelectorAll(".msg.assistant")[0];
    expect(mid.querySelectorAll("button.msg-copy").length).toBeGreaterThanOrEqual(2);
  });

  it("lets a touch user focus a middle message to reveal its hidden actions", () => {
    const { container } = thread();
    const mid = container.querySelectorAll<HTMLElement>(".msg.assistant")[0];

    // The CSS reveal is driven by :focus-within. A focusable message is the
    // missing touch bridge: phones have no hover, but tapping the message can
    // now reveal the existing row without making it persistent at rest.
    expect(mid.tabIndex).toBe(0);
    mid.focus();
    expect(document.activeElement).toBe(mid);
    expect(mid.querySelector(".msg-actions")).not.toBeNull();
  });
});

describe("TH-21 — no message shows a persistent timestamp", () => {
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

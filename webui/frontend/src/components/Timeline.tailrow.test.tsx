// @vitest-environment jsdom
//
// TAIL-ROW — the thread's LAST assistant answer surrenders its inline action row
// (message Copy) to the bottom of .tl-inner, PAST the turn's artifact
// / changes cards, where it sits on the same bottom row as the goal verdict.
//
// Why this exists (P3): before this change the final answer's action icons
// rendered inline under the prose — i.e. ABOVE the turn's README artifact card
// and its "Edited N files" changes card. A reader who finished the answer plus
// both cards and wanted copy/continue had to scroll back UP past the cards to
// find the buttons. Codex draws them AFTER the turn's full content, beside the
// goal: `⧉ ↗ ⧉ │ ⊘ Goal achieved in N`. R51 already hoisted the goal footer
// there; this test pins the action row into that same bottom region.
//
// What is pinned:
//   • last answer + outcome card → action row is a sibling of, and DOM-AFTER,
//     the `.changes-outcome` card (i.e. inside .tl-tail-row, past outcomeSlot);
//   • the last answer's bubble itself carries NO inline `.msg-actions`;
//   • a MIDDLE answer keeps its hover-only inline `.msg-actions`;
//   • with no goal verdict the action row still renders at the bottom.
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { TimelineView } from "./Timeline";
import type { BubbleItem } from "../timeline";

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

const outcomeCard = (
  <div className="changes-outcome">Edited 14 files</div>
);

// A settled two-turn thread (u1 → a1 → u2 → a2) with a changes card and a goal
// verdict on the final turn. a2 is the thread's last assistant answer.
function settledThread(opts: { goal?: boolean } = {}) {
  return render(
    <TimelineView
      items={[user("u1"), assistant("a1", "First pass."), user("u2"), assistant("a2", "All set.")]}
      pending={[]}
      typing=""
      showSys={false}
      outcomeSlot={outcomeCard}
      goalVerdict={opts.goal ? { elapsed: "3h 47m 26s" } : null}
    />,
  );
}

describe("TAIL-ROW — last answer's action row lands after the turn's cards", () => {
  it("renders the final action row inside .tl-tail-row, DOM-after the changes card", () => {
    const { container } = settledThread({ goal: true });

    const tail = container.querySelector(".tl-tail-row");
    expect(tail).not.toBeNull();

    const card = container.querySelector(".changes-outcome")!;
    const tailActions = tail!.querySelector(".msg-actions")!;
    expect(tailActions).not.toBeNull();

    // the tail action row comes AFTER the changes card in document order
    expect(card.compareDocumentPosition(tailActions) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    // and the goal verdict shares that same bottom row
    const footer = tail!.querySelector(".turn-footer")!;
    expect(footer).not.toBeNull();
    expect(footer.textContent).toContain("Goal achieved in 3h 47m 26s");
  });

  it("strips the inline action row from the last answer's bubble", () => {
    const { container } = settledThread({ goal: true });
    const last = container.querySelector(".msg.assistant.msg-last")!;
    expect(last).not.toBeNull();
    expect(last.textContent).toContain("All set.");
    // the last answer no longer carries its own inline action row — it moved down
    expect(last.querySelector(".msg-actions")).toBeNull();
  });

  it("keeps a MIDDLE answer's inline action row in place", () => {
    const { container } = settledThread({ goal: true });
    const mid = container.querySelectorAll(".msg.assistant")[0];
    expect(mid.classList.contains("msg-last")).toBe(false);
    // its hover-only row is still inline in the bubble column
    expect(mid.querySelector(".msg-actions")).not.toBeNull();
    expect(mid.querySelectorAll("button.msg-copy")).toHaveLength(1);
  });

  it("still renders the bottom action row when the turn has no goal verdict", () => {
    const { container } = settledThread({ goal: false });
    const tail = container.querySelector(".tl-tail-row")!;
    expect(tail).not.toBeNull();
    // action row present, goal badge absent
    expect(tail.querySelector(".msg-actions")).not.toBeNull();
    expect(tail.querySelector(".turn-footer")).toBeNull();
    // and it is still after the changes card
    const card = container.querySelector(".changes-outcome")!;
    const tailActions = tail.querySelector(".msg-actions")!;
    expect(card.compareDocumentPosition(tailActions) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

describe("TAIL-ROW — a live turn keeps the last answer's row inline", () => {
  it("does not hoist the action row while the run is active", () => {
    const { container } = render(
      <TimelineView
        items={[user("u1"), assistant("a1", "Working on it.")]}
        pending={[]}
        typing=""
        showSys={false}
        active
      />,
    );
    // no bottom tail row while active; the persistent row stays inside .msg-last
    expect(container.querySelector(".tl-tail-row")).toBeNull();
    const last = container.querySelector(".msg.assistant.msg-last")!;
    expect(last.querySelector(".msg-actions")).not.toBeNull();
  });
});

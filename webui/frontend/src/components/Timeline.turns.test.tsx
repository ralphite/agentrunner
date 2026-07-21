// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";

import { TimelineView, shortTime, workedLabel } from "./Timeline";
import type { BubbleItem, TimelineItem, ToolItem, TurnItem, WorkFold } from "../timeline";

// jsdom ships no ResizeObserver, and rendering a USER message mounts
// CollapsibleUserText, which observes its own box to decide whether to offer a
// "Show more" toggle. These tests count turn boundaries, not fold heights — a
// no-op observer is the honest stub (the layout it would report is 0 in jsdom
// anyway, so nothing here depends on it firing).
class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as any).ResizeObserver ??= NoopResizeObserver;

afterEach(cleanup);

const user = (key: string, text = "do the thing"): BubbleItem => ({ kind: "user", key, text });
const assistant = (key: string, text = "Done."): BubbleItem => ({ kind: "assistant", key, text });
const tool = (key: string): ToolItem => ({
  kind: "tool",
  key,
  name: "Read",
  args: { file_path: "/x" },
  background: false,
  status: "done",
  statusText: "",
});
// A "turn" marker carries generation_started; it is filtered out of the reader's
// feed but is what completedTurnDurations measures a turn's duration from.
const turn = (key: string, ts: string): TurnItem => ({ kind: "turn", key, gen: 1, ts });

// ---------------------------------------------------------------------------
// TR-1 — turns are separated by a 1px rule across the prose column.
//
// The gold master (qa/codex-reference/codex-session-thread.jpg) closes every turn
// with a hairline spanning the FULL body column. We drew nothing at all: a live
// 10-turn session measured `.turn-sep` === 0, which leaves a long thread as one
// undifferentiated scroll with no navigation unit.
// ---------------------------------------------------------------------------
describe("TR-1 — turn separator", () => {
  // N user turns → N-1 rules: the rule opens a turn, and the first turn opens
  // nothing above it.
  const thread = (turns: number): TimelineItem[] => {
    const items: TimelineItem[] = [];
    for (let i = 0; i < turns; i++) {
      items.push(user("u" + i), tool("t" + i), assistant("a" + i));
    }
    return items;
  };

  it("draws no rule on a single-turn thread — there is no boundary to mark", () => {
    const { container } = render(
      <TimelineView items={thread(1)} pending={[]} typing="" showSys={false} />,
    );
    expect(container.querySelectorAll(".turn-sep")).toHaveLength(0);
  });

  it("draws N-1 rules for N user turns (the live 10-turn session ⇒ 9)", () => {
    const { container } = render(
      <TimelineView items={thread(10)} pending={[]} typing="" showSys={false} />,
    );
    expect(container.querySelectorAll(".turn-sep")).toHaveLength(9);
  });

  it("puts the rule ABOVE each user message but the first, never above the first", () => {
    const { container } = render(
      <TimelineView items={thread(3)} pending={[]} typing="" showSys={false} />,
    );
    const col = container.querySelector(".tl-inner") as HTMLElement;
    const kids = Array.from(col.children);
    const users = kids.filter((el) => el.classList.contains("msg") && el.classList.contains("user"));
    expect(users).toHaveLength(3);
    // first user message: whatever precedes it, it is not a rule
    expect(users[0].previousElementSibling?.classList.contains("turn-sep") ?? false).toBe(false);
    // every later one is opened by a rule
    for (const u of users.slice(1)) {
      expect(u.previousElementSibling?.classList.contains("turn-sep")).toBe(true);
    }
  });

  it("hangs the rule off .tl-inner itself, so it spans the column and not a card", () => {
    const { container } = render(
      <TimelineView items={thread(2)} pending={[]} typing="" showSys={false} />,
    );
    const sep = container.querySelector(".turn-sep") as HTMLElement;
    // a direct child of the column inherits the column's content box — the same
    // left edge an assistant .msg-col starts at, and the column's right edge
    expect(sep.parentElement?.classList.contains("tl-inner")).toBe(true);
    expect(sep.getAttribute("role")).toBe("separator");
  });

});

// ---------------------------------------------------------------------------
// TR-2 — a timestamp that can't say WHICH DAY is useless on a multi-day session.
//
// `shortTime` used to emit a bare "11:31 PM". A live session showed "11:31 PM"
// above "12:40 AM" — two different days, with nothing on screen saying so.
// `now` is injected, so these pin the tiers rather than the wall clock.
// ---------------------------------------------------------------------------
describe("TR-2 — timestamp date tiers", () => {
  // Local-time constructors throughout: the tiers are local calendar days, and
  // hard-coded Z strings would flip a tier depending on the runner's timezone.
  const at = (y: number, m: number, d: number, h: number, min: number) =>
    new Date(y, m - 1, d, h, min).toISOString();
  const now = new Date(2026, 6 - 1, 12, 15, 0); // Fri 12 Jun 2026, 3:00 PM local
  const hhmm = (ts: string) =>
    new Date(ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

  it("today → time only", () => {
    const ts = at(2026, 6, 12, 10, 14);
    expect(shortTime(ts, now)).toBe(hhmm(ts));
  });

  it("earlier today, just after midnight → still time only (same calendar day)", () => {
    const ts = at(2026, 6, 12, 0, 40);
    expect(shortTime(ts, now)).toBe(hhmm(ts));
  });

  it("yesterday → weekday + time, even though it is < 24h ago", () => {
    // 11:50 PM yesterday is ~15h before `now` — a 24h-chunk rule would call it
    // "today" and print a bare time, which is the exact midnight bug TR-2 is about
    const ts = at(2026, 6, 11, 23, 50);
    const out = shortTime(ts, now)!;
    expect(out).toBe(`${new Date(ts).toLocaleDateString([], { weekday: "long" })} ${hhmm(ts)}`);
    expect(out).not.toBe(hhmm(ts));
  });

  it("within the last 6 days → 'Friday 10:14 PM' (Codex's form)", () => {
    const ts = at(2026, 6, 8, 22, 14);
    const out = shortTime(ts, now)!;
    expect(out).toBe(`${new Date(ts).toLocaleDateString([], { weekday: "long" })} ${hhmm(ts)}`);
    expect(out).toContain(hhmm(ts));
  });

  it("exactly 7 days ago → date, NOT a weekday (the name would repeat today's)", () => {
    const ts = at(2026, 6, 5, 22, 14);
    const out = shortTime(ts, now)!;
    expect(out).toBe(
      `${new Date(ts).toLocaleDateString([], { month: "short", day: "numeric" })}, ${hhmm(ts)}`,
    );
    expect(out).not.toContain(new Date(ts).toLocaleDateString([], { weekday: "long" }));
  });

  it("older → 'Jul 3, 10:14 PM'", () => {
    const ts = at(2026, 5, 22, 22, 14);
    expect(shortTime(ts, now)).toBe(
      `${new Date(ts).toLocaleDateString([], { month: "short", day: "numeric" })}, ${hhmm(ts)}`,
    );
  });

  it("a future ts (clock skew) degrades to the plain time, not 'in 3 days'", () => {
    const ts = at(2026, 6, 15, 9, 0);
    expect(shortTime(ts, now)).toBe(hhmm(ts));
  });

  it("keeps returning null for a missing or unparseable timestamp", () => {
    expect(shortTime(undefined, now)).toBeNull();
    expect(shortTime("not a date", now)).toBeNull();
  });

  it("renders the dated label on the message action row", () => {
    const ts = at(2026, 6, 8, 22, 14);
    // the component reads the real clock, so assert the shape it must produce
    // for a timestamp that is definitively old rather than pinning a tier here.
    // a1 is a MIDDLE answer here: the thread's LAST answer deliberately carries
    // no timestamp (TH-21 / TAIL-ROW), so the dated label is pinned on the
    // hover-only row of an earlier message, which is where a tier label shows.
    const { container } = render(
      <TimelineView
        items={[{ ...assistant("a1"), ts }, user("u2"), assistant("a2")]}
        pending={[]}
        typing=""
        showSys={false}
      />,
    );
    const label = container.querySelector(".msg-time")!.textContent!;
    expect(label).toBe(shortTime(ts)!);
    // 2026-06-08 will never again be "today", so the label must carry a date
    expect(label).not.toBe(hhmm(ts));
  });
});

// ---------------------------------------------------------------------------
// TR-6 — an empty "Worked for 2s" is a dead row.
//
// foldWork emits a fold with a duration but ZERO children whenever a turn's work
// detail is empty. That rendered as a caret-less, un-clickable "Worked for 2s"
// with nothing behind it — a live session showed three in a row.
// ---------------------------------------------------------------------------
describe("TR-6 — empty work folds are dropped", () => {
  // A settled turn with no tool activity: the turn marker gives the duration,
  // the assistant answer closes it, and there is no work detail in between.
  const emptyTurn: TimelineItem[] = [
    turn("t1", "2026-07-11T18:35:00Z"),
    user("u1"),
    { ...assistant("a1"), ts: "2026-07-11T18:35:02Z" },
  ];

  it("renders no 'Worked' row for a turn whose fold has no children", () => {
    const { container } = render(
      <TimelineView items={emptyTurn} pending={[]} typing="" showSys={false} />,
    );
    expect(container.querySelectorAll(".worked")).toHaveLength(0);
    expect(container.textContent).not.toMatch(/Worked/);
    // the turn's real content is untouched
    expect(container.querySelector(".msg.assistant")).not.toBeNull();
  });

  it("still renders the row — with a caret — when the fold has work to show", () => {
    const items: TimelineItem[] = [
      turn("t1", "2026-07-11T18:35:00Z"),
      user("u1"),
      tool("k1"),
      { ...assistant("a1"), ts: "2026-07-11T18:35:02Z" },
    ];
    const { container } = render(
      <TimelineView items={items} pending={[]} typing="" showSys={false} />,
    );
    const row = container.querySelector("button.worked-row") as HTMLButtonElement;
    expect(row).not.toBeNull();
    expect(row.textContent).toMatch(/Worked/);
    // every surviving row is expandable — that is the whole point of TR-6
    expect(row.disabled).toBe(false);
    expect(row.querySelector(".worked-caret")).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// THREAD-2 — an interrupted turn's fold head must show its real work-span.
//
// Only a settled turn (user prompt → final answer) carries a durationMs. A turn
// cut short by a step limit or stalled on an approval never settles, so its head
// used to degrade to "Worked · N steps" — even though the elapsed was right
// there on screen ("Goal cancelled 00:34"). foldWork now dates those turns from
// their generation_started markers (startMs..endMs); workedLabel must surface it.
// ---------------------------------------------------------------------------
describe("THREAD-2 — workedLabel prefers the real work-span", () => {
  const fold = (over: Partial<WorkFold>): WorkFold => ({
    kind: "fold",
    key: "f",
    children: [tool("t1")],
    ...over,
  });

  it("shows the settled turn's stored duration unchanged", () => {
    expect(workedLabel(fold({ durationMs: 30000 }))).toBe("Worked for 30s");
  });

  it("dates an interrupted turn from its startMs..endMs span (the 00:34 case)", () => {
    // No durationMs (never settled), tool step carries no ts — yet the span is
    // real: 34s from the first gen step to the interruption.
    const f = fold({ startMs: 1_000_000, endMs: 1_034_000 });
    expect(f.durationMs).toBeUndefined();
    expect(workedLabel(f)).toBe("Worked for 34s");
  });

  it("prefers durationMs over the span when both are present (settled, not displaced)", () => {
    expect(workedLabel(fold({ durationMs: 12000, startMs: 0, endMs: 999_000 }))).toBe("Worked for 12s");
  });

  it("falls back to a step count when no time data exists at all", () => {
    const f = fold({ children: [tool("t1"), tool("t2")] });
    expect(f.durationMs).toBeUndefined();
    expect(f.startMs).toBeUndefined();
    expect(workedLabel(f)).toBe("Worked · 2 steps");
  });

  it("does not fabricate a span from a zero-or-negative window (degrades to steps)", () => {
    // endMs <= startMs is not a real span; the head must not read "Worked for 0s".
    expect(workedLabel(fold({ startMs: 5000, endMs: 5000 }))).toBe("Worked · 1 step");
  });
});

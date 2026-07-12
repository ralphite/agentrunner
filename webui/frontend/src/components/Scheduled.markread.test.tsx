// @vitest-environment jsdom
//
// SC-21 — the tab row's right end. Codex puts a quiet `✓ Mark all as read` there,
// opposite the All / Active / Paused tabs, and it is the only way to clear a whole
// screenful of blue dots without opening every row. We had the per-row path (the ⋯
// menu's "Mark as read") and the store action behind it, so the bulk action is a
// button, not a new backend.
//
// The two things these tests pin are the ones that make it honest rather than
// decorative:
//   1. clicking it actually zeroes the dots (it calls markRead, it does not just
//      look like it);
//   2. it is scoped to what you can SEE — the rows the current tab and the search
//      box leave on screen — and it does not render at all when there is nothing
//      unread in that view (no dead grey control, and no silent clearing of rows
//      that were filtered away).
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import { Scheduled } from "./Scheduled";
import { useStore } from "../store";
import type { Session } from "../types";

afterEach(cleanup);

const sessions: Session[] = [
  {
    id: "20250101-100000-digest",
    status: "idle",
    turns: 4,
    title: "Nightly digest run",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "cron",
    cadence: "Daily at 6:00 AM",
    nextRunAt: new Date(Date.now() + 3600_000).toISOString(),
  },
  {
    id: "20250101-090000-sweep",
    status: "idle",
    turns: 2,
    title: "Hourly log sweep",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "interval",
    cadence: "Every 1h",
    nextRunAt: new Date(Date.now() + 600_000).toISOString(),
  },
];

const mount = (unread: string[]) => {
  useStore.setState({
    runs: [],
    sessions,
    sessionsReady: true,
    unread,
    archived: [],
    pinned: [],
    renames: {},
    modal: null,
  });
  return render(<Scheduled />);
};

const markAll = () => screen.queryByRole("button", { name: /Mark all as read/ });

describe("Mark all as read sits at the right end of the tab row (SC-21)", () => {
  it("appears — with Codex's exact words — as soon as a scheduled row is unread", () => {
    const { container } = mount(["20250101-100000-digest"]);
    expect(markAll()).not.toBeNull();
    expect(markAll()!.textContent).toContain("Mark all as read");
    expect(container.querySelectorAll(".sched-unread").length).toBe(1);
    // …and it lives in the filters row, beside the tabs — not in the page heading.
    expect(markAll()!.closest(".sched-filters")).not.toBeNull();
  });

  it("clicking it zeroes the dots: the store's unread set is cleared, so the button retires itself", () => {
    const { container } = mount(["20250101-100000-digest", "20250101-090000-sweep"]);
    expect(container.querySelectorAll(".sched-unread").length).toBe(2);

    fireEvent.click(markAll()!);

    expect(useStore.getState().unread).toEqual([]);
    expect(container.querySelectorAll(".sched-unread").length).toBe(0);
    expect(markAll()).toBeNull();
  });

  it("does not render when nothing is unread — a bulk action with nothing to act on is not a control", () => {
    mount([]);
    expect(markAll()).toBeNull();
  });

  it("is scoped to the rows in view: a search that hides the unread row hides the button too", () => {
    mount(["20250101-100000-digest"]);
    expect(markAll()).not.toBeNull();

    fireEvent.change(screen.getByLabelText("Search scheduled tasks"), {
      target: { value: "Hourly" },
    });

    // Only the read row survives the query, so there is nothing here to mark —
    // and the click that would have cleared an off-screen row is gone with it.
    expect(screen.queryByText("Nightly digest run")).toBeNull();
    expect(markAll()).toBeNull();
    expect(useStore.getState().unread).toEqual(["20250101-100000-digest"]);
  });
});

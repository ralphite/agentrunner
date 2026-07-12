// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { CommandPalette } from "./CommandPalette";
import { useStore } from "../store";
import { quickSwitchTasks } from "../viewModels";
import type { Session } from "../types";

// INC-41 RH-3 — the palette used to badge only the *non*-attention rows, so on a
// machine whose nine quick-switch slots were all attention tasks it rendered
// zero ⌘-badges and no Tasks group at all, while ⌘1..9 kept working. These tests
// pin the Codex shape: nine badged Tasks rows (unread dot or not) + an overflow
// `Unread tasks` group, with the badge numbers matching the real key binding.

const task = (id: string, status: string): Session => ({ id, status, turns: 1, title: `Task ${id}` });
// Twelve tasks, every one of them waiting on an approval: the exact live-store
// shape from qa/runs/2026-07-11-round18/before/live-palette-dark-1440.png.
const allAttention = Array.from({ length: 12 }, (_, i) =>
  task(`t${String(12 - i).padStart(2, "0")}`, "waiting_approval"),
);

const open = (sessions: Session[]) => {
  useStore.setState({ sessions, runs: [], archived: [], renames: {} });
  return render(<CommandPalette onClose={() => {}} />);
};

// Rows are buttons; the group header is a sibling div, so read groups off the
// rendered list order rather than the DOM tree.
const rows = () => Array.from(screen.getByRole("listbox").querySelectorAll(".cmdk-item"));

afterEach(cleanup);

describe("CommandPalette task groups (RH-3)", () => {
  it("shows a Tasks group whose nine rows each carry a ⌘-digit badge", () => {
    open(allAttention);
    expect(screen.getByText("Tasks")).toBeTruthy();
    const badged = rows().filter((r) => r.querySelector(".cmdk-kbd"));
    expect(badged).toHaveLength(9);
    // Every one of them is an attention row (blue dot) — and still badged.
    badged.forEach((r) => expect(r.querySelector(".status-dot.unread")).toBeTruthy());
    // Badges read ⌘1…⌘9 (Ctrl elsewhere) in row order.
    expect(badged.map((r) => r.querySelector(".cmdk-kbd")!.textContent!.replace(/^\D+/, ""))).toEqual([
      "1", "2", "3", "4", "5", "6", "7", "8", "9",
    ]);
  });

  it("numbers the badges exactly as the global ⌘-digit binding jumps (App.tsx)", () => {
    open(allAttention);
    const expected = quickSwitchTasks(allAttention).map((s) => s.title || s.id);
    const badged = rows().filter((r) => r.querySelector(".cmdk-kbd"));
    badged.forEach((r, i) => {
      // Badge says ⌘(i+1); the row it rides is quickSwitchTasks[i] — which is
      // precisely what App.tsx opens for that digit.
      expect(r.querySelector(".cmdk-kbd")!.textContent).toMatch(new RegExp(`${i + 1}$`));
      expect(within(r as HTMLElement).getByText(expected[i])).toBeTruthy();
    });
  });

  it("puts attention tasks past the ninth digit in a badge-less Unread tasks group", () => {
    open(allAttention);
    expect(screen.getByText("Unread tasks")).toBeTruthy();
    const unbadgedTasks = rows().filter(
      (r) => r.querySelector(".status-dot") && !r.querySelector(".cmdk-kbd"),
    );
    expect(unbadgedTasks.map((r) => r.querySelector(".cmdk-label")!.textContent)).toEqual([
      "Task t03",
      "Task t02",
      "Task t01",
    ]);
  });

  it("omits the Unread tasks group when nothing overflows", () => {
    open([task("t02", "idle"), task("t01", "completed")]);
    expect(screen.getByText("Tasks")).toBeTruthy();
    expect(screen.queryByText("Unread tasks")).toBeNull();
    expect(rows().filter((r) => r.querySelector(".cmdk-kbd"))).toHaveLength(2);
  });

  it("opens the task its badge advertises", () => {
    const select = vi.fn();
    useStore.setState({ select });
    open(allAttention);
    const third = rows().filter((r) => r.querySelector(".cmdk-kbd"))[2];
    expect(third.querySelector(".cmdk-kbd")!.textContent).toMatch(/3$/);
    fireEvent.click(third);
    expect(select).toHaveBeenCalledWith(quickSwitchTasks(allAttention)[2].id);
  });

  it("drops the badges once the user types a query (no key jumps to a filtered row)", () => {
    open(allAttention);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "t0" } });
    expect(rows().some((r) => r.querySelector(".cmdk-kbd"))).toBe(false);
  });
});

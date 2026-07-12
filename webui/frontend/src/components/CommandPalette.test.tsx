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

type State = Partial<ReturnType<typeof useStore.getState>>;
const open = (sessions: Session[], state: State = {}, props: { onOpenSettings?: () => void } = {}) => {
  useStore.setState({ sessions, runs: [], archived: [], unread: [], renames: {}, ...state });
  return render(<CommandPalette onClose={() => {}} {...props} />);
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
    // Every one of them is an attention row (amber "needs approval" dot, CP-6)
    // — and still badged.
    badged.forEach((r) => expect(r.querySelector(".status-dot.appr")).toBeTruthy());
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

// INC-41 CP-5/6/7/8 — the palette's five Codex gaps: ↓ walked the selection out
// of the scroll box (Enter then opened an invisible task), every task dot was
// painted the same "new activity" blue regardless of status, archived tasks came
// back through search unmarked, and ⌘K could reach neither Scheduled nor
// Settings.

describe("CommandPalette keyboard scrolling (CP-5)", () => {
  const list = () => screen.getByRole("listbox");
  const sel = () => list().querySelector(".cmdk-item.sel")!;
  const arrow = (key: "ArrowDown" | "ArrowUp", times = 1) => {
    for (let i = 0; i < times; i++) fireEvent.keyDown(screen.getByRole("combobox"), { key });
  };

  it("scrolls the keyboard-selected row into view on every move", () => {
    // jsdom has no layout — and no scrollIntoView at all — so the call itself is
    // the only observable proof the selected row is kept on screen. It is also
    // exactly what the palette was missing.
    const scrolled: Element[] = [];
    const into = vi.fn(function (this: Element) {
      scrolled.push(this);
    });
    Object.defineProperty(Element.prototype, "scrollIntoView", { value: into, configurable: true, writable: true });
    try {
      open(allAttention);
      into.mockClear(); // drop the mount-time call for idx 0
      scrolled.length = 0;
      arrow("ArrowDown", 8);
      expect(into).toHaveBeenCalledTimes(8);
      expect(into).toHaveBeenLastCalledWith({ block: "nearest" });
      // The row it scrolled to is the row Enter would open.
      expect(scrolled[scrolled.length - 1]).toBe(sel());
      arrow("ArrowUp");
      expect(into).toHaveBeenCalledTimes(9);
      expect(scrolled[scrolled.length - 1]).toBe(sel());
    } finally {
      Reflect.deleteProperty(Element.prototype, "scrollIntoView");
    }
  });

  it("ignores mouseenter fired by rows scrolling under a parked pointer", () => {
    open(allAttention);
    const r = rows();
    fireEvent.mouseMove(list()); // pointer is live: hover owns the selection
    fireEvent.mouseEnter(r[4]);
    expect(r[4].className).toContain("sel");

    arrow("ArrowDown"); // keyboard takes over → idx 5
    expect(r[5].className).toContain("sel");
    // A row sliding under the (stationary) mouse must not yank the selection back.
    fireEvent.mouseEnter(r[1]);
    expect(r[5].className).toContain("sel");
    expect(r[1].className).not.toContain("sel");

    // A real pointer move hands control back to the mouse.
    fireEvent.mouseMove(list());
    fireEvent.mouseEnter(r[1]);
    expect(r[1].className).toContain("sel");
  });
});

describe("CommandPalette status dots (CP-6)", () => {
  const dotOf = (label: string) =>
    rows().find((r) => r.querySelector(".cmdk-label")!.textContent === label)!.querySelector(".status-dot")!;

  it("colours each dot by friendlyStatus, exactly like the sidebar rail", () => {
    open([
      task("t05", "waiting_approval"),
      task("t04", "running"),
      task("t03", "crashed"),
      task("t02", "stranded"),
      task("t01", "completed"),
    ]);
    expect(dotOf("Task t05").className).toBe("status-dot appr");
    expect(dotOf("Task t04").className).toBe("status-dot run");
    expect(dotOf("Task t03").className).toBe("status-dot crash");
    expect(dotOf("Task t02").className).toBe("status-dot stranded");
    // Quiet statuses keep the gutter but no colour (and no false "unread" blue).
    expect(dotOf("Task t01").className).toBe("status-dot");
    expect((dotOf("Task t01") as HTMLElement).style.visibility).toBe("hidden");
    expect(rows().some((r) => r.querySelector(".status-dot.unread"))).toBe(false);
  });

  it("keeps the blue unread dot for tasks with genuinely new activity", () => {
    open([task("t02", "waiting_approval"), task("t01", "completed")], { unread: ["t01"] });
    expect(dotOf("Task t01").className).toBe("status-dot unread");
    expect(dotOf("Task t01").getAttribute("title")).toBe("New activity");
    // Unread wins over status for t01 only — t02 still shows its approval amber.
    expect(dotOf("Task t02").className).toBe("status-dot appr");
    expect(dotOf("Task t02").getAttribute("title")).toBe("Needs approval");
  });
});

describe("CommandPalette archived search hits (CP-7)", () => {
  const sessions = [task("t02", "idle"), task("t01", "idle")];

  it("files archived matches under their own Archived group, after live tasks", () => {
    open(sessions, { archived: ["t01"] });
    // Empty query: archived tasks stay out of the switcher entirely.
    expect(rows().map((r) => r.querySelector(".cmdk-label")!.textContent)).not.toContain("Task t01");

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "task t" } });
    expect(screen.getByText("Archived")).toBeTruthy();
    const labels = rows().map((r) => r.querySelector(".cmdk-label")!.textContent);
    // Reachable — but last, and under an honest header rather than posing as a
    // live task in the Tasks group.
    expect(labels.indexOf("Task t01")).toBeGreaterThan(labels.indexOf("Task t02"));
    const taskRows = Array.from(screen.getByRole("listbox").children).map((c) => c.textContent);
    expect(taskRows.join("|")).toContain("Archived");
  });

  it("does not label live search hits as archived", () => {
    open(sessions, { archived: [] });
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "task t" } });
    expect(screen.queryByText("Archived")).toBeNull();
    expect(screen.getByText("Tasks")).toBeTruthy();
  });
});

describe("CommandPalette destinations (CP-8)", () => {
  const command = (label: string) => rows().find((r) => r.textContent!.startsWith(label))!;

  it("can go to Scheduled — the app's other top-level page", () => {
    const showPage = vi.fn();
    useStore.setState({ showPage });
    open([]);
    fireEvent.click(command("Go to Scheduled"));
    expect(showPage).toHaveBeenCalledWith("scheduled");
  });

  it("opens Settings through the same handler as the gear / ⌘,", () => {
    const onOpenSettings = vi.fn();
    open([], {}, { onOpenSettings });
    fireEvent.click(command("Open settings"));
    expect(onOpenSettings).toHaveBeenCalledTimes(1);
  });

  it("hides the settings row when the host gives it nowhere to go", () => {
    open([]);
    expect(rows().some((r) => r.textContent!.startsWith("Open settings"))).toBe(false);
  });
});

// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// The sidebar hits /health and /git on mount; nothing here depends on those, so
// stub the module with never-settling promises (same pattern as loadingStates).
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy({}, { get: () => () => new Promise(() => {}) }),
}));

import { Sidebar } from "./Sidebar";
import { SHORTCUT_GROUPS, keyLabel } from "../shortcuts";
import { useStore } from "../store";

afterEach(cleanup);

describe("sidebar search entry point (RH-5)", () => {
  it("magnifier opens the ⌘K command palette instead of an inline filter", () => {
    const onOpenPalette = vi.fn();
    const { container } = render(<Sidebar onOpenPalette={onOpenPalette} />);
    fireEvent.click(screen.getByLabelText("Search tasks"));
    expect(onOpenPalette).toHaveBeenCalled();
    // The second search surface is gone — one entry point, not two.
    expect(container.querySelector(".side-search")).toBeNull();
    expect(screen.queryByPlaceholderText(/Search title, id, or workspace/)).toBeNull();
  });
});

describe("current task visibility (SB-1)", () => {
  // Ten tasks in one project: with cap=6, s3…s9 (ids sort newest-first) fall
  // behind "Show more". Opening one of them must still put it on the rail.
  const manySessions = Array.from({ length: 10 }, (_v, i) => ({
    id: `2026071${i}-000000-task-${i}`,
    status: "idle",
    turns: 1,
    title: `Task ${i}`,
    workspace: "/repo/app",
  }));

  const mount = (currentSid: string | null, projects: Record<string, any> = {}) => {
    useStore.setState({
      sessions: manySessions as any,
      sessionsReady: true,
      currentSid,
      archived: [],
      pinned: [],
      unread: [],
      renames: {},
      projects,
    });
    return render(<Sidebar />);
  };

  it("renders the current row even when it sits past the cap", () => {
    // Newest-first: task-9 leads, so task-2 is the 8th row — well past cap=6.
    const sid = "20260712-000000-task-2";
    const { container } = mount(sid);
    const rows = [...container.querySelectorAll(".project-task-wrap")];
    // The cap still holds for everyone else: 6 capped rows + the current one.
    expect(rows.length).toBe(7);
    const current = container.querySelector(".project-task-wrap.current");
    expect(current).toBeTruthy();
    expect(current!.textContent).toContain("Task 2");
    // …and "Show more" still offers the rest.
    expect(container.querySelector(".show-more")).toBeTruthy();
  });

  it("un-folds the group holding the current row without persisting the change", () => {
    const toggleProjectFolded = vi.fn();
    useStore.setState({ toggleProjectFolded });
    const { container } = mount("20260712-000000-task-2", { "/repo/app": { folded: true } });
    expect(container.querySelector(".project-task-wrap.current")).toBeTruthy();
    // The fold stays the user's: nothing writes it back.
    expect(toggleProjectFolded).not.toHaveBeenCalled();
  });

  it("keeps a folded group collapsed when the current task lives elsewhere", () => {
    const { container } = mount(null, { "/repo/app": { folded: true } });
    expect(container.querySelectorAll(".project-task-wrap").length).toBe(0);
  });
});

describe("New task shortcut badge (RH-4)", () => {
  it("badges the New task row with the key the app actually binds", () => {
    useStore.setState({ sessions: [] });
    const { container } = render(<Sidebar />);
    const badge = container.querySelector(".primary-nav .nav-kbd");
    expect(badge).toBeTruthy();

    // The badge must render the same tokens the shortcut catalog registers for
    // New task — that catalog is what Settings → Keyboard shortcuts shows, and
    // App.tsx is what actually fires. One string, three surfaces.
    const registered = SHORTCUT_GROUPS.find((g) => g.title === "Global")!.items.find(
      (i) => i.label === "New task",
    );
    expect(registered).toBeTruthy();
    expect(badge!.textContent).toBe(registered!.keys.map(keyLabel).join(""));
    // …and it sits on the New task row, not on Scheduled.
    expect(badge!.closest("button")!.textContent).toContain("New task");
  });
});

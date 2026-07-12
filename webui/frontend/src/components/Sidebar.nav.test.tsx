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

describe("Projects section truncation + group fold (SB-4)", () => {
  // 12 projects, one task each. Ids are creation stamps, so p11 is the newest
  // group and p00 the oldest — the section renders p11…p04 (8) and hides the
  // last four behind Show more.
  const spread = Array.from({ length: 12 }, (_v, i) => ({
    id: `20260701-0000${String(i).padStart(2, "0")}-task`,
    status: "idle",
    turns: 1,
    title: `Task ${i}`,
    workspace: `/repo/p${String(i).padStart(2, "0")}`,
  }));

  const mount = (over: Record<string, any> = {}) => {
    useStore.setState({
      sessions: spread as any,
      sessionsReady: true,
      currentSid: null,
      archived: [],
      pinned: [],
      unread: [],
      renames: {},
      projects: {},
      toggleProjectFolded: vi.fn(),
      ...over,
    });
    return render(<Sidebar />);
  };

  const headings = (container: HTMLElement) =>
    [...container.querySelectorAll(".project-group")].map(
      (group) => group.querySelector(".project-heading")!.textContent,
    );

  afterEach(() => localStorage.clear());

  it("renders only the 8 newest project groups, with the rest behind Show more", () => {
    const { container } = mount();
    expect(container.querySelectorAll(".project-group")).toHaveLength(8);
    expect(headings(container)[0]).toContain("p11");
    expect(headings(container)[7]).toContain("p04");

    const showMore = container.querySelector(".projects-show-more")!;
    expect(showMore.textContent).toContain("Show more");
    // The four withheld groups are counted, not silently dropped.
    expect(showMore.textContent).toContain("4");
  });

  it("Show more reveals every group; Show less puts them back", () => {
    const { container } = mount();
    fireEvent.click(container.querySelector(".projects-show-more")!);
    expect(container.querySelectorAll(".project-group")).toHaveLength(12);
    expect(headings(container)[11]).toContain("p00");

    const showLess = container.querySelector(".projects-show-more")!;
    expect(showLess.textContent).toContain("Show less");
    fireEvent.click(showLess);
    expect(container.querySelectorAll(".project-group")).toHaveLength(8);
  });

  it("collapsing a group hides its tasks and persists across a remount", () => {
    const { container } = mount();
    const first = container.querySelector(".project-group")!;
    expect(first.querySelectorAll(".project-task-wrap")).toHaveLength(1);
    fireEvent.click(first.querySelector(".project-heading")!);
    expect(first.querySelectorAll(".project-task-wrap")).toHaveLength(0);
    // The heading itself survives — a collapsed group is one row, not zero.
    expect(first.querySelector(".project-heading")!.textContent).toContain("p11");
    expect(JSON.parse(localStorage.getItem("ar.sidebar.collapsedProjects")!)).toEqual(["/repo/p11"]);

    // Refresh: the fold is restored from localStorage alone (the server overlay
    // is empty here — toggleProjectFolded is stubbed), i.e. no round-trip needed.
    cleanup();
    const remounted = mount().container;
    const again = remounted.querySelector(".project-group")!;
    expect(again.querySelector(".project-heading")!.textContent).toContain("p11");
    expect(again.querySelectorAll(".project-task-wrap")).toHaveLength(0);
    // Its neighbours are untouched.
    expect([...remounted.querySelectorAll(".project-group")][1].querySelectorAll(".project-task-wrap")).toHaveLength(1);
  });

  it("re-expands a collapsed group on a second click and clears it from storage", () => {
    const { container } = mount();
    const heading = container.querySelector(".project-heading")!;
    fireEvent.click(heading);
    fireEvent.click(heading);
    expect(container.querySelector(".project-group")!.querySelectorAll(".project-task-wrap")).toHaveLength(1);
    expect(JSON.parse(localStorage.getItem("ar.sidebar.collapsedProjects")!)).toEqual([]);
  });

  it("keeps the current task's group rendered and expanded even past the limit", () => {
    // p00 is the 12th group — beyond the 8 the section shows — *and* collapsed.
    localStorage.setItem("ar.sidebar.collapsedProjects", JSON.stringify(["/repo/p00"]));
    const { container } = mount({ currentSid: "20260701-000000-task" });

    // 8 + the current group, appended at the tail so the top never shuffles.
    const groups = [...container.querySelectorAll(".project-group")];
    expect(groups).toHaveLength(9);
    expect(headings(container)[8]).toContain("p00");

    // …and its row is on the rail: the fold cannot hide where you are.
    const current = container.querySelector(".project-task-wrap.current");
    expect(current).toBeTruthy();
    expect(current!.textContent).toContain("Task 0");
    expect(groups[8].contains(current!)).toBe(true);
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

describe("footer says the product name once (SB-12)", () => {
  it("the account badge carries daemon status only — the wordmark owns the name", () => {
    useStore.setState({ sessions: [], health: { daemonUp: true, version: "ar 1.2.3" } as any });
    const { container } = render(<Sidebar />);

    const badge = container.querySelector(".account-badge")!;
    expect(badge.textContent).toMatch(/^AR\s*Connected/);
    // The product name is on the brand row, and only there.
    expect(badge.textContent).not.toContain("AgentRunner");
    expect(container.querySelector(".account-meta b")).toBeNull();
    expect(container.querySelector(".brand-main")!.textContent).toBe("AgentRunner");
  });

  it("keeps the offline line red-and-clickable after the trim", () => {
    useStore.setState({ sessions: [], health: { daemonUp: false } as any });
    const { container } = render(<Sidebar />);

    // The red styling hangs off `.account-avatar.offline + .account-meta` — if
    // the meta column ever stops being the avatar's next sibling, the outage
    // goes silently grey. (The click → restart path is covered in
    // loadingStates.test.tsx.)
    const avatar = container.querySelector(".account-avatar.offline")!;
    expect(avatar.nextElementSibling!.className).toContain("account-meta");
    expect(avatar.nextElementSibling!.textContent).toContain("Daemon offline — restart");
  });
});

describe("brand row is a wordmark, not a logo tile (SB-13)", () => {
  it("drops the filled accent square from the brand button", () => {
    useStore.setState({ sessions: [] });
    const { container } = render(<Sidebar />);
    const brand = container.querySelector(".brand-main")!;
    expect(brand.querySelector("svg")).toBeNull();
    expect(brand.innerHTML).not.toContain("bg-accent");
  });
});

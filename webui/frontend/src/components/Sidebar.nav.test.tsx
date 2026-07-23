// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// The sidebar hits /health and /git on mount; nothing here depends on those, so
// stub the module with never-settling promises (same pattern as loadingStates).
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy({}, { get: () => () => new Promise(() => {}) }),
}));

import { Sidebar } from "./Sidebar";
import { SHORTCUT_GROUPS, keyLabel } from "../shortcuts";
import {
  clampSidebarWidth,
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
  useStore,
} from "../store";

afterEach(cleanup);

describe("sidebar search entry point (RH-5)", () => {
  it("magnifier opens the ⌘K command palette instead of an inline filter", () => {
    const onOpenPalette = vi.fn();
    const { container } = render(<Sidebar onOpenPalette={onOpenPalette} />);
    fireEvent.click(screen.getByLabelText("Search sessions"));
    expect(onOpenPalette).toHaveBeenCalled();
    // The second search surface is gone — one entry point, not two.
    expect(container.querySelector(".side-search")).toBeNull();
    expect(screen.queryByPlaceholderText(/Search title, id, or workspace/)).toBeNull();
  });
});

describe("mobile sidebar dismissal", () => {
  it("offers a direct close control in addition to the outside scrim", () => {
    const onHide = vi.fn();
    render(<Sidebar onHide={onHide} />);
    fireEvent.click(screen.getByRole("button", { name: "Close sidebar" }));
    expect(onHide).toHaveBeenCalledTimes(1);
  });

  it("keeps the row menu-free while preserving the complete right-click menu", () => {
    const select = vi.fn();
    useStore.setState({
      sessions: [{
        id: "20260712-120000-mobile-actions",
        status: "idle",
        turns: 1,
        title: "Mobile actions",
        workspace: "/repo/mobile",
      }] as any,
      sessionsReady: true,
      currentSid: null,
      archived: [],
      pinned: [],
      unread: [],
      renames: {},
      projects: {},
      select,
    });
    const { container } = render(<Sidebar />);

    const row = container.querySelector(".project-session-wrap")!;
    expect(row.querySelector(".menu-trigger")).toBeNull();
    expect(container.querySelector(".session-open")).toBeNull();
    expect(row.querySelector(".session-quick-actions")).toBeTruthy();

    fireEvent.contextMenu(row, { clientX: 20, clientY: 30 });
    const labels = screen.getAllByRole("menuitem").map((item) => item.textContent?.trim());
    expect(labels).toEqual(expect.arrayContaining(["Pin", "Rename…", "Mark as unread", "Archive"]));
    expect(labels).not.toContain("Session ID");
    expect(labels).not.toContain("Session link");
    // Opening management actions must not also navigate into the session.
    expect(select).not.toHaveBeenCalled();
  });
});

describe("sidebar session row states and hover actions (INC-92)", () => {
  const managed = {
    id: "20260722-120000-managed",
    status: "running",
    turns: 2,
    title: "Managed worktree session",
    workspace: "/Users/test/.local/share/agentrunner/worktrees/app-main-20260722-120000",
  };
  const local = {
    id: "20260722-110000-local",
    status: "waiting:input",
    turns: 1,
    title: "Local session",
    workspace: "/repo/local",
  };

  const mount = (over: Record<string, any> = {}) => {
    useStore.setState({
      sessions: [managed, local] as any,
      sessionsReady: true,
      currentSid: managed.id,
      archived: [],
      showArchived: true,
      pinned: [],
      unread: [],
      renames: {},
      projects: {},
      toggleProjectFolded: vi.fn(),
      togglePin: (sid: string) => useStore.setState((state) => ({
        pinned: state.pinned.includes(sid) ? state.pinned.filter((id) => id !== sid) : [...state.pinned, sid],
      })),
      toggleArchive: (sid: string) => useStore.setState((state) => ({
        archived: state.archived.includes(sid) ? state.archived.filter((id) => id !== sid) : [...state.archived, sid],
      })),
      ...over,
    });
    return render(<Sidebar />);
  };

  afterEach(() => localStorage.clear());

  it("shows managed-worktree and running state without a row ellipsis", () => {
    const { container } = mount();
    const rows = [...container.querySelectorAll(".project-session-wrap")];
    const worktreeRow = rows.find((row) => row.textContent?.includes("Managed worktree session"))!;
    const localRow = rows.find((row) => row.textContent?.includes("Local session"))!;

    expect(worktreeRow.classList.contains("current")).toBe(true);
    expect(worktreeRow.querySelector('[aria-label="Worktree session"]')).toBeTruthy();
    expect(worktreeRow.querySelector('[aria-label="Session running"]')).toBeTruthy();
    expect(worktreeRow.querySelector(".session-state-icons.running")).toBeTruthy();
    expect(localRow.querySelector('[aria-label="Worktree session"]')).toBeNull();
    expect(localRow.querySelector('[aria-label="Session running"]')).toBeNull();
    expect(worktreeRow.querySelector(".menu-trigger")).toBeNull();
    expect(localRow.querySelector(".menu-trigger")).toBeNull();
  });

  it("switches the reversible hover actions between pin/archive states", () => {
    const { container } = mount();
    let row = [...container.querySelectorAll(".project-session-wrap")].find((item) => item.textContent?.includes("Managed worktree session"))!;
    expect(row.querySelector(".session-quick-actions")).toBeTruthy();
    fireEvent.mouseEnter(row);

    fireEvent.click(row.querySelector('button[aria-label="Pin Managed worktree session"]')!);
    row = [...container.querySelectorAll(".project-session-wrap")].find((item) => item.textContent?.includes("Managed worktree session"))!;
    expect(row.querySelector('button[aria-label="Unpin Managed worktree session"]')).toBeTruthy();

    fireEvent.click(row.querySelector('button[aria-label="Archive Managed worktree session"]')!);
    row = [...container.querySelectorAll(".project-session-wrap")].find((item) => item.textContent?.includes("Managed worktree session"))!;
    expect(row.querySelector('button[aria-label="Unarchive Managed worktree session"]')).toBeTruthy();
    expect(row.querySelector('[aria-label="Session running"]')).toBeTruthy();
  });

  it("opens the same complete menu from right-click and Shift+F10, then returns focus on Escape", async () => {
    const { container } = mount();
    const row = [...container.querySelectorAll(".project-session-wrap")].find((item) => item.textContent?.includes("Local session"))!;

    fireEvent.contextMenu(row, { clientX: 30, clientY: 40 });
    expect(screen.getAllByRole("menuitem").map((item) => item.textContent?.trim())).toEqual([
      "Pin", "Rename…", "Mark as unread", "Archive",
    ]);
    fireEvent.keyDown(document, { key: "Escape" });

    const opener = row.querySelector<HTMLButtonElement>(".project-session")!;
    opener.focus();
    fireEvent.keyDown(opener, { key: "F10", shiftKey: true });
    expect(screen.getAllByRole("menuitem").map((item) => item.textContent?.trim())).toEqual([
      "Pin", "Rename…", "Mark as unread", "Archive",
    ]);
    await waitFor(() => expect(document.activeElement).toBe(screen.getAllByRole("menuitem")[0]));
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});

describe("mobile sidebar chrome touch targets", () => {
  it("keeps the top and footer icon controls at least 44px without growing the brand row", () => {
    useStore.setState({ sessions: [] });
    const { container } = render(<Sidebar />);

    const controls = [
      screen.getByRole("button", { name: "Search sessions" }),
      screen.getByRole("button", { name: "Close sidebar" }),
      screen.getByRole("button", { name: "More options" }),
    ];
    for (const control of controls) {
      expect(control.className).toContain("max-[900px]:w-[44px]!");
      expect(control.className).toContain("max-[900px]:h-[44px]!");
    }

    const brandRow = container.querySelector(".brand-main")!.parentElement!;
    expect(brandRow.className).toContain("min-h-[44px]");
    expect(brandRow.className).toContain("max-[900px]:pt-0!");
    expect(brandRow.className).toContain("max-[900px]:pb-0!");
  });
});

describe("current session visibility (SB-1)", () => {
  // Ten sessions in one project: with cap=6, s3…s9 (ids sort newest-first) fall
  // behind "Show more". Opening one of them must still put it on the rail.
  const manySessions = Array.from({ length: 10 }, (_v, i) => ({
    id: `2026071${i}-000000-session-${i}`,
    status: "idle",
    turns: 1,
    title: `Session ${i}`,
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

  afterEach(() => localStorage.clear());

  it("renders the current row even when it sits past the cap", () => {
    // Newest-first: session-9 leads, so session-2 is the 8th row — well past cap=6.
    const sid = "20260712-000000-session-2";
    const { container } = mount(sid);
    const rows = [...container.querySelectorAll(".project-session-wrap")];
    // The cap still holds for everyone else: 6 capped rows + the current one.
    expect(rows.length).toBe(7);
    const current = container.querySelector(".project-session-wrap.current");
    expect(current).toBeTruthy();
    expect(current!.textContent).toContain("Session 2");
    // …and "Show more" still offers the rest.
    expect(container.querySelector(".show-more")).toBeTruthy();
  });

  it("keeps a persisted fold even when the group holds the current session", () => {
    const toggleProjectFolded = vi.fn();
    useStore.setState({ toggleProjectFolded });
    const { container } = mount("20260712-000000-session-2", { "/repo/app": { folded: true } });
    expect(container.querySelector(".project-heading")!.getAttribute("aria-expanded")).toBe("false");
    expect(container.querySelector(".project-session-wrap.current")).toBeNull();
    expect(toggleProjectFolded).not.toHaveBeenCalled();
  });

  it("lets the current session's project collapse and expand again", () => {
    const toggleProjectFolded = vi.fn();
    useStore.setState({ toggleProjectFolded });
    const { container } = mount("20260712-000000-session-2");
    const heading = container.querySelector(".project-heading")!;

    expect(heading.getAttribute("aria-expanded")).toBe("true");
    expect(container.querySelector(".project-session-wrap.current")).toBeTruthy();

    fireEvent.click(heading);
    expect(heading.getAttribute("aria-expanded")).toBe("false");
    expect(container.querySelector(".project-session-wrap.current")).toBeNull();
    expect(JSON.parse(localStorage.getItem("ar.sidebar.collapsedProjects")!)).toEqual(["/repo/app"]);
    expect(toggleProjectFolded).toHaveBeenLastCalledWith("/repo/app", true);

    fireEvent.click(heading);
    expect(heading.getAttribute("aria-expanded")).toBe("true");
    expect(container.querySelector(".project-session-wrap.current")).toBeTruthy();
    expect(toggleProjectFolded).toHaveBeenLastCalledWith("/repo/app", false);
  });

  it("keeps a folded group collapsed when the current session lives elsewhere", () => {
    const { container } = mount(null, { "/repo/app": { folded: true } });
    expect(container.querySelectorAll(".project-session-wrap").length).toBe(0);
  });
});

describe("Projects section truncation + group fold (SB-4)", () => {
  // 12 projects, one session each. Ids are creation stamps, so p11 is the newest
  // group and p00 the oldest — the section renders p11…p04 (8) and hides the
  // last four behind Show more.
  const spread = Array.from({ length: 12 }, (_v, i) => ({
    id: `20260701-0000${String(i).padStart(2, "0")}-session`,
    status: "idle",
    turns: 1,
    title: `Session ${i}`,
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

  it("collapsing a group hides its sessions and persists across a remount", () => {
    const { container } = mount();
    const first = container.querySelector(".project-group")!;
    expect(first.querySelectorAll(".project-session-wrap")).toHaveLength(1);
    fireEvent.click(first.querySelector(".project-heading")!);
    expect(first.querySelectorAll(".project-session-wrap")).toHaveLength(0);
    // The heading itself survives — a collapsed group is one row, not zero.
    expect(first.querySelector(".project-heading")!.textContent).toContain("p11");
    expect(JSON.parse(localStorage.getItem("ar.sidebar.collapsedProjects")!)).toEqual(["/repo/p11"]);

    // Refresh: the fold is restored from localStorage alone (the server overlay
    // is empty here — toggleProjectFolded is stubbed), i.e. no round-trip needed.
    cleanup();
    const remounted = mount().container;
    const again = remounted.querySelector(".project-group")!;
    expect(again.querySelector(".project-heading")!.textContent).toContain("p11");
    expect(again.querySelectorAll(".project-session-wrap")).toHaveLength(0);
    // Its neighbours are untouched.
    expect([...remounted.querySelectorAll(".project-group")][1].querySelectorAll(".project-session-wrap")).toHaveLength(1);
  });

  it("re-expands a collapsed group on a second click and clears it from storage", () => {
    const { container } = mount();
    const heading = container.querySelector(".project-heading")!;
    fireEvent.click(heading);
    fireEvent.click(heading);
    expect(container.querySelector(".project-group")!.querySelectorAll(".project-session-wrap")).toHaveLength(1);
    expect(JSON.parse(localStorage.getItem("ar.sidebar.collapsedProjects")!)).toEqual([]);
  });

  it("keeps the current session's project heading rendered past the limit without overriding its fold", () => {
    // p00 is the 12th group — beyond the 8 the section shows — *and* collapsed.
    localStorage.setItem("ar.sidebar.collapsedProjects", JSON.stringify(["/repo/p00"]));
    const { container } = mount({ currentSid: "20260701-000000-session" });

    // 8 + the current group, appended at the tail so the top never shuffles.
    const groups = [...container.querySelectorAll(".project-group")];
    expect(groups).toHaveLength(9);
    expect(headings(container)[8]).toContain("p00");

    // The user's fold still wins: only the current project heading is anchored.
    const heading = groups[8].querySelector(".project-heading")!;
    expect(heading.getAttribute("aria-expanded")).toBe("false");
    expect(groups[8].querySelector(".project-session-wrap.current")).toBeNull();

    fireEvent.click(heading);
    const current = groups[8].querySelector(".project-session-wrap.current");
    expect(current).toBeTruthy();
    expect(current!.textContent).toContain("Session 0");
  });
});

describe("project group icon is always a closed folder (SIDEBAR-FOLDER-ICON)", () => {
  // Two real-workspace groups so both render a heading; one is collapsed via
  // localStorage, the other expanded. Codex's gold keeps the closed Folder on
  // every group regardless of fold — expanded state rides the caret alone.
  const twoGroups = [
    { id: "20260710-000000-a", status: "idle", turns: 1, title: "Session A", workspace: "/repo/aaa" },
    { id: "20260709-000000-b", status: "idle", turns: 1, title: "Session B", workspace: "/repo/bbb" },
  ];

  const mount = () => {
    useStore.setState({
      sessions: twoGroups as any,
      sessionsReady: true,
      currentSid: null,
      archived: [],
      pinned: [],
      unread: [],
      renames: {},
      projects: {},
      toggleProjectFolded: vi.fn(),
    });
    return render(<Sidebar />);
  };

  afterEach(() => localStorage.clear());

  it("renders the identical closed-folder icon whether a group is expanded or collapsed", () => {
    // Collapse the newest group (/repo/aaa) and leave the other expanded.
    localStorage.setItem("ar.sidebar.collapsedProjects", JSON.stringify(["/repo/aaa"]));
    const { container } = mount();

    const groups = [...container.querySelectorAll(".project-group")];
    expect(groups).toHaveLength(2);

    const collapsed = groups.find((g) => g.querySelectorAll(".project-session-wrap").length === 0)!;
    const expanded = groups.find((g) => g.querySelectorAll(".project-session-wrap").length > 0)!;
    expect(collapsed).toBeTruthy();
    expect(expanded).toBeTruthy();

    const collapsedFolder = collapsed.querySelector(".proj-folder")!;
    const expandedFolder = expanded.querySelector(".proj-folder")!;
    expect(collapsedFolder).toBeTruthy();
    expect(expandedFolder).toBeTruthy();
    // FolderOpen and Folder differ in their SVG paths, so an identical icon
    // markup on both states proves we never swap to FolderOpen when expanded.
    expect(expandedFolder.innerHTML).toBe(collapsedFolder.innerHTML);

    // The fold state is carried by the caret instead: open on the expanded group.
    expect(expanded.querySelector(".proj-caret.open")).toBeTruthy();
    expect(collapsed.querySelector(".proj-caret.open")).toBeNull();
    expect(collapsed.querySelector(".proj-caret")).toBeTruthy();
  });
});

describe("project hover and management controls (INC-87)", () => {
  const projectSessions = [
    { id: "20260721-120000-app", status: "idle", turns: 2, title: "App chat", workspace: "/repo/app" },
    { id: "20260720-120000-app", status: "idle", turns: 1, title: "Older app chat", workspace: "/repo/app" },
    { id: "20260719-120000-lib", status: "idle", turns: 1, title: "Lib chat", workspace: "/repo/lib" },
  ];

  const mount = (over: Record<string, any> = {}) => {
    useStore.setState({
      sessions: projectSessions as any,
      sessionsReady: true,
      currentSid: null,
      archived: [],
      pinned: [],
      unread: [],
      renames: {},
      projects: {},
      modal: null,
      prompt: null,
      toggleProjectFolded: vi.fn(),
      toggleProjectPinned: vi.fn(),
      setProjectRemoved: vi.fn(),
      newSessionForProject: vi.fn(),
      openModal: (modal: any) => useStore.setState({ modal }),
      openPrompt: (prompt: any) => useStore.setState({ prompt }),
      ...over,
    });
    return render(<Sidebar />);
  };

  afterEach(() => localStorage.clear());

  it("shows project summary plus menu and new-chat controls on the heading row", () => {
    const { container } = mount();
    const app = [...container.querySelectorAll(".project-group")].find((group) => group.textContent?.includes("App chat"))!;
    const headingRow = app.querySelector(".project-heading-row")!;
    fireEvent.mouseEnter(headingRow);

    const preview = container.querySelector(".project-preview")!;
    expect(preview.textContent).toContain("app");
    expect(preview.textContent).toContain("2 chats");
    expect(preview.textContent).toContain("/repo/app");
    expect(screen.getByRole("button", { name: "More actions for app" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "New chat in app" })).toBeTruthy();
    const actions = headingRow.querySelector(".project-heading-actions")!;
    expect(actions.contains(screen.getByRole("button", { name: "More actions for app" }))).toBe(true);
    expect(actions.contains(screen.getByRole("button", { name: "New chat in app" }))).toBe(true);
  });

  it("renders the six requested project actions from the visible menu trigger", () => {
    mount();
    fireEvent.click(screen.getByRole("button", { name: "More actions for app" }));
    expect(screen.getAllByRole("menuitem").map((item) => item.textContent?.trim())).toEqual([
      "Pin project",
      "Reveal in Finder",
      "Create permanent worktree",
      "Rename project",
      "Archive chats",
      "Remove",
    ]);
  });

  it("returns focus to the project heading after its keyboard context menu closes with Escape", async () => {
    const { container } = mount();
    const heading = container.querySelector<HTMLButtonElement>(".project-heading")!;
    heading.focus();

    fireEvent.keyDown(heading, { key: "F10", shiftKey: true });
    await waitFor(() => expect(document.activeElement).toBe(screen.getAllByRole("menuitem")[0]));
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it("keeps pinned projects first while preserving recency inside each partition", () => {
    const { container } = mount({ projects: { "/repo/lib": { pinned: true } } });
    const headings = [...container.querySelectorAll(".project-heading")].map((heading) => heading.textContent?.trim());
    expect(headings).toEqual(["lib", "app"]);
  });

  it("removes only the rail projection and offers an explicit restore path", async () => {
    const setProjectRemoved = vi.fn(async (key: string, removed: boolean) => {
      const current = useStore.getState().projects;
      useStore.setState({ projects: { ...current, [key]: { ...current[key], removed } } });
    });
    const { container } = mount({ setProjectRemoved });
    fireEvent.click(screen.getByRole("button", { name: "More actions for app" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Remove" }));

    const modal = useStore.getState().modal;
    expect(modal?.kind).toBe("confirm");
    if (modal?.kind !== "confirm") throw new Error("confirm modal not opened");
    expect(modal.body).toContain("chats, journal, and files stay intact");
    await act(async () => { await modal.onConfirm(); });

    expect(container.textContent).not.toContain("App chat");
    expect(useStore.getState().sessions).toHaveLength(3);
    fireEvent.click(screen.getByRole("button", { name: /Show removed projects/ }));
    expect(container.textContent).toContain("App chat");
    expect(screen.getByRole("button", { name: "More actions for app" })).toBeTruthy();
  });

  it("starts a project-scoped chat while rename remains in the menu", () => {
    const newSessionForProject = vi.fn();
    mount({ newSessionForProject });

    fireEvent.click(screen.getByRole("button", { name: "New chat in app" }));
    expect(newSessionForProject).toHaveBeenCalledWith("/repo/app");

    fireEvent.click(screen.getByRole("button", { name: "More actions for app" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Rename project" }));
    expect(useStore.getState().prompt?.title).toBe("Rename project");
  });

  it("opens the existing worktree prompt flow", () => {
    mount();
    fireEvent.click(screen.getByRole("button", { name: "More actions for app" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Create permanent worktree" }));
    expect(useStore.getState().prompt?.title).toBe("Create permanent worktree");
  });

  it("allows duplicate project names without a resting path subtitle", () => {
    const duplicateNames = [
      { id: "20260722-140000-a", status: "idle", turns: 1, title: "Alpha", workspace: "/Users/a/workspace" },
      { id: "20260722-130000-b", status: "idle", turns: 1, title: "Beta", workspace: "/Users/b/workspace" },
    ];
    const { container } = mount({ sessions: duplicateNames });
    const groups = [...container.querySelectorAll(".project-group")];

    expect(groups).toHaveLength(2);
    expect(groups.map((group) => group.querySelector(".proj-heading-name")?.textContent)).toEqual(["workspace", "workspace"]);
    expect(container.querySelector(".project-hint")).toBeNull();
    expect(groups.map((group) => group.querySelector(".project-heading")?.getAttribute("title"))).toEqual([
      "/Users/a/workspace",
      "/Users/b/workspace",
    ]);

    fireEvent.mouseEnter(groups[0].querySelector(".project-heading-row")!);
    expect(container.querySelector(".project-preview-path")?.textContent).toContain("/Users/a/workspace");
    fireEvent.mouseLeave(groups[0].querySelector(".project-heading-row")!);
    fireEvent.mouseEnter(groups[1].querySelector(".project-heading-row")!);
    expect(container.querySelector(".project-preview-path")?.textContent).toContain("/Users/b/workspace");
  });
});

describe("sidebar section folding and resize (INC-87)", () => {
  const sessions = [
    { id: "20260721-130000-pinned", status: "idle", turns: 1, title: "Pinned chat", workspace: "/repo/app" },
    { id: "20260721-120000-project", status: "idle", turns: 1, title: "Project chat", workspace: "/repo/app" },
  ];

  const mount = () => {
    useStore.setState({
      sessions: sessions as any,
      sessionsReady: true,
      currentSid: null,
      archived: [],
      pinned: [sessions[0].id],
      unread: [],
      renames: {},
      projects: {},
      sidebarWidth: SIDEBAR_DEFAULT_WIDTH,
      setSidebarWidth: (width: number) => {
        const next = clampSidebarWidth(width);
        localStorage.setItem("arwebui.sidebarWidth", String(next));
        useStore.setState({ sidebarWidth: next });
      },
      toggleProjectFolded: vi.fn(),
    });
    return render(<Sidebar />);
  };

  afterEach(() => {
    localStorage.clear();
    document.body.classList.remove("sidebar-resizing");
  });

  it("folds Pinned and Projects independently and restores both folds after remount", () => {
    localStorage.clear();
    const { container } = mount();
    const pinnedToggle = screen.getByRole("button", { name: "Pinned" });
    const projectsToggle = screen.getByRole("button", { name: "Projects" });
    expect(pinnedToggle.getAttribute("aria-expanded")).toBe("true");
    expect(projectsToggle.getAttribute("aria-expanded")).toBe("true");

    fireEvent.click(pinnedToggle);
    expect(container.querySelector(".pinned-section .project-session-wrap")).toBeNull();
    expect(container.querySelector(".projects-section .project-session-wrap")).toBeTruthy();
    fireEvent.click(projectsToggle);
    expect(container.querySelector(".projects-section .project-group")).toBeNull();
    expect(JSON.parse(localStorage.getItem("ar.sidebar.foldedSections")!)).toEqual(["pinned", "projects"]);

    cleanup();
    const remounted = mount().container;
    expect(remounted.querySelector(".pinned-section .project-session-wrap")).toBeNull();
    expect(remounted.querySelector(".projects-section .project-group")).toBeNull();
  });

  it("supports pointer, keyboard and reset resizing with hard clamps", () => {
    localStorage.clear();
    mount();
    expect(SIDEBAR_DEFAULT_WIDTH).toBe(320);
    const handle = screen.getByRole("separator", { name: "Resize sidebar" });
    expect(handle.className).toContain("max-[900px]:hidden!");

    const pointer = (type: string, clientX: number) => {
      const event = new Event(type, { bubbles: true });
      Object.defineProperties(event, {
        button: { value: 0 },
        clientX: { value: clientX },
      });
      return event;
    };
    fireEvent(handle, pointer("pointerdown", SIDEBAR_DEFAULT_WIDTH));
    fireEvent(window, pointer("pointermove", 999));
    fireEvent(window, pointer("pointerup", 999));
    expect(useStore.getState().sidebarWidth).toBe(SIDEBAR_MAX_WIDTH);
    expect(localStorage.getItem("arwebui.sidebarWidth")).toBe(String(SIDEBAR_MAX_WIDTH));

    fireEvent.keyDown(handle, { key: "Home" });
    expect(useStore.getState().sidebarWidth).toBe(SIDEBAR_MIN_WIDTH);
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(useStore.getState().sidebarWidth).toBe(SIDEBAR_MIN_WIDTH + 16);
    fireEvent.keyDown(handle, { key: "End" });
    expect(useStore.getState().sidebarWidth).toBe(SIDEBAR_MAX_WIDTH);
    fireEvent.doubleClick(handle);
    expect(useStore.getState().sidebarWidth).toBe(SIDEBAR_DEFAULT_WIDTH);
  });

  it("clamps invalid and out-of-range persisted values", () => {
    expect(clampSidebarWidth(Number.NaN)).toBe(SIDEBAR_DEFAULT_WIDTH);
    expect(clampSidebarWidth(40)).toBe(SIDEBAR_MIN_WIDTH);
    expect(clampSidebarWidth(900)).toBe(SIDEBAR_MAX_WIDTH);
  });
});

describe("New session shortcut discoverability (RH-4)", () => {
  it("keeps nav rows badge-free — the shortcut lives on the row's title, not a resting pill", () => {
    useStore.setState({ sessions: [] });
    const { container } = render(<Sidebar />);
    // Golden Codex: every primary-nav row is a clean icon+label with no resting
    // shortcut badge. The key stays discoverable via title= and the ⌘K palette.
    expect(container.querySelector(".primary-nav .nav-kbd")).toBeNull();

    // The New session row's title must carry the same tokens the shortcut catalog
    // registers — that catalog is what Settings → Keyboard shortcuts shows, and
    // App.tsx is what actually fires. One string, three surfaces.
    const registered = SHORTCUT_GROUPS.find((g) => g.title === "Global")!.items.find(
      (i) => i.label === "New session",
    );
    expect(registered).toBeTruthy();
    const tokens = registered!.keys.map(keyLabel).join("");
    const newSessionBtn = Array.from(
      container.querySelectorAll<HTMLButtonElement>(".primary-nav button"),
    ).find((b) => b.textContent?.includes("New session"));
    expect(newSessionBtn).toBeTruthy();
    expect(newSessionBtn!.getAttribute("title")).toContain(tokens);
  });
});

describe("New session is chromeless on the home landing (NAV-NEWSESSION-ACTIVE-FILL)", () => {
  const navButton = (container: HTMLElement, label: string) =>
    Array.from(
      container.querySelectorAll<HTMLButtonElement>(".primary-nav button"),
    ).find((b) => b.textContent?.includes(label))!;

  it("does not paint the New session row active on the home page — only real pages get the fill", () => {
    // Home is the New-session landing, not a selected destination: Codex keeps
    // its new-session action chromeless there, so ours must carry no resting `active` fill.
    useStore.setState({ sessions: [], currentSid: null, currentPage: "home" });
    const { container } = render(<Sidebar />);

    const newSession = navButton(container, "New session");
    expect(newSession).toBeTruthy();
    expect(newSession.className).not.toContain("active");
  });

  it("keeps the fill for a real destination like Scheduled", () => {
    useStore.setState({ sessions: [], currentSid: null, currentPage: "scheduled" });
    const { container } = render(<Sidebar />);

    const scheduled = navButton(container, "Scheduled");
    expect(scheduled).toBeTruthy();
    expect(scheduled.className).toContain("active");
  });
});

describe("footer says the product name once (SB-12)", () => {
  it("renders connected as inert status and keeps the build only in its tooltip", () => {
    useStore.setState({ sessions: [], health: { daemonUp: true, version: "ar 1.2.3" } as any });
    const { container } = render(<Sidebar />);

    const badge = container.querySelector(".account-badge")!;
    expect(badge.tagName).toBe("DIV");
    expect(badge.getAttribute("role")).toBe("status");
    expect(badge.textContent).toMatch(/^AR\s*Connected$/);
    expect(badge.textContent).not.toContain("1.2.3");
    expect(badge.getAttribute("title")).toContain("1.2.3");
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
    expect(avatar.closest("button")).not.toBeNull();
    expect(avatar.nextElementSibling!.className).toContain("account-meta");
    expect(avatar.nextElementSibling!.textContent).toContain("Daemon offline — restart");
  });
});

describe("workspace-less sessions live in a flat Sessions section (SB-13)", () => {
  const mount = (sessions: any[], over: Record<string, any> = {}) => {
    useStore.setState({
      sessions: sessions as any,
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

  const section = (container: HTMLElement, cls: string) => container.querySelector(`.${cls}`);

  it("puts a workspace-less session under Sessions — no folder, no caret, no fake project", () => {
    const { container } = mount([
      { id: "20260710-000000-loose", status: "idle", turns: 1, title: "Loose session" },
      { id: "20260709-000000-repo", status: "idle", turns: 1, title: "Repo session", workspace: "/repo/app" },
    ]);

    // The fake folder is gone: no group is named "Other sessions", and every
    // project heading that *is* rendered names a real directory.
    const headings = [...container.querySelectorAll(".project-heading")].map((h) => h.textContent);
    expect(headings.join(" ")).not.toContain("Other sessions");
    expect(headings).toHaveLength(1);
    expect(headings[0]).toContain("app");

    const sessions = section(container, "sessions-section")!;
    expect(sessions).toBeTruthy();
    expect(sessions.querySelector(".section-label")!.textContent).toBe("Sessions");
    expect(sessions.textContent).toContain("Loose session");
    // Flat: the row sits at the Pinned indent (no `.nested`), and the section
    // carries none of the project chrome that asserts a directory.
    const row = sessions.querySelector(".project-session-wrap")!;
    expect(row.className).not.toContain("nested");
    expect(sessions.querySelector(".proj-folder")).toBeNull();
    expect(sessions.querySelector(".proj-caret")).toBeNull();
    expect(sessions.querySelector(".project-heading")).toBeNull();
  });

  it("renders no Sessions section at all when every session has a workspace", () => {
    const { container } = mount([
      { id: "20260709-000000-repo", status: "idle", turns: 1, title: "Repo session", workspace: "/repo/app" },
    ]);
    // An empty heading is worse than no heading.
    expect(section(container, "sessions-section")).toBeNull();
    expect(container.textContent).not.toContain("Other sessions");
  });

  it("shows typed ask/child approval blockers instead of Ready, even when unread", () => {
    const { container } = mount(
      [
        { id: "20260710-000003-combined", status: "waiting:input", turns: 1, title: "Combined blockers", attention: { approvals: 1, answers: 1 } },
        { id: "20260710-000002-approval", status: "waiting:input", turns: 1, title: "Child approval", attention: { approvals: 2 } },
        { id: "20260710-000001-answer", status: "waiting:input", turns: 1, title: "Structured ask", attention: { answers: 1 } },
        { id: "20260710-000000-failed", status: "failed", turns: 1, title: "Failed with blockers", attention: { approvals: 1, answers: 1 } },
      ],
      { unread: ["20260710-000002-approval"] },
    );
    const combined = container.querySelector<HTMLButtonElement>('button[aria-label^="Combined blockers"]')!;
    const approval = container.querySelector<HTMLButtonElement>('button[aria-label^="Child approval"]')!;
    const answer = container.querySelector<HTMLButtonElement>('button[aria-label^="Structured ask"]')!;
    const failed = container.querySelector<HTMLButtonElement>('button[aria-label^="Failed with blockers"]')!;
    expect(combined.getAttribute("aria-label")).toContain("2 actions needed");
    expect(combined.querySelector(".status-count")!.textContent).toBe("2");
    expect(combined.querySelector(".status-count")!.getAttribute("title")).toBe("2 actions needed");
    expect(approval.getAttribute("aria-label")).toContain("2 actions needed");
    expect(approval.querySelector(".status-count")!.textContent).toBe("2");
    expect(answer.getAttribute("aria-label")).toContain("Needs answer");
    expect(answer.querySelector(".status-dot")!.getAttribute("title")).toBe("Needs answer");
    expect(failed.getAttribute("aria-label")).toContain("Failed");
    expect(failed.querySelector(".status-dot")!.className).toBe("status-dot crash");
    expect(failed.querySelector(".status-count")).toBeNull();
  });

  it("keeps a pinned workspace-less session in Pinned only — never twice", () => {
    const { container } = mount(
      [
        { id: "20260710-000000-loose", status: "idle", turns: 1, title: "Loose session" },
        { id: "20260708-000000-other", status: "idle", turns: 1, title: "Other loose" },
      ],
      { pinned: ["20260710-000000-loose"] },
    );
    const pinned = section(container, "pinned-section")!;
    expect(pinned.textContent).toContain("Loose session");
    const sessions = section(container, "sessions-section")!;
    expect(sessions.textContent).toContain("Other loose");
    expect(sessions.textContent).not.toContain("Loose session");
    // …and exactly one row on the whole rail carries that title.
    const titles = [...container.querySelectorAll(".project-session-title")].map((t) => t.textContent);
    expect(titles.filter((t) => t === "Loose session")).toHaveLength(1);
  });

  it("caps the section at 6 rows and reveals the rest behind Show more / Show less", () => {
    const many = Array.from({ length: 9 }, (_v, i) => ({
      id: `2026071${i}-000000-loose-${i}`,
      status: "idle",
      turns: 1,
      title: `Loose ${i}`,
    }));
    const { container } = mount(many);
    const sessions = () => section(container, "sessions-section")!;
    expect(sessions().querySelectorAll(".project-session-wrap")).toHaveLength(6);

    const showMore = sessions().querySelector(".show-more")!;
    expect(showMore.textContent).toContain("Show more");
    expect(showMore.textContent).toContain("3"); // the withheld ones are counted
    fireEvent.click(showMore);
    expect(sessions().querySelectorAll(".project-session-wrap")).toHaveLength(9);

    const showLess = sessions().querySelector(".show-more")!;
    expect(showLess.textContent).toContain("Show less");
    fireEvent.click(showLess);
    expect(sessions().querySelectorAll(".project-session-wrap")).toHaveLength(6);
  });
});

describe("footer actions collapse into one overflow menu (SB-12)", () => {
  const mount = (over: Record<string, any> = {}) => {
    useStore.setState({ sessions: [], theme: "dark", ...over });
    return render(<Sidebar onOpenSettings={vi.fn()} />);
  };

  it("keeps the account row to identity + status: no loose icon buttons", () => {
    const { container } = mount();
    const foot = container.querySelector(".side-foot")!;
    // One badge, one `…` trigger — the Settings / Help / Theme buttons that used
    // to sit here are behind the trigger now.
    expect(foot.querySelector(".account-badge")).toBeTruthy();
    expect(foot.querySelector(".menu-trigger")).toBeTruthy();
    expect(screen.queryByLabelText("Open settings")).toBeNull();
    expect(screen.queryByLabelText("Help and keyboard shortcuts")).toBeNull();
    expect(screen.queryByLabelText("Toggle theme")).toBeNull();
    // The presence dot (and its offline/outage behaviour) stays on the row.
    expect(foot.querySelector(".account-presence")).toBeTruthy();
  });

  it("carries all three actions — with their shortcuts — inside the menu", () => {
    const onOpenSettings = vi.fn();
    const cycleTheme = vi.fn();
    const openHelp = vi.fn();
    useStore.setState({ sessions: [], theme: "dark", cycleTheme, openHelp });
    const { container } = render(<Sidebar onOpenSettings={onOpenSettings} />);

    fireEvent.click(container.querySelector(".side-foot .menu-trigger")!);
    const items = [...container.querySelectorAll(".side-foot [role='menuitem']")];
    expect(items).toHaveLength(3);
    const text = items.map((i) => i.textContent).join("|");
    expect(text).toContain("Settings");
    expect(text).toContain("Keyboard shortcuts & help");
    expect(text).toContain("Theme: dark");
    // The keys the icon buttons hid in tooltips are now on the rows themselves.
    expect(text).toContain(`${keyLabel("mod")},`);
    expect(items[0].getAttribute("title")).toBe("Settings (⌘,)");
    expect(items[1].getAttribute("title")).toBe("Keyboard shortcuts & help (?)");

    // Every action still fires.
    fireEvent.click(items[2]);
    expect(cycleTheme).toHaveBeenCalled();
    fireEvent.click(container.querySelector(".side-foot .menu-trigger")!);
    fireEvent.click([...container.querySelectorAll(".side-foot [role='menuitem']")][1]);
    expect(openHelp).toHaveBeenCalled();
    fireEvent.click(container.querySelector(".side-foot .menu-trigger")!);
    fireEvent.click([...container.querySelectorAll(".side-foot [role='menuitem']")][0]);
    expect(onOpenSettings).toHaveBeenCalled();
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

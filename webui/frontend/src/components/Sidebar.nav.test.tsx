// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";

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

  it("exposes session actions without hover or right-click", () => {
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

    const actions = screen.getByRole("button", { name: "More actions for Mobile actions" });
    expect(actions.closest("span")!.className).toContain("max-[900px]:inline-flex");
    expect(container.querySelector(".session-open")!.getAttribute("class")).toContain("max-[900px]:hidden!");
    expect(screen.getByRole("button", { name: "Pin session" }).className).toContain("max-[900px]:hidden!");
    expect(screen.getByRole("button", { name: "Archive session" }).className).toContain("max-[900px]:hidden!");

    fireEvent.click(actions);
    const labels = screen.getAllByRole("menuitem").map((item) => item.textContent);
    expect(labels).toEqual(expect.arrayContaining(["Pin", "Rename…", "Mark as unread", "Archive"]));
    // Opening management actions must not also navigate into the session.
    expect(select).not.toHaveBeenCalled();
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

  it("un-folds the group holding the current row without persisting the change", () => {
    const toggleProjectFolded = vi.fn();
    useStore.setState({ toggleProjectFolded });
    const { container } = mount("20260712-000000-session-2", { "/repo/app": { folded: true } });
    expect(container.querySelector(".project-session-wrap.current")).toBeTruthy();
    // The fold stays the user's: nothing writes it back.
    expect(toggleProjectFolded).not.toHaveBeenCalled();
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

  it("keeps the current session's group rendered and expanded even past the limit", () => {
    // p00 is the 12th group — beyond the 8 the section shows — *and* collapsed.
    localStorage.setItem("ar.sidebar.collapsedProjects", JSON.stringify(["/repo/p00"]));
    const { container } = mount({ currentSid: "20260701-000000-session" });

    // 8 + the current group, appended at the tail so the top never shuffles.
    const groups = [...container.querySelectorAll(".project-group")];
    expect(groups).toHaveLength(9);
    expect(headings(container)[8]).toContain("p00");

    // …and its row is on the rail: the fold cannot hide where you are.
    const current = container.querySelector(".project-session-wrap.current");
    expect(current).toBeTruthy();
    expect(current!.textContent).toContain("Session 0");
    expect(groups[8].contains(current!)).toBe(true);
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
      openModal: (modal: any) => useStore.setState({ modal }),
      openPrompt: (prompt: any) => useStore.setState({ prompt }),
      ...over,
    });
    return render(<Sidebar />);
  };

  afterEach(() => localStorage.clear());

  it("shows project summary plus menu and rename controls on the heading row", () => {
    const { container } = mount();
    const app = [...container.querySelectorAll(".project-group")].find((group) => group.textContent?.includes("App chat"))!;
    fireEvent.mouseEnter(app.querySelector(".project-heading-row")!);

    const preview = container.querySelector(".project-preview")!;
    expect(preview.textContent).toContain("app");
    expect(preview.textContent).toContain("2 chats");
    expect(preview.textContent).toContain("/repo/app");
    expect(screen.getByRole("button", { name: "More actions for app" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Rename project app" })).toBeTruthy();
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

  it("opens the existing worktree and rename prompt flows", () => {
    mount();
    fireEvent.click(screen.getByRole("button", { name: "More actions for app" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Create permanent worktree" }));
    expect(useStore.getState().prompt?.title).toBe("Create permanent worktree");

    fireEvent.click(screen.getByRole("button", { name: "Rename project app" }));
    expect(useStore.getState().prompt?.title).toBe("Rename project");
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

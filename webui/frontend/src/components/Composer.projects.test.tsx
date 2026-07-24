// @vitest-environment jsdom
//
// INC-41 HM-9 — the composer's project picker searches EVERY project, not the
// five it happens to be showing.
//
// The regression these tests lock down: `recentWorkspaces` stopped at 5 and the
// "Search projects" box filtered that truncated list. The live store holds 202
// distinct workspaces, so the box was a lie — it looked like a global project
// search, but typing the name of a project that was visibly sitting in the
// sidebar of the same frame answered "No projects found", and the user's own
// main repo could only be reached by hand-typing an absolute path. So we assert:
//   1. idle, the picker still lists only the 5 most recent (no 202-row dump),
//   2. a query reaches the WHOLE set — the 7th, the 9th, the last project,
//   3. "No projects found" appears only when nothing actually matches,
//   4. picking a searched-out project updates the chip.
//
// Note: the popover panel may render through a portal, so every assertion below
// is a document-level query — never "is a descendant of the composer container".
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  agents: vi.fn(async () => [{ name: "dev", source: "shipped", yaml: "name: dev\nsystem_prompt: test\ntools: []\n" }]),
  gitBranches: vi.fn(async () => ({ isRepo: true, current: "main", branches: ["main"], dirty: 0, hasCommits: true })),
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/ws" })),
  newSession: vi.fn(async () => ({ sid: "20260712-000000-x" })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    agents: mocks.agents,
    gitBranches: mocks.gitBranches,
    makeWorkspace: mocks.makeWorkspace,
    newSession: mocks.newSession,
  },
}));

import { Composer } from "./Composer";
import { useStore } from "../store";

window.matchMedia = ((q: string) =>
  ({ matches: false, media: q, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;

const setNarrowViewport = (matches: boolean) => {
  window.matchMedia = ((q: string) =>
    ({ matches: matches && q === "(max-width: 480px)", media: q, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;
};

// Ten distinct projects, oldest id first. Session ids are creation stamps, so
// "newest first" is a descending id sort: proj10 is the most recent, proj1 the
// oldest — i.e. proj6..proj1 all sit *outside* the 5-row idle window.
const PROJECTS = Array.from({ length: 10 }, (_, i) => `/repos/proj${i + 1}`);
const SESSIONS = PROJECTS.map((workspace, i) => ({
  id: `2026071${i}-000000-s${i + 1}`,
  workspace,
  status: "idle",
  kind: "session",
})) as any[];

const mount = (extra: any[] = [], projectSeed?: { workspace: string; requestId: number }) => {
  useStore.setState({
    sessions: [...SESSIONS, ...extra],
    sessionsReady: true,
    refreshSessions: async () => {},
    select: vi.fn(),
    toast: vi.fn(),
    openPrompt: vi.fn(),
  } as any);
  return render(<Composer variant="home" onError={() => {}} projectSeed={projectSeed} />);
};

// The project chip; the panel it opens may be portaled, so we reach the list
// through the document, not through the render container.
const chip = (c: HTMLElement) => c.querySelector<HTMLButtonElement>(".cx-env-control.project")!;
const list = () => document.querySelector<HTMLElement>(".cx-project-list")!;
const rows = () => [...list().querySelectorAll<HTMLElement>(".pop-item .pop-title")].map((n) => n.textContent);
const search = () => screen.getByLabelText("Search projects") as HTMLInputElement;
const type = (q: string) => fireEvent.change(search(), { target: { value: q } });

beforeEach(() => {
  localStorage.clear();
  mocks.gitBranches.mockClear();
  setNarrowViewport(false);
});
afterEach(cleanup);

describe("project picker searches every project (HM-9)", () => {
  it("applies repeated project-row New chat seeds without remounting or losing the draft", async () => {
    localStorage.setItem("arwebui.lastProject", "/repos/proj10");
    const firstSeed = { workspace: "/repos/proj3", requestId: 1 };
    useStore.setState({ newSessionProject: firstSeed } as any);
    const { container, rerender } = mount([], firstSeed);
    const textarea = container.querySelector<HTMLTextAreaElement>("textarea")!;

    await vi.waitFor(() => expect(chip(container).textContent).toContain("proj3"));
    expect(localStorage.getItem("arwebui.lastProject")).toBe("/repos/proj3");
    expect(document.activeElement).toBe(textarea);
    expect(useStore.getState().newSessionProject).toBeNull();

    fireEvent.change(textarea, { target: { value: "keep this draft" } });
    rerender(<Composer variant="home" onError={() => {}} projectSeed={{ workspace: "/repos/proj2", requestId: 2 }} />);

    await vi.waitFor(() => expect(chip(container).textContent).toContain("proj2"));
    expect(textarea.value).toBe("keep this draft");
    expect(localStorage.getItem("arwebui.lastProject")).toBe("/repos/proj2");
    expect(mocks.gitBranches).toHaveBeenCalledWith("/repos/proj2");
  });

  it("gives the project popover a shrinkable wrapper for long mobile project names", () => {
    const { container } = mount();
    const projectChip = chip(container);

    expect(projectChip.parentElement?.classList.contains("cx-env-project-wrap")).toBe(true);
    expect(projectChip.parentElement?.parentElement?.classList.contains("cx-env-strip")).toBe(true);
  });

  it("keeps a long mobile branch in the second environment row", async () => {
    setNarrowViewport(true);
    const { container } = mount();
    await vi.waitFor(() => expect(container.querySelector(".cx-env-control.branch")).not.toBeNull());
    const branch = container.querySelector<HTMLButtonElement>(".cx-env-control.branch")!;

    expect(branch.parentElement?.classList.contains("flex-1")).toBe(true);
    expect(branch.classList.contains("w-full")).toBe(true);
  });

  it("shows only Select project until a project is chosen", async () => {
    localStorage.setItem("arwebui.lastProject", "");
    const { container } = mount();

    expect(container.querySelectorAll(".cx-env-control")).toHaveLength(1);
    expect(chip(container).textContent).toContain("Select project");
    expect(container.querySelector(".cx-env-control.branch")).toBeNull();
    expect(container.textContent).not.toContain("New worktree");
    expect(container.textContent).not.toContain("No branch");

    fireEvent.click(chip(container));
    fireEvent.click(within(list()).getByRole("button", { name: /proj10/ }));
    await vi.waitFor(() => expect(container.querySelectorAll(".cx-env-control")).toHaveLength(3));
    expect(container.querySelector(".cx-env-control.branch")).not.toBeNull();
  });

  it("reveals the action row when a focused short-phone composer is clipped", () => {
    setNarrowViewport(true);
    const scrollIntoView = vi.fn();
    const raf = vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callback(0);
      return 1;
    });
    Object.defineProperty(Element.prototype, "scrollIntoView", { value: scrollIntoView, configurable: true });
    const rect = vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
      return { bottom: this.classList.contains("cx-card") ? 569 : 0 } as DOMRect;
    });
    const innerHeight = window.innerHeight;
    Object.defineProperty(window, "innerHeight", { value: 500, configurable: true });

    try {
      mount();
      expect(scrollIntoView).toHaveBeenCalledWith({ block: "end", inline: "nearest" });
    } finally {
      Object.defineProperty(window, "innerHeight", { value: innerHeight, configurable: true });
      rect.mockRestore();
      raf.mockRestore();
      Reflect.deleteProperty(Element.prototype, "scrollIntoView");
    }
  });

  it("idle: lists only the 5 most recent — opening it doesn't dump the whole store", () => {
    const { container } = mount();
    fireEvent.click(chip(container));

    expect(rows()).toEqual(["proj10", "proj9", "proj8", "proj7", "proj6"]);
    expect(screen.queryByText("No projects found")).toBeNull();
  });

  it("typing finds a project OUTSIDE those five — the old cap answered 'No projects found'", () => {
    const { container } = mount();
    fireEvent.click(chip(container));

    // proj2 is the 9th most recent: never in the idle window, and the exact
    // shape of the live bug (searching `qa57-browser`, a project the sidebar
    // was showing in the same frame, found nothing).
    expect(rows()).not.toContain("proj2");
    type("proj2");
    expect(rows()).toEqual(["proj2"]);
    expect(screen.queryByText("No projects found")).toBeNull();

    // …and the oldest one, at the very bottom of a 10-deep history. The exact
    // name wins the top row; proj10 still matches as a prefix hit.
    type("proj1");
    expect(rows()).toEqual(["proj1", "proj10"]);
  });

  it("ranks the repo you meant above its own worktrees (path-only hits sink)", () => {
    // The live shape of this: searching "agentrunner" hits 107 workspaces,
    // because every scratch worktree lives *under* ~/dev2/agentrunner. The repo
    // itself must not be buried under the newer children whose paths mention it.
    const { container } = mount([
      { id: "20260799-000000-wt", workspace: "/repos/proj3/wt/hotfix-9", status: "idle", kind: "session" },
    ]);
    fireEvent.click(chip(container));

    type("proj3");
    // hotfix-9 is the NEWEST session, so pure recency would put it first; it is
    // a path-only match, so it sorts behind the project that is actually named.
    expect(rows()).toEqual(["proj3", "hotfix-9"]);
  });

  it("a partial query matches across the whole set, newest first", () => {
    const { container } = mount();
    fireEvent.click(chip(container));

    type("proj");
    expect(rows()).toEqual(PROJECTS.map((p) => p.split("/").pop()).reverse()); // proj10 … proj1
    expect(rows()).toHaveLength(10);
  });

  it("'No projects found' only when nothing actually matches", () => {
    const { container } = mount();
    fireEvent.click(chip(container));

    type("nope-not-a-project");
    expect(rows()).toEqual([]);
    expect(screen.getByText("No projects found")).toBeTruthy();
  });

  it("choosing a searched-out project updates the chip", async () => {
    const { container } = mount();
    // Cold start seeds the newest real project (RH-1), so the chip opens on proj10.
    await vi.waitFor(() => expect(chip(container).textContent).toContain("proj10"));

    fireEvent.click(chip(container));
    type("proj3");
    fireEvent.click(within(list()).getByRole("button", { name: /proj3/ }));

    await vi.waitFor(() => expect(chip(container).textContent).toContain("proj3"));
    expect(localStorage.getItem("arwebui.lastProject")).toBe("/repos/proj3");
    expect(mocks.gitBranches).toHaveBeenCalledWith("/repos/proj3");

    // Reopening keeps the selection visible even though it aged out of the top 5.
    fireEvent.click(chip(container));
    expect(rows()).toEqual(["proj3", "proj10", "proj9", "proj8", "proj7"]);
    expect(list().querySelector(".pop-item .pop-check")).toBeTruthy();
  });
});

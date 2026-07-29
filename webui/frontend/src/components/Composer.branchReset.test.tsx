// @vitest-environment jsdom
//
// QA 2026-07-29 — switching the composer's project must reset the starting
// branch to the target repo's own current branch.
//
// The regression these tests lock down: the branch state only changed when the
// new repo's `gitBranches` response landed, so between the project switch and
// that response the PREVIOUS project's branch kept hanging in state. A send in
// that window called `makeWorktree` with the old repo's branch and failed with
// "Couldn't find a commit named '<old-branch>' to branch from". So we assert:
//   1. switching projects re-labels the branch chip with the new repo's branch,
//   2. a send racing the discovery still starts the worktree from the NEW
//      repo's current branch — never the previous project's selection,
//   3. a stale in-flight refresh from the old project cannot overwrite the new
//      project's branch state after the switch.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  agents: vi.fn(async () => [{ name: "dev", source: "shipped", yaml: "name: dev\nsystem_prompt: test\ntools: []\n" }]),
  gitBranches: vi.fn(),
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/ws" })),
  makeWorktree: vi.fn(async (repo: string, _branch: string, _ref: string) => ({ path: `${repo}/wt/auto` })),
  newSession: vi.fn(async (_body: { workspace: string }) => ({ sid: "20260729-000000-x" })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    agents: mocks.agents,
    gitBranches: mocks.gitBranches,
    makeWorkspace: mocks.makeWorkspace,
    makeWorktree: mocks.makeWorktree,
    newSession: mocks.newSession,
  },
}));

import { Composer } from "./Composer";
import { useStore } from "../store";

window.matchMedia = ((q: string) =>
  ({ matches: false, media: q, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;

// The exact QA shape: the previously viewed project sits on a feature branch;
// the project the user switches to is a fresh repo whose only branch is main.
type BranchInfo = { isRepo: boolean; current: string; branches: string[]; dirty: number; hasCommits: boolean };
const BRANCHES: Record<string, BranchInfo> = {
  "/repos/previous": { isRepo: true, current: "claude/analyze-oss", branches: ["claude/analyze-oss", "main"], dirty: 0, hasCommits: true },
  "/repos/fresh": { isRepo: true, current: "main", branches: ["main"], dirty: 0, hasCommits: true },
};

// Dirs listed here answer only when the test releases them — that in-flight
// window is exactly where the regression lived.
let holdDirs: Set<string>;
let held: Array<{ dir: string; resolve: (info: BranchInfo) => void }>;

// `previous` carries the newer session, so the cold-start seed (RH-1) opens
// the composer on it — the QA starting position.
const SESSIONS = [
  { id: "20260701-000000-s1", workspace: "/repos/fresh", status: "idle", kind: "session" },
  { id: "20260728-000000-s2", workspace: "/repos/previous", status: "idle", kind: "session" },
] as any[];

const mount = () => {
  useStore.setState({
    sessions: SESSIONS,
    sessionsReady: true,
    refreshSessions: async () => {},
    select: vi.fn(),
    toast: vi.fn(),
    openPrompt: vi.fn(),
  } as any);
  return render(<Composer variant="home" onError={() => {}} />);
};

const projectChip = (c: HTMLElement) => c.querySelector<HTMLButtonElement>(".cx-env-control.project")!;
const branchChip = (c: HTMLElement) => c.querySelector<HTMLButtonElement>(".cx-env-control.branch");
const projectList = () => document.querySelector<HTMLElement>(".cx-project-list")!;
const switchToFresh = (c: HTMLElement) => {
  fireEvent.click(projectChip(c));
  fireEvent.click(within(projectList()).getByRole("button", { name: /fresh/ }));
};

beforeEach(() => {
  localStorage.clear();
  holdDirs = new Set();
  held = [];
  mocks.gitBranches.mockReset();
  mocks.gitBranches.mockImplementation((dir: string) => {
    if (holdDirs.has(dir)) return new Promise<BranchInfo>((resolve) => held.push({ dir, resolve }));
    return Promise.resolve({ ...(BRANCHES[dir] ?? { isRepo: false, current: "", branches: [], dirty: 0, hasCommits: false }) });
  });
  mocks.makeWorktree.mockClear();
  mocks.newSession.mockClear();
});
afterEach(cleanup);

describe("switching projects resets the starting branch (QA 2026-07-29)", () => {
  it("re-labels the branch chip with the new repo's current branch", async () => {
    const { container } = mount();
    await vi.waitFor(() => expect(branchChip(container)?.textContent).toContain("claude/analyze-oss"));

    switchToFresh(container);

    await vi.waitFor(() => expect(branchChip(container)?.textContent).toContain("main"));
    expect(branchChip(container)?.textContent).not.toContain("claude/analyze-oss");
  });

  it("a send racing the discovery starts the worktree from the NEW repo's branch", async () => {
    const { container } = mount();
    await vi.waitFor(() => expect(branchChip(container)?.textContent).toContain("claude/analyze-oss"));

    switchToFresh(container);
    // No waiting here: the send races the new repo's branch discovery — the
    // QA repro was "switch project, immediately hit Enter".
    const textarea = screen.getByPlaceholderText("Do anything");
    fireEvent.change(textarea, { target: { value: "go" } });
    fireEvent.keyDown(textarea, { key: "Enter" });

    await vi.waitFor(() => expect(mocks.newSession).toHaveBeenCalled());
    expect(mocks.makeWorktree).toHaveBeenCalledTimes(1);
    const [repo, , ref] = mocks.makeWorktree.mock.calls[0];
    expect(repo).toBe("/repos/fresh");
    expect(ref).toBe("main"); // the live failure sent "claude/analyze-oss" here
    expect((mocks.newSession.mock.calls[0][0] as any).workspace).toBe("/repos/fresh/wt/auto");
  });

  it("a stale refresh from the previous project cannot overwrite the new state", async () => {
    const { container } = mount();
    await vi.waitFor(() => expect(branchChip(container)?.textContent).toContain("claude/analyze-oss"));

    // Open the branch picker with the old project's refresh held in flight…
    holdDirs.add("/repos/previous");
    fireEvent.click(branchChip(container)!);
    expect(held).toHaveLength(1);

    // …switch projects while that request is still pending (the click opens
    // the project popover; the branch popover stays mounted alongside it)…
    switchToFresh(container);
    await vi.waitFor(() => expect(branchChip(container)?.textContent).toContain("main"));

    // …then let the old project's answer land late. It must be dropped: the
    // chip keeps the new branch AND the open picker lists only the new repo's
    // branches (the regression kept the label but swapped the list under it).
    await act(async () => {
      held[0].resolve({ ...BRANCHES["/repos/previous"] });
    });

    expect(branchChip(container)?.textContent).toContain("main");
    expect(branchChip(container)?.textContent).not.toContain("claude/analyze-oss");
    const listed = [...document.querySelectorAll(".cx-branch-popover .pop-item .pop-title")].map((n) => n.textContent);
    expect(listed).toEqual(["main"]);
  });
});

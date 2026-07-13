// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";

// INC-41 TH-3 — the resting Supervision panel. A session with no goal, no
// subagents and nothing to approve used to spend ~325px (28% of the panel) on
// three titled blocks whose entire content was a negation: "No active goal" /
// "No subagents" / "Nothing needs you". Codex's Environment panel simply omits
// the groups that have nothing in them. These tests pin both halves of that:
// empty groups don't render at all, and a *fully* quiet panel still says so —
// once, on one dim line — so it never reads as broken.

// Any AR.<method> that has no stub in `mocks.stubs` returns a promise that never
// settles: the panel must not depend on a network round-trip to lay itself out.
// The Environment tests below (RD-A/RD-D) install real stubs for `diff` +
// `gitBranches` for the length of one test.
const mocks = vi.hoisted(() => ({ stubs: {} as Record<string, (...args: any[]) => any> }));
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy(
    {},
    {
      get:
        (_t, key: string) =>
        (...args: any[]) =>
          mocks.stubs[key] ? mocks.stubs[key](...args) : new Promise(() => {}),
    },
  ),
}));

import { ENV_REFRESH_MS, SupervisionPanel, type GoalState } from "./SupervisionPanel";
import { useStore } from "../store";
import type { InspectNode } from "./Subagents";
import { fireEvent } from "@testing-library/react";

const noop = () => {};

function panelTree(over: Partial<React.ComponentProps<typeof SupervisionPanel>> = {}) {
  return (
    <div className="session-view">
      <SupervisionPanel
        loading={false}
        goal={null}
        goalEdit={null}
        progress={[]}
        artifacts={[]}
        children={[]}
        backgroundWork={[]}
        approvals={0}
        sessionIdle={true}
        recovery={false}
        onGoalEdit={noop}
        onGoalSave={noop}
        onGoalDiscard={noop}
        onGoalAction={noop}
        onOpenArtifact={noop}
        onOpenChild={noop}
        onKillWork={noop}
        onInspect={noop}
        onClose={noop}
        {...over}
      />
    </div>
  );
}

function renderPanel(over: Partial<React.ComponentProps<typeof SupervisionPanel>> = {}) {
  const result = render(panelTree(over));
  return {
    ...result,
    // rerender with the same wrapper, so a test can tick refreshKey.
    update: (next: Partial<React.ComponentProps<typeof SupervisionPanel>> = {}) =>
      result.rerender(panelTree({ ...over, ...next })),
  };
}

const goal: GoalState = { goal: "ship the panel", checks: 2, max_checks: 5 };
const child: InspectNode = { session: "s-sub-1", agent: "worker", status: "running" } as InspectNode;

beforeEach(() => {
  // No current session: EnvironmentSection (which fetches its own diff) stays
  // out of the way, and the settled-goal lookup never fires. The three groups
  // under test are the whole panel body here.
  useStore.setState({ currentSid: null, modal: null });
  for (const k of Object.keys(mocks.stubs)) delete mocks.stubs[k];
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("TH-3 · resting Supervision panel", () => {
  it("renders no titled empty block — one dim line stands in for all three", () => {
    const { container } = renderPanel();

    // The three negations are gone, titles included.
    expect(screen.queryByText("Goal")).toBeNull();
    expect(screen.queryByText("Agents")).toBeNull();
    expect(screen.queryByText("Attention")).toBeNull();
    expect(screen.queryByText(/No active goal/i)).toBeNull();
    expect(screen.queryByText(/No subagents/i)).toBeNull();
    expect(container.querySelectorAll(".supervision-empty").length).toBe(0);

    // …replaced by exactly one quiet line, so the panel doesn't read as broken.
    const quiet = container.querySelectorAll(".supervision-quiet");
    expect(quiet.length).toBe(1);
    expect(quiet[0].textContent).toContain("Nothing needs you");
    // Run details still closes the panel.
    expect(screen.getByRole("button", { name: /run details/i })).toBeTruthy();
  });

  it("shows one indeterminate line while inspect is in flight (not three)", () => {
    const { container } = renderPanel({ loading: true });

    const quiet = container.querySelectorAll(".supervision-quiet");
    expect(quiet.length).toBe(1);
    expect(quiet[0].textContent).toContain("Checking…");
    expect(container.querySelectorAll("section.supervision-section").length).toBe(0);
    expect(screen.queryByText(/Nothing needs you/i)).toBeNull();
  });
});

// INC-41 DF-D4 — the panel's `Supervision` title bar was a word-for-word second
// copy of the topbar pill that opens the panel, 54px above it and always on the
// same screen; RV-1 had already deleted the Changes rail's twin of it. Gone, and
// its ✕ with it — the close affordance now floats on the panel's first row.
describe("DF-D4 · no title bar that repeats the topbar pill", () => {
  it("renders no .supervision-head, and no visible rail heading", () => {
    const { container } = renderPanel();

    expect(container.querySelector(".supervision-head")).toBeNull();
    // TH-15 · the rail answers to ONE name, inside and out: the topbar pill, the
    // first section's label and the region's accessible name all say Environment.
    // "Supervision" is not a word the user ever sees.
    expect(screen.queryByText("Supervision")).toBeNull();
    expect(container.querySelector("aside.supervision-panel")!.getAttribute("aria-label")).toBe(
      "Environment",
    );
  });

  it("still closes: the ✕ survives the title bar, in the panel's first row", () => {
    const onClose = vi.fn();
    const { container } = renderPanel({ onClose });

    // Zero-height sticky slot: it costs the panel no height, so the first real
    // section starts at the very top of the panel.
    const slot = container.querySelector(".supervision-close-slot")!;
    expect(slot).toBeTruthy();
    expect(slot).toBe(container.querySelector("aside.supervision-panel")!.firstElementChild);
    expect(slot.classList).toContain("sticky");
    expect(slot.classList).toContain("h-0");
    expect(slot.classList).toContain("justify-end");

    // Mobile: the close control shares the first section's heading line at the
    // panel's right edge instead of consuming a blank 40px row on the left.
    const close = screen.getByRole("button", { name: /hide environment/i });
    expect(close.classList).toContain("icon-only");
    expect(close.classList).toContain("h-6");
    expect(close.classList).toContain("w-6");

    fireEvent.click(close);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("TH-3 · groups with content are untouched", () => {
  it("renders the Goal section for an active goal — and drops the merged line", () => {
    renderPanel({ goal });

    expect(screen.getByText("Goal")).toBeTruthy();
    expect(screen.getByText("ship the panel")).toBeTruthy();
    expect(screen.getByText("2/5 checks")).toBeTruthy();
    // A goal means something *is* going on: no "Nothing needs you" alongside it.
    expect(screen.queryByText(/Nothing needs you/i)).toBeNull();
    // The other two groups are still empty, so they still don't render.
    expect(screen.queryByText("Agents")).toBeNull();
    expect(screen.queryByText("Attention")).toBeNull();
  });

  it("renders the Agents section when subagents exist", () => {
    const { container } = renderPanel({ children: [child] });

    expect(screen.getByText("Agents")).toBeTruthy();
    expect(container.querySelector(".supervision-agents")).not.toBeNull();
    expect(screen.queryByText(/Nothing needs you/i)).toBeNull();
    expect(screen.queryByText("Goal")).toBeNull();
  });

  it("renders the Attention section when something needs a human", () => {
    const { container } = renderPanel({ approvals: 2 });

    expect(screen.getByText("Attention")).toBeTruthy();
    expect(container.querySelector(".attention-row")?.textContent).toContain("Approval requested");
    expect(screen.queryByText(/Nothing needs you/i)).toBeNull();
  });

  it("still flags idle background work, and keeps the Background section", () => {
    const session = { handle: "h1", tool: "spawn_agent", detail: "agent=worker session=review" } as any;
    const { container } = renderPanel({ backgroundWork: [session], sessionIdle: true });

    expect(screen.getByText("Attention")).toBeTruthy();
    expect(container.querySelector(".attention-row")?.textContent).toContain(
      "Background work still running",
    );
    expect(screen.getByText("Background work")).toBeTruthy();
    expect(screen.queryByText(/Nothing needs you/i)).toBeNull();
  });

  it("keeps quiet when background work runs mid-turn (session not idle)", () => {
    const session = { handle: "h1", tool: "spawn_agent", detail: "agent=worker session=review" } as any;
    const { container } = renderPanel({ backgroundWork: [session], sessionIdle: false });

    // Nothing needs the human yet — Attention stays out, the one dim line stands
    // in, and Background work still lists the running session.
    expect(screen.queryByText("Attention")).toBeNull();
    expect(container.querySelectorAll(".supervision-quiet").length).toBe(1);
    expect(screen.getByText("Background work")).toBeTruthy();
  });
});

// ===== INC-41 RD-A / RD-D / RD-E · the Environment block =====
// A dirty tree: 2 tracked files (+3 / −1) AND 2 untracked files — the ordinary
// case for a coding turn, and the one the old row got wrong (it printed "+3 −1"
// and dropped both new files on the floor).
const DIRTY_DIFF = [
  "diff --git a/a.ts b/a.ts",
  "--- a/a.ts",
  "+++ b/a.ts",
  "@@ -1,2 +1,3 @@",
  " ctx",
  "+added one",
  "+added two",
  "-removed",
  "diff --git a/b.ts b/b.ts",
  "--- a/b.ts",
  "+++ b/b.ts",
  "@@ -1 +1 @@",
  "+new line",
].join("\n");

const dirty = (diff = DIRTY_DIFF, untracked = ["asset.bin", "img/shot.png"]) => ({
  workspace: "/tmp/wt-20260712",
  known: true,
  isRepo: true,
  nested: false,
  diff,
  untracked,
});

function stubEnv(payload: any = dirty()) {
  const diff = vi.fn(() => Promise.resolve(payload));
  mocks.stubs.diff = diff;
  mocks.stubs.gitBranches = () => Promise.resolve({ isRepo: true, current: "main", branches: [] });
  useStore.setState({ currentSid: "s1" });
  return diff;
}

describe("RD-A · the Environment rows are live, not a mount-time snapshot", () => {
  it("re-reads git when refreshKey ticks — but at most once per throttle window", async () => {
    vi.useFakeTimers();
    const diff = stubEnv();

    const { update } = renderPanel({ refreshKey: 0 });
    await act(async () => {}); // let the mount fetch settle
    // Leading edge: the panel reads git the moment it appears.
    expect(diff).toHaveBeenCalledTimes(1);

    // A live turn streams events; refreshKey ticks with each one. `ar diff` shells
    // out to git, so a burst inside the window must NOT become a burst of fetches.
    await act(async () => {
      update({ refreshKey: 1 });
    });
    await act(async () => {
      update({ refreshKey: 2 });
    });
    await act(async () => {
      update({ refreshKey: 3 });
    });
    expect(diff).toHaveBeenCalledTimes(1);

    // …but the panel must not end the burst holding a stale tree: the trailing
    // edge fires once the window closes. (This is the whole bug: the rail used to
    // say "clean" while the thread said "Edited 12 files".)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ENV_REFRESH_MS);
    });
    expect(diff).toHaveBeenCalledTimes(2);

    // After a quiet stretch, the next event is a leading edge again — immediate.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ENV_REFRESH_MS);
    });
    await act(async () => {
      update({ refreshKey: 4 });
    });
    expect(diff).toHaveBeenCalledTimes(3);
  });

  it("does not re-read when nothing in the session changed", async () => {
    vi.useFakeTimers();
    const diff = stubEnv();

    const { update } = renderPanel({ refreshKey: 7 });
    await act(async () => {});
    expect(diff).toHaveBeenCalledTimes(1);

    // Same refreshKey, unrelated prop change (the panel re-renders constantly).
    await act(async () => {
      update({ approvals: 1 });
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ENV_REFRESH_MS * 3);
    });
    expect(diff).toHaveBeenCalledTimes(1);
  });
});

describe("RD-D · the Changes row states what it knows", () => {
  it("leads with the file count, then ± lines, and still counts untracked files", async () => {
    stubEnv();
    const { container } = renderPanel();

    const val = await screen.findByText("2 files");
    expect(val).toBeTruthy();
    const row = container.querySelectorAll(".env-row")[0];
    expect(row.textContent).toContain("Changes");
    // Codex's order: files first, then the lines.
    expect(row.textContent).toContain("2 files");
    expect(row.textContent).toContain("+3");
    expect(row.textContent).toContain("−1");
    // The regression this closes: untracked files used to render ONLY when there
    // were no tracked changes at all — so a normal turn (edits + new files) never
    // showed them.
    expect(row.textContent).toContain("2 new");
  });

  it("still reports a purely-untracked tree", async () => {
    stubEnv(dirty("", ["one.bin", "two.bin", "three.bin"]));
    const { container } = renderPanel();

    await screen.findByText(/3 new/);
    const row = container.querySelectorAll(".env-row")[0];
    expect(row.textContent).not.toContain("files"); // no tracked file was touched
    expect(row.textContent).not.toContain("+");
  });

  it("says nothing at all on a clean tree (ENV-3)", async () => {
    stubEnv(dirty("", []));
    const { container } = renderPanel();

    await screen.findByText("Environment");
    const row = container.querySelectorAll(".env-row")[0];
    expect(row.querySelector(".env-row-val")).toBeNull();
    expect(row.textContent).not.toContain("0 files");
  });
});

describe("RD-E · Background work rides under Environment", () => {
  it("is the second section on the panel — above Goal / Attention", async () => {
    stubEnv();
    const session = { handle: "h1", tool: "spawn_agent", detail: "agent=worker session=review" } as any;
    const { container } = renderPanel({ backgroundWork: [session], goal, approvals: 1 });

    await screen.findByText("Environment");
    const labels = [...container.querySelectorAll(".supervision-label")].map((el) =>
      (el.textContent || "").trim(),
    );
    expect(labels[0]).toBe("Environment");
    expect(labels[1]).toBe("Background work");
    // …and it is no longer the last thing on the rail, below everything else.
    expect(labels.indexOf("Background work")).toBeLessThan(labels.indexOf("Goal"));
    expect(labels.indexOf("Background work")).toBeLessThan(labels.indexOf("Attention"));
  });
});

// ===== INC-41 RD-C · the Worktree drawer is an action drawer ==================
// It expanded onto a path and a `Copy path` button and stopped there — while the
// three things a user actually does to a worktree (apply it back onto the
// project, remove it, open it in an editor) sat in the OTHER right rail's `…`
// menu and the sidebar's context menu. The two rails share one slot, so acting
// on the worktree meant closing the panel that was showing it to you. These pin
// the fix: the actions are IN the drawer, they are the same actions (same
// endpoints, same confirmations — worktreeActions.ts, which DiffView now calls
// too), and an in-place session is offered the ones that would only ever error.
const WT = { worktree: true, mainRepo: "/Users/me/agentrunner" };

// Expand the Worktree row. `/^worktree/i` because once the drawer is open the
// name "Remove worktree…" would otherwise match the same query.
async function openWorktree(payload: any = { ...dirty(), ...WT }) {
  stubEnv(payload);
  const result = renderPanel();
  await screen.findByText("Environment");
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: /^worktree/i }));
  });
  return result;
}

describe("RD-C · the worktree's actions live where the worktree is shown", () => {
  it("offers Apply / Open in VS Code / Remove in the drawer — not just a path", async () => {
    const { container } = await openWorktree();

    expect(screen.getByRole("button", { name: /apply to project/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /open in vs code/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /remove worktree/i })).toBeTruthy();
    // The path and its Copy button are still there — this is an addition.
    expect(container.querySelector(".env-path")!.textContent).toBe("/tmp/wt-20260712");
    expect(screen.getByRole("button", { name: /copy path/i })).toBeTruthy();
  });

  it("Remove asks first — the same confirmation DiffView's menu raises", async () => {
    const remove = vi.fn(() => Promise.resolve({ status: "ok", mainRepo: WT.mainRepo }));
    mocks.stubs.removeWorktree = remove;
    await openWorktree();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /remove worktree/i }));
    });
    // Nothing has been deleted yet: a destructive action must never be one click.
    expect(remove).not.toHaveBeenCalled();
    const modal = useStore.getState().modal as any;
    expect(modal?.kind).toBe("confirm");
    expect(modal.title).toBe("Remove worktree?");
    expect(modal.danger).toBe(true);

    // …and only the confirmation actually removes it (force=false — the backend's
    // dirty-worktree guard still gets its say).
    await act(async () => {
      await modal.onConfirm();
    });
    expect(remove).toHaveBeenCalledWith("s1", false);
  });

  it("Apply asks first too, then applies onto the main repo", async () => {
    const apply = vi.fn(() => Promise.resolve({ status: "ok", mainRepo: WT.mainRepo, applied: "1" }));
    mocks.stubs.applyWorktree = apply;
    await openWorktree();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /apply to project/i }));
    });
    expect(apply).not.toHaveBeenCalled();
    const modal = useStore.getState().modal as any;
    expect(modal?.kind).toBe("confirm");
    expect(modal.title).toBe("Apply changes to project?");
    expect(modal.body).toContain(WT.mainRepo);

    await act(async () => {
      await modal.onConfirm();
    });
    expect(apply).toHaveBeenCalledWith("s1");
  });

  it("Open in VS Code launches the workspace through the existing launcher", async () => {
    const openIn = vi.fn(() => Promise.resolve({ status: "ok" }));
    mocks.stubs.openIn = openIn;
    await openWorktree();

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /open in vs code/i }));
    });
    expect(openIn).toHaveBeenCalledWith("/tmp/wt-20260712", "vscode");
  });

  it("nothing to apply → the row is inert rather than an error waiting to happen", async () => {
    await openWorktree({ ...dirty("", []), ...WT });

    expect(screen.getByRole("button", { name: /apply to project/i })).toHaveProperty("disabled", true);
    // Removing an unused worktree is still perfectly legitimate.
    expect(screen.getByRole("button", { name: /remove worktree/i })).toHaveProperty("disabled", false);
  });

  it("a session running in the repo itself is offered neither Apply nor Remove", async () => {
    // No worktree: there is nothing to apply back and nothing to prune. Gated
    // exactly as DiffView's `…` menu gates them.
    await openWorktree({ ...dirty(), worktree: false, mainRepo: "" });

    expect(screen.queryByRole("button", { name: /apply to project/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /remove worktree/i })).toBeNull();
    // The path, its Copy button and the editor launcher act on the workspace
    // itself, which every one of these sessions has — they stay.
    expect(screen.getByRole("button", { name: /copy path/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /open in vs code/i })).toBeTruthy();
  });
});

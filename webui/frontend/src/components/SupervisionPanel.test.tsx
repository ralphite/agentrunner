// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

// INC-41 TH-3 — the resting Supervision panel. A session with no goal, no
// subagents and nothing to approve used to spend ~325px (28% of the panel) on
// three titled blocks whose entire content was a negation: "No active goal" /
// "No subagents" / "Nothing needs you". Codex's Environment panel simply omits
// the groups that have nothing in them. These tests pin both halves of that:
// empty groups don't render at all, and a *fully* quiet panel still says so —
// once, on one dim line — so it never reads as broken.

// Any AR.<method> we don't stub returns a promise that never settles: the panel
// must not depend on a network round-trip to lay itself out.
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy({}, { get: () => () => new Promise(() => {}) }),
}));

import { SupervisionPanel, type GoalState } from "./SupervisionPanel";
import { useStore } from "../store";
import type { InspectNode } from "./Subagents";

const noop = () => {};

function renderPanel(over: Partial<React.ComponentProps<typeof SupervisionPanel>> = {}) {
  return render(
    <div className="session-view">
      <SupervisionPanel
        loading={false}
        goal={null}
        goalEdit={null}
        progress={[]}
        artifacts={[]}
        children={[]}
        tasks={[]}
        approvals={0}
        sessionIdle={true}
        recovery={false}
        onGoalEdit={noop}
        onGoalSave={noop}
        onGoalDiscard={noop}
        onGoalAction={noop}
        onOpenArtifact={noop}
        onOpenChild={noop}
        onKillTask={noop}
        onInspect={noop}
        onClose={noop}
        {...over}
      />
    </div>,
  );
}

const goal: GoalState = { goal: "ship the panel", checks: 2, max_checks: 5 };
const child: InspectNode = { session: "s-sub-1", agent: "worker", status: "running" } as InspectNode;

beforeEach(() => {
  // No current session: EnvironmentSection (which fetches its own diff) stays
  // out of the way, and the settled-goal lookup never fires. The three groups
  // under test are the whole panel body here.
  useStore.setState({ currentSid: null });
});
afterEach(() => cleanup());

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
    const task = { handle: "h1", tool: "spawn_agent", detail: "agent=worker task=review" } as any;
    const { container } = renderPanel({ tasks: [task], sessionIdle: true });

    expect(screen.getByText("Attention")).toBeTruthy();
    expect(container.querySelector(".attention-row")?.textContent).toContain(
      "Background work still running",
    );
    expect(screen.getByText("Background work")).toBeTruthy();
    expect(screen.queryByText(/Nothing needs you/i)).toBeNull();
  });

  it("keeps quiet when background work runs mid-turn (session not idle)", () => {
    const task = { handle: "h1", tool: "spawn_agent", detail: "agent=worker task=review" } as any;
    const { container } = renderPanel({ tasks: [task], sessionIdle: false });

    // Nothing needs the human yet — Attention stays out, the one dim line stands
    // in, and Background work still lists the running task.
    expect(screen.queryByText("Attention")).toBeNull();
    expect(container.querySelectorAll(".supervision-quiet").length).toBe(1);
    expect(screen.getByText("Background work")).toBeTruthy();
  });
});

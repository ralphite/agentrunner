// @vitest-environment jsdom
//
// G58 / INC-102 (决策 #21): the Scheduled page's Repeating create/suggestion
// route no longer builds a fresh-child IterationDriver series — it opens an
// ordinary session (round 1 = the prompt) and attaches an in-session schedule
// so every wake continues the SAME conversation. This test pins that route:
// api.newSession + api.scheduleAttach, landing directly on the session id
// newSession returns (no run-id polling needed — there is no run).
//
// Goal (immediate) and Best of N (parallel) are untouched by this increment —
// they still build a driver spec via api.startRun({kind:"drive", ...}); the
// second describe block pins that the interval/cron reroute does not leak
// into those two schedule kinds.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  agents: vi.fn(async () => [{ name: "dev", source: "shipped", yaml: "name: dev\nsystem_prompt: test\ntools: []\n" }]),
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/qa88-scratch" })),
  newSession: vi.fn(async () => ({ sid: "20260723-043024-qa88-durable" })),
  scheduleAttach: vi.fn(async () => ({})),
  startRun: vi.fn(async () => ({ runId: "run1" })),
  runs: vi.fn(async () => [
    {
      id: "run1",
      kind: "drive",
      label: "QA88 goal",
      workspace: "/tmp/qa88-scratch",
      sessionId: "20260723-goal-durable",
      status: "running",
      startedAt: "2026-07-23T04:30:25Z",
    },
  ]),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    agents: mocks.agents,
    makeWorkspace: mocks.makeWorkspace,
    newSession: mocks.newSession,
    scheduleAttach: mocks.scheduleAttach,
    startRun: mocks.startRun,
    runs: mocks.runs,
  },
}));

import { Modals } from "./Modals";
import { useStore } from "../store";

beforeEach(() => {
  const select = vi.fn();
  const selectRun = vi.fn();
  useStore.setState({
    modal: { kind: "run", preset: "repeating", prompt: "Reply QA88 only" },
    prompt: null,
    select,
    selectRun,
    refreshRuns: vi.fn(async () => {}),
    refreshSessions: vi.fn(async () => {}),
    toast: vi.fn(),
    openModal: (modal: any) => useStore.setState({ modal }),
  } as any);
  for (const m of Object.values(mocks)) m.mockClear();
});

afterEach(cleanup);

describe("scheduled creation route (G58: in-session schedule, not a driver series)", () => {
  it("creates a session (round 1 = the prompt) and attaches the schedule, landing on that session", async () => {
    render(<Modals />);

    fireEvent.click(screen.getByRole("button", { name: "Start schedule" }));

    await waitFor(() => {
      expect(useStore.getState().select).toHaveBeenCalledWith("20260723-043024-qa88-durable");
    });
    expect(mocks.newSession).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace: "/tmp/qa88-scratch",
        message: "Reply QA88 only",
      }),
    );
    expect(mocks.scheduleAttach).toHaveBeenCalledWith(
      "20260723-043024-qa88-durable",
      expect.objectContaining({
        schedule: "interval",
        interval: "5m",
        prompt: "Reply QA88 only",
      }),
    );
    expect(useStore.getState().refreshSessions).toHaveBeenCalledOnce();
    expect(useStore.getState().selectRun).not.toHaveBeenCalled();
    expect(mocks.startRun).not.toHaveBeenCalled();
  });
});

describe("Goal / Best of N stay on the legacy driver (unaffected by G58)", () => {
  it("Goal (schedule: immediate) still starts a driver run, not an in-session schedule", async () => {
    useStore.setState({
      modal: { kind: "run", preset: "goal", prompt: "Keep the build green" },
    } as any);
    render(<Modals />);
    expect(screen.getByRole("dialog", { name: "Set a goal" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Start schedule" }));

    await waitFor(() => {
      expect(useStore.getState().select).toHaveBeenCalledWith("20260723-goal-durable");
    });
    expect(mocks.startRun).toHaveBeenCalledWith(expect.objectContaining({ kind: "drive" }));
    expect(mocks.newSession).not.toHaveBeenCalled();
    expect(mocks.scheduleAttach).not.toHaveBeenCalled();
  });

  it("Best of N (schedule: parallel) still starts a driver run, not an in-session schedule", async () => {
    useStore.setState({
      modal: { kind: "run", preset: "best-of-n", prompt: "Fix the flaky test" },
    } as any);
    render(<Modals />);
    expect(screen.getByRole("dialog", { name: "Best of N" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Start schedule" }));

    await waitFor(() => {
      expect(useStore.getState().select).toHaveBeenCalledWith("20260723-goal-durable");
    });
    expect(mocks.startRun).toHaveBeenCalledWith(expect.objectContaining({ kind: "drive" }));
    expect(mocks.newSession).not.toHaveBeenCalled();
    expect(mocks.scheduleAttach).not.toHaveBeenCalled();
  });
});

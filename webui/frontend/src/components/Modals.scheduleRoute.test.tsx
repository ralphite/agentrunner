// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/qa88-scratch" })),
  startRun: vi.fn(async () => ({ runId: "run1" })),
  runs: vi.fn(async () => [
    {
      id: "run1",
      kind: "drive",
      label: "QA88 schedule",
      workspace: "/tmp/qa88-scratch",
      sessionId: "20260723-043024-qa88-durable",
      status: "running",
      startedAt: "2026-07-23T04:30:25Z",
      schedule: "interval",
      cadence: "Every 7d",
      nextRunAt: "2026-07-29T21:30:25-07:00",
    },
  ]),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    makeWorkspace: mocks.makeWorkspace,
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
  mocks.makeWorkspace.mockClear();
  mocks.startRun.mockClear();
  mocks.runs.mockClear();
});

afterEach(cleanup);

describe("scheduled creation route", () => {
  it("lands on the durable daemon session instead of process-local #run:run1", async () => {
    render(<Modals />);

    fireEvent.click(screen.getByRole("button", { name: "Start schedule" }));

    await waitFor(() => {
      expect(useStore.getState().select).toHaveBeenCalledWith("20260723-043024-qa88-durable");
    });
    expect(useStore.getState().refreshSessions).toHaveBeenCalledOnce();
    expect(useStore.getState().selectRun).not.toHaveBeenCalled();
    expect(mocks.runs).toHaveBeenCalled();
  });
});

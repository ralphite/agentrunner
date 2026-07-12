// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

// Nothing here depends on the network; stub the API with never-settling promises
// (same pattern as Sidebar.nav / loadingStates).
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy({}, { get: () => () => new Promise(() => {}) }),
}));

import { Scheduled, hasRhythm } from "./Scheduled";
import { useStore } from "../store";
import type { Run, Session } from "../types";

afterEach(cleanup);

// The page's admission rule (SC-1): a row belongs here only if it will fire
// again on its own. The schedule kind is the server's word for that
// (webui/schedule.go): interval / cron / self_paced tick; immediate (a one-shot
// task or a run-until-verified goal) and parallel (Best of N) do not, and a
// plain `submit` run carries no schedule at all.
const runs: Run[] = [
  {
    id: "r-once",
    kind: "submit",
    label: "One-shot: fix the flaky test",
    workspace: "/repo/app",
    status: "done",
    startedAt: "2026-07-10T10:00:00Z",
  },
  {
    id: "r-goal",
    kind: "drive",
    label: "Goal: keep working until green",
    workspace: "/repo/app",
    status: "running",
    startedAt: "2026-07-10T11:00:00Z",
    schedule: "immediate",
    cadence: "Runs once",
  },
  {
    id: "r-bestof",
    kind: "drive",
    label: "Best-of-N: three attempts at the refactor",
    workspace: "/repo/app",
    status: "running",
    startedAt: "2026-07-10T12:00:00Z",
    schedule: "parallel",
    cadence: "Best of 3",
  },
  {
    id: "r-loop",
    kind: "drive",
    label: "Loop: watch the build",
    workspace: "/repo/app",
    status: "running",
    startedAt: "2026-07-10T13:00:00Z",
    schedule: "interval",
    cadence: "Every 30m",
    nextRunAt: "2099-01-01T00:00:00Z",
  },
];

const sessions: Session[] = [
  {
    id: "20260710-140000-cron",
    status: "running",
    turns: 3,
    title: "Weekly status update draft",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "cron",
    cadence: "Saturdays at 4:00 AM",
  },
  {
    id: "20260710-150000-goal",
    status: "running",
    turns: 2,
    title: "Driver goal: land the migration",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "immediate",
    cadence: "Runs once",
  },
  {
    id: "20260710-160000-plain",
    status: "idle",
    turns: 1,
    title: "An ordinary chat session",
    workspace: "/repo/app",
    kind: "session",
  },
];

const mount = () => {
  useStore.setState({ runs, sessions, sessionsReady: true, unread: [], archived: [], renames: {} });
  return render(<Scheduled />);
};

describe("Scheduled admits only rhythmic work (SC-1)", () => {
  it("keeps repeating work and drops one-shot / best-of-N rows", () => {
    const { container } = mount();
    const rows = [...container.querySelectorAll(".scheduled-row")];
    // Two rhythmic things exist across runs + driver sessions: the interval run
    // and the cron driver. Everything else is one-shot by construction.
    expect(rows).toHaveLength(2);

    expect(screen.getByText("Loop: watch the build")).toBeTruthy();
    expect(screen.getByText("Weekly status update draft")).toBeTruthy();

    expect(screen.queryByText("One-shot: fix the flaky test")).toBeNull();
    expect(screen.queryByText("Goal: keep working until green")).toBeNull();
    expect(screen.queryByText("Best-of-N: three attempts at the refactor")).toBeNull();
    expect(screen.queryByText("Driver goal: land the migration")).toBeNull();
    expect(screen.queryByText("An ordinary chat session")).toBeNull();
  });

  it("never fabricates a 'Runs once' cadence — the phrase is gone from the page", () => {
    mount();
    expect(screen.queryByText(/Runs once/)).toBeNull();
    expect(screen.queryByText(/Best of 3/)).toBeNull();
    // The rows that remain lead with their real rhythm.
    expect(screen.getByText("Every 30m")).toBeTruthy();
    expect(screen.getByText("Saturdays at 4:00 AM")).toBeTruthy();
  });

  it("names the third filter Finished — we have no paused state to show (SC-7)", () => {
    mount();
    expect(screen.getByRole("tab", { name: "Finished" })).toBeTruthy();
    expect(screen.queryByRole("tab", { name: "Paused" })).toBeNull();
  });
});

describe("hasRhythm", () => {
  it("accepts the schedule kinds that fire again on their own", () => {
    expect(hasRhythm({ schedule: "interval" })).toBe(true);
    expect(hasRhythm({ schedule: "cron" })).toBe(true);
    expect(hasRhythm({ schedule: "self_paced" })).toBe(true);
  });

  it("rejects one-shot and parallel work, and a run with no schedule at all", () => {
    expect(hasRhythm({ schedule: "immediate", cadence: "Runs once" })).toBe(false);
    expect(hasRhythm({ schedule: "parallel", cadence: "Best of 4" })).toBe(false);
    expect(hasRhythm({})).toBe(false);
  });

  it("takes a computed next tick as proof of a rhythm", () => {
    // A live interval/cron series is the only thing the server dates.
    expect(hasRhythm({ nextRunAt: "2099-01-01T00:00:00Z" })).toBe(true);
  });
});

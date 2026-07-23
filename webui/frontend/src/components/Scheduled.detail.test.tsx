// @vitest-environment jsdom
//
// G56 is a real selection journey, not a card-render test: a canonical series
// opens the safe typed detail route, acts on the durable cadence, can cross into
// its history, restores focus on Back, and degrades to history for old journals.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { scheduleDetail, schedule, sessions } = vi.hoisted(() => ({
  scheduleDetail: vi.fn(),
  schedule: vi.fn(async () => ({})),
  sessions: vi.fn(async () => [] as any[]),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy(
    { scheduleDetail, schedule, sessions },
    { get: (target, key) => key in target ? target[key as keyof typeof target] : vi.fn(async () => ({})) },
  ),
}));

import { Scheduled } from "./Scheduled";
import { useStore } from "../store";
import type { ScheduleDetail, Session } from "../types";

const sid = "20260723-043024-nightly";
const canonical: Session = {
  id: sid,
  status: "active",
  turns: 4,
  title: "Nightly dependency audit",
  workspace: "/repo/product",
  kind: "driver",
  schedule: "interval",
  cadence: "Every 30m",
  nextRunAt: "2099-01-01T00:00:00Z",
  scheduleControl: true,
  scheduleDetail: true,
};
const activeDetail: ScheduleDetail = {
  kind: "series",
  sessionId: sid,
  name: "Nightly dependency audit",
  status: "idle",
  prompt: "Audit dependencies and report only actionable changes.",
  workspace: "/repo/product",
  agent: "auditor",
  provider: "gemini",
  model: "gemini-2.5-pro",
  thinkingEnabled: true,
  thinkingBudgetTokens: 4096,
  schedule: "interval",
  cadence: "Every 30m",
  interval: "30m",
  overlap: "coalesce",
  iterations: 3,
  maxIterations: 12,
  nextRunAt: "2099-01-01T00:00:00Z",
  scheduleControl: true,
};

function mount(session: Session = canonical) {
  useStore.setState({
    runs: [],
    sessions: [session],
    sessionsReady: true,
    currentSid: null,
    currentRunId: null,
    currentPage: "scheduled",
    scheduledDetailSid: null,
    unread: [session.id],
    archived: [],
    pinned: [],
    renames: {},
    modal: null,
    toasts: [],
  });
  return render(<Scheduled />);
}

beforeEach(() => {
  window.history.replaceState(null, "", "/");
  scheduleDetail.mockReset();
  scheduleDetail.mockResolvedValue(activeDetail);
  schedule.mockClear();
  sessions.mockReset();
  sessions.mockResolvedValue([]);
});
afterEach(cleanup);

describe("typed Scheduled detail journey (G56)", () => {
  it("opens the safe detail route, marks the row read, and shows the product projection", async () => {
    const { container } = mount();
    fireEvent.click(screen.getByText("Nightly dependency audit"));

    await screen.findByRole("heading", { name: "Nightly dependency audit" });
    expect(scheduleDetail).toHaveBeenCalledWith(sid);
    expect(window.location.hash).toBe(`#scheduled:${sid}`);
    expect(useStore.getState().unread).toEqual([]);
    expect(container.querySelector(".scheduled-shell")?.classList.contains("has-detail")).toBe(true);
    expect(screen.getByText("Active")).toBeTruthy();
    expect(screen.getByText(activeDetail.prompt!)).toBeTruthy();
    expect(screen.getByText("auditor")).toBeTruthy();
    expect(screen.getByText("gemini · gemini-2.5-pro")).toBeTruthy();
    expect(screen.getByText("4,096 token budget")).toBeTruthy();
    expect(screen.getByText("3 of 12")).toBeTruthy();
    expect(screen.queryByText(/SECRET_SYSTEM_PROMPT|verifier command|tool config/i)).toBeNull();
  });

  it("pauses in place, refreshes the durable state, then opens iteration history", async () => {
    const paused = { ...activeDetail, status: "paused", nextRunAt: undefined };
    scheduleDetail.mockResolvedValueOnce(activeDetail).mockResolvedValue(paused);
    sessions.mockResolvedValue([{ ...canonical, status: "paused", nextRunAt: undefined }]);
    mount();
    fireEvent.click(screen.getByText("Nightly dependency audit"));

    fireEvent.click(await screen.findByRole("button", { name: "Pause" }));
    await waitFor(() => expect(schedule).toHaveBeenCalledWith(sid, "pause"));
    await screen.findByRole("button", { name: "Resume" });
    expect(screen.getByText("Paused", { selector: "dd" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Open history" }));
    expect(useStore.getState().currentSid).toBe(sid);
    expect(useStore.getState().scheduledDetailSid).toBeNull();
    expect(window.location.hash).toBe(`#${sid}`);
  });

  it("restores focus on Escape and reloads from a direct detail route", async () => {
    mount();
    const row = screen.getByText("Nightly dependency audit").closest("button")!;
    fireEvent.click(row);
    await screen.findByRole("button", { name: "Close schedule details" });
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(row));
    expect(window.location.hash).toBe("#scheduled");

    useStore.getState().showScheduledDetail(sid);
    await waitFor(() => expect(scheduleDetail).toHaveBeenCalledTimes(2));
    expect(window.location.hash).toBe(`#scheduled:${sid}`);
  });

  it("keeps the error visible, retries, and never invents detail for a legacy journal", async () => {
    scheduleDetail.mockRejectedValueOnce(new Error("journal unavailable")).mockResolvedValue(activeDetail);
    const { unmount } = mount();
    fireEvent.click(screen.getByText("Nightly dependency audit"));
    expect((await screen.findByRole("alert")).textContent).toContain("journal unavailable");
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByText(activeDetail.prompt!);

    unmount();
    scheduleDetail.mockClear();
    const legacy = { ...canonical, id: "legacy-series", scheduleDetail: undefined, scheduleControl: undefined };
    mount(legacy);
    fireEvent.click(screen.getByText("Nightly dependency audit"));
    expect(scheduleDetail).not.toHaveBeenCalled();
    expect(useStore.getState().currentSid).toBe("legacy-series");
    expect(window.location.hash).toBe("#legacy-series");
  });
});

// @vitest-environment jsdom
//
// SC-17 — the Scheduled hub could diagnose a broken schedule and do nothing about
// it: the row menu was Pin / Rename / Mark as unread / Archive / Copy, five ways
// to tidy a series and zero ways to fix one, on the single screen that prints
// "Needs recovery" in amber. The daemon calls have always existed
// (POST /api/sessions/{sid}/{resume,retry,stop,close}) and SessionView has always
// made them. These tests pin that the menu now reaches them — with real spies, so
// "the item is rendered" cannot pass for "the item does something".
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// vi.mock is hoisted above the imports, so the spies have to be too.
const { resume, retry, schedule, stopSession, stopRun } = vi.hoisted(() => ({
  resume: vi.fn(async () => ({})),
  retry: vi.fn(async () => ({})),
  schedule: vi.fn(async () => ({})),
  stopSession: vi.fn(async () => ({})),
  stopRun: vi.fn(async () => ({})),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: { resume, retry, schedule, stopSession, stopRun },
}));

import { Scheduled } from "./Scheduled";
import { useStore } from "../store";
import type { Session } from "../types";

afterEach(cleanup);
beforeEach(() => {
  [resume, retry, schedule, stopSession, stopRun].forEach((f) => f.mockClear());
});

const sessions: Session[] = [
  {
    id: "20250101-100000-stranded",
    status: "stranded",
    turns: 2,
    title: "Broken: the host died mid-tick",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "interval",
    cadence: "Every 30m",
  },
  {
    id: "20250101-090000-running",
    status: "running",
    turns: 5,
    title: "Live: an iteration is executing",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "cron",
    cadence: "Daily at 6:00 AM",
    scheduleControl: true,
  },
  {
    id: "20250101-080000-paused",
    status: "paused",
    turns: 3,
    title: "Paused: nightly digest",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "interval",
    cadence: "Every 1h",
    scheduleControl: true,
  },
];

const mount = () => {
  useStore.setState({
    runs: [],
    sessions,
    sessionsReady: true,
    unread: [],
    archived: [],
    pinned: [],
    renames: {},
    modal: null,
  });
  return render(<Scheduled />);
};

const menuFor = (title: string) => {
  fireEvent.contextMenu(screen.getByText(title).closest(".scheduled-row-wrap")!);
};

describe("the row menu can act on the schedule, not just tidy it (SC-17)", () => {
  it("resumes a stranded series from the hub that says it needs recovery", () => {
    mount();
    menuFor("Broken: the host died mid-tick");
    fireEvent.click(screen.getByRole("menuitem", { name: "Resume" }));
    expect(resume).toHaveBeenCalledWith("20250101-100000-stranded");
  });

  it("starts a replacement driver without opening the session", () => {
    mount();
    menuFor("Broken: the host died mid-tick");
    fireEvent.click(screen.getByRole("menuitem", { name: "Retry" }));
    expect(retry).toHaveBeenCalledWith("20250101-100000-stranded");
  });

  it("pauses an active series and resumes only the durably paused row", () => {
    mount();
    menuFor("Live: an iteration is executing");
    fireEvent.click(screen.getByRole("menuitem", { name: "Pause" }));
    expect(schedule).toHaveBeenCalledWith("20250101-090000-running", "pause");

    menuFor("Paused: nightly digest");
    const items = screen.getAllByRole("menuitem").map((item) => item.textContent);
    expect(items).toContain("Resume");
    expect(items).not.toContain("Pause");
    expect(items).not.toContain("Retry");
    expect(items).not.toContain("Cancel series…");
    fireEvent.click(screen.getByRole("menuitem", { name: "Resume" }));
    expect(schedule).toHaveBeenCalledWith("20250101-080000-paused", "resume");
    expect(resume).not.toHaveBeenCalledWith("20250101-080000-paused");
  });

  it("does not expose Pause on a legacy driver journal", () => {
    useStore.setState({
      runs: [],
      sessions: [{
        ...sessions[1],
        id: "20250101-legacy",
        title: "Legacy scheduled history",
        scheduleControl: undefined,
      }],
      sessionsReady: true,
      unread: [], archived: [], pinned: [], renames: {}, modal: null,
    });
    render(<Scheduled />);
    menuFor("Legacy scheduled history");
    expect(screen.queryByRole("menuitem", { name: "Pause" })).toBeNull();
  });

  it("cancels a live series through the confirm modal — the series' own domain terminal (INC-83)", async () => {
    const { container } = mount();
    menuFor("Live: an iteration is executing");
    const items = [...container.querySelectorAll(".ctx-menu [role='menuitem']")].map((e) => e.textContent);
    expect(items).not.toContain("Resume"); // a healthy running series has nothing to recover
    expect(items).not.toContain("Close…"); // no session lifecycle verbs on this page
    fireEvent.click(screen.getByRole("menuitem", { name: "Cancel series…" }));

    // Nothing is cancelled yet: the modal is up.
    expect(stopSession).not.toHaveBeenCalled();
    const modal = useStore.getState().modal;
    expect(modal?.kind).toBe("confirm");

    await (modal as { onConfirm: () => Promise<void> }).onConfirm();
    expect(stopSession).toHaveBeenCalledWith("20250101-090000-running");
  });
});

describe("INC-80.3 · the series SESSION row is canonical", () => {
  it("hides a drive run row once its session landed in the sessions list", () => {
    useStore.setState({
      runs: [
        {
          id: "run7", kind: "drive", label: "drive: nightly", workspace: "/repo/app",
          sessionId: "20250101-090000-running", status: "running",
          startedAt: "2025-01-01T09:00:00Z", schedule: "cron", cadence: "Daily at 6:00 AM",
        } as any,
      ],
      sessions,
      sessionsReady: true,
      unread: [], archived: [], pinned: [], renames: {}, modal: null,
    });
    render(<Scheduled />);
    // Exactly one row for the cron series: the session's title, not the
    // run label — one piece of scheduled work, one row.
    expect(screen.getByText("Live: an iteration is executing")).toBeTruthy();
    expect(screen.queryByText("drive: nightly")).toBeNull();
  });

  it("keeps the run row while the session id is still unknown", () => {
    useStore.setState({
      runs: [
        {
          id: "run8", kind: "drive", label: "drive: warming-up", workspace: "/repo/app",
          status: "running", startedAt: "2025-01-01T09:00:00Z",
          schedule: "interval", cadence: "Every 30m",
        } as any,
      ],
      sessions: [],
      sessionsReady: true,
      unread: [], archived: [], pinned: [], renames: {}, modal: null,
    });
    render(<Scheduled />);
    expect(screen.getByText("drive: warming-up")).toBeTruthy();
  });
});

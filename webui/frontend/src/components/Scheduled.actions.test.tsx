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
const { resume, retry, stopSession, closeSession, stopRun } = vi.hoisted(() => ({
  resume: vi.fn(async () => ({})),
  retry: vi.fn(async () => ({})),
  stopSession: vi.fn(async () => ({})),
  closeSession: vi.fn(async () => ({})),
  stopRun: vi.fn(async () => ({})),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: { resume, retry, stopSession, closeSession, stopRun },
}));

import { Scheduled } from "./Scheduled";
import { useStore } from "../store";
import type { Session } from "../types";

afterEach(cleanup);
beforeEach(() => {
  [resume, retry, stopSession, closeSession, stopRun].forEach((f) => f.mockClear());
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

  it("retries the last message without opening the session", () => {
    mount();
    menuFor("Broken: the host died mid-tick");
    fireEvent.click(screen.getByRole("menuitem", { name: "Retry" }));
    expect(retry).toHaveBeenCalledWith("20250101-100000-stranded");
  });

  it("stops the series that is actually executing — and only that one", () => {
    const { container } = mount();
    menuFor("Broken: the host died mid-tick");
    let items = [...container.querySelectorAll(".ctx-menu [role='menuitem']")].map((e) => e.textContent);
    expect(items).not.toContain("Stop"); // nothing is running on a dead series

    fireEvent.keyDown(document.body, { key: "Escape" });
    menuFor("Live: an iteration is executing");
    items = [...container.querySelectorAll(".ctx-menu [role='menuitem']")].map((e) => e.textContent);
    expect(items).not.toContain("Resume"); // …and a healthy running series has nothing to recover
    fireEvent.click(screen.getByRole("menuitem", { name: "Stop" }));
    expect(stopSession).toHaveBeenCalledWith("20250101-090000-running");
  });

  it("asks before closing — Close is destructive, so it goes through the confirm modal", async () => {
    mount();
    menuFor("Live: an iteration is executing");
    fireEvent.click(screen.getByRole("menuitem", { name: "Close…" }));

    // Nothing is closed yet: the modal is.
    expect(closeSession).not.toHaveBeenCalled();
    const modal = useStore.getState().modal;
    expect(modal?.kind).toBe("confirm");

    await (modal as { onConfirm: () => Promise<void> }).onConfirm();
    expect(closeSession).toHaveBeenCalledWith("20250101-090000-running");
  });
});

// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// Nothing here depends on the network; stub the API with never-settling promises
// (same pattern as Sidebar.nav / loadingStates).
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy({}, { get: () => () => new Promise(() => {}) }),
}));

import { Scheduled, hasRhythm, isLimitStatus } from "./Scheduled";
import { useStore } from "../store";
import type { Run, Session } from "../types";

afterEach(cleanup);

// The page's admission rule (SC-1): a row belongs here only if it will fire
// again on its own. The schedule kind is the server's word for that
// (webui/schedule.go): interval / cron / self_paced tick; immediate (a one-shot
// session or a run-until-verified goal) and parallel (Best of N) do not, and a
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

// SC-10 / SC-11 fixtures — one of each kind of series this page has to tell
// apart. All four are rhythmic (SC-1), so all four are on the page; what differs
// is whether the series is still going and whether it is broken.
const seriesSessions: Session[] = [
  {
    // Healthy, and idle THIS SECOND because it fires every 30 minutes — the case
    // the old tick-level rule filed under "Finished".
    id: "20260712-100000-healthy",
    status: "idle",
    turns: 4,
    title: "Healthy: watch the queue",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "interval",
    cadence: "Every 30m",
    nextRunAt: "2099-01-01T00:00:00Z",
  },
  {
    // The live regression (driver 20260712-033455-cx3-cadence-61b9): claims a
    // 30-minute rhythm, has no next tick, and has been dead for hours.
    id: "20260712-090000-stranded",
    status: "stranded",
    turns: 2,
    title: "Broken: cadence driver that died",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "interval",
    cadence: "Every 30m",
  },
  {
    id: "20260712-080000-crashed",
    status: "crashed",
    turns: 1,
    title: "Broken: nightly sweep that crashed",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "cron",
    cadence: "Daily at 6:00 AM",
  },
  {
    id: "20260712-070000-done",
    status: "satisfied",
    turns: 6,
    title: "Finished: the goal was met",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "self_paced",
    cadence: "Self-paced",
  },
];

const mountSeries = () => {
  useStore.setState({
    runs: [],
    sessions: seriesSessions,
    sessionsReady: true,
    unread: [],
    archived: [],
    renames: {},
  });
  return render(<Scheduled />);
};

const tab = (name: string) => fireEvent.click(screen.getByRole("tab", { name }));
const titles = (c: HTMLElement) =>
  [...c.querySelectorAll(".scheduled-row .scheduled-copy b")].map((e) => e.textContent);

describe("Active is a fact about the series, not about this instant (SC-11)", () => {
  it("keeps a healthy between-ticks series in Active — it is not Finished", () => {
    const { container } = mountSeries();
    tab("Active");
    // The whole bug: a series with a future tick is ACTIVE even though nothing
    // is executing right now. Before this rule the Active tab was structurally
    // empty (live: All=3, Active=0) because no cadence session is ever mid-tick
    // when you happen to look.
    expect(titles(container)).toContain("Healthy: watch the queue");

    tab("Finished");
    expect(titles(container)).not.toContain("Healthy: watch the queue");
  });

  it("keeps BROKEN series in Active — a dead schedule is waiting on you, not done", () => {
    const { container } = mountSeries();
    tab("Active");
    expect(titles(container)).toContain("Broken: cadence driver that died");
    expect(titles(container)).toContain("Broken: nightly sweep that crashed");

    tab("Finished");
    const finished = titles(container);
    expect(finished).not.toContain("Broken: cadence driver that died");
    expect(finished).not.toContain("Broken: nightly sweep that crashed");
  });

  it("Finished holds only terminal series — nothing more will ever happen there", () => {
    const { container } = mountSeries();
    tab("Finished");
    expect(titles(container)).toEqual(["Finished: the goal was met"]);

    tab("Active");
    expect(titles(container)).not.toContain("Finished: the goal was met");
    // …and Active is no longer empty by construction: everything still ticking
    // or still needing you is here.
    expect(titles(container)).toHaveLength(3);
  });
});

describe("a broken schedule says so on screen (SC-10)", () => {
  it("shows the stranded series' state as visible text, not a tooltip", () => {
    const { container } = mountSeries();
    // The status phrase used to live only in title=, so the row rendered as
    // "Every 30m · Ran 4h ago" behind a gray circle — indistinguishable from a
    // healthy one. It has to be readable.
    const warn = screen.getByText("Needs recovery");
    expect(warn).toBeTruthy();
    expect(warn.className).toContain("sched-warn");
    expect(warn.className).toContain("is-stranded");

    // It takes the next-run slot (there is no next run) and never wears the
    // live-fact blue.
    expect(warn.className).not.toContain("sched-next");

    // Both halves of the row carry the alarm: the leading glyph (no longer the
    // healthy gray Circle) and the phrase.
    const broken = screen.getByText("Broken: cadence driver that died").closest(".scheduled-row")!;
    expect(broken.querySelector(".sched-glyph.sched-warn.is-stranded")).toBeTruthy();

    // A crash is the louder of the two, and says a different word.
    const crashed = screen.getByText("Failed");
    expect(crashed.className).toContain("sched-warn");
    expect(crashed.className).toContain("is-crash");
    expect(container.querySelectorAll(".sched-glyph.sched-warn.is-crash").length).toBe(1);
  });

  it("leaves a healthy row unmarked — the warning has to mean something", () => {
    mountSeries();
    const healthy = screen.getByText("Healthy: watch the queue").closest(".scheduled-row")!;
    expect(healthy.querySelector(".sched-warn")).toBeNull();
    expect(healthy.querySelector(".sched-next")).toBeTruthy();
  });

  it("finds a broken series by searching for its state", () => {
    const { container } = mountSeries();
    fireEvent.change(screen.getByLabelText("Search scheduled runs"), {
      target: { value: "needs recovery" },
    });
    expect(titles(container)).toEqual(["Broken: cadence driver that died"]);
  });
});

describe("every row wears its state on its left (SCH-ICON)", () => {
  it("leaves no row with an empty icon slot", () => {
    const { container } = mountSeries();
    const rows = [...container.querySelectorAll(".scheduled-row")];
    expect(rows).toHaveLength(4);
    // The finished row used to render `.sched-blank` — an empty span in a column
    // that still reserved the icon's width, so it read as an icon that had failed
    // to load. Every row is anchored now.
    for (const row of rows) expect(row.querySelector(".sched-glyph")).toBeTruthy();
    expect(container.querySelector(".sched-blank")).toBeNull();
  });

  it("spends the alert colour on the broken rows and nothing else", () => {
    const { container } = mountSeries();
    // Adding a glyph to every row must not add an alarm to every row: exactly the
    // two genuinely broken series are allowed to be coloured (SC-10 / SC-16).
    expect(container.querySelectorAll(".sched-glyph.sched-warn")).toHaveLength(2);
    for (const title of ["Healthy: watch the queue", "Finished: the goal was met"]) {
      const row = screen.getByText(title).closest(".scheduled-row")!;
      expect(row.querySelector(".sched-glyph")!.className).not.toContain("sched-warn");
    }
  });

  it("steps a finished row back a shade, and never a live or a broken one", () => {
    mountSeries();
    const quiet = (t: string) => screen.getByText(t).closest(".scheduled-row")!.className.includes("is-quiet");
    // Codex greys the whole paused row, title included; the rows still ticking —
    // and the broken one, which needs you — keep their emphasis.
    expect(quiet("Finished: the goal was met")).toBe(true);
    expect(quiet("Healthy: watch the queue")).toBe(false);
    expect(quiet("Broken: cadence driver that died")).toBe(false);
    expect(quiet("Broken: nightly sweep that crashed")).toBe(false);
  });
});

// SC-12 / SC-13 / SC-14 — one driver session (the thing most rows are) and one
// interval RUN, both titled the way real ones are: with the prompt that made
// them.
const promptTitled: Session[] = [
  {
    id: "20260712-033455-cx3",
    status: "idle",
    turns: 5,
    title: "Append one line with the current timestamp to notes.md, then commit it (use write_file or bash).",
    workspace: "/Users/me/scratch",
    kind: "driver",
    schedule: "interval",
    cadence: "Every 30m",
    nextRunAt: "2099-01-01T00:00:00Z",
  },
];
const promptRuns: Run[] = [
  {
    id: "r-live",
    kind: "drive",
    label: "Watch the build and report failures. Ping me when it goes red.",
    workspace: "/Users/me/agentrunner",
    status: "running",
    startedAt: "2026-07-11T09:00:00Z",
    schedule: "interval",
    cadence: "Every 10m",
    nextRunAt: "2099-01-01T00:00:00Z",
  },
];

type StoreState = ReturnType<typeof useStore.getState>;

const mountRich = (over: Partial<StoreState> = {}) => {
  useStore.setState({
    runs: promptRuns,
    sessions: promptTitled,
    sessionsReady: true,
    unread: [],
    archived: [],
    pinned: [],
    renames: {},
    ...over,
  });
  return render(<Scheduled />);
};

describe("a row is titled with a NAME, not the prompt (SC-13)", () => {
  it("derives a scannable name and keeps the whole prompt one hover away", () => {
    const { container } = mountRich();
    const row = container.querySelector(".scheduled-row")!;
    const shown = row.querySelector(".scheduled-copy b")!.textContent!;

    // The live row: 96 characters of instructions, of which the first clause is
    // the only part that identifies the session.
    expect(shown).toBe("Append one line with the current timestamp to…");
    expect(shown.length).toBeLessThan(50);
    expect(shown).not.toContain("use write_file or bash");

    // Nothing is hidden: the raw prompt is the row's tooltip…
    expect(row.getAttribute("title")).toContain("(use write_file or bash)");
    // …and it is still searchable, so shortening the label made nothing
    // unfindable.
    fireEvent.change(screen.getByLabelText("Search scheduled runs"), { target: { value: "write_file" } });
    expect(container.querySelectorAll(".scheduled-row")).toHaveLength(1);
  });

  it("lets a user rename win over the derived name", () => {
    const { container } = mountRich({ renames: { "20260712-033455-cx3": "Timestamp notes" } });
    expect(titles(container)).toContain("Timestamp notes");
  });
});

describe("a scheduled row can be acted on (SC-12)", () => {
  it("opens the row menu on right-click, with the actions the row actually has", () => {
    const { container } = mountRich();
    const session = screen.getByText(/^Append one line/).closest(".scheduled-row-wrap")!;
    fireEvent.contextMenu(session);

    const menu = container.querySelector(".ctx-menu")!;
    expect(menu).toBeTruthy();
    const items = [...menu.querySelectorAll("[role='menuitem']")].map((e) => e.textContent);
    // SC-17: the schedule's own lifecycle leads; the housekeeping follows. This
    // row is healthy and idle between ticks, so there is nothing to resume and
    // nothing to stop — but it can still be retried or closed.
    expect(items).toEqual([
      "Retry",
      "Close…",
      "Pin",
      "Rename…",
      "Mark as unread",
      "Archive",
      "Session ID",
      "Session link",
    ]);
  });

  it("offers Stop on a running run — the hub could not stop anything before", () => {
    const { container } = mountRich();
    const run = screen.getByText(/^Watch the build/).closest(".scheduled-row-wrap")!;
    fireEvent.contextMenu(run);
    const items = [...container.querySelectorAll(".ctx-menu [role='menuitem']")].map((e) => e.textContent);
    expect(items[0]).toBe("Stop");
    // …and never the session-only actions, which would be no-ops on a run.
    expect(items).not.toContain("Archive");
    expect(items).not.toContain("Rename…");
  });

  it("acts: Pin from the menu pins the session and the row says so", () => {
    const { container } = mountRich();
    fireEvent.contextMenu(screen.getByText(/^Append one line/).closest(".scheduled-row-wrap")!);
    fireEvent.click(screen.getByRole("menuitem", { name: "Pin" }));

    expect(useStore.getState().pinned).toContain("20260712-033455-cx3");
    expect(container.querySelector(".scheduled-row .sched-pinned")).toBeTruthy();
  });

  it("carries a ⋯ button per row that opens the same menu, and never clicks the row", () => {
    const { container } = mountRich();
    const before = useStore.getState().currentSid;
    const more = container.querySelectorAll(".sched-more");
    expect(more).toHaveLength(2); // every row has one

    fireEvent.click(more[0]);
    expect(container.querySelector(".ctx-menu")).toBeTruthy();
    // Opening the menu must not also open the session.
    expect(useStore.getState().currentSid).toBe(before);
  });

  it("reaches the menu from the keyboard (Shift+F10), as the sidebar rows do", () => {
    const { container } = mountRich();
    fireEvent.keyDown(container.querySelector(".scheduled-row")!, { key: "F10", shiftKey: true });
    expect(container.querySelector(".ctx-menu")).toBeTruthy();
  });
});

describe("a search hit is visible on the row it returns (SC-14)", () => {
  it("names the project when the project is what matched", () => {
    const { container } = mountRich();
    // Before: searching the workspace returned this row with the word "scratch"
    // nowhere on it — the result looked broken.
    fireEvent.change(screen.getByLabelText("Search scheduled runs"), { target: { value: "scratch" } });
    const rows = [...container.querySelectorAll(".scheduled-row")];
    expect(rows).toHaveLength(1);
    const chip = rows[0].querySelector(".sched-project-chip")!;
    expect(chip).toBeTruthy();
    expect(chip.textContent!.toLowerCase()).toContain("scratch");
  });

  it("keeps the chip off when the project is not what you searched for (SC-4)", () => {
    const { container } = mountRich();
    // Resting state: two facts on the sub-line, no project anywhere.
    expect(container.querySelector(".sched-project-chip")).toBeNull();
    expect(screen.queryByText(/scratch/i)).toBeNull();

    // A query that matched something already visible does not summon it either.
    fireEvent.change(screen.getByLabelText("Search scheduled runs"), { target: { value: "Every 30m" } });
    expect(container.querySelectorAll(".scheduled-row")).toHaveLength(1);
    expect(container.querySelector(".sched-project-chip")).toBeNull();
  });
});

// SC-16 — the three terminal reasons that mean "I did exactly what you configured
// and then stopped", plus one genuinely broken series as the control. friendlyStatus
// files all three under cls "stranded" (right for the session header's banner, fatal
// here), which is why this page judges the raw status word instead.
const limitSessions: Session[] = [
  {
    id: "20250101-100000-iter",
    status: "max_iterations",
    turns: 8,
    title: "Ran its configured 20 iterations",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "interval",
    cadence: "Every 30m",
  },
  {
    id: "20250101-090000-budget",
    status: "limit_exceeded",
    turns: 4,
    title: "Spent its configured token budget",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "cron",
    cadence: "Daily at 6:00 AM",
  },
  {
    id: "20250101-080000-steps",
    status: "max_generation_steps",
    turns: 3,
    title: "Hit its generation-step ceiling",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "self_paced",
    cadence: "Self-paced",
  },
  {
    id: "20250101-070000-stranded",
    status: "stranded",
    turns: 2,
    title: "Genuinely broken: host died",
    workspace: "/repo/app",
    kind: "driver",
    schedule: "interval",
    cadence: "Every 30m",
  },
];

const mountLimits = () => {
  useStore.setState({
    runs: [],
    sessions: limitSessions,
    sessionsReady: true,
    unread: [],
    archived: [],
    pinned: [],
    renames: {},
  });
  return render(<Scheduled />);
};

describe("a configured limit is a finish, not a failure (SC-16)", () => {
  it("spends no alert colour on a series that stopped where you told it to", () => {
    const { container } = mountLimits();
    // Live before this rule: 3/3 rows amber with a WarningCircle, because
    // friendlyStatus calls max_iterations "stranded". Exactly ONE row on this
    // page is actually broken, and it is the only one allowed to say so.
    expect(container.querySelectorAll(".sched-warn.is-stranded, .sched-warn.is-crash")).toHaveLength(2); // the glyph + the phrase of the ONE broken row
    expect(screen.getByText("Needs recovery")).toBeTruthy();

    expect(screen.queryByText("Iteration limit reached")).toBeNull();
    expect(screen.queryByText("Budget limit reached")).toBeNull();
    expect(screen.queryByText("Step limit reached")).toBeNull();

    for (const title of ["Ran its configured 20 iterations", "Spent its configured token budget", "Hit its generation-step ceiling"]) {
      const row = screen.getByText(title).closest(".scheduled-row")!;
      expect(row.querySelector(".sched-warn")).toBeNull();
      // …and it settles like every other finished row: a NEUTRAL glyph (SCH-ICON
      // — it used to be an empty slot, which read as an icon that failed to
      // load), a quiet title, and "Ran 2d ago" where the alarm used to be.
      const glyph = row.querySelector(".sched-glyph")!;
      expect(glyph).toBeTruthy();
      expect(glyph.className).not.toContain("sched-warn");
      expect(row.className).toContain("is-quiet");
      expect(row.querySelector(".sched-sub")!.textContent).toMatch(/Ran .+ ago/);
    }
  });

  it("files a limit-reached series under Finished — it will never fire again", () => {
    const { container } = mountLimits();
    tab("Active");
    // The whole Active tab used to be these rows (live: All=3 / Active=3 /
    // Finished=0), which made the one honest question this screen answers —
    // "is my background work still running?" — answer wrong.
    expect(titles(container)).toEqual(["Genuinely broken: host died"]);

    tab("Finished");
    expect(titles(container)).toEqual([
      "Ran its configured 20 iterations",
      "Spent its configured token budget",
      "Hit its generation-step ceiling",
    ]);
  });

  it("offers no Resume on a limit row — there is nothing broken to recover", () => {
    const { container } = mountLimits();
    fireEvent.contextMenu(screen.getByText("Ran its configured 20 iterations").closest(".scheduled-row-wrap")!);
    let items = [...container.querySelectorAll(".ctx-menu [role='menuitem']")].map((e) => e.textContent);
    expect(items).not.toContain("Resume");
    // A terminal row has no live conversation to retry or close either.
    expect(items).not.toContain("Retry");
    expect(items).not.toContain("Close…");

    // The broken one does (SC-17).
    fireEvent.keyDown(document.body, { key: "Escape" });
    fireEvent.contextMenu(screen.getByText("Genuinely broken: host died").closest(".scheduled-row-wrap")!);
    items = [...container.querySelectorAll(".ctx-menu [role='menuitem']")].map((e) => e.textContent);
    expect(items.slice(0, 3)).toEqual(["Resume", "Retry", "Close…"]);
  });
});

describe("isLimitStatus", () => {
  it("recognises every terminal reason that means 'configured limit reached'", () => {
    expect(isLimitStatus("max_iterations")).toBe(true);
    expect(isLimitStatus("max_generation_steps")).toBe(true);
    expect(isLimitStatus("limit_exceeded")).toBe(true);
    expect(isLimitStatus("budget_exhausted")).toBe(true);
    expect(isLimitStatus("MAX_TOKENS")).toBe(true);
  });

  it("leaves the states that really are broken alone", () => {
    expect(isLimitStatus("stranded")).toBe(false);
    expect(isLimitStatus("crashed")).toBe(false);
    expect(isLimitStatus("running")).toBe(false);
    expect(isLimitStatus("")).toBe(false);
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

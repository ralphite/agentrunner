// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// INC-41 TH-14 / TH-15 — the chrome around a settled session.
//
// TH-14: one ending, said once. A step-limited session with a cancelled goal
// used to pin TWO banners over the composer (`terminal-alert` + `gbar`, 93px)
// and then say it a third time in the right rail's GOAL block — while the
// thread itself was squeezed to 630px of a 900px window. Codex pins nothing at
// all; we keep the one banner that carries the action, and the goal's outcome
// rides inside it.
//
// TH-15: one rail, one name, one door. The topbar carried `Changes` AND
// `Supervision`; the panel `Supervision` opened was titled `Environment`, whose
// first row was `Changes`. Three names, two doors, one object.

const { arMock } = vi.hoisted(() => ({ arMock: {} as Record<string, (...args: any[]) => any> }));
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy(
    {},
    {
      get: (_target, prop: string) => (...args: any[]) =>
        arMock[prop] ? arMock[prop](...args) : new Promise(() => {}),
    },
  ),
  uploadURL: (path: string) => path,
  diffPath: () => "",
}));

import { SessionView } from "./SessionView";
import { Modals } from "./Modals";
import { useStore } from "../store";

const SID = "20260711-011831-what-is-the-project-297d";
const GOAL = "TH-3 live check: verify the Environment rail renders a Goal section";
// goal_attached → goal_cancelled spans 34s, so the banner's elapsed reads 00:34.
const EVENTS = [
  { seq: 1, type: "goal_attached", ts: "2026-07-11T01:18:31.000Z", payload: { goal: GOAL } },
  { seq: 2, type: "input_received", ts: "2026-07-11T01:18:32.000Z", payload: { source: "cli", text: "what is the project" } },
  { seq: 3, type: "assistant_message", ts: "2026-07-11T01:18:40.000Z", payload: { text: "AgentRunner." } },
  { seq: 4, type: "goal_cancelled", ts: "2026-07-11T01:19:05.000Z", payload: {} },
];

class FakeEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  addEventListener = vi.fn();
}

// A settled, step-limited session that carried a goal — the exact shape TH-14
// is about (live session `20260711-011831-…` on 8809).
beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  (globalThis as any).EventSource = FakeEventSource;
  // jsdom ships none of these; the thread mounts a full TimelineView here (this
  // is the whole session view, not a slice), which observes and scrolls.
  (globalThis as any).ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  (window.HTMLElement.prototype as any).scrollIntoView = () => {};
  // Query-aware stub driven by innerWidth: useBreakpoint now reads
  // matchMedia (the CSS seam), and these specs express viewport as
  // innerWidth — keep one truth by deriving matches from it.
  (window as any).matchMedia = (q: string) => ({
    matches: (() => {
      const m = /max-width:\s*(\d+)px/.exec(q);
      return m ? window.innerWidth <= Number(m[1]) : false;
    })(),
    addEventListener: () => {},
    removeEventListener: () => {},
  });
  (window as any).innerWidth = 1401;
  localStorage.clear();
  arMock.events = async (_sid: string, after: number) => (after ? [] : EVENTS);
  arMock.rawEvents = async () => EVENTS;
  arMock.inspect = async () => ({ goal: null, children: [], progress: [], artifacts: [] });
  arMock.ps = async () => [];
  arMock.queue = async () => [];
  arMock.diff = async () => ({
    workspace: "/tmp/wt-th14",
    known: true,
    isRepo: true,
    nested: false,
    diff: "diff --git a/x.ts b/x.ts\n--- a/x.ts\n+++ b/x.ts\n@@ -1 +1 @@\n-old\n+new\n",
    untracked: [],
  });
  arMock.gitBranches = async () => ({ isRepo: true, current: "main" });
  useStore.setState({
    health: null,
    sessionsReady: true,
    currentSid: SID,
    sessions: [{ id: SID, title: "what is the project", status: "max_generation_steps", workspace: "/tmp/wt-th14" } as any],
  });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("TH-14 · one terminal banner above the composer", () => {
  it("folds the goal's outcome into the terminal alert instead of stacking a second bar", async () => {
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".terminal-alert")).not.toBeNull());
    // The step-limit alert is the ONE piece of pinned chrome: it carries the
    // action ("Continue in new session"), so it is the one that survives.
    expect(screen.getByText("Step limit reached")).toBeTruthy();
    expect(screen.getByRole("button", { name: /continue in new session/i })).toBeTruthy();

    // …and the goal banner that used to sit under it is gone — its label and
    // elapsed now ride in the alert's meta tail, on the same row.
    expect(container.querySelector(".gbar")).toBeNull();
    expect(container.querySelectorAll(".terminal-alert, .gbar").length).toBe(1);

    const meta = container.querySelector(".terminal-alert-meta")!;
    expect(meta).not.toBeNull();
    expect(meta.textContent).toContain("Goal cancelled");
    expect(meta.textContent).toContain("00:34");

    // Responsive regression anchor (QA v2sim): the continue/inspect variant
    // must use the same stacking grid as the resume variant — the old flex row
    // crushed the text column to 4px on a 390px phone.
    expect(container.querySelector(".terminal-alert")!.className).toContain("grid");
    // The goal text has no room on the row, so it stays one hover away.
    expect(meta.getAttribute("title")).toBe(GOAL);
    // The tail belongs to the alert — not to a sibling banner.
    expect(meta.closest(".terminal-alert")).not.toBeNull();
  });

  it("collapses the rail's settled-goal block to one line (the banner already said it)", async () => {
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".goal-settled-line")).not.toBeNull());
    const line = container.querySelector(".goal-settled-line")!;
    // The phase + the goal itself — the one thing the banner had no room for.
    expect(line.textContent).toContain("Cancelled");
    expect(line.textContent).toContain(GOAL);
    // Not the elapsed / check count a third time, and not a titled Goal block.
    expect(line.textContent).not.toContain("00:34");
    expect(container.querySelector(".goal-meta-settled")).toBeNull();
    expect(container.querySelector(".supervision-panel .goal-actions")).toBeNull();
  });

  it("still gives the goal its own banner when there is no terminal alert to ride on", async () => {
    // Same events, but a session that ended normally: no terminal alert exists,
    // so the goal's ending must not vanish with it.
    useStore.setState({
      sessions: [{ id: SID, title: "what is the project", status: "completed", workspace: "/tmp/wt-th14" } as any],
    });
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".gbar")).not.toBeNull());
    expect(container.querySelector(".terminal-alert")).toBeNull();
    expect(screen.getByText("Goal cancelled")).toBeTruthy();
  });

  it("opens the formatted Run details projection instead of a raw JSON wall", async () => {
    useStore.setState({
      sessions: [{
        id: SID,
        title: "scheduled series",
        status: "max_iterations",
        workspace: "/tmp/wt-th14",
        kind: "driver",
      } as any],
    });
    arMock.inspect = vi.fn(async () => ({
      kind: "driver",
      spec: "loop",
      status: "ended",
      reason: "max_iterations",
      gen_steps: 2,
      turns: 1,
      entries: [
        { gen_step: 1, kind: "iteration", name: "completed", detail: "pass=false score=0" },
        { gen_step: 2, kind: "iteration", name: "completed", detail: "pass=false score=0" },
      ],
      usage: { input_tokens: 5058, output_tokens: 184, billed: 5242 },
      children: [],
    }));

    const { container } = render(<><SessionView sid={SID} /><Modals /></>);
    await waitFor(() => expect(container.querySelector(".terminal-alert")).not.toBeNull());
    fireEvent.click(container.querySelector(".terminal-alert-action")!);

    await waitFor(() => expect(screen.getByRole("dialog", { name: "Run details" })).toBeTruthy());
    expect(arMock.inspect).toHaveBeenCalledWith(SID);
    expect(screen.getByText("Overview")).toBeTruthy();
    expect(screen.getByText("Usage")).toBeTruthy();
    expect(screen.getByText("Activity")).toBeTruthy();
    expect(container.querySelector(".run-details")).not.toBeNull();
    expect(container.querySelector(".rd-raw summary")?.textContent).toBe("Raw run data");
    expect(container.querySelector(".viewer-modal")).toBeNull();
  });
});

describe("TH-13/14 · compact live goal controls", () => {
  beforeEach(() => {
    localStorage.setItem("arwebui.supervision", "0");
    const activeEvents = [
      { seq: 1, type: "goal_attached", ts: new Date(Date.now() - 65_000).toISOString(), payload: { goal: GOAL, budget: { max_checks: 5 } } },
    ];
    arMock.events = async (_sid: string, after: number) => after ? [] : activeEvents;
    arMock.rawEvents = async () => activeEvents;
    arMock.inspect = async () => ({
      goal: { goal: GOAL, checks: 2, max_checks: 5, paused: false, verifiers: 1 },
      children: [],
      progress: [
        { id: "discover", title: "Inspect requirements", status: "done" },
        { id: "build", title: "Implement release", status: "running" },
      ],
      artifacts: [],
    });
    arMock.goal = vi.fn(async () => ({}));
    useStore.setState({
      sessions: [{ id: SID, title: "release goal", status: "waiting:input", workspace: "/tmp/wt-th14" } as any],
    });
  });

  it("keeps objective and checks in Environment instead of repeating them across the composer", async () => {
    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".gbar-live")).not.toBeNull());

    const bar = container.querySelector(".gbar-live")!;
    expect(bar.textContent).toContain("Pursuing goal");
    expect(bar.textContent).not.toContain(GOAL);
    expect(bar.textContent).not.toContain("2/5 checks");

    const details = screen.getByRole("button", { name: "Open goal details" });
    details.focus();
    fireEvent.click(details);
    await waitFor(() => expect(screen.getByRole("complementary", { name: "Environment" })).toBeTruthy());
    const panel = screen.getByRole("complementary", { name: "Environment" });
    expect(panel.textContent).toContain(GOAL);
    expect(panel.textContent).toContain("2/5 checks");
    expect(panel.textContent).toContain("Progress");
    expect(panel.textContent).toContain("Implement release");

    fireEvent.click(screen.getByRole("button", { name: "Hide Environment" }));
    await waitFor(() => expect(screen.queryByRole("complementary", { name: "Environment" })).toBeNull());
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.activeElement).toBe(details);
  });

  it("keeps pause and edit as direct quick actions", async () => {
    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".gbar-live")).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Pause goal" }));
    await waitFor(() => expect(arMock.goal).toHaveBeenCalledWith(SID, { action: "pause" }));

    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    expect((container.querySelector(".gbar-input") as HTMLInputElement).value).toBe(GOAL);
    expect(screen.getByRole("button", { name: "Save" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Discard" })).toBeTruthy();
  });

  it("keeps the current progress step visible and opens the full checklist", async () => {
    render(<SessionView sid={SID} />);
    const summary = await screen.findByRole("button", { name: "Open progress details" });
    expect(summary.textContent).toContain("Step 2 / 2");
    expect(summary.textContent).toContain("Implement release");
    expect(summary.textContent).toContain("1/2");

    summary.focus();
    fireEvent.click(summary);
    await waitFor(() => expect(screen.getByRole("complementary", { name: "Environment" })).toBeTruthy());
    expect(screen.getByRole("complementary", { name: "Environment" }).textContent).toContain("Inspect requirements");
    expect(screen.getByRole("complementary", { name: "Environment" }).textContent).toContain("Implement release");

    fireEvent.click(screen.getByRole("button", { name: "Hide Environment" }));
    await waitFor(() => expect(screen.queryByRole("complementary", { name: "Environment" })).toBeNull());
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.activeElement).toBe(summary);
  });
});

describe("scheduled selected-iteration semantics", () => {
  const driverSession = () => useStore.setState({
    sessions: [{ id: SID, title: "scheduled series", status: "max_iterations", workspace: "/tmp/wt-th14", kind: "driver" } as any],
  });

  it("does not call a normal interval series a Best-of-N winner", async () => {
    driverSession();
    arMock.events = async (_sid: string, after: number) => after ? [] : [
      { seq: 1, type: "series_started", payload: { series_id: SID, kind: "interval" } },
      { seq: 2, type: "series_iteration", payload: { n: 1, reason: "completed" } },
      { seq: 3, type: "series_ended", payload: { reason: "max_iterations", iterations: 1, best_iter: 1 } },
    ];

    render(<SessionView sid={SID} />);

    await waitFor(() => expect(screen.getByText(/Selected iteration: #1/)).toBeTruthy());
    expect(screen.getByRole("button", { name: "Apply selected iteration" })).toBeTruthy();
    expect(screen.queryByText(/Best-of-N winner/)).toBeNull();
  });

  it("keeps winner language for an actual best-of-N series", async () => {
    driverSession();
    arMock.events = async (_sid: string, after: number) => after ? [] : [
      { seq: 1, type: "series_started", payload: { series_id: SID, kind: "best_of_n" } },
      { seq: 2, type: "series_iteration", payload: { n: 1, reason: "completed" } },
      { seq: 3, type: "series_ended", payload: { reason: "satisfied", iterations: 1, best_iter: 1 } },
    ];

    render(<SessionView sid={SID} />);

    await waitFor(() => expect(screen.getByText(/Best-of-N winner: #1/)).toBeTruthy());
    expect(screen.getByRole("button", { name: "Apply winner" })).toBeTruthy();
  });
});

describe("TH-15 · one rail, one name, one door", () => {
  it("leaves exactly one tool pill in the topbar, named Environment", async () => {
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());
    const tools = [...container.querySelectorAll(".session-topbar .topbar-tool")].map((b) => b.textContent!.trim());
    expect(tools).toEqual(["Environment"]);
    const environment = container.querySelector(".session-topbar .topbar-tool")!;
    expect(environment.querySelector(".topbar-tool-label")!.textContent).toBe("Environment");
    expect(environment.getAttribute("aria-label")).toMatch(/Environment/);
    // The word the pill used to say — and the second door it used to sit next to.
    expect(tools).not.toContain("Supervision");
    expect(tools).not.toContain("Changes");
  });

  it("opens the rail on Environment, whose first row is Changes — and that row opens the diff", async () => {
    const { container } = render(<SessionView sid={SID} />);

    // The rail is open on a wide viewport; its accessible name matches the pill.
    await waitFor(() => expect(container.querySelector("aside.supervision-panel")).not.toBeNull());
    expect(container.querySelector("aside.supervision-panel")!.getAttribute("aria-label")).toBe("Environment");
    await waitFor(() => expect(container.querySelector(".supervision-env")).not.toBeNull());
    const rows = [...container.querySelectorAll(".supervision-env .env-row-label")].map((e) => e.textContent);
    expect(rows[0]).toBe("Changes");

    // TH-15 · this row used to open the diff by synthesising a click on the
    // topbar pill we just deleted. It drives the view directly now.
    fireEvent.click(container.querySelector(".supervision-env .env-row")!);
    await waitFor(() => expect(container.querySelector(".changes-panel")).not.toBeNull());
  });

  it("offers only the other view and omits Copy link", async () => {
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: /more session actions/i }));
    expect(screen.queryByRole("menuitem", { name: "Conversation" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Copy link" })).toBeNull();
    fireEvent.click(screen.getByRole("menuitem", { name: "Changes" }));
    await waitFor(() => expect(container.querySelector(".changes-panel")).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: /more session actions/i }));
    expect(screen.getByRole("menuitem", { name: "Conversation" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Changes" })).toBeNull();
  });

  it("returns focus to the stable menu trigger after closing Changes", async () => {
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());
    const trigger = screen.getByRole("button", { name: "More session actions" });
    fireEvent.click(trigger);
    await new Promise((resolve) => requestAnimationFrame(resolve));
    const changes = screen.getByRole("menuitem", { name: "Changes" });
    changes.focus();
    expect(document.activeElement).toBe(changes);

    fireEvent.click(changes);
    await waitFor(() => expect(container.querySelector(".changes-panel")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Close changes" }));
    await waitFor(() => expect(container.querySelector(".changes-panel")).toBeNull());
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.activeElement).toBe(trigger);
  });

  it("returns an Environment-row launch to the stable Environment trigger", async () => {
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".supervision-env")).not.toBeNull());
    const trigger = screen.getByRole("button", { name: "Environment" });
    const changes = container.querySelector<HTMLButtonElement>(".supervision-env .env-row")!;
    changes.focus();
    expect(document.activeElement).toBe(changes);

    fireEvent.click(changes);
    await waitFor(() => expect(container.querySelector(".changes-panel")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Close changes" }));
    await waitFor(() => expect(container.querySelector(".changes-panel")).toBeNull());
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.activeElement).toBe(trigger);
  });
});

describe("mobile session topbar", () => {
  it("closes Environment when the mobile navigation drawer opens", async () => {
    (window as any).innerWidth = 390;
    const { container, rerender } = render(<SessionView sid={SID} mobileNavigationOpen={false} />);
    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Environment" }));
    await waitFor(() => expect(container.querySelector("aside.supervision-panel")).not.toBeNull());

    rerender(<SessionView sid={SID} mobileNavigationOpen />);
    await waitFor(() => expect(container.querySelector("aside.supervision-panel")).toBeNull());
  });

  it("reserves the sidebar slot and keeps secondary recovery actions in the menu", async () => {
    (window as any).innerWidth = 390;
    arMock.resume = vi.fn(async () => {});
    arMock.events = async (_sid: string, after: number) =>
      after
        ? []
        : [
            { seq: 1, type: "input_received", payload: { source: "cli", text: "hi" } },
            { seq: 2, type: "checkpoint_barrier", payload: { barrier_id: "bar-mobile" } },
          ];
    useStore.setState({
      // QA-0719 S7: "interrupted" (a deliberate user stop) no longer raises
      // the recovery banner — only a genuinely stranded session does.
      sessions: [{ id: SID, title: "深度黑盒 QA 任务 2026-07-12-A", status: "stranded", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());

    const topbar = container.querySelector(".session-topbar")!;
    const navSlot = topbar.querySelector(".session-topbar-nav-slot")!;
    expect(navSlot).toBe(topbar.firstElementChild);
    expect(navSlot.classList.contains("h-9")).toBe(true);
    expect(navSlot.classList.contains("w-9")).toBe(true);
    expect(topbar.querySelector(".tt-title")!.textContent).toBe("深度黑盒 QA 任务 2026-07-12-A");

    // Recovery is the only safe current-state action. Retrying a stranded
    // message could repeat work that finished before the host disappeared.
    expect(topbar.querySelector('button[aria-label="Resume session"]')).not.toBeNull();
    expect(topbar.querySelector('button[aria-label="Retry session"]')).toBeNull();
    expect(topbar.querySelector('button[aria-label="Fork session from checkpoint"]')).toBeNull();

    const recovery = container.querySelector(".terminal-alert")!;
    expect(recovery.classList.contains("grid")).toBe(true);
    expect(screen.getByText("Session needs recovery").classList.contains("block")).toBe(true);
    expect(screen.getByText(/previous host stopped/i).classList.contains("block")).toBe(true);
    const recoveryAction = recovery.querySelector<HTMLButtonElement>(".terminal-alert-action")!;
    expect(recoveryAction.classList.contains("w-full")).toBe(true);
    fireEvent.click(recoveryAction);
    await waitFor(() => expect(arMock.resume).toHaveBeenCalledWith(SID));

    fireEvent.click(screen.getByRole("button", { name: "More session actions" }));
    expect(screen.queryByRole("menuitem", { name: "Retry last message" })).toBeNull();
    expect(screen.getByRole("menuitem", { name: "Continue in new session…" })).toBeTruthy();
  });

  it("does not spend title width on a navigation slot in desktop chrome", async () => {
    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());

    expect(container.querySelector(".session-topbar-nav-slot")).toBeNull();
  });
});

describe("session failure chrome", () => {
  it("prefers the detailed provider failure over the generic failed terminal alert", async () => {
    arMock.events = async (_sid: string, after: number) =>
      after
        ? []
        : [
            { seq: 1, type: "input_received", payload: { source: "cli", text: "check health" } },
            { seq: 2, type: "activity_started", payload: { activity_id: "llm-t1", kind: "llm", name: "complete", attempt: 1 } },
            {
              seq: 3,
              type: "activity_failed",
              payload: {
                activity_id: "llm-t1",
                attempt: 1,
                error: { class: "provider_server", message: "500 internal", retryable: true },
              },
            },
          ];
    useStore.setState({
      sessions: [{ id: SID, title: "failed session", status: "failed", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".turn-error")).not.toBeNull());
    expect(container.querySelectorAll(".turn-error")).toHaveLength(1);
    expect(container.querySelector(".terminal-alert")).toBeNull();
    expect(screen.getByText("The model provider had a server error")).toBeTruthy();
    expect(screen.queryByText("Session failed")).toBeNull();
  });
});

describe("sub-agent session identity", () => {
  it("uses inspect's agent spec as the child header title", async () => {
    const childSid = `${SID}-sub-call_1_2-a1`;
    arMock.inspect = async () => ({ spec: "worker_b", goal: null, children: [], progress: [], artifacts: [] });
    useStore.setState({
      currentSid: childSid,
      sessions: [{ id: childSid, title: "Run the three-worker QA delegation now.", status: "completed", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={childSid} />);
    await waitFor(() => expect(container.querySelector(".tt-title")?.textContent).toBe("worker_b"));
    expect(container.querySelector(".readonly-tag")?.textContent).toContain("Read-only sub-agent");
    expect(container.querySelector(".tt-title")?.getAttribute("title")).toContain("Run the three-worker QA delegation now.");
  });
});

describe("single Continue entry point", () => {
  it("keeps Continue in Advanced and never adds a topbar fork button", async () => {
    arMock.events = async (_sid: string, after: number) =>
      after
        ? []
        : [
            { seq: 1, type: "input_received", payload: { source: "cli", text: "hi" } },
            { seq: 2, type: "checkpoint_barrier", payload: { barrier_id: "bar-t1" } },
          ];
    useStore.setState({
      sessions: [{ id: SID, title: "forkable", status: "completed", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());
    expect(screen.queryByRole("button", { name: /fork session from checkpoint/i })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /more session actions/i }));
    expect(screen.getByRole("menuitem", { name: "Continue in new session…" })).toBeTruthy();
  });
});

describe("single Stop entry point", () => {
  it("keeps Stop in the composer, not the session topbar or menu", async () => {
    useStore.setState({
      sessions: [{ id: SID, title: "running session", status: "running", workspace: "/tmp/wt-th14" } as any],
    });
    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());

    expect(container.querySelector('.session-topbar button[aria-label="Stop active turn"]')).toBeNull();
    expect(container.querySelector('.cx button[aria-label="Stop active turn"]')).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /more session actions/i }));
    expect(screen.queryByRole("menuitem", { name: "Stop" })).toBeNull();
    const labels = [...document.querySelectorAll(".menu-label")].map((label) => label.textContent);
    expect(labels).not.toContain("Run");
  });

  it("renders a deliberate interrupt as Stopped + Retry, never Resume recovery", async () => {
    useStore.setState({
      sessions: [{ id: SID, title: "stopped session", status: "interrupted", workspace: "/tmp/wt-th14" } as any],
    });
    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());

    expect(screen.getByRole("button", { name: "Retry session" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Resume session" })).toBeNull();
    expect(screen.queryByText("Session needs recovery")).toBeNull();
    expect(screen.getByPlaceholderText("Ask for follow-up changes")).toBeTruthy();
  });
});

describe("running Queue projection", () => {
  it("replaces the optimistic bubble with one withdrawable durable card", async () => {
    const text = "keep this for the next turn";
    arMock.send = vi.fn(async () => ({ status: "queued" }));
    arMock.queue = vi.fn()
      .mockResolvedValueOnce([])
      .mockResolvedValue([{ command_id: "cmd-queue-1", text, revoked: false }]);
    arMock.unqueue = vi.fn(async () => ({ status: "revoked" }));
    useStore.setState({
      sessions: [{ id: SID, title: "running session", status: "running", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector('[role="group"][aria-label="Delivery mode"]')).not.toBeNull());

    fireEvent.change(screen.getByPlaceholderText("Ask for follow-up changes"), { target: { value: text } });
    fireEvent.click(container.querySelector("button.cx-send:not(.cx-stop)")!);

    await waitFor(() => expect(arMock.send).toHaveBeenCalledWith(SID, text, [], [], "queue", undefined));
    await waitFor(() => expect(screen.getByRole("button", { name: "Withdraw" })).toBeTruthy());
    expect(screen.getAllByText(text)).toHaveLength(1);
    expect(screen.queryByText("queued…")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Withdraw" }));
    await waitFor(() => expect(arMock.unqueue).toHaveBeenCalledWith(SID, "cmd-queue-1"));
    await waitFor(() => expect(screen.queryByText(text)).toBeNull());
  });

  it("uses Cmd+Enter to send one message with the opposite Steer mode", async () => {
    const text = "steer the current turn";
    arMock.send = vi.fn(async () => ({ status: "steering" }));
    useStore.setState({
      sessions: [{ id: SID, title: "running session", status: "running", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector('[role="group"][aria-label="Delivery mode"]')).not.toBeNull());
    const composer = screen.getByPlaceholderText("Ask for follow-up changes");
    fireEvent.change(composer, { target: { value: text } });
    fireEvent.keyDown(composer, { key: "Enter", metaKey: true });

    await waitFor(() => expect(arMock.send).toHaveBeenCalledWith(SID, text, [], [], "steer", undefined));
    expect(screen.getByText(text)).toBeTruthy();
    expect(screen.getByText("steering…")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Withdraw" })).toBeNull();
  });
});

describe("ask_user compatibility answer projection", () => {
  it("removes the optimistic composer answer on the synchronous answered acknowledgement", async () => {
    const text = "Beta";
    const baseEvents = EVENTS.slice(0, 2);
    arMock.events = vi.fn(async (_sid: string, after: number) => after ? [] : baseEvents);
    arMock.inspect = async () => ({
      goal: null,
      children: [],
      progress: [],
      artifacts: [],
      waiting: {
        kind: "input",
        ask_questions: [{ question: "Pick one", options: [{ label: "Alpha" }, { label: "Beta" }] }],
      },
    });
    arMock.send = vi.fn(async () => ({ status: "delivered" }));
    useStore.setState({
      sessions: [{ id: SID, title: "asking session", status: "waiting:input", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(screen.getByText("Pick one")).toBeTruthy());
    fireEvent.change(screen.getByPlaceholderText("Ask for follow-up changes"), { target: { value: text } });
    fireEvent.click(container.querySelector("button.cx-send:not(.cx-stop)")!);

    await waitFor(() => expect(arMock.send).toHaveBeenCalledWith(SID, text, [], [], undefined, undefined));
    await waitFor(() => expect(screen.queryByText("queued…")).toBeNull(), { timeout: 3000 });
    expect(container.querySelector(".bubble.pending")).toBeNull();
  });

  it("keeps a child read-only except for its exact structured answer path", async () => {
    const childSID = SID + "-sub-call_1_0-a1";
    arMock.events = vi.fn(async () => []);
    arMock.inspect = async () => ({
      spec: "release-reviewer",
      children: [],
      progress: [],
      artifacts: [],
      waiting: {
        kind: "input",
        ask_questions: [{
          question: "Choose the release channel",
          options: [{ label: "Stable" }, { label: "Beta" }, { label: "Canary" }],
        }],
      },
    });
    arMock.answer = vi.fn(async () => ({ status: "accepted" }));
    arMock.skipAnswer = vi.fn(async () => ({ status: "accepted" }));
    useStore.setState({
      currentSid: childSID,
      sessions: [{ id: SID, title: "parent", status: "waiting:input", workspace: "/tmp/wt-th14" } as any],
    });

    render(<SessionView sid={childSID} />);
    await waitFor(() => expect(screen.getByText("Choose the release channel")).toBeTruthy());
    expect(screen.getByText("Sub-agent · answer requested")).toBeTruthy();
    expect(screen.queryByPlaceholderText("Ask for follow-up changes")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Beta" }));
    fireEvent.click(screen.getByRole("button", { name: "Submit" }));
    await waitFor(() => expect(arMock.answer).toHaveBeenCalledWith(childSID, ["1:2"]));
  });
});

// INC-41 RD-B · the Environment rail is a floating card, not a layout column.
// It used to be a 288px grid track: opening it re-laid-out the conversation
// (main 1176→888, the thread's text column sliding 144px left) — a glance at the
// environment moved the sentence you were reading. The rail is `position:absolute`
// now (tw.css) and the layout stays on its single-column track whether
// the rail is open or shut. The Changes review pane keeps its real split.
describe("RD-B · opening the rail does not re-lay-out the thread", () => {
  it("keeps the single-column track while the Environment rail is open", async () => {
    const { container } = render(<SessionView sid={SID} />);

    // Rail open (wide viewport) …
    await waitFor(() => expect(container.querySelector("aside.supervision-panel")).not.toBeNull());
    const layout = container.querySelector(".session-layout")!;
    expect(layout.classList.contains("single")).toBe(true);
    expect(layout.classList.contains("changes")).toBe(false);

    // … and shut: the exact same track, so `main` never changes width.
    fireEvent.click(screen.getByRole("button", { name: /hide environment/i }));
    await waitFor(() => expect(container.querySelector("aside.supervision-panel")).toBeNull());
    expect(container.querySelector(".session-layout")!.classList.contains("single")).toBe(true);
  });

  it("still gives the Changes review pane a real column", async () => {
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".supervision-env")).not.toBeNull());
    fireEvent.click(container.querySelector(".supervision-env .env-row")!);
    await waitFor(() => expect(container.querySelector(".changes-panel")).not.toBeNull());
    const layout = container.querySelector(".session-layout")!;
    expect(layout.classList.contains("changes")).toBe(true);
    expect(layout.classList.contains("single")).toBe(false);
  });
});

describe("INC-98.3g · child approval truth", () => {
  it("uses the delegation worktree and protects the decision card at 1280px", async () => {
    const childSid = `${SID}-sub-call_1_0-a1`;
    const childPath = `/tmp/agentrunner/sessions/${SID}/sub/call_1_0-a1/worktree`;
    (window as any).innerWidth = 1280;
    arMock.events = async (_sid: string, after: number) => after ? [] : EVENTS.slice(1, 3);
    arMock.inspect = async () => ({
      goal: null,
      progress: [],
      artifacts: [],
      delegations: [{ assigned_to: childSid, workspace: { mode: "isolated", path: childPath } }],
      children: [{
        call_id: "call_1_0",
        agent: "worker",
        session: childSid,
        report: {
          status: "waiting",
          waiting: { kind: "approval", approval_id: "apr-child", tool: "bash", args: '{"command":"pwd"}' },
        },
      }],
    });
    useStore.setState({
      sessions: [{ id: SID, title: "child approval", status: "waiting:input", workspace: "/repo/parent" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(screen.getByRole("region", { name: "Approval required" })).toBeTruthy());
    expect(screen.getByText("Requested by worker")).toBeTruthy();
    expect(screen.getByText("Child worktree")).toBeTruthy();
    expect(screen.getByText("worker · isolated")).toBeTruthy();
    expect(container.querySelector(".approval-scope")?.getAttribute("title")).toBe(childPath);
    await waitFor(() => expect(container.querySelector("aside.supervision-panel")).toBeNull());
    expect(screen.getByRole("button", { name: /Environment/i })).toBeTruthy();
    expect(container.querySelector(".topbar-attention")).not.toBeNull();
  });
});

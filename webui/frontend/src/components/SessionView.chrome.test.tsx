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

  it("keeps Changes reachable from the ··· menu", async () => {
    const { container } = render(<SessionView sid={SID} />);

    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: /more session actions/i }));
    // Scoped to the menu: `Changes` is also the rail's first row (the primary
    // door) — that's the point of TH-15, so both must exist and neither is the
    // deleted topbar pill.
    fireEvent.click(screen.getByRole("menuitem", { name: "Changes" }));
    await waitFor(() => expect(container.querySelector(".changes-panel")).not.toBeNull());
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
      sessions: [{ id: SID, title: "深度黑盒 QA 任务 2026-07-12-A", status: "interrupted", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());

    const topbar = container.querySelector(".session-topbar")!;
    const navSlot = topbar.querySelector(".session-topbar-nav-slot")!;
    expect(navSlot).toBe(topbar.firstElementChild);
    expect(navSlot.classList.contains("h-9")).toBe(true);
    expect(navSlot.classList.contains("w-9")).toBe(true);
    expect(topbar.querySelector(".tt-title")!.textContent).toBe("深度黑盒 QA 任务 2026-07-12-A");

    // Recovery is the current-state action. Retry and fork remain reachable
    // without competing with the title as two more unlabeled mobile icons.
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
    expect(screen.getByRole("menuitem", { name: "Retry last message" })).toBeTruthy();
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

describe("fork button", () => {
  it("shows a topbar fork button only after the journal has a checkpoint", async () => {
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
    expect(screen.getByRole("button", { name: /fork session from checkpoint/i })).toBeTruthy();
  });

  it("keeps the fork button out of the topbar before a checkpoint exists", async () => {
    useStore.setState({
      sessions: [{ id: SID, title: "not forkable yet", status: "completed", workspace: "/tmp/wt-th14" } as any],
    });

    const { container } = render(<SessionView sid={SID} />);
    await waitFor(() => expect(container.querySelector(".session-topbar")).not.toBeNull());
    expect(screen.queryByRole("button", { name: /fork session from checkpoint/i })).toBeNull();
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

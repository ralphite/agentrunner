// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

// INC-41 L1/L2/L3 — one family of bugs: an UNKNOWN state (first fetch still in
// flight, or a fetch that failed) was painted as a DETERMINATE one (this task is
// empty / the daemon is down / we're still checking, forever). These tests pin
// the distinction on all three surfaces.

// The whole api module is proxied: any AR.<method> not explicitly stubbed
// returns a promise that never settles — i.e. "still loading", which is exactly
// the state under test.
const { arMock } = vi.hoisted(() => ({ arMock: {} as Record<string, (...args: any[]) => any> }));
// ApiError is taken from the real module: the not-found verdict it carries
// (status/code) is the contract under test, so a hand-rolled stub would prove
// nothing.
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

import { TimelineView } from "./Timeline";
import { SessionView, isSessionNotFound } from "./SessionView";
import { Sidebar } from "./Sidebar";
import { ApiError } from "../api";
import { useStore } from "../store";
import type { BubbleItem } from "../timeline";

// The backend answers an unknown session id with a real 404 + code (INC-41 L5);
// api.ts raises that as an ApiError whose message still folds in error+stderr.
const notFound = () =>
  new ApiError(
    'ar inspect: exit status 2\nagentrunner: no session matches "ghost-9999"',
    404,
    "session_not_found",
  );
// A pre-L5 webui binary 502s with only the CLI prose — still recognised.
const legacyNotFound = () =>
  new Error('ar inspect: exit status 2\nagentrunner: no session matches "ghost-9999"');
const transient = () =>
  new ApiError("ar inspect: exit status 1\ndial tcp: connection refused", 502);

const bubble = (key: string, text: string): BubbleItem => ({ kind: "assistant", key, text });

class FakeEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  addEventListener = vi.fn();
}

beforeEach(() => {
  for (const key of Object.keys(arMock)) delete arMock[key];
  (globalThis as any).EventSource = FakeEventSource;
  // jsdom ships neither of these; the composer reads matchMedia on mount.
  (window as any).matchMedia = () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  });
  // Supervision only mounts on a wide viewport (SessionView reads innerWidth).
  (window as any).innerWidth = 1400;
  localStorage.clear();
  useStore.setState({ health: null, sessions: [], sessionsReady: true, currentSid: null });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("L1 · timeline loading vs genuinely empty", () => {
  it("renders skeleton bubbles (not 'No messages yet') while the first fetch is in flight", () => {
    const { container } = render(
      <TimelineView items={[]} pending={[]} typing="" showSys={false} loading />,
    );
    expect(container.querySelector(".tl-skeleton")).not.toBeNull();
    expect(container.querySelectorAll(".tl-skel-bubble").length).toBeGreaterThanOrEqual(2);
    expect(screen.queryByText("No messages yet")).toBeNull();
  });

  it("renders the empty state only once loading has settled with no messages", () => {
    const { container } = render(
      <TimelineView items={[]} pending={[]} typing="" showSys={false} loading={false} />,
    );
    expect(container.querySelector(".tl-skeleton")).toBeNull();
    expect(screen.getByText("No messages yet")).toBeTruthy();
  });

  it("never shows the skeleton once messages exist, even if loading is still true", () => {
    const { container } = render(
      <TimelineView items={[bubble("a1", "hello from history")]} pending={[]} typing="" showSys={false} loading />,
    );
    expect(container.querySelector(".tl-skeleton")).toBeNull();
    expect(screen.getByText("hello from history")).toBeTruthy();
    expect(screen.queryByText("No messages yet")).toBeNull();
  });
});

describe("L2 · unknown session id", () => {
  it("tells apart the daemon's not-found verdict from a transient failure", () => {
    expect(isSessionNotFound(notFound())).toBe(true);
    expect(isSessionNotFound(transient())).toBe(false);
    expect(isSessionNotFound(new Error("Failed to fetch"))).toBe(false);
    expect(isSessionNotFound(undefined)).toBe(false);
  });

  it("judges by status/code, not by the CLI's wording (INC-41 L5)", () => {
    // The machine-readable verdict alone is enough — even with prose the
    // frontend has never seen, and even if the CLI stops saying "no session
    // matches" entirely.
    expect(isSessionNotFound(new ApiError("ar inspect: exit status 2\nreworded prose", 404))).toBe(true);
    expect(
      isSessionNotFound(new ApiError("ar inspect: gone", 502, "session_not_found")),
    ).toBe(true);
    // Any other 5xx stays transient, code or not.
    expect(isSessionNotFound(new ApiError("ar inspect: daemon dial: refused", 502))).toBe(false);
    expect(isSessionNotFound(new ApiError("ar inspect: boom", 500))).toBe(false);
    // Back-compat: a stale webui binary still only has the CLI prose to offer.
    expect(isSessionNotFound(legacyNotFound())).toBe(true);
  });

  it("renders a Task not found card with a way back — and no composer", async () => {
    arMock.events = async () => {
      throw notFound();
    };
    arMock.inspect = async () => {
      throw notFound();
    };
    arMock.ps = async () => [];
    const { container } = render(<SessionView sid="ghost-9999" />);

    await waitFor(() => expect(screen.getByText("Task not found")).toBeTruthy());
    expect(container.querySelector("textarea")).toBeNull(); // composer is gone
    expect(container.querySelector(".timeline .tl-empty")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /back to all tasks/i }));
    expect(useStore.getState().currentSid).toBeNull();
  });

  it("settles Supervision's spinners when inspect fails transiently (no permanent 'Checking…')", async () => {
    arMock.events = async () => [];
    arMock.ps = async () => [];
    arMock.queue = async () => [];
    arMock.inspect = async () => {
      throw transient();
    };
    render(<SessionView sid="sess-real-1" />);

    // The failing inspect must still end the loading state: the indeterminate
    // "Checking…" placeholder resolves to the determinate resting line. (Since
    // INC-41 TH-3 the empty Goal/Agents/Attention blocks no longer render at
    // all — one dim "Nothing needs you" row stands in for all three.)
    await waitFor(() => expect(screen.getByText(/Nothing needs you/i)).toBeTruthy());
    expect(screen.queryByText(/Checking…/)).toBeNull();
    // A transient error is NOT a missing session.
    expect(screen.queryByText("Task not found")).toBeNull();
  });
});

describe("L3 · daemon badge tri-state", () => {
  it("shows a neutral Connecting… (never offline) while health is unknown", () => {
    useStore.setState({ health: null });
    const { container } = render(<Sidebar />);

    expect(screen.getByText("Connecting…")).toBeTruthy();
    expect(screen.queryByText(/Daemon offline/)).toBeNull();
    expect(screen.queryByText(/^Connected/)).toBeNull();

    const avatar = container.querySelector(".account-avatar")!;
    expect(avatar.className).toContain("connecting");
    expect(avatar.className).not.toContain("offline");

    // Unknown is inert: clicking must not fire a daemon restart.
    const restart = vi.fn();
    arMock.daemonStart = restart;
    fireEvent.click(container.querySelector(".account-badge")!);
    expect(restart).not.toHaveBeenCalled();
  });

  it("shows the red offline badge only once health actually reports daemonUp:false", () => {
    useStore.setState({ health: { daemonUp: false } as any });
    const { container } = render(<Sidebar />);

    expect(screen.getByText("Daemon offline — restart")).toBeTruthy();
    expect(container.querySelector(".account-avatar")!.className).toContain("offline");

    const restart = vi.fn(async () => ({}));
    arMock.daemonStart = restart;
    fireEvent.click(container.querySelector(".account-badge")!);
    expect(restart).toHaveBeenCalled();
  });

  it("shows Connected when the daemon is up", () => {
    useStore.setState({ health: { daemonUp: true, version: "ar 1.2.3" } as any });
    const { container } = render(<Sidebar />);

    expect(screen.getByText(/^Connected/)).toBeTruthy();
    expect(container.querySelector(".account-avatar")!.className).toContain("online");
  });
});

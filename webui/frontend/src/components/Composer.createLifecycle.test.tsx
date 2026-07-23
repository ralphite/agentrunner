// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  agents: vi.fn(async () => [{ name: "dev", source: "shipped", yaml: "name: dev\nsystem_prompt: test\ntools: []\n" }]),
  newSession: vi.fn(),
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/ws" })),
  gitBranches: vi.fn(async () => ({ isRepo: false, current: "", branches: [], dirty: 0 })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    agents: mocks.agents,
    newSession: mocks.newSession,
    makeWorkspace: mocks.makeWorkspace,
    gitBranches: mocks.gitBranches,
  },
}));

import { Composer } from "./Composer";
import { useStore } from "../store";

window.matchMedia = ((query: string) =>
  ({ matches: false, media: query, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;

function mount({
  refreshSessions = vi.fn(async () => {}),
  select = vi.fn(),
  onError = vi.fn(),
}: {
  refreshSessions?: () => Promise<void>;
  select?: (sid: string) => void;
  onError?: (message: string) => void;
} = {}) {
  useStore.setState({
    sessions: [],
    sessionsReady: true,
    refreshSessions,
    select,
    toast: vi.fn(),
  } as any);
  return {
    refreshSessions,
    select,
    onError,
    ...render(<Composer variant="home" onError={onError} />),
  };
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  mocks.newSession.mockReset();
  mocks.newSession.mockResolvedValue({ sid: "20260723-000000-created" });
});
afterEach(cleanup);

describe("New session creation lifecycle", () => {
  it("routes to the durable sid even when the following sidebar refresh fails", async () => {
    const refreshSessions = vi.fn(async () => {
      throw new Error("session list unavailable");
    });
    const { select, onError } = mount({ refreshSessions });
    const textarea = screen.getByPlaceholderText("Do anything");

    fireEvent.change(textarea, { target: { value: "Create exactly one session" } });
    fireEvent.keyDown(textarea, { key: "Enter" });

    await waitFor(() => expect(onError).toHaveBeenCalledWith("session list unavailable"));
    expect(mocks.newSession).toHaveBeenCalledOnce();
    expect(select).toHaveBeenCalledOnce();
    expect(select).toHaveBeenCalledWith("20260723-000000-created");
  });

  it("coalesces two immediate Enter submissions into one create request", async () => {
    let resolveCreate!: (value: { sid: string }) => void;
    mocks.newSession.mockImplementationOnce(() => new Promise((resolve) => {
      resolveCreate = resolve;
    }));
    const { select } = mount();
    const textarea = screen.getByPlaceholderText("Do anything");

    fireEvent.change(textarea, { target: { value: "Create once" } });
    fireEvent.keyDown(textarea, { key: "Enter" });
    fireEvent.keyDown(textarea, { key: "Enter" });

    await waitFor(() => expect(mocks.newSession).toHaveBeenCalledOnce());
    resolveCreate({ sid: "20260723-000000-created" });
    await waitFor(() => expect(select).toHaveBeenCalledOnce());
  });
});

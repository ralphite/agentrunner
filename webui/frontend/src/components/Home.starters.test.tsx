// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  gitBranches: vi.fn(async () => ({ isRepo: false, current: "", branches: [], dirty: 0 })),
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/ws" })),
  newSession: vi.fn(async () => ({ sid: "20260722-000000-starter" })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    gitBranches: mocks.gitBranches,
    makeWorkspace: mocks.makeWorkspace,
    newSession: mocks.newSession,
  },
}));

import { Home } from "./Home";
import { useStore } from "../store";

window.matchMedia = ((query: string) =>
  ({ matches: false, media: query, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;

const mount = () => {
  useStore.setState({
    sessions: [],
    sessionsReady: true,
    newSessionProject: null,
    health: { ok: true, daemonUp: true, versionMatch: true },
    refreshSessions: async () => {},
    refreshRuns: async () => {},
    select: vi.fn(),
    selectRun: vi.fn(),
    toast: vi.fn(),
    openModal: vi.fn(),
    openPrompt: vi.fn(),
  } as any);
  return render(<Home />);
};

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  mocks.newSession.mockClear();
});
afterEach(cleanup);

describe("New session starter intents", () => {
  it.each([
    ["Explore and understand code", "Explore", "Explore and document an API"],
    ["Build a new feature, app, or tool", "Build", "Build an internal tool"],
    ["Review code and suggest changes", "Review", "Review and refactor my code"],
    ["Fix issues and failures", "Fix", "Fix merge conflicts"],
  ])("turns %s into a short intent plus four concrete follow-ups", async (card, seed, lastFollowup) => {
    const { container } = mount();
    const textarea = container.querySelector<HTMLTextAreaElement>(".cx-home textarea")!;

    fireEvent.click(screen.getByRole("button", { name: card }));

    expect(textarea.value).toBe(seed);
    expect(screen.queryByRole("button", { name: card })).toBeNull();
    const followups = screen.getByLabelText(`${seed} suggestions`);
    expect(within(followups).getAllByRole("button")).toHaveLength(4);
    expect(within(followups).getByRole("button", { name: lastFollowup })).toBeTruthy();
    expect(mocks.newSession).not.toHaveBeenCalled();

    fireEvent.click(within(followups).getByRole("button", { name: lastFollowup }));
    expect(textarea.value).toBe(lastFollowup);
    expect(screen.queryByLabelText(`${seed} suggestions`)).toBeNull();
    expect(mocks.newSession).not.toHaveBeenCalled();

    fireEvent.change(textarea, { target: { value: "" } });
    await waitFor(() => expect(screen.getByRole("button", { name: card })).toBeTruthy());
  });
});

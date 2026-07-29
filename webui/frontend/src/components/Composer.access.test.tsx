// @vitest-environment jsdom
//
// The approval posture is the user's control panel, not a launch-time setting.
// Every posture stays reachable mid-session, and a switch has to move both
// halves of what "posture" means: the spec's permissions block (all that
// separates Full access from Ask — both run under the `default` mode) and the
// runtime mode (what makes Auto-accept edits and Plan different). Sending only
// the mode command used to leave a session launched at Full access ungated
// after "switching" to Ask, which is why those rows were disabled instead.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  mode: vi.fn(async () => ({})),
  switchAgent: vi.fn(async (_sid: string, _spec: string, _extra: unknown[], _model: unknown) => ({})),
  agents: vi.fn(async () => [{
    name: "dev",
    description: "Dev",
    source: "shipped",
    yaml: "name: dev\nsystem_prompt: test\ntools: []\npermissions:\n  - { action: ask }\n",
  }]),
  gitBranches: vi.fn(async () => ({ isRepo: false, current: "", branches: [], dirty: 0 })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    agents: mocks.agents,
    mode: mocks.mode,
    switchAgent: mocks.switchAgent,
    gitBranches: mocks.gitBranches,
  },
}));

import { Composer } from "./Composer";
import { useStore } from "../store";

window.matchMedia = ((q: string) =>
  ({ matches: false, media: q, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;

const SID = "20260729-000000-access";

const mount = (mode = "default") => {
  useStore.setState({
    sessions: [],
    sessionsReady: true,
    refreshSessions: async () => {},
    select: vi.fn(),
    toast: vi.fn(),
  } as any);
  return render(
    <Composer
      variant="session"
      sid={SID}
      workspace="/tmp/ws"
      mode={mode}
      running={false}
      onSend={vi.fn()}
      actions={undefined as any}
      onError={() => {}}
    />,
  );
};

const openAccessMenu = () => {
  const pill = document.querySelector<HTMLButtonElement>(".cx-mode.session")!;
  fireEvent.click(pill);
};
// The session picker is a dialog popover, so its rows are plain .pop-item
// buttons rather than menuitems.
const rows = () => [...document.querySelectorAll<HTMLButtonElement>(".cx-access-menu .pop-item")];
const row = (title: string) =>
  rows().find((item) => item.querySelector(".pop-title")?.textContent?.trim() === title)!;

beforeEach(() => {
  localStorage.clear();
  mocks.mode.mockClear();
  mocks.switchAgent.mockClear();
  mocks.agents.mockClear();
});
afterEach(cleanup);

describe("session approval posture", () => {
  it("offers every posture without disabling any of them", () => {
    mount();
    openAccessMenu();

    const items = rows();
    expect(items.map((item) => item.querySelector(".pop-title")?.textContent?.trim())).toEqual([
      "Full access",
      "Ask to approve",
      "Auto-accept edits",
      "Plan · read-only",
    ]);
    expect(items.some((item) => item.disabled)).toBe(false);
    // The old copy told the user these were fixed once the session started.
    expect(document.querySelector(".cx-pop-note")?.textContent).not.toContain("fixed once the session starts");
  });

  it("enters plan mid-session", async () => {
    mount();
    openAccessMenu();
    await act(async () => {
      fireEvent.click(row("Plan · read-only"));
    });

    expect(mocks.mode).toHaveBeenCalledWith(SID, "plan");
  });

  it("leaves plan without waiting for an exit_plan_mode approval", async () => {
    mount("plan");
    openAccessMenu();
    await act(async () => {
      fireEvent.click(row("Ask to approve"));
    });

    expect(mocks.mode).toHaveBeenCalledWith(SID, "default");
  });

  it("rewrites the permissions block too, so Full and Ask really differ", async () => {
    mount();
    openAccessMenu();
    await act(async () => {
      fireEvent.click(row("Full access"));
    });

    expect(mocks.switchAgent).toHaveBeenCalled();
    const spec = mocks.switchAgent.mock.calls[0][1];
    expect(spec).toContain("{ action: allow }");
    expect(mocks.mode).toHaveBeenCalledWith(SID, "default");

    mocks.switchAgent.mockClear();
    openAccessMenu();
    await act(async () => {
      fireEvent.click(row("Ask to approve"));
    });
    const askSpec = mocks.switchAgent.mock.calls[0][1];
    expect(askSpec).toContain("{ action: ask }");
    expect(askSpec).toContain("{ tool: read_file, action: allow }");
  });
});

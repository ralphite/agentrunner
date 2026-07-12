// @vitest-environment jsdom
//
// INC-41 CP-3 — the model menu's root page IS the effort slider.
//
// The regression these tests lock down: reasoning effort used to live behind a
// drill-in page (pill → "Effort" row → level = 3 clicks), because we had taken
// Codex's *Advanced* page for its root. Codex opens straight onto a dot slider:
// pill → dot = 2 clicks, and the pill then names the level ("5.6 Sol Extra
// High"). So we assert three things end to end:
//   1. one click on the pill puts every level on screen (no drill-in row),
//   2. one click on a dot moves the level and the pill's suffix follows,
//   3. that level reaches the *spec* — the thinking budget `ar new` is handed.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  newSession: vi.fn(async (_body: { spec: string }) => ({ sid: "20260711-000000-effort" })),
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/ws" })),
  gitBranches: vi.fn(async () => ({ isRepo: false, current: "", branches: [], dirty: 0 })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    newSession: mocks.newSession,
    makeWorkspace: mocks.makeWorkspace,
    gitBranches: mocks.gitBranches,
  },
}));

import { Composer } from "./Composer";
import { EFFORT_LEVELS, effortById } from "../specs";
import { useStore } from "../store";

// jsdom ships no matchMedia; the composer asks it for the ≤480px placeholder.
window.matchMedia = ((q: string) =>
  ({ matches: false, media: q, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;

const mount = () => {
  useStore.setState({
    sessions: [],
    sessionsReady: true,
    refreshSessions: async () => {},
    select: vi.fn(),
    toast: vi.fn(),
  } as any);
  return render(<Composer variant="home" onError={() => {}} />);
};

const pill = (c: HTMLElement) => c.querySelector<HTMLButtonElement>(".cx-model")!;
const dot = (label: string) => screen.getByRole("button", { name: label });

beforeEach(() => {
  localStorage.clear();
  mocks.newSession.mockClear();
});
afterEach(cleanup);

describe("model menu root = effort slider (CP-3)", () => {
  it("one click on the pill lands on the slider — every level is reachable, no drill-in", () => {
    const { container } = mount();
    fireEvent.click(pill(container));

    // The slider itself: one track, one stop per EFFORT_LEVELS entry.
    const slider = container.querySelector('[role="slider"]')!;
    expect(slider).not.toBeNull();
    const stops = [...slider.querySelectorAll(".cx-effort-stop")].map((s) => s.getAttribute("data-effort"));
    expect(stops).toEqual(EFFORT_LEVELS.map((e) => e.id));

    // Models stay one click away too, and the old "drill into Effort" row is gone.
    expect(screen.getByRole("menuitem", { name: /Gemini Pro/ })).toBeTruthy();
    expect(container.querySelector(".cx-model-row")).toBeNull();
    // Advanced survives, but as a secondary collapsible (thinking-budget override).
    expect(screen.getByRole("button", { name: "Advanced" })).toBeTruthy();
  });

  it("clicking a dot moves the level in ONE click and the pill names it", () => {
    const { container } = mount();
    // Default effort is Off, which the pill deliberately leaves unwritten.
    expect(pill(container).textContent).toContain("Gemini Flash");
    expect(container.querySelector(".cx-pill-sub")).toBeNull();

    fireEvent.click(pill(container)); // click 1: open
    fireEvent.click(dot("High")); // click 2: choose — no third click

    expect(container.querySelector(".cx-pill-sub")!.textContent).toBe("High");
    const slider = container.querySelector('[role="slider"]')!;
    expect(slider.getAttribute("aria-valuetext")).toBe("High");
    expect(slider.getAttribute("aria-valuenow")).toBe(String(EFFORT_LEVELS.findIndex((e) => e.id === "high")));
    // The chosen dot is the lit one; the lower levels read as passed.
    expect(slider.querySelector(".cx-effort-stop.on")!.getAttribute("data-effort")).toBe("high");
    expect([...slider.querySelectorAll(".cx-effort-stop.done")].map((s) => s.getAttribute("data-effort"))).toEqual([
      "off",
      "light",
      "medium",
    ]);
  });

  it("←/→ on the focused track move one level at a time", () => {
    const { container } = mount();
    fireEvent.click(pill(container));
    const slider = container.querySelector('[role="slider"]')!;

    fireEvent.keyDown(slider, { key: "ArrowRight" }); // off → light
    expect(container.querySelector(".cx-pill-sub")!.textContent).toBe("Light");
    fireEvent.keyDown(slider, { key: "ArrowRight" }); // light → medium
    expect(container.querySelector(".cx-pill-sub")!.textContent).toBe("Medium");
    fireEvent.keyDown(slider, { key: "ArrowLeft" }); // back to light
    expect(container.querySelector(".cx-pill-sub")!.textContent).toBe("Light");

    // Clamped at the ends: ArrowLeft at "off" stays "off" (pill drops the suffix).
    fireEvent.keyDown(slider, { key: "ArrowLeft" });
    expect(container.querySelector(".cx-pill-sub")).toBeNull();
    fireEvent.keyDown(slider, { key: "ArrowLeft" });
    expect(container.querySelector(".cx-pill-sub")).toBeNull();
  });

  it("the dot's level reaches the spec: `ar new` gets that level's thinking budget", async () => {
    const { container } = mount();
    fireEvent.click(pill(container));
    fireEvent.click(dot("Extra High"));

    fireEvent.change(screen.getByPlaceholderText("Do anything"), { target: { value: "ship it" } });
    fireEvent.keyDown(screen.getByPlaceholderText("Do anything"), { key: "Enter" });

    await vi.waitFor(() => expect(mocks.newSession).toHaveBeenCalled());
    const spec: string = mocks.newSession.mock.calls[0][0].spec;
    const budget = effortById("xhigh").budget; // 24576
    expect(spec).toContain(`thinking: { enabled: true, budget_tokens: ${budget} }`);
    // max_tokens = answer room + budget, so thinking can't starve the answer.
    expect(spec).toContain(`max_tokens: ${4096 + budget}`);
  });
});

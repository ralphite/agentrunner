// @vitest-environment jsdom
//
// Mobile model-menu parity: the root stays compact and each dimension swaps to
// a dedicated page. This keeps Effort and Advanced reachable on short phones,
// preserves the selected state, and never submits a surrounding form.
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

window.matchMedia = ((q: string) =>
  ({ matches: q === "(max-width: 480px)", media: q, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;

const mount = (onSubmit = vi.fn()) => {
  useStore.setState({
    sessions: [],
    sessionsReady: true,
    refreshSessions: async () => {},
    select: vi.fn(),
    toast: vi.fn(),
  } as any);
  return {
    onSubmit,
    ...render(
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit(event);
        }}
      >
        <Composer variant="home" onError={() => {}} />
      </form>,
    ),
  };
};

const pill = (container: HTMLElement) => container.querySelector<HTMLButtonElement>(".cx-model")!;
const item = (title: string) =>
  [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')].find(
    (button) => button.querySelector(".pop-title")?.textContent?.trim() === title,
  )!;
const openMenu = (container: HTMLElement) => fireEvent.click(pill(container));

beforeEach(() => {
  localStorage.clear();
  mocks.newSession.mockClear();
});
afterEach(cleanup);

describe("Composer model / effort menu mobile hierarchy", () => {
  it("opens a compact bounded root with Model, Effort, Speed, and Advanced only", () => {
    const { container, onSubmit } = mount();
    expect(pill(container).type).toBe("button");

    openMenu(container);
    const menu = container.querySelector<HTMLElement>(".cx-model-menu")!;
    expect(menu.style.width).toBe("320px");
    expect(menu.style.maxWidth).toBe("calc(100vw - 32px)");
    expect([...menu.querySelectorAll(".pop-title")].map((node) => node.textContent?.trim())).toEqual(["Model", "Effort", "Speed", "Advanced"]);
    expect(menu.querySelector('[role="slider"]')).toBeNull();
    expect(item("Model").querySelector(".pop-right")?.textContent).toContain("Gemini Flash");
    expect(item("Effort").querySelector(".pop-right")?.textContent).toContain("Medium");
    expect(item("Speed").querySelector(".pop-right")?.textContent).toContain("Standard");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("bolds the Model/Effort/Speed root labels and keeps Advanced out of that group", () => {
    // MODEL-ROOT-LABEL-WEIGHT: only the three root rows live inside
    // .cx-model-roots (which the stylesheet renders semibold via
    // `.cx-model-roots .pop-title`); Advanced stays outside so its label is
    // not bolded and reads as a secondary action.
    const { container } = mount();
    openMenu(container);
    const menu = container.querySelector<HTMLElement>(".cx-model-menu")!;
    const roots = menu.querySelector<HTMLElement>(".cx-model-roots")!;
    expect(roots).toBeTruthy();
    expect([...roots.querySelectorAll(".pop-title")].map((node) => node.textContent?.trim())).toEqual(["Model", "Effort", "Speed"]);
    // Advanced's title lives outside .cx-model-roots (so it is not semibold)…
    expect(item("Advanced").closest(".cx-model-roots")).toBeNull();
    expect(item("Advanced").closest(".cx-model-advanced")).toBeTruthy();
    // …and its caret is inline inside the label rather than pushed to .pop-right.
    expect(item("Advanced").querySelector(".pop-title .cx-model-adv-chev")).toBeTruthy();
    expect(item("Advanced").querySelector(".pop-right")).toBeNull();
  });

  it("swaps between pages and restores the selected Model and Effort state", () => {
    const { container } = mount();
    openMenu(container);

    fireEvent.click(item("Model"));
    expect(screen.getByRole("button", { name: "Back to model menu" })).toBeTruthy();
    expect(item("Gemini Flash").querySelector(".pop-check")).toBeTruthy();
    fireEvent.click(item("Gemini Pro"));

    openMenu(container);
    expect(item("Model").querySelector(".pop-right")?.textContent).toContain("Gemini Pro");
    fireEvent.click(item("Effort"));
    expect([...document.querySelectorAll(".cx-model-menu .pop-title")].map((node) => node.textContent)).toEqual(
      EFFORT_LEVELS.map((level) => level.label),
    );
    expect(item("Medium").querySelector(".pop-check")).toBeTruthy();
    fireEvent.click(item("Extra High"));

    expect(pill(container).textContent).toContain("Gemini Pro");
    expect(pill(container).textContent).toContain("Extra High");
    openMenu(container);
    expect(item("Effort").querySelector(".pop-right")?.textContent).toContain("Extra High");
    fireEvent.click(item("Effort"));
    expect(item("Extra High").querySelector(".pop-check")).toBeTruthy();
  });

  it("keeps Advanced on its own returnable page", () => {
    const { container } = mount();
    openMenu(container);
    fireEvent.click(item("Advanced"));

    expect(screen.getByRole("button", { name: "Back to model menu" })).toBeTruthy();
    expect(item("Custom model id…")).toBeTruthy();
    expect(item("Thinking budget override…")).toBeTruthy();
    expect(item("Model")).toBeFalsy();
    expect(item("Effort")).toBeFalsy();

    fireEvent.click(screen.getByRole("button", { name: "Back to model menu" }));
    expect(item("Model")).toBeTruthy();
    expect(item("Effort")).toBeTruthy();
    expect(item("Speed")).toBeTruthy();
    expect(item("Advanced")).toBeTruthy();
  });

  it("keeps Speed as a returnable Standard page", () => {
    const { container } = mount();
    openMenu(container);
    fireEvent.click(item("Speed"));

    expect(screen.getByRole("button", { name: "Back to model menu" })).toBeTruthy();
    expect(item("Standard").querySelector(".pop-check")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Back to model menu" }));
    expect(item("Speed").querySelector(".pop-right")?.textContent).toContain("Standard");
  });

  it("does not submit a draft while navigating or choosing an effort", () => {
    const { container, onSubmit } = mount();
    const textarea = screen.getByPlaceholderText("Do anything");
    fireEvent.change(textarea, { target: { value: "keep this draft" } });

    openMenu(container);
    fireEvent.click(item("Effort"));
    fireEvent.click(screen.getByRole("button", { name: "Back to model menu" }));
    fireEvent.click(item("Effort"));
    fireEvent.click(item("High"));

    expect((textarea as HTMLTextAreaElement).value).toBe("keep this draft");
    expect(onSubmit).not.toHaveBeenCalled();
    expect(mocks.newSession).not.toHaveBeenCalled();
  });

  it("sends the selected effort's thinking budget in the spec", async () => {
    const { container } = mount();
    openMenu(container);
    fireEvent.click(item("Effort"));
    fireEvent.click(item("Extra High"));

    fireEvent.change(screen.getByPlaceholderText("Do anything"), { target: { value: "ship it" } });
    fireEvent.keyDown(screen.getByPlaceholderText("Do anything"), { key: "Enter" });

    await vi.waitFor(() => expect(mocks.newSession).toHaveBeenCalled());
    const spec: string = mocks.newSession.mock.calls[0][0].spec;
    const budget = effortById("xhigh").budget;
    expect(spec).toContain(`thinking: { enabled: true, budget_tokens: ${budget} }`);
    expect(spec).toContain(`max_tokens: ${4096 + budget}`);
  });
});

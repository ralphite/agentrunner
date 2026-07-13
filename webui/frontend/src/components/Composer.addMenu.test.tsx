// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  gitBranches: vi.fn(async () => ({ isRepo: false, current: "", branches: [], dirty: 0 })),
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/ws" })),
  newSession: vi.fn(async () => ({ sid: "20260713-000000-add-menu" })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    gitBranches: mocks.gitBranches,
    makeWorkspace: mocks.makeWorkspace,
    newSession: mocks.newSession,
  },
}));

import { Composer } from "./Composer";
import { useStore } from "../store";

window.matchMedia = ((query: string) =>
  ({ matches: query === "(max-width: 480px)", media: query, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList) as typeof window.matchMedia;

const mount = (onSubmit = vi.fn()) => {
  useStore.setState({
    sessions: [],
    sessionsReady: true,
    refreshSessions: async () => {},
    select: vi.fn(),
    toast: vi.fn(),
    openModal: vi.fn(),
  } as any);
  return {
    onSubmit,
    ...render(
      <form onSubmit={onSubmit}>
        <Composer variant="home" onError={() => {}} />
      </form>,
    ),
  };
};

const openAddMenu = () => fireEvent.click(screen.getByRole("button", { name: "Add and advanced options" }));

beforeEach(() => {
  localStorage.clear();
  mocks.newSession.mockClear();
});
afterEach(cleanup);

describe("Composer add and advanced menu", () => {
  it("keeps every root action in the compact Codex-style grouped menu", () => {
    mount();
    openAddMenu();

    const menu = document.querySelector<HTMLElement>(".cx-add-menu")!;
    expect(menu.style.width).toBe("320px");
    expect(menu.style.maxWidth).toBe("calc(100vw - 32px)");
    expect(menu.classList.contains("[&_.pop-body]:flex-row")).toBe(true);
    expect(menu.classList.contains("[&_.pop-desc]:truncate")).toBe(true);
    expect([...menu.querySelectorAll(".pop-section-label")].map((label) => label.textContent)).toEqual(["Add", "Advanced", "Agent"]);
    expect([...menu.querySelectorAll("[role=menuitem] .pop-title")].map((item) => item.textContent)).toEqual([
      "Files and folders",
      "Goal",
      "Loop",
      "Best of N",
      "Plan mode",
      "Background run",
      "Agent",
    ]);
  });

  it("reaches the Agent page without submitting the composer or an outer form", () => {
    const { onSubmit, container } = mount();
    const trigger = screen.getByRole("button", { name: "Add and advanced options" });
    expect(trigger.getAttribute("type")).toBe("button");

    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: "Agent Dev" }));

    expect(screen.getByRole("button", { name: "Back to add menu" })).toBeTruthy();
    expect(container.querySelector(".cx-add-agent")).toBeTruthy();
    expect(onSubmit).not.toHaveBeenCalled();
    expect(mocks.newSession).not.toHaveBeenCalled();
  });
});

// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  agents: vi.fn(async () => [
    { name: "dev", description: "Dev", source: "shipped", yaml: "name: dev\nsystem_prompt: test\ntools: []\n" },
    {
      name: "lead",
      description: "Drafts a team sharing one workspace · for collaboration",
      source: "shipped",
      yaml: "name: lead\nsystem_prompt: Lead it.\ntools: []\nagents_dynamic: true\n",
    },
  ]),
  gitBranches: vi.fn(async () => ({ isRepo: false, current: "", branches: [], dirty: 0 })),
  makeWorkspace: vi.fn(async () => ({ path: "/tmp/ws" })),
  newSession: vi.fn(async () => ({ sid: "20260713-000000-add-menu" })),
}));

vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: {
    agents: mocks.agents,
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
  mocks.agents.mockClear();
  mocks.newSession.mockClear();
});
afterEach(cleanup);

describe("Composer add and advanced menu", () => {
  it("confirms only a restricted-to-Full access escalation and preserves the draft on cancel", async () => {
    localStorage.setItem("arwebui.lastAccess", "ask");
    mount();
    const draft = screen.getByPlaceholderText("Do anything") as HTMLTextAreaElement;
    fireEvent.change(draft, { target: { value: "keep this draft" } });

    fireEvent.click(screen.getByRole("button", { name: "Ask to approve" }));
    fireEvent.click(screen.getByRole("menuitem", { name: /Full access Nothing is gated/ }));

    const openModal = useStore.getState().openModal as ReturnType<typeof vi.fn>;
    expect(openModal).toHaveBeenCalledOnce();
    const cancelled = openModal.mock.calls[0][0];
    expect(cancelled).toMatchObject({
      kind: "confirm",
      title: "Turn on Full Access?",
      confirmLabel: "Turn on Full Access",
      danger: true,
    });
    expect(cancelled.details.map((detail: { title: string }) => detail.title)).toEqual(["Files and folders", "Terminal commands", "Internet access"]);
    expect(cancelled.details[0].body).toContain("delete files anywhere on this computer");
    expect(cancelled.note).toContain("prompt injection");
    expect(screen.getByRole("button", { name: "Ask to approve" })).toBeTruthy();
    expect(localStorage.getItem("arwebui.lastAccess")).toBe("ask");
    expect(draft.value).toBe("keep this draft");

    cancelled.onClose();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Ask to approve" }));
    fireEvent.click(screen.getByRole("button", { name: "Ask to approve" }));
    fireEvent.click(screen.getByRole("menuitem", { name: /Full access Nothing is gated/ }));
    const confirmed = openModal.mock.calls[1][0];
    await act(async () => { await confirmed.onConfirm(); });
    confirmed.onClose();

    expect(screen.getByRole("button", { name: "Full access" })).toBeTruthy();
    expect(localStorage.getItem("arwebui.lastAccess")).toBe("full");
    expect(draft.value).toBe("keep this draft");
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Full access" }));

    fireEvent.click(screen.getByRole("button", { name: "Full access" }));
    fireEvent.click(screen.getByRole("menuitem", { name: /Full access Nothing is gated/ }));
    expect(openModal).toHaveBeenCalledTimes(2);
  });

  it("keeps every root action in the compact Codex-style grouped menu", () => {
    mount();
    openAddMenu();

    const menu = document.querySelector<HTMLElement>(".cx-add-menu")!;
    expect(menu.style.width).toBe("320px");
    expect(menu.style.maxWidth).toBe("calc(100vw - 32px)");
    expect(menu.classList.contains("[&_.pop-body]:flex-row")).toBe(true);
    expect(menu.classList.contains("[&_.pop-desc]:truncate")).toBe(true);
    expect([...menu.querySelectorAll(".pop-section-label")].map((label) => label.textContent)).toEqual(["Add", "Advanced"]);
    expect([...menu.querySelectorAll("[role=menuitem] .pop-title")].map((item) => item.textContent)).toEqual([
      "Files and folders",
      "Goal",
      "Plan mode",
      "Automation",
    ]);
  });

  it("reaches nested automation and Agent pages without submitting the composer or an outer form", async () => {
    const { onSubmit, container } = mount();
    const trigger = screen.getByRole("button", { name: "Add and advanced options" });
    expect(trigger.getAttribute("type")).toBe("button");

    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: /Automation/ }));
    expect(screen.getByRole("menuitem", { name: "Back to add menu" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Loop Repeat on a cadence" })).toBeTruthy();
    await waitFor(() => expect(mocks.agents).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("menuitem", { name: "Agent Dev" }));

    expect(screen.getByRole("menuitem", { name: "Back to automation menu" })).toBeTruthy();
    expect(container.querySelector(".cx-add-agent")).toBeTruthy();
    expect(onSubmit).not.toHaveBeenCalled();
    expect(mocks.newSession).not.toHaveBeenCalled();
  });

  it("opens the YAML editor with the persona currently selected in the composer", async () => {
    mount();
    openAddMenu();
    await waitFor(() => expect(mocks.agents).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("menuitem", { name: "Automation Dev" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Agent Dev" }));
    fireEvent.click(screen.getByRole("menuitem", { name: /Team Lead Drafts a team sharing one workspace/ }));

    openAddMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Automation Team Lead" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Agent Team Lead" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Edit agent spec (YAML)…" }));

    const openModal = useStore.getState().openModal as ReturnType<typeof vi.fn>;
    expect(openModal).toHaveBeenCalledOnce();
    const modal = openModal.mock.calls[0][0];
    expect(modal.kind).toBe("new");
    expect(modal.spec).toContain("name: lead\n");
    expect(modal.spec).toContain("agents_dynamic: true");
    expect(modal.worker).toBeUndefined();
    expect(modal).toMatchObject({
      provider: "gemini",
      model: "gemini-flash-latest",
      effort: "medium",
    });
  });

  it("reuses the single composer for Goal and keeps advanced checks behind the Goal chip", () => {
    mount();
    openAddMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: /^Goal / }));

    expect(screen.getByPlaceholderText("Describe your goal, define measurable outcomes for best results")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Start goal" })).toBeNull();
    expect(mocks.newSession).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Goal" }));
    expect(screen.getByText("Goal options")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Done when (command)" })).toBeTruthy();
    expect((screen.getByRole("spinbutton", { name: "Max rounds" }) as HTMLInputElement).value).toBe("10");

    fireEvent.click(screen.getByRole("button", { name: "Exit Goal mode" }));
    expect(screen.getByPlaceholderText("Do anything")).toBeTruthy();
    expect(mocks.newSession).not.toHaveBeenCalled();
  });

  it("toggles Plan mode off through Add and restores the prior access posture", () => {
    mount();
    expect(screen.getByRole("button", { name: "Ask to approve" })).toBeTruthy();

    openAddMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: /^Plan mode Turn plan mode on/ }));
    expect(screen.getByRole("button", { name: "Plan · read-only" })).toBeTruthy();
    expect(screen.getByPlaceholderText("Describe what to plan…")).toBeTruthy();

    openAddMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: /^Plan mode Turn plan mode off/ }));
    expect(screen.getByRole("button", { name: "Ask to approve" })).toBeTruthy();
    expect(screen.getByPlaceholderText("Do anything")).toBeTruthy();
    expect(mocks.newSession).not.toHaveBeenCalled();
  });
});

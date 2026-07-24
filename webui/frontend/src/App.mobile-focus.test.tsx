// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./App";
import { AppServicesProvider } from "./app/appServices";
import { AppStoreProvider, createAppStore } from "./store";
import { createStoryAppServices } from "./storybook/appServices";

const FOCUSABLE = [
  "a[href]",
  "button",
  "input:not([type='hidden'])",
  "select",
  "textarea",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

beforeEach(() => {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: /max-width: (680|900|1100|1400)px/.test(query),
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
});

afterEach(() => {
  document.body.innerHTML = "";
});

function renderMobileApp() {
  const harness = createStoryAppServices();
  const store = createAppStore(harness.services);
  store.setState({
    sessions: [{
      id: "20260723-120000-mobile-context",
      status: "idle",
      turns: 1,
      title: "Mobile context owner",
      workspace: "/repo/mobile-context",
    }] as any,
    sessionsReady: true,
  });
  return render(
    <AppServicesProvider services={harness.services}>
      <AppStoreProvider store={store}>
        <AppShell />
      </AppStoreProvider>
    </AppServicesProvider>,
  );
}

async function openSidebar() {
  const trigger = screen.getByRole("button", { name: "Show sidebar" });
  trigger.focus();
  fireEvent.click(trigger);
  const search = screen.getByRole("button", { name: "Search sessions" });
  await waitFor(() => expect(document.activeElement).toBe(search));
  return search;
}

async function expectSidebarClosed() {
  await waitFor(() => {
    expect(screen.getByRole("button", { name: "Show sidebar" })).toBeTruthy();
  });
}

describe("mobile sidebar focus scope", () => {
  it("enters, wraps, and closes on Escape", async () => {
    const { container } = renderMobileApp();

    await openSidebar();
    const sidebar = container.querySelector<HTMLElement>(".app > .sidebar")!;
    const focusable = Array.from(
      sidebar.querySelectorAll<HTMLElement>(FOCUSABLE),
    ).filter((element) => !element.hasAttribute("disabled") && !element.hidden);
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    last.focus();
    fireEvent.keyDown(last, { key: "Tab" });
    expect(document.activeElement).toBe(first);

    fireEvent.keyDown(document, { key: "Escape" });
    await expectSidebarClosed();
  });

  it("lets a nested session menu own Escape without closing the sidebar", async () => {
    const { container } = renderMobileApp();

    await openSidebar();
    const row = container.querySelector<HTMLElement>(
      '[data-session-id="20260723-120000-mobile-context"]',
    )!;
    const opener = row.querySelector<HTMLButtonElement>(".project-session")!;
    fireEvent.contextMenu(row, { clientX: 30, clientY: 40 });

    const firstItem = await screen.findByRole("menuitem", { name: "Pin" });
    await waitFor(() => expect(document.activeElement).toBe(firstItem));
    fireEvent.keyDown(firstItem, { key: "Escape" });

    await waitFor(() => {
      expect(screen.queryByRole("menu")).toBeNull();
      expect(container.querySelector(".app.collapsed")).toBeNull();
      expect(screen.queryByRole("button", { name: "Show sidebar" })).toBeNull();
      expect(document.activeElement).toBe(opener);
    });
  });

  it("keeps the touch More menu, Tab handoff, and Rename inside the sidebar layer", async () => {
    const { container } = renderMobileApp();

    await openSidebar();
    const trigger = screen.getByRole("button", {
      name: "More actions for Mobile context owner",
    });
    fireEvent.click(trigger);
    const firstItem = await screen.findByRole("menuitem", { name: "Pin" });
    await waitFor(() => expect(document.activeElement).toBe(firstItem));
    fireEvent.keyDown(firstItem, { key: "Escape" });

    await waitFor(() => {
      expect(screen.queryByRole("menu")).toBeNull();
      expect(container.querySelector(".app.collapsed")).toBeNull();
      expect(screen.queryByRole("button", { name: "Show sidebar" })).toBeNull();
      expect(document.activeElement).toBe(trigger);
    });

    fireEvent.click(trigger);
    const reopenedItem = await screen.findByRole("menuitem", { name: "Pin" });
    await waitFor(() => expect(document.activeElement).toBe(reopenedItem));
    fireEvent.keyDown(reopenedItem, { key: "Tab" });
    await waitFor(() => {
      expect(screen.queryByRole("menu")).toBeNull();
      expect(container.querySelector(".app.collapsed")).toBeNull();
      expect(document.activeElement).not.toBe(reopenedItem);
    });

    fireEvent.click(trigger);
    fireEvent.click(await screen.findByRole("menuitem", { name: "Rename…" }));
    const input = await screen.findByPlaceholderText("Session name");
    await waitFor(() => expect(document.activeElement).toBe(input));
    fireEvent.keyDown(input, { key: "Escape" });

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(container.querySelector(".app.collapsed")).toBeNull();
      expect(document.activeElement).toBe(trigger);
    });
  });

  it("lets a nested session menu hand off Tab and restore focus after Rename", async () => {
    const { container } = renderMobileApp();

    await openSidebar();
    const row = container.querySelector<HTMLElement>(
      '[data-session-id="20260723-120000-mobile-context"]',
    )!;
    const opener = row.querySelector<HTMLButtonElement>(".project-session")!;
    fireEvent.contextMenu(row, { clientX: 30, clientY: 40 });

    const firstItem = await screen.findByRole("menuitem", { name: "Pin" });
    await waitFor(() => expect(document.activeElement).toBe(firstItem));
    fireEvent.keyDown(firstItem, { key: "Tab" });
    await waitFor(() => {
      expect(screen.queryByRole("menu")).toBeNull();
      expect(container.querySelector(".app.collapsed")).toBeNull();
      expect(document.activeElement).not.toBe(firstItem);
    });

    fireEvent.contextMenu(row, { clientX: 30, clientY: 40 });
    fireEvent.click(await screen.findByRole("menuitem", { name: "Rename…" }));
    const input = await screen.findByPlaceholderText("Session name");
    await waitFor(() => expect(document.activeElement).toBe(input));
    fireEvent.keyDown(input, { key: "Escape" });

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(container.querySelector(".app.collapsed")).toBeNull();
      expect(document.activeElement).toBe(opener);
    });
  });
});

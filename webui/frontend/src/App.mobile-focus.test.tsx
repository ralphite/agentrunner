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
  store.setState({ sessionsReady: true });
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
});

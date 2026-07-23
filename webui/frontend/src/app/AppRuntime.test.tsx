// @vitest-environment jsdom
import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AppStoreProvider, createAppStore } from "../store";
import { createStoryAppServices } from "../storybook/appServices";
import { AppServicesProvider } from "./appServices";
import { AppRuntime } from "./AppRuntime";
import { AppShell } from "./AppShell";

describe("AppRuntimeController", () => {
  it("owns polling, route subscription, permission request, and cleanup", () => {
    const harness = createStoryAppServices({ hash: "scheduled" });
    const store = createAppStore(harness.services);
    const setInterval = vi.spyOn(harness.services.clock, "setInterval");
    const clearInterval = vi.spyOn(harness.services.clock, "clearInterval");
    const stopListening = vi.fn();
    const listen = vi.spyOn(harness.services.navigation, "listen").mockReturnValue(stopListening);
    const requestPermission = vi.spyOn(harness.services.notifications, "requestPermission");
    const refreshHealth = vi.spyOn(store.getState(), "refreshHealth");
    const refreshSessions = vi.spyOn(store.getState(), "refreshSessions");
    const refreshRuns = vi.spyOn(store.getState(), "refreshRuns");
    const refreshProjects = vi.spyOn(store.getState(), "refreshProjects");

    const view = render(
      <AppRuntime store={store} services={harness.services}>
        <div>shell</div>
      </AppRuntime>,
    );

    expect(refreshHealth).toHaveBeenCalledOnce();
    expect(refreshSessions).toHaveBeenCalledOnce();
    expect(refreshRuns).toHaveBeenCalledOnce();
    expect(refreshProjects).toHaveBeenCalledOnce();
    expect(setInterval).toHaveBeenCalledTimes(4);
    expect(listen).toHaveBeenCalledOnce();
    expect(store.getState().currentPage).toBe("scheduled");

    window.dispatchEvent(new Event("pointerdown"));
    expect(requestPermission).toHaveBeenCalledOnce();

    view.unmount();
    expect(clearInterval).toHaveBeenCalledTimes(4);
    expect(stopListening).toHaveBeenCalledOnce();
  });

  it("mounts AppShell without starting runtime I/O", () => {
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
    const base = createStoryAppServices();
    const calls: string[] = [];
    const api = new Proxy(base.services.api, {
      get(target, property, receiver) {
        const value = Reflect.get(target, property, receiver);
        if (typeof value !== "function") return value;
        return (..._args: unknown[]) => {
          calls.push(String(property));
          return Promise.reject(new Error(`unexpected ${String(property)}`));
        };
      },
    });
    const harness = createStoryAppServices({ api });
    const store = createAppStore(harness.services);
    store.setState({ sessionsReady: true });

    const view = render(
      <AppServicesProvider services={harness.services}>
        <AppStoreProvider store={store}>
          <AppShell />
        </AppStoreProvider>
      </AppServicesProvider>,
    );

    expect(calls).toEqual([]);
    view.unmount();
  });
});

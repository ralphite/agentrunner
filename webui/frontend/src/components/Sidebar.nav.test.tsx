// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// The sidebar hits /health and /git on mount; nothing here depends on those, so
// stub the module with never-settling promises (same pattern as loadingStates).
vi.mock("../api", async () => ({
  ...(await vi.importActual<typeof import("../api")>("../api")),
  AR: new Proxy({}, { get: () => () => new Promise(() => {}) }),
}));

import { Sidebar } from "./Sidebar";
import { SHORTCUT_GROUPS, keyLabel } from "../shortcuts";
import { useStore } from "../store";

afterEach(cleanup);

describe("sidebar search entry point (RH-5)", () => {
  it("magnifier opens the ⌘K command palette instead of an inline filter", () => {
    const onOpenPalette = vi.fn();
    const { container } = render(<Sidebar onOpenPalette={onOpenPalette} />);
    fireEvent.click(screen.getByLabelText("Search tasks"));
    expect(onOpenPalette).toHaveBeenCalled();
    // The second search surface is gone — one entry point, not two.
    expect(container.querySelector(".side-search")).toBeNull();
    expect(screen.queryByPlaceholderText(/Search title, id, or workspace/)).toBeNull();
  });
});

describe("New task shortcut badge (RH-4)", () => {
  it("badges the New task row with the key the app actually binds", () => {
    useStore.setState({ sessions: [] });
    const { container } = render(<Sidebar />);
    const badge = container.querySelector(".primary-nav .nav-kbd");
    expect(badge).toBeTruthy();

    // The badge must render the same tokens the shortcut catalog registers for
    // New task — that catalog is what Settings → Keyboard shortcuts shows, and
    // App.tsx is what actually fires. One string, three surfaces.
    const registered = SHORTCUT_GROUPS.find((g) => g.title === "Global")!.items.find(
      (i) => i.label === "New task",
    );
    expect(registered).toBeTruthy();
    expect(badge!.textContent).toBe(registered!.keys.map(keyLabel).join(""));
    // …and it sits on the New task row, not on Scheduled.
    expect(badge!.closest("button")!.textContent).toContain("New task");
  });
});

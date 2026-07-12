// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Menu, MenuItem } from "./Menu";

afterEach(cleanup);

function measure(anchor: { left: number; right: number; top: number; bottom: number }) {
  Object.defineProperty(window, "innerWidth", { value: 390, configurable: true });
  Object.defineProperty(window, "innerHeight", { value: 844, configurable: true });
  Object.defineProperty(HTMLElement.prototype, "offsetWidth", {
    configurable: true,
    get(this: HTMLElement) { return this.classList.contains("menu-pop") ? 210 : 0; },
  });
  const wrap = document.querySelector(".pop-wrap") as HTMLElement;
  wrap.getBoundingClientRect = () => ({
    ...anchor,
    width: anchor.right - anchor.left,
    height: anchor.bottom - anchor.top,
    x: anchor.left,
    y: anchor.top,
  }) as DOMRect;
}

describe("Menu viewport placement", () => {
  it("drops a task-topbar menu down inside a phone viewport", () => {
    const changes = vi.fn();
    render(
      <Menu label="More" ariaLabel="More task actions">
        <MenuItem onClick={changes}>Changes</MenuItem>
      </Menu>,
    );
    measure({ left: 350, right: 382, top: 8, bottom: 40 });

    fireEvent.click(screen.getByRole("button", { name: "More task actions" }));
    const panel = document.querySelector(".menu-pop") as HTMLElement;
    expect(panel.style.position).toBe("fixed");
    expect(panel.style.top).toBe("48px");
    expect(panel.style.bottom).toBe("auto");
    expect(panel.style.left).toBe("172px");
    expect(panel.style.maxHeight).toBe("788px");

    fireEvent.click(screen.getByRole("menuitem", { name: "Changes" }));
    expect(changes).toHaveBeenCalledTimes(1);
    expect(document.querySelector(".menu-pop")).toBeNull();
  });

  it("drops a sidebar-footer menu up", () => {
    render(
      <Menu label="More" ariaLabel="More options">
        <MenuItem onClick={() => {}}>Settings</MenuItem>
      </Menu>,
    );
    measure({ left: 350, right: 382, top: 804, bottom: 836 });

    fireEvent.click(screen.getByRole("button", { name: "More options" }));
    const panel = document.querySelector(".menu-pop") as HTMLElement;
    expect(panel.style.top).toBe("auto");
    expect(panel.style.bottom).toBe("48px");
  });
});

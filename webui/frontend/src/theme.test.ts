// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";
import { applyTheme } from "./theme";

describe("theme boot contract", () => {
  beforeEach(() => {
    document.documentElement.removeAttribute("data-theme");
    document.head.innerHTML = '<meta name="theme-color" content="#0f0f11">';
    window.matchMedia = vi.fn().mockReturnValue({ matches: false }) as unknown as typeof window.matchMedia;
  });

  it("pins explicit themes and keeps browser chrome in sync", () => {
    applyTheme("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute("content")).toBe("#0f0f11");

    applyTheme("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute("content")).toBe("#ffffff");
  });

  it("leaves System unpinned and resolves the current system color", () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: true }) as unknown as typeof window.matchMedia;
    applyTheme("system");
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute("content")).toBe("#0f0f11");
  });
});

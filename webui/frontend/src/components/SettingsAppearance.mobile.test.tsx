// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { SettingsAppearance } from "./SettingsAppearance";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe("SettingsAppearance mobile layout", () => {
  it("separates copy from stable full-width controls on narrow screens", () => {
    const { container } = render(<SettingsAppearance query="" />);

    const themeCards = container.querySelector(".rs-themecards")!;
    const previews = container.querySelectorAll(".rs-tp");
    const fontStepper = screen.getByRole("button", { name: "Decrease UI font size" }).parentElement!;
    const contrast = screen.getByRole("slider", { name: "Contrast" });

    expect(themeCards.classList.contains("max-[500px]:gap-1.5")).toBe(true);
    expect(screen.getByRole("button", { name: "System" }).classList.contains("on")).toBe(true);
    expect(previews).toHaveLength(3);
    previews.forEach((preview) => {
      expect(preview.classList.contains("h-14")).toBe(true);
      expect(preview.classList.contains("max-[500px]:h-12")).toBe(true);
    });
    expect(fontStepper.classList.contains("grid-cols-[32px_64px_32px]")).toBe(true);
    expect(fontStepper.classList.contains("justify-end")).toBe(true);
    expect(contrast.classList.contains("min-w-0")).toBe(true);
    expect(contrast.classList.contains("flex-1")).toBe(true);
  });

  it("uses stacked copy and compact cards at the mobile breakpoint", () => {
    const { container } = render(<SettingsAppearance query="" />);
    const rows = container.querySelectorAll(".rs-row");
    const themeLabel = screen.getByText("Theme");
    const themeDesc = screen.getByText("Follow the system, or pin a light or dark appearance.");

    expect(rows.length).toBeGreaterThan(4);
    rows.forEach((row) => {
      expect(row.classList.contains("max-[500px]:rounded-[8px]")).toBe(true);
      expect(row.classList.contains("max-[500px]:p-2.5")).toBe(true);
    });
    // Label + description share a single min-w-0 stacking wrapper so the copy
    // reads title-over-subtitle at every breakpoint (matches General's row
    // grammar) instead of the description floating to the far right on desktop.
    expect(themeLabel.parentElement).toBe(themeDesc.parentElement);
    expect(themeLabel.parentElement!.classList.contains("min-w-0")).toBe(true);
  });
});

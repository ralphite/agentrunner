// @vitest-environment jsdom

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SHORTCUT_GROUPS } from "../shortcuts";
import { Shortcuts } from "./Shortcuts";

describe("Shortcuts mobile overlay", () => {
  it("keeps every binding and exposes the narrow-screen close action", () => {
    const onClose = vi.fn();
    const { container } = render(<Shortcuts onClose={onClose} />);

    const close = screen.getByRole("button", {
      name: "Close keyboard shortcuts",
    });
    expect(close.classList.contains("sc-close")).toBe(true);
    expect(container.querySelector(".sc-head")?.classList.contains("sc-head")).toBe(true);
    expect(container.querySelector(".sc-search")?.classList.contains("sc-search")).toBe(true);

    const rows = container.querySelectorAll(".sc-row");
    const expectedRows = SHORTCUT_GROUPS.reduce(
      (total, group) => total + group.items.length,
      0,
    );
    expect(rows).toHaveLength(expectedRows);
    for (const row of rows) {
      expect(row.querySelectorAll("kbd").length).toBeGreaterThan(0);
    }

    fireEvent.click(close);
    expect(onClose).toHaveBeenCalledOnce();
  });
});

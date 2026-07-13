// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SHORTCUT_GROUPS } from "../shortcuts";
import { SettingsShortcuts } from "./SettingsShortcuts";

describe("SettingsShortcuts mobile layout", () => {
  it("stacks shortcut keys without dropping bindings", () => {
    const { container } = render(<SettingsShortcuts query="" />);
    const rows = container.querySelectorAll(".rs-sc-row");
    const expectedRows = SHORTCUT_GROUPS.reduce((total, group) => total + group.items.length, 0);

    expect(rows).toHaveLength(expectedRows);
    expect([...rows[0].classList]).toEqual(expect.arrayContaining(["min-w-0", "max-[520px]:flex-col", "max-[520px]:items-start"]));

    for (const row of rows) {
      expect([...(row.querySelector(".rs-sc-label")?.classList ?? [])]).toEqual(expect.arrayContaining(["min-w-0", "max-w-full"]));
      expect([...(row.querySelector(".rs-sc-keys")?.classList ?? [])]).toEqual(
        expect.arrayContaining(["shrink-0", "max-[520px]:max-w-full", "max-[520px]:justify-start"]),
      );
      expect(row.querySelectorAll("kbd").length).toBeGreaterThan(0);
    }

    expect(screen.getByText("Open the New session composer (⌘N is reserved by the browser)").classList.contains("[overflow-wrap:anywhere]")).toBe(true);
  });
});

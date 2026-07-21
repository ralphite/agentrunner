// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SettingsGeneral } from "./SettingsGeneral";

afterEach(cleanup);

describe("SettingsGeneral mobile layout", () => {
  it("stacks section content and keeps confirmation actions in stable columns", () => {
    const onReset = vi.fn();
    render(<SettingsGeneral query="" onReset={onReset} />);

    // SETTINGS-GENERAL-CHROME (R81): each row is now an rs-row card; the flex
    // layout that stacks on mobile moved to the inner .rs-row-head container.
    const statusHead = screen.getByText("Status").closest(".rs-row-head")!;
    const resetHead = screen.getByText("Reset settings").closest(".rs-row-head")!;
    expect(statusHead.className).toContain("max-[500px]:flex-col");
    expect(resetHead.className).toContain("max-[500px]:flex-col");

    fireEvent.click(screen.getByRole("button", { name: "Reset to defaults" }));
    const actions = screen.getByRole("button", { name: "Reset" }).parentElement!;
    expect(actions.className).toContain("max-[500px]:grid-cols-2");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("button", { name: "Reset to defaults" })).toBeTruthy();
    expect(onReset).not.toHaveBeenCalled();
  });
});

// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SettingsGeneral } from "./SettingsGeneral";

afterEach(cleanup);

describe("SettingsGeneral mobile layout", () => {
  it("stacks section content and keeps confirmation actions in stable columns", () => {
    const onReset = vi.fn();
    render(<SettingsGeneral query="" onReset={onReset} />);

    const statusSection = screen.getByText("Status").closest("section")!;
    const resetSection = screen.getByText("Reset settings").closest("section")!;
    expect(statusSection.className).toContain("max-[500px]:flex-col");
    expect(resetSection.className).toContain("max-[500px]:flex-col");

    fireEvent.click(screen.getByRole("button", { name: "Reset to defaults" }));
    const actions = screen.getByRole("button", { name: "Reset" }).parentElement!;
    expect(actions.className).toContain("max-[500px]:grid-cols-2");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("button", { name: "Reset to defaults" })).toBeTruthy();
    expect(onReset).not.toHaveBeenCalled();
  });
});

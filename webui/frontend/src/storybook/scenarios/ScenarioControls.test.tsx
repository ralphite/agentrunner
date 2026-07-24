// @vitest-environment jsdom

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ScenarioControls } from "./ScenarioControls";
import { ScenarioRunner } from "./ScenarioRunner";

describe("ScenarioControls", () => {
  it("drives Next, Reset, Replay and speed through one runner", async () => {
    const step = vi.fn();
    const runner = new ScenarioRunner({
      context: {},
      steps: [{ id: "inspect", title: "Inspect the canvas", run: step }],
      recreateContext: () => ({}),
    });
    render(<ScenarioControls runner={runner} />);

    fireEvent.change(screen.getByRole("combobox", { name: "Playback speed" }), {
      target: { value: "2" },
    });
    expect(runner.getSnapshot().speed).toBe(2);

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("completed"));
    expect(step).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("idle"));

    fireEvent.click(screen.getByRole("button", { name: "Replay" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("completed"));
    expect(step).toHaveBeenCalledTimes(2);
  });
});

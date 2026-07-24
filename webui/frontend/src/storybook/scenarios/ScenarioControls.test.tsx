// @vitest-environment jsdom

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ScenarioControls } from "./ScenarioControls";
import { ScenarioRunner } from "./ScenarioRunner";

afterEach(cleanup);

describe("ScenarioControls", () => {
  it("does not dismiss production overlays when transport controls are pressed", () => {
    const runner = new ScenarioRunner({
      context: {},
      steps: [{ id: "inspect", title: "Inspect the canvas", run: vi.fn() }],
      recreateContext: () => ({}),
    });
    const outsideMouseDown = vi.fn();
    document.addEventListener("mousedown", outsideMouseDown);
    render(<ScenarioControls runner={runner} />);

    fireEvent.mouseDown(screen.getByRole("button", { name: "Next" }));
    expect(outsideMouseDown).not.toHaveBeenCalled();

    fireEvent.mouseDown(document.body);
    expect(outsideMouseDown).toHaveBeenCalledTimes(1);
    document.removeEventListener("mousedown", outsideMouseDown);
  });

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

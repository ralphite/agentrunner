import { useMemo } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { ScenarioControls } from "./ScenarioControls";
import {
  ScenarioExecutionError,
  ScenarioRunner,
  type ScenarioSnapshot,
  type ScenarioStatus,
} from "./ScenarioRunner";

const meta = {
  title: "Workbench/Demo/ScenarioControls",
  component: ScenarioControls,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ScenarioControls>;

export default meta;
type Story = StoryObj<typeof meta>;

function InteractiveFixture() {
  const runner = useMemo(
    () =>
      new ScenarioRunner({
        context: {},
        steps: [
          { id: "open", title: "Open the session", run: () => undefined },
          { id: "send", title: "Send a message", run: () => undefined },
        ],
        recreateContext: () => ({}),
      }),
    [],
  );
  return <ScenarioControls runner={runner} />;
}

export const Default: Story = {
  render: () => <InteractiveFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.selectOptions(
      canvas.getByRole("combobox", { name: "Playback speed" }),
      "2",
    );
    await userEvent.click(canvas.getByRole("button", { name: "Next" }));
    await waitFor(() =>
      expect(canvas.getByRole("status")).toHaveTextContent("paused"),
    );
    await userEvent.click(canvas.getByRole("button", { name: "Reset" }));
    await waitFor(() =>
      expect(canvas.getByRole("status")).toHaveTextContent("idle"),
    );
  },
};

function staticRunner(status: ScenarioStatus): ScenarioRunner<Record<string, never>> {
  const currentStep =
    status === "idle" || status === "disposed"
      ? null
      : { id: "send", title: "Send a deliberately long demo step", index: 1 };
  const snapshot: ScenarioSnapshot = {
    status,
    stepIndex: status === "idle" ? 0 : 1,
    stepCount: 4,
    completedSteps: status === "completed" ? 4 : status === "idle" ? 0 : 1,
    currentStep,
    owner: status === "running" ? "manual" : null,
    speed: 1,
    epoch: 0,
    error:
      status === "failed"
        ? new ScenarioExecutionError(
            "step",
            new Error("The scripted API response did not arrive"),
            currentStep,
          )
        : null,
  };
  return {
    subscribe: () => () => undefined,
    getSnapshot: () => snapshot,
    play: async () => undefined,
    pause: () => true,
    next: async () => undefined,
    reset: async () => undefined,
    replay: async () => undefined,
    setSpeed: () => undefined,
  } as unknown as ScenarioRunner<Record<string, never>>;
}

const statuses: ScenarioStatus[] = [
  "idle",
  "running",
  "paused",
  "completed",
  "failed",
  "resetting",
  "disposed",
];

export const AllPlaybackStates: Story = {
  render: () => (
    <div className="grid gap-px bg-line">
      {statuses.map((status) => (
        <ScenarioControls
          key={status}
          label={`Demo playback: ${status}`}
          autoPlay={status === "running"}
          onAutoPlayChange={() => undefined}
          runner={staticRunner(status)}
        />
      ))}
    </div>
  ),
};

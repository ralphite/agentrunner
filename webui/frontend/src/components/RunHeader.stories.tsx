import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { RunHeader } from "./RunParts";

const meta = {
  title: "Components/Runs/Run Header",
  component: RunHeader,
  args: {
    title: "Verify every Storybook component state",
    status: "running",
    kind: "drive",
    onStop: fn(),
  },
  decorators: [
    (Story) => (
      <div className="w-full min-w-0">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RunHeader>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {};

export const LifecycleMatrix: Story = {
  render: (args) => (
    <div className="flex flex-col">
      {[
        { title: "Queued one-time run", status: "queued", kind: "once" },
        { title: "Running drive", status: "running", kind: "drive" },
        { title: "Paused schedule", status: "paused", kind: "scheduled" },
        { title: "Completed audit", status: "done", kind: "once" },
        { title: "Failed browser verification", status: "failed", kind: "drive" },
        {
          title:
            "A very long run title that verifies truncation while status and actions remain visible",
          status: "running",
          kind: "drive",
        },
      ].map((state) => (
        <RunHeader key={state.title} {...args} {...state} />
      ))}
    </div>
  ),
};

export const MissingMetadata: Story = {
  args: {
    title: "Run details unavailable",
    status: undefined,
    kind: undefined,
    onStop: undefined,
  },
};

export const RunningWithoutStop: Story = {
  args: {
    title: "Running under external control",
    status: "running",
    kind: "scheduled",
    onStop: undefined,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("running")).toBeVisible();
    await expect(canvas.getByText("scheduled run")).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: "Stop run" }),
    ).not.toBeInTheDocument();
  },
};

export const KeyboardStop: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const stop = canvas.getByRole("button", { name: "Stop run" });
    stop.focus();
    await expect(stop).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onStop).toHaveBeenCalled();
  },
};

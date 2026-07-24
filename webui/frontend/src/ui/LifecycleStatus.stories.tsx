import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  LifecycleStatus,
  type LifecycleStatusState,
} from "./LifecycleStatus";

const states: LifecycleStatusState[] = [
  "running",
  "done",
  "waiting",
  "idle",
  "attention",
  "failed",
];

const meta = {
  title: "Foundations/Feedback/Lifecycle Status",
  component: LifecycleStatus,
  parameters: { layout: "centered" },
  args: {
    accessibleLabel: "Agent is running",
    role: "status",
    state: "running",
    visibleLabel: "Running",
  },
} satisfies Meta<typeof LifecycleStatus>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const status = canvas.getByRole("status", { name: "Agent is running" });
    await expect(status).toHaveTextContent("Running");
    await expect(status).toHaveAttribute("data-lifecycle-state", "running");
    await expect(status).toHaveAttribute("aria-busy", "true");
  },
};

export const LifecycleMatrix: Story = {
  render: () => (
    <div className="grid grid-cols-2 gap-x-8 gap-y-4 p-2">
      {states.map((state) => (
        <LifecycleStatus
          accessibleLabel={`Agent status: ${state}`}
          key={state}
          state={state}
          visibleLabel={state[0].toUpperCase() + state.slice(1)}
        />
      ))}
    </div>
  ),
};

export const IconOnly: Story = {
  render: () => (
    <div className="flex items-center gap-5">
      {states.map((state) => (
        <LifecycleStatus
          accessibleLabel={`Agent status: ${state}`}
          key={state}
          size="md"
          state={state}
        />
      ))}
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("img", { name: "Agent status: waiting" }),
    ).toHaveTextContent("");
  },
};

export const LongVisibleCopy: Story = {
  render: () => (
    <div className="max-w-[300px]">
      <LifecycleStatus
        accessibleLabel="Scheduled review is waiting for an external approval"
        state="attention"
        visibleLabel="Waiting for an unusually long external approval status"
      />
    </div>
  ),
};

export const ReducedMotion: Story = {
  args: {
    accessibleLabel: "Background review is running",
    size: "md",
    state: "running",
    visibleLabel: undefined,
  },
};

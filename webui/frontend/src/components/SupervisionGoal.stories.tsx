import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { GoalDerived } from "../timeline";
import { GoalSection, type GoalState } from "./SupervisionParts";

const activeGoal: GoalState = {
  goal: "Ship a browser-verified component system without changing production behavior",
  checks: 3,
  max_checks: 7,
  verifiers: 2,
};

const actions = {
  onGoalEdit: fn(),
  onGoalSave: fn(),
  onGoalDiscard: fn(),
  onGoalAction: fn(),
};

function GoalSurface({ children }: { children: React.ReactNode }) {
  return (
    <div className="session-view min-h-[360px] min-w-0 w-full p-3 sm:p-6">
      <div
        className="supervision-panel session-side"
        style={{
          position: "relative",
          inset: "auto",
          margin: "0 auto",
          width: 344,
          maxWidth: "100%",
        }}
      >
        {children}
      </div>
    </div>
  );
}

const meta = {
  title: "Components/Supervision/Goal Section",
  component: GoalSection,
  args: {
    loading: false,
    goal: activeGoal,
    goalEdit: null,
    settledGoal: null,
    goalEchoed: false,
    ...actions,
  },
  render: (args) => <GoalSurface><GoalSection {...args} /></GoalSurface>,
} satisfies Meta<typeof GoalSection>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Active: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Pause" }));
    await expect(args.onGoalAction).toHaveBeenCalledWith("pause");
  },
};

export const PausedSelfCertified: Story = {
  args: {
    goal: {
      ...activeGoal,
      paused: true,
      verifiers: 0,
      checks: 7,
    },
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Paused")).toBeVisible();
    await expect(canvas.getByText("Self-certified")).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "Resume" }));
    await expect(args.onGoalAction).toHaveBeenCalledWith("resume");
  },
};

export const Editing: Story = {
  args: {
    goalEdit: "Ship the final component system after browser review",
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByRole("textbox", { name: "Goal" });
    await expect(input).toHaveFocus();
    await userEvent.keyboard("{Escape}");
    await expect(args.onGoalDiscard).toHaveBeenCalledOnce();
  },
};

const settled: GoalDerived[] = [
  {
    phase: "achieved",
    goal: "Complete every Supervision state",
    checks: 7,
    elapsedMs: 94_000,
  },
  {
    phase: "stopped",
    goal: "Stop when further work no longer adds value",
    checks: 2,
    elapsedMs: 31_000,
  },
  {
    phase: "cancelled",
    goal: "Cancel a superseded implementation direction",
    checks: 1,
    elapsedMs: 8_000,
  },
];

export const SettledOutcomes: Story = {
  render: (args) => (
    <div className="grid min-h-[420px] gap-4 p-6 md:grid-cols-3">
      {settled.map((item) => (
        <GoalSurface key={item.phase}>
          <GoalSection {...args} goal={null} settledGoal={item} />
        </GoalSurface>
      ))}
    </div>
  ),
};

export const SettledEchoedCompact: Story = {
  args: {
    goal: null,
    settledGoal: settled[2],
    goalEchoed: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByText("Goal")).not.toBeInTheDocument();
    await expect(canvas.getByText("Cancelled")).toBeVisible();
  },
};

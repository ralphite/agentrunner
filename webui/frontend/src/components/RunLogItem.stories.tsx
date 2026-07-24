import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { RunLogEmptyState, RunLogItem } from "./RunParts";

const meta = {
  title: "Components/Runs/Run Log Item",
  component: RunLogItem,
  args: {
    line: {
      raw: "{}",
      kind: "message",
      text: "Browser coverage and keyboard checks are complete.",
    },
  },
  decorators: [
    (Story) => (
      <div className="runlog max-w-[760px] p-3 font-mono text-[12px] leading-[1.5]">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof RunLogItem>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Message: Story = {};

export const EventMatrix: Story = {
  render: () => (
    <>
      <RunLogItem
        line={{
          raw: "{}",
          kind: "session_start",
          text: "session story-child-1",
          iter: 1,
        }}
      />
      <RunLogItem
        line={{
          raw: "{}",
          kind: "tool_call",
          text: "$ npm run test:storybook",
        }}
      />
      <RunLogItem
        line={{
          raw: "{}",
          kind: "tool_result",
          text: "→ bash 206 stories passed",
        }}
      />
      <RunLogItem
        line={{
          raw: "plain stderr",
          kind: "",
          text: "plain stderr output without an event kind",
        }}
      />
      <RunLogItem
        line={{
          raw: "{}",
          kind: "message",
          text:
            "A long output line verifies that wrapping remains readable and never expands the run view beyond the available width. ".repeat(
              3,
            ),
        }}
      />
    </>
  ),
};

export const SuccessfulVerdict: Story = {
  args: {
    line: {
      raw: "{}",
      kind: "run_end",
      text: "",
      verdict: { reason: "satisfied", n: 1, ok: true },
    },
  },
};

export const FailedVerdict: Story = {
  args: {
    line: {
      raw: "driver stalled: 3 iterations",
      kind: "driver",
      text: "",
      verdict: { reason: "stalled", n: 3, ok: false },
    },
  },
};

export const IterationVerdict: Story = {
  args: {
    line: {
      raw: "{}",
      kind: "run_end",
      text: "",
      iter: 4,
      verdict: { reason: "satisfied", n: 4, ok: true },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("iteration 4")).toBeVisible();
    await expect(
      canvas.getByText("■ series satisfied · 4 iterations"),
    ).toBeVisible();
  },
};

export const WaitingForOutput: Story = {
  render: () => <RunLogEmptyState />,
};

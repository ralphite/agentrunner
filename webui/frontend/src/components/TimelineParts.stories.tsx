import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { BubbleItem } from "../timeline";
import {
  TimelineEmptyState,
  TimelineJumpToLatest,
  TimelineLoadingState,
  TimelinePendingMessage,
  TimelineTailActions,
} from "./Timeline";

const assistant: BubbleItem = {
  kind: "assistant",
  key: "story-final-answer",
  text: "All component states are covered and ready for browser QA.",
};
const onJump = fn();

const meta = {
  title: "Components/Timeline/Timeline Chrome",
  component: TimelinePendingMessage,
  args: {
    message: {
      id: 1,
      text: "Verify the Storybook interaction states.",
      imgs: [],
      files: 0,
      delivery: "queue",
    },
  },
  decorators: [
    (Story) => (
      <div className="timeline relative min-h-56 p-6">
        <div className="tl-inner">
          <Story />
        </div>
      </div>
    ),
  ],
} satisfies Meta<typeof TimelinePendingMessage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const PendingQueued: Story = {};

export const PendingSteering: Story = {
  args: {
    message: {
      id: 2,
      text: "Use the browser result to steer the active session.",
      imgs: [],
      files: 0,
      delivery: "steer",
    },
  },
};

export const PendingAttachmentsAndLongCopy: Story = {
  args: {
    message: {
      id: 3,
      text:
        "Review the attached implementation evidence and verify that a long pending instruction remains readable without breaking the thread layout. ".repeat(
          2,
        ),
      imgs: [],
      files: 4,
      delivery: "queue",
    },
  },
};

export const TailActions: Story = {
  render: () => <TimelineTailActions lastAssistant={assistant} />,
};

export const TailActionsWithGoalVerdict: Story = {
  render: () => (
    <TimelineTailActions
      lastAssistant={assistant}
      goalVerdict={{ elapsed: "2m 14s" }}
    />
  ),
};

export const TailGoalVerdictOnly: Story = {
  render: () => <TimelineTailActions goalVerdict={{ elapsed: "38s" }} />,
};

export const JumpStateMatrix: Story = {
  render: () => (
    <div className="grid grid-cols-2 gap-3">
      {[0, 1, 12, 145].map((unseen) => (
        <div
          className="relative min-h-24 rounded border border-line"
          key={unseen}
        >
          <TimelineJumpToLatest unseen={unseen} onJump={fn()} />
        </div>
      ))}
    </div>
  ),
};

export const JumpKeyboardInteraction: Story = {
  render: () => <TimelineJumpToLatest unseen={3} onJump={onJump} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const jump = canvas.getByRole("button", {
      name: "3 new updates; jump to latest",
    });
    jump.focus();
    await expect(jump).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(onJump).toHaveBeenCalled();
  },
};

export const Loading: Story = {
  render: () => <TimelineLoadingState />,
};

export const Empty: Story = {
  render: () => <TimelineEmptyState />,
};

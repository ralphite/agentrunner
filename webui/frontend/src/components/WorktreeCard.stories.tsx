import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { WorktreeCard } from "./WorktreeCard";

const meta = {
  title: "Components/Settings/Worktree Card",
  component: WorktreeCard,
  args: {
    workspace: "/Users/demo/agentrunner",
    sessions: [
      { id: "settings-story", title: "Build Settings stories" },
      { id: "browser-qa", title: "Verify all browser states" },
    ],
    onOpenSession: fn(),
  },
  decorators: [
    (Story) => (
      <div className="rs-panel mx-auto max-w-[760px] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof WorktreeCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const MultipleSessions: Story = {};

export const SingleSession: Story = {
  args: {
    workspace: "/Users/demo/docs",
    sessions: [{ id: "docs", title: "Prepare delivery notes" }],
  },
};

export const EmptySessions: Story = {
  args: {
    workspace: "/Users/demo/unused-worktree",
    sessions: [],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("0 conversations")).toBeVisible();
    await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
  },
};

export const LongContent: Story = {
  args: {
    workspace:
      "/Users/demo/projects/a-very-deep-and-descriptive-workspace-name/with-an-even-longer-managed-worktree-suffix",
    sessions: [
      {
        id: "long",
        title:
          "A very long session title that verifies wrapping without expanding the settings panel beyond its available width",
      },
      { id: "short", title: "Short follow-up" },
    ],
  },
};

export const KeyboardOpen: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const session = canvas.getByRole("button", {
      name: "Build Settings stories",
    });
    session.focus();
    await expect(session).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onOpenSession).toHaveBeenCalledWith("settings-story");
  },
};

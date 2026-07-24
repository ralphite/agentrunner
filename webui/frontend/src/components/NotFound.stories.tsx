import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { SessionNotFound } from "./NotFound";

const meta = {
  title: "Components/Feedback/SessionNotFound",
  component: SessionNotFound,
  parameters: {
    layout: "centered",
  },
  args: {
    sid: "01JY7MISSING9W8Z6Q",
    onBack: fn(),
  },
} satisfies Meta<typeof SessionNotFound>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardBack: Story = {
  args: {
    onBack: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("alert")).toHaveTextContent("01JY7MISSING9W8Z6Q");
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    const back = canvas.getByRole("button", { name: "Back to all sessions" });
    await expect(back).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onBack).toHaveBeenCalledOnce();
  },
};

export const LongSessionId: Story = {
  args: {
    sid: "01JY7MISSING9W8Z6Q-archived-worktree-session-with-a-very-long-id",
  },
};

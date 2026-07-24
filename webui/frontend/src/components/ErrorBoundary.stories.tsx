import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { ErrorBoundary } from "./ErrorBoundary";

function BrokenView(): never {
  throw new Error("Story fixture failed to render");
}

const meta = {
  title: "Components/Feedback/ErrorBoundary",
  component: ErrorBoundary,
  parameters: {
    layout: "centered",
  },
  args: {
    resetKey: "story",
    children: <div>Healthy component</div>,
  },
} satisfies Meta<typeof ErrorBoundary>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const RenderError: Story = {
  args: {
    children: <BrokenView />,
  },
};

export const KeyboardRecovery: Story = {
  args: {
    children: <BrokenView />,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("This view hit a render error.")).toBeVisible();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Retry" })).toHaveFocus();
  },
};

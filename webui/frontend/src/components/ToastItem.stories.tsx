import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ToastItem } from "./ToastItem";

const meta = {
  title: "Components/Feedback/Toast Item",
  component: ToastItem,
  args: {
    toast: {
      id: 1,
      kind: "info",
      text: "Storybook coverage updated.",
    },
    onDismiss: fn(),
  },
  decorators: [
    (Story) => (
      <div className="toast-stack relative p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ToastItem>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Info: Story = {};

export const Error: Story = {
  args: {
    toast: {
      id: 2,
      kind: "error",
      text: "The browser check could not finish.",
    },
  },
};

export const ErrorWithDetails: Story = {
  args: {
    toast: {
      id: 3,
      kind: "error",
      text: "The browser check could not finish.",
      details:
        "playwright: locator('button').click: element was detached\nRetry the interaction after the Story finishes rendering.",
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText("Details", { selector: "summary" }));
    await expect(canvas.getByText(/element was detached/)).toBeVisible();
  },
};

export const LongContent: Story = {
  args: {
    toast: {
      id: 4,
      kind: "info",
      text:
        "A long informational notification verifies that wrapping, close affordance alignment, and the available width remain readable without hiding the outcome.",
    },
  },
};

export const LongDetailsOverflow: Story = {
  args: {
    toast: {
      id: 5,
      kind: "error",
      text: "The browser check produced a long diagnostic.",
      details: Array.from(
        { length: 24 },
        (_, index) => `diagnostic line ${index + 1}: locator remained detached during the interaction`,
      ).join("\n"),
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText("Details", { selector: "summary" }));
    const details = canvasElement.querySelector<HTMLPreElement>("pre");
    await expect(details).toBeVisible();
    await expect(details!.scrollHeight).toBeGreaterThan(details!.clientHeight);
    await expect(getComputedStyle(details!).overflowY).toBe("auto");
    details!.focus();
    await expect(details).toHaveFocus();
  },
};

export const KeyboardDismiss: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const dismiss = canvas.getByRole("button", {
      name: "Dismiss notification",
    });
    dismiss.focus();
    await expect(dismiss).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onDismiss).toHaveBeenCalledWith(1);
  },
};

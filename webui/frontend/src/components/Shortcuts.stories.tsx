import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { Shortcuts } from "./Shortcuts";

const meta = {
  title: "Components/Navigation/Shortcuts",
  component: Shortcuts,
  args: {
    onClose: fn(),
  },
} satisfies Meta<typeof Shortcuts>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: { onClose: fn() },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const search = canvas.getByRole("textbox", { name: "Search keyboard shortcuts" });
    await expect(search).toHaveFocus();
    await userEvent.type(search, "settings");
    await expect(canvas.getByText("Open settings")).toBeVisible();
    await userEvent.keyboard("{Escape}");
    await expect(args.onClose).toHaveBeenCalledOnce();
  },
};

export const NoMatches: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      canvas.getByRole("textbox", { name: "Search keyboard shortcuts" }),
      "definitely-not-a-shortcut",
    );
    await expect(canvas.getByText("No matching shortcuts")).toBeVisible();
  },
};

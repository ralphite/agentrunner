import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { SettingsShortcuts } from "./SettingsShortcuts";

const meta = {
  title: "Components/Settings/Shortcuts",
  component: SettingsShortcuts,
  args: {
    query: "",
  },
  render: (args) => (
    <StoryAppFrame>
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsShortcuts {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Keyboard shortcuts" })).toBeVisible();
    await expect(canvas.getByText("Command palette", { selector: ".rs-sc-label > span" })).toBeVisible();
    await expect(canvas.getByText("Composer", { selector: ".rs-sc-grouptitle" })).toBeVisible();
  },
} satisfies Meta<typeof SettingsShortcuts>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: {
    query: "command palette",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Keyboard shortcuts" })).toBeVisible();
    await expect(canvas.getByText("Command palette", { selector: ".rs-sc-label > span" })).toBeVisible();
    await expect(canvas.getByText("Open selection")).toBeVisible();
    await expect(canvas.queryByText("Composer", { selector: ".rs-sc-grouptitle" })).not.toBeInTheDocument();
    await expect(canvas.queryAllByRole("button")).toHaveLength(0);
  },
};

export const NoMatches: Story = {
  args: {
    query: "voice dictation",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No shortcuts match “voice dictation”.")).toBeVisible();
    await expect(canvas.queryByText("Global", { selector: ".rs-sc-grouptitle" })).not.toBeInTheDocument();
  },
};

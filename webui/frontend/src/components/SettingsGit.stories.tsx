import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { SettingsGit } from "./SettingsGit";

const meta = {
  title: "Components/Settings/Git",
  component: SettingsGit,
  args: {
    query: "",
  },
  render: (args) => (
    <StoryAppFrame>
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsGit {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Git" })).toBeVisible();
    await expect(canvas.getByRole("textbox", { name: "Commit message template" })).toHaveValue("changes from agent session");
  },
} satisfies Meta<typeof SettingsGit>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const template = canvas.getByRole("textbox", { name: "Commit message template" });
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    await expect(template).toHaveFocus();
    await userEvent.clear(template);
    await userEvent.keyboard("fix: preserve keyboard focus");
    await expect(template).toHaveValue("fix: preserve keyboard focus");
  },
};

export const CustomTemplate: Story = {
  render: (args) => (
    <StoryAppFrame
      services={{
        local: {
          "arwebui.git": JSON.stringify({ commitTemplate: "feat: {summary}" }),
        },
      }}
    >
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsGit {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("textbox", { name: "Commit message template" })).toHaveValue("feat: {summary}");
  },
};

export const NoMatches: Story = {
  args: {
    query: "branch prefix",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No Git settings match “branch prefix”.")).toBeVisible();
    await expect(canvas.queryByRole("textbox")).not.toBeInTheDocument();
  },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { SettingsArchived } from "./SettingsArchived";

const archivedState = {
  sessions: [
    {
      id: "archived-review",
      status: "completed",
      turns: 6,
      title: "Review the component system",
      workspace: "/Users/demo/agentrunner",
    },
    {
      id: "archived-a11y",
      status: "waiting_for_approval",
      turns: 3,
      title: "Audit keyboard navigation",
      workspace: "/Users/demo/agentrunner",
    },
    {
      id: "archived-docs",
      status: "completed",
      turns: 2,
      title: "Document delivery evidence",
      workspace: "/Users/demo/docs",
    },
  ],
  sessionsReady: true,
  archived: ["archived-review", "archived-a11y", "archived-docs"],
} satisfies Partial<AppState>;

const meta = {
  title: "Components/Settings/Archived",
  component: SettingsArchived,
  args: {
    query: "",
    onClose: fn(),
  },
  render: (args) => (
    <StoryAppFrame initialState={archivedState}>
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsArchived {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Archived sessions" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Open Review the component system" })).toBeVisible();
    await expect(canvas.getAllByRole("button", { name: "Unarchive" })).toHaveLength(3);
  },
} satisfies Meta<typeof SettingsArchived>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Open Review the component system" })).toHaveFocus();
    await userEvent.tab();
    const restore = canvas.getAllByRole("button", { name: "Unarchive" })[0];
    await expect(restore).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.queryByRole("button", { name: "Open Review the component system" })).not.toBeInTheDocument();
  },
};

export const Empty: Story = {
  render: (args) => (
    <StoryAppFrame initialState={{ sessions: [], sessionsReady: true, archived: [] }}>
      <div className="mx-auto max-w-[760px] p-6">
        <SettingsArchived {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No archived sessions")).toBeVisible();
    await expect(canvas.getByText("Archived conversations will appear here.")).toBeVisible();
  },
};

export const NoMatches: Story = {
  args: {
    query: "nonexistent workspace",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No matches")).toBeVisible();
    await expect(canvas.getByText("No archived session matches “nonexistent workspace”.")).toBeVisible();
  },
};

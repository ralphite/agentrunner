import { DotsThree, PencilSimple, Trash } from "@phosphor-icons/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { Menu, MenuItem, MenuLabel } from "./Menu";

const onRename = fn();
const onDelete = fn();

const meta = {
  title: "Components/Overlays/Menu",
  component: Menu,
  parameters: {
    layout: "centered",
  },
  args: {
    label: null,
    children: null,
  },
  render: () => (
    <Menu label={<DotsThree size={18} />} ariaLabel="Session actions">
      <MenuLabel>Component migration</MenuLabel>
      <MenuItem onClick={onRename}><PencilSimple size={16} /> Rename…</MenuItem>
      <MenuItem onClick={onDelete} danger><Trash size={16} /> Delete</MenuItem>
    </Menu>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Session actions" }));
    await expect(canvas.getByRole("menu")).toBeVisible();
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Rename…" })).toHaveFocus());
  },
} satisfies Meta<typeof Menu>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Session actions" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Rename…" })).toHaveFocus());
    await userEvent.keyboard("{ArrowDown}");
    await expect(canvas.getByRole("menuitem", { name: "Delete" })).toHaveFocus();
    await userEvent.keyboard("{Escape}");
    await expect(trigger).toHaveFocus();
  },
};

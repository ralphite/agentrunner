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

export const Closed: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", { name: "Session actions" }),
    ).toHaveAttribute("aria-expanded", "false");
    await expect(canvas.queryByRole("menu")).not.toBeInTheDocument();
  },
};

export const KeyboardWrapAndSelectionReturn: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Session actions" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    const rename = canvas.getByRole("menuitem", { name: "Rename…" });
    const remove = canvas.getByRole("menuitem", { name: "Delete" });
    await waitFor(() => expect(rename).toHaveFocus());
    await userEvent.keyboard("{ArrowUp}");
    await expect(remove).toHaveFocus();
    await userEvent.keyboard("{Home}");
    await expect(rename).toHaveFocus();
    await userEvent.keyboard("{End}{Enter}");
    await expect(canvas.queryByRole("menu")).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};

export const LongOverflow: Story = {
  render: () => (
    <Menu label={<DotsThree size={18} />} ariaLabel="Long actions">
      <MenuLabel>
        An intentionally long component migration label that exercises overflow
      </MenuLabel>
      {Array.from({ length: 24 }, (_, index) => (
        <MenuItem key={index} onClick={() => {}}>
          <PencilSimple size={16} />
          {`Action ${index + 1} with an intentionally long descriptive label`}
        </MenuItem>
      ))}
      <MenuItem onClick={() => {}} danger>
        <Trash size={16} /> Delete every generated artifact
      </MenuItem>
    </Menu>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Long actions" }));
    const menu = canvas.getByRole("menu");
    await waitFor(() =>
      expect(menu.scrollHeight).toBeGreaterThan(menu.clientHeight),
    );
    await expect(
      canvas.getByRole("menuitem", {
        name: /Action 1 with an intentionally long descriptive label/,
      }),
    ).toBeVisible();
  },
};

export const SemanticPseudoStates: Story = {
  parameters: {
    pseudo: {
      hover: ".menu-trigger",
      focusVisible: ".menu-item:not(.danger)",
      active: ".menu-item.danger",
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Session actions" });
    await waitFor(() => expect(trigger).toBeVisible());
    await userEvent.click(trigger);
    const rename = canvas.getByRole("menuitem", { name: "Rename…" });
    await waitFor(() => expect(rename).toBeVisible());
    await expect(canvas.getByRole("menuitem", { name: "Delete" })).toHaveClass(
      "danger",
    );
  },
};

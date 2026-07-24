import { useLayoutEffect, useRef, useState } from "react";
import { Archive, PencilSimple, Trash } from "@phosphor-icons/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { humanPause } from "../storybook/humanPlayback";
import { ContextMenu } from "./ContextMenu";
import { MenuItem, MenuLabel } from "./Menu";

const onClose = fn();

function ContextMenuFixture(props: React.ComponentProps<typeof ContextMenu>) {
  const invokingRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  useLayoutEffect(() => {
    invokingRef.current?.focus();
    setOpen(true);
  }, []);
  const children = props.children || (
    <>
      <MenuLabel>Session actions</MenuLabel>
      <MenuItem onClick={() => {}}>
        <PencilSimple size={16} /> Rename…
      </MenuItem>
      <MenuItem onClick={() => {}}>
        <Archive size={16} /> Archive
      </MenuItem>
      <MenuItem onClick={() => {}} danger>
        <Trash size={16} /> Delete
      </MenuItem>
    </>
  );
  return (
    <div className="min-h-[320px] bg-bg p-4 text-ink">
      <button ref={invokingRef}>Invoking session</button>
      {open && (
        <ContextMenu
          {...props}
          onClose={() => {
            props.onClose();
            setOpen(false);
          }}
        >
          {children}
        </ContextMenu>
      )}
    </div>
  );
}

const meta = {
  title: "Components/Overlays/ContextMenu",
  component: ContextMenu,
  parameters: {
    layout: "fullscreen",
  },
  args: {
    x: 72,
    y: 64,
    onClose,
    children: null,
  },
  render: (args) => <ContextMenuFixture {...args} />,
} satisfies Meta<typeof ContextMenu>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(canvas.getByRole("menuitem", { name: "Rename…" })).toHaveFocus(),
    );
    await userEvent.keyboard("{End}");
    await expect(
      canvas.getByRole("menuitem", { name: "Delete" }),
    ).toHaveFocus();
    await userEvent.keyboard("{Home}");
    await expect(
      canvas.getByRole("menuitem", { name: "Rename…" }),
    ).toHaveFocus();
    await humanPause();
    await userEvent.keyboard("{Escape}");
    await expect(onClose).toHaveBeenCalled();
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Invoking session" }),
      ).toHaveFocus(),
    );
  },
};

export const ViewportEdgeLongContent: Story = {
  args: {
    x: 1270,
    y: 710,
    children: (
      <>
        <MenuLabel>
          An exceptionally long session title that must remain inside the
          context menu
        </MenuLabel>
        <MenuItem onClick={() => {}}>
          <PencilSimple size={16} /> Rename a session with a very long action
          label…
        </MenuItem>
        <MenuItem onClick={() => {}} danger>
          <Trash size={16} /> Remove permanently
        </MenuItem>
      </>
    ),
  },
  play: async ({ canvasElement }) => {
    const menu = within(canvasElement).getByRole("menu");
    await waitFor(() => expect(menu).toBeVisible());
    const rect = menu.getBoundingClientRect();
    await expect(rect.right).toBeLessThanOrEqual(window.innerWidth - 8);
    await expect(rect.bottom).toBeLessThanOrEqual(window.innerHeight - 8);
    await expect(
      within(menu).getByText(/exceptionally long session title/),
    ).toBeVisible();
  },
};

import { useLayoutEffect, useRef, useState } from "react";
import { Archive, PencilSimple, Trash } from "@phosphor-icons/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  expect,
  fireEvent,
  fn,
  userEvent,
  waitFor,
  within,
} from "storybook/test";
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
      <MenuItem onClick={() => {}} disabled>
        Export unavailable
      </MenuItem>
      <div hidden>
        <MenuItem onClick={() => {}}>Hidden action</MenuItem>
      </div>
      <div aria-hidden="true" style={{ display: "none" }}>
        <MenuItem onClick={() => {}}>ARIA hidden action</MenuItem>
      </div>
      <div style={{ display: "none" }}>
        <MenuItem onClick={() => {}}>CSS hidden action</MenuItem>
      </div>
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
      <div className="flex items-center gap-2">
        <button>Before context menu</button>
        <button
          ref={invokingRef}
          onClick={() => setOpen(true)}
          onContextMenu={(event) => {
            event.preventDefault();
            setOpen(true);
          }}
          onKeyDown={(event) => {
            if (
              (event.shiftKey && event.key === "F10") ||
              event.key === "ContextMenu"
            ) {
              event.preventDefault();
              setOpen(true);
            }
          }}
        >
          Invoking session
        </button>
      </div>
      {open && (
        <ContextMenu
          {...props}
          ariaLabel={props.ariaLabel || "Session actions"}
          returnFocus={props.returnFocus || invokingRef.current}
          onClose={() => {
            props.onClose();
            setOpen(false);
          }}
        >
          {children}
        </ContextMenu>
      )}
      <button className="mt-3">After context menu</button>
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
    ariaLabel: "Session actions",
    returnFocus: null,
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
    const invoking = canvas.getByRole("button", { name: "Invoking session" });
    const before = canvas.getByRole("button", {
      name: "Before context menu",
    });
    const after = canvas.getByRole("button", { name: "After context menu" });
    const rename = () =>
      canvas.getByRole("menuitem", { name: "Rename…" });
    const archive = () =>
      canvas.getByRole("menuitem", { name: "Archive" });
    const remove = () =>
      canvas.getByRole("menuitem", { name: "Delete" });
    const expectOneRovingItem = async (active: HTMLElement) => {
      const all = canvas.getAllByRole("menuitem", { hidden: true });
      await expect(all.filter((item) => item.tabIndex === 0)).toEqual([active]);
      await expect(
        canvas.getByRole("menuitem", { name: "Export unavailable" }),
      ).toBeDisabled();
      await expect(
        canvas.getByRole("menuitem", { name: "Export unavailable" }).tabIndex,
      ).toBe(-1);
      await expect(
        canvas.getByRole("menuitem", {
          name: "Hidden action",
          hidden: true,
        }).tabIndex,
      ).toBe(-1);
    };

    await waitFor(() => expect(rename()).toHaveFocus());
    await expectOneRovingItem(rename());
    await userEvent.keyboard("{ArrowDown}");
    await expect(archive()).toHaveFocus();
    await expectOneRovingItem(archive());
    await userEvent.keyboard("{ArrowUp}");
    await expect(rename()).toHaveFocus();
    await userEvent.keyboard("{End}");
    await expect(remove()).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}");
    await expect(rename()).toHaveFocus();
    await userEvent.keyboard("{ArrowUp}");
    await expect(remove()).toHaveFocus();
    await userEvent.keyboard("{Home}");
    await expect(rename()).toHaveFocus();

    await userEvent.tab();
    await waitFor(() => expect(after).toHaveFocus());
    await expect(canvas.queryByRole("menu")).toBeNull();

    invoking.focus();
    await userEvent.keyboard("{Shift>}{F10}{/Shift}");
    await waitFor(() => expect(rename()).toHaveFocus());
    await userEvent.tab({ shift: true });
    await waitFor(() => expect(before).toHaveFocus());
    await expect(canvas.queryByRole("menu")).toBeNull();

    invoking.focus();
    fireEvent.keyDown(invoking, { key: "ContextMenu" });
    await waitFor(() => expect(rename()).toHaveFocus());
    await humanPause();
    await userEvent.keyboard("{Escape}");
    await expect(onClose).toHaveBeenCalled();
    await waitFor(() => expect(invoking).toHaveFocus());

    fireEvent.contextMenu(invoking);
    await waitFor(() => expect(rename()).toHaveFocus());
    await userEvent.click(archive());
    await waitFor(() => expect(invoking).toHaveFocus());
    await expect(canvas.queryByRole("menu")).toBeNull();
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
        <MenuItem
          onClick={() => {}}
          title="Rename a session with a very long action label…"
        >
          <PencilSimple size={16} /> Rename a session with a very long action
          label…
        </MenuItem>
        {Array.from({ length: 24 }, (_, index) => (
          <MenuItem
            key={index}
            onClick={() => {}}
            title={`Additional context action ${index + 1}`}
          >
            Additional context action {index + 1}
          </MenuItem>
        ))}
        <MenuItem onClick={() => {}} danger title="Remove permanently">
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
    await waitFor(() =>
      expect(menu.scrollHeight).toBeGreaterThan(menu.clientHeight),
    );
    menu.scrollTop = menu.scrollHeight;
    fireEvent.scroll(menu);
    await expect(menu).toBeVisible();
    await expect(
      within(menu).getByRole("menuitem", { name: "Remove permanently" }),
    ).toBeVisible();
  },
};

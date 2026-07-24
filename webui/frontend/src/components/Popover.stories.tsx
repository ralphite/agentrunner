import { GearSix, Monitor, Moon, Sun } from "@phosphor-icons/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { PopItem, Popover, PopSection } from "./Popover";

const onLight = fn();
const onDark = fn();

const meta = {
  title: "Components/Overlays/Popover",
  component: Popover,
  parameters: {
    layout: "centered",
  },
  args: {
    trigger: () => null,
    children: () => null,
  },
  render: () => (
    <Popover
      trigger={(open, toggle) => (
        <button onClick={toggle} aria-label="Appearance" aria-expanded={open}>
          <GearSix size={17} />
        </button>
      )}
    >
      {(close) => (
        <PopSection label="Theme">
          <PopItem
            icon={<Monitor size={16} />}
            title="System"
            desc="Follow this device"
            active
            onClick={close}
          />
          <PopItem icon={<Sun size={16} />} title="Light" onClick={() => { onLight(); close(); }} />
          <PopItem icon={<Moon size={16} />} title="Dark" onClick={() => { onDark(); close(); }} />
          <PopItem title="Unavailable theme" disabled />
        </PopSection>
      )}
    </Popover>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Appearance" }));
    await expect(canvas.getByRole("menu")).toBeVisible();
    await expect(canvas.getByRole("menuitem", { name: /System/ })).toBeVisible();
  },
} satisfies Meta<typeof Popover>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Appearance" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: /System/ })).toHaveFocus());
    await userEvent.keyboard("{End}");
    await expect(canvas.getByRole("menuitem", { name: "Dark" })).toHaveFocus();
    await userEvent.keyboard("{Escape}");
    await expect(trigger).toHaveFocus();
  },
};

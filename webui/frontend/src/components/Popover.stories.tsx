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

export const KeyboardWrapSkipAndSelectionReturn: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Appearance" });
    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");
    const system = canvas.getByRole("menuitem", { name: /System/ });
    const dark = canvas.getByRole("menuitem", { name: "Dark" });
    await waitFor(() => expect(system).toHaveFocus());
    await userEvent.keyboard("{ArrowUp}");
    await expect(dark).toHaveFocus();
    await userEvent.keyboard("{Home}");
    await expect(system).toHaveFocus();
    await userEvent.keyboard("{End}{Enter}");
    await expect(canvas.queryByRole("menu")).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};

export const DialogAutofocus: Story = {
  render: () => (
    <Popover
      panelRole="dialog"
      ariaLabel="Search choices"
      trigger={(open, toggle) => (
        <button onClick={toggle} aria-expanded={open}>
          Search choices
        </button>
      )}
    >
      {() => (
        <label>
          Query
          <input data-popover-autofocus aria-label="Choice query" />
        </label>
      )}
    </Popover>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Search choices" }));
    await waitFor(() =>
      expect(canvas.getByRole("textbox", { name: "Choice query" })).toHaveFocus(),
    );
  },
};

export const DownwardOverflow: Story = {
  parameters: {
    layout: "fullscreen",
  },
  render: () => (
    <div className="min-h-screen p-3">
      <Popover
        trigger={(open, toggle) => (
          <button onClick={toggle} aria-expanded={open}>
            Open long menu
          </button>
        )}
      >
        {() => (
          <PopSection label="Long menu">
            {Array.from({ length: 30 }, (_, index) => (
              <PopItem
                key={index}
                title={`Overflow item ${index + 1}`}
                desc="A deliberately repeated description"
              />
            ))}
          </PopSection>
        )}
      </Popover>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Open long menu" }));
    const menu = canvas.getByRole("menu");
    await expect(menu).toHaveClass("pop-down");
    await waitFor(() =>
      expect(menu.scrollHeight).toBeGreaterThan(menu.clientHeight),
    );
  },
};

export const PopItemStateMatrix: Story = {
  parameters: {
    pseudo: {
      hover: ".story-pop-hover .pop-item",
      focusVisible: ".story-pop-focus .pop-item",
      active: ".story-pop-pressed .pop-item",
    },
  },
  render: () => (
    <Popover
      trigger={(open, toggle) => (
        <button onClick={toggle} aria-expanded={open}>
          Item states
        </button>
      )}
    >
      {() => (
        <PopSection label="States">
          <div className="story-pop-hover">
            <PopItem title="Hover row" desc="Pointer reveal" />
          </div>
          <div className="story-pop-focus">
            <PopItem title="Focus row" desc="Keyboard focus" />
          </div>
          <div className="story-pop-pressed">
            <PopItem title="Pressed row" />
          </div>
          <PopItem title="Selected row" active />
          <PopItem title="Danger row" danger />
          <PopItem title="Right detail row" right="⌘K" />
          <PopItem
            title="A very long popover row title that must truncate inside the available width"
            disabled
          />
        </PopSection>
      )}
    </Popover>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Item states" }));
    const hover = canvas.getByRole("menuitem", { name: /Hover row/ });
    await waitFor(() => expect(hover).toBeVisible());
    const selected = canvas.getByRole("menuitem", { name: /Selected row/ });
    await expect(selected.querySelector(".pop-check")).toBeVisible();
    await expect(selected).toHaveClass("active");
    await expect(selected).toHaveAttribute("aria-current", "true");
    await expect(canvas.getByRole("menuitem", { name: /Danger row/ })).toHaveClass("danger");
    await expect(canvas.getByRole("menuitem", { name: /Right detail row/ })).toHaveTextContent("⌘K");
    await expect(
      canvas.getByRole("menuitem", {
        name: /A very long popover row title/,
      }),
    ).toBeDisabled();
  },
};

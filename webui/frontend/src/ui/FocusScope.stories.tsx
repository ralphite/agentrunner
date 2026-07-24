import { useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { FocusScope } from "./FocusScope";

const meta = {
  title: "Foundations/Behavior/FocusScope",
  component: FocusScope,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof FocusScope>;

export default meta;
type Story = StoryObj<typeof meta>;

const scopeStyle = {
  display: "grid",
  gap: 12,
  minWidth: 320,
  padding: 20,
  border: "1px solid var(--line)",
  borderRadius: 12,
  background: "var(--panel)",
};

export const FirstFocusSelector: Story = {
  render: () => (
    <FocusScope style={scopeStyle} initialFocus="[data-initial]">
      <button type="button">Before</button>
      <input data-initial aria-label="Preferred field" />
    </FocusScope>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("textbox", { name: "Preferred field" }),
    ).toHaveFocus();
  },
};

function RefFocusFixture() {
  const preferred = useRef<HTMLInputElement>(null);
  return (
    <FocusScope style={scopeStyle} initialFocus={preferred}>
      <button type="button">Before</button>
      <input ref={preferred} aria-label="Ref field" />
    </FocusScope>
  );
}

export const FirstFocusRef: Story = {
  render: () => <RefFocusFixture />,
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("textbox", { name: "Ref field" }),
    ).toHaveFocus();
  },
};

export const TabAndShiftTabWrap: Story = {
  render: () => (
    <FocusScope style={scopeStyle}>
      <button type="button">First action</button>
      <button type="button">Last action</button>
    </FocusScope>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const first = canvas.getByRole("button", { name: "First action" });
    const last = canvas.getByRole("button", { name: "Last action" });
    await expect(first).toHaveFocus();

    await userEvent.tab({ shift: true });
    await expect(last).toHaveFocus();
    await userEvent.tab();
    await expect(first).toHaveFocus();
  },
};

export const Escape: Story = {
  args: {
    onEscape: fn(),
  },
  render: (args) => (
    <FocusScope {...args} style={scopeStyle}>
      <button type="button">Inside scope</button>
    </FocusScope>
  ),
  play: async ({ args }) => {
    await userEvent.keyboard("{Escape}");
    await expect(args.onEscape).toHaveBeenCalledOnce();
  },
};

function RestoreFixture() {
  const [open, setOpen] = useState(false);
  return (
    <div style={{ display: "grid", gap: 12 }}>
      <button type="button" onClick={() => setOpen(true)}>Open scope</button>
      {open && (
        <FocusScope
          style={scopeStyle}
          initialFocus="[data-close]"
          onEscape={() => setOpen(false)}
        >
          <button data-close type="button" onClick={() => setOpen(false)}>
            Close scope
          </button>
        </FocusScope>
      )}
    </div>
  );
}

export const RestoreFocusOnUnmount: Story = {
  render: () => <RestoreFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "Open scope" });
    await userEvent.click(trigger);
    await expect(canvas.getByRole("button", { name: "Close scope" })).toHaveFocus();
    await userEvent.keyboard("{Escape}");
    await expect(trigger).toHaveFocus();
  },
};

export const NoFocusableFallback: Story = {
  render: () => (
    <FocusScope style={scopeStyle} aria-label="Empty focus scope">
      <span>Nothing interactive</span>
    </FocusScope>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByLabelText("Empty focus scope"),
    ).toHaveFocus();
  },
};

export const FiltersUnavailableTargets: Story = {
  render: () => (
    <FocusScope style={scopeStyle} initialFocus="[data-disabled]">
      <button data-disabled type="button" disabled>Disabled</button>
      <button type="button" hidden>Hidden</button>
      <button type="button" tabIndex={-1}>Programmatic only</button>
      <button type="button">Available</button>
    </FocusScope>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Available" }),
    ).toHaveFocus();
  },
};

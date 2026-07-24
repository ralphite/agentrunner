import { useCallback, useRef, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { FocusScope, useFocusScope } from "./FocusScope";

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

function ResolverRootFixture() {
  const rootRef = useRef<HTMLDivElement>(null);
  const resolveRoot = useCallback(() => rootRef.current, []);
  useFocusScope(resolveRoot, {
    initialFocus: "[data-resolver-target]",
  });
  return (
    <div ref={rootRef} style={scopeStyle}>
      <button type="button">Before resolver target</button>
      <input data-resolver-target aria-label="Resolver target" />
    </div>
  );
}

export const FunctionRootResolver: Story = {
  render: () => <ResolverRootFixture />,
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("textbox", { name: "Resolver target" }),
    ).toHaveFocus();
  },
};

function TransferWithoutRestoreFixture() {
  const [open, setOpen] = useState(false);
  const destination = useRef<HTMLButtonElement>(null);
  return (
    <div style={{ display: "grid", gap: 12 }}>
      <button type="button" onClick={() => setOpen(true)}>Open transfer scope</button>
      <button ref={destination} type="button">Transfer destination</button>
      {open && (
        <FocusScope
          style={scopeStyle}
          shouldRestoreFocus={() => false}
          initialFocus="[data-transfer]"
        >
          <button
            data-transfer
            type="button"
            onClick={() => {
              destination.current?.focus();
              setOpen(false);
            }}
          >
            Transfer focus
          </button>
        </FocusScope>
      )}
    </div>
  );
}

export const SuppressedRestoreTransfer: Story = {
  render: () => <TransferWithoutRestoreFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Open transfer scope" }));
    await userEvent.click(canvas.getByRole("button", { name: "Transfer focus" }));
    await expect(
      canvas.getByRole("button", { name: "Transfer destination" }),
    ).toHaveFocus();
  },
};

function DisconnectedTriggerFallbackFixture() {
  const [open, setOpen] = useState(false);
  const [triggerVisible, setTriggerVisible] = useState(true);
  return (
    <div style={{ display: "grid", gap: 12 }}>
      {triggerVisible && (
        <button type="button" onClick={() => setOpen(true)}>Open fallback scope</button>
      )}
      <button data-focus-restore-fallback type="button">Stable fallback</button>
      {open && (
        <FocusScope
          style={scopeStyle}
          initialFocus="[data-remove-trigger]"
          onEscape={() => setOpen(false)}
        >
          <button
            data-remove-trigger
            type="button"
            onClick={() => setTriggerVisible(false)}
          >
            Remove opener
          </button>
        </FocusScope>
      )}
    </div>
  );
}

export const DisconnectedTriggerFallback: Story = {
  render: () => <DisconnectedTriggerFallbackFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Open fallback scope" }));
    await userEvent.click(canvas.getByRole("button", { name: "Remove opener" }));
    await userEvent.keyboard("{Escape}");
    await expect(
      canvas.getByRole("button", { name: "Stable fallback" }),
    ).toHaveFocus();
  },
};

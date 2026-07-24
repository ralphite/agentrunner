import { useRef } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { FindBar } from "./FindBar";

function FindBarFixture({ onClose }: { onClose: () => void }) {
  const scopeRef = useRef<HTMLDivElement>(null);
  return (
    <div className="mx-auto max-w-2xl p-4">
      <FindBar scope={() => scopeRef.current} onClose={onClose} />
      <div
        ref={scopeRef}
        aria-label="Conversation preview"
        className="mt-4 space-y-3 rounded-xl border border-line bg-panel p-4 text-sm text-ink"
      >
        <p>The agent inspected the component hierarchy.</p>
        <p>A second agent verified the keyboard flow.</p>
        <p>The final response includes agent evidence.</p>
      </div>
    </div>
  );
}

const meta = {
  title: "Components/Navigation/FindBar",
  component: FindBar,
  parameters: {
    layout: "fullscreen",
  },
  args: {
    scope: () => null,
    onClose: fn(),
  },
  render: ({ onClose }) => <FindBarFixture onClose={onClose} />,
} satisfies Meta<typeof FindBar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: {
    onClose: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByRole("textbox");

    await expect(input).toHaveFocus();
    await userEvent.type(input, "agent");
    await expect(canvas.getByText("1 / 3")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Next match" })).toBeEnabled();

    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByText("2 / 3")).toBeVisible();
    await userEvent.keyboard("{Shift>}{Enter}{/Shift}");
    await expect(canvas.getByText("1 / 3")).toBeVisible();

    await userEvent.keyboard("{Escape}");
    await expect(args.onClose).toHaveBeenCalledOnce();
  },
};

export const NoMatches: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(canvas.getByRole("textbox"), "not-in-this-conversation");
    await expect(canvas.getByText("0 / 0")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Previous match" })).toBeDisabled();
    await expect(canvas.getByRole("button", { name: "Next match" })).toBeDisabled();
  },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
  CommandPaletteItem,
  type CommandPaletteItemModel,
} from "./CommandPaletteItem";

const baseItem: CommandPaletteItemModel = {
  id: "story-command",
  label: "New session",
  group: "Commands",
  run: fn(),
};

const meta = {
  title: "Components/Navigation/Command Palette Item",
  component: CommandPaletteItem,
  args: {
    item: baseItem,
    selected: false,
    onSelect: fn(),
    onHover: fn(),
  },
  decorators: [
    (Story) => (
      <div className="cmdk max-w-xl p-3">
        <div
          className="cmdk-list"
          role="listbox"
          aria-label="Command palette results"
        >
          <Story />
        </div>
      </div>
    ),
  ],
} satisfies Meta<typeof CommandPaletteItem>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Command: Story = {};

export const SelectedCommand: Story = {
  args: {
    selected: true,
    item: {
      ...baseItem,
      label: "Open settings",
      hint: "⌘,",
    },
  },
};

export const SessionStateMatrix: Story = {
  render: (args) => (
    <>
      {[
        {
          label: "Ready without attention",
          hint: "agentrunner",
        },
        {
          label: "Unread session",
          hint: "agentrunner",
          dot: "unread",
          dotTitle: "New activity",
        },
        {
          label: "Running session",
          hint: "workspace",
          dot: "run",
          dotTitle: "Running",
        },
        {
          label: "Waiting for approval",
          hint: "workspace",
          dot: "appr",
          dotTitle: "Needs approval",
        },
        {
          label: "Unread approval keeps attention priority",
          hint: "workspace",
          dot: "appr",
          dotTitle: "Needs approval",
        },
        {
          label: "Multiple requested actions",
          hint: "workspace",
          dot: "appr",
          dotTitle: "Needs attention",
          actionCount: 3,
        },
        {
          label: "Needs recovery",
          hint: "workspace",
          dot: "stranded",
          dotTitle: "Needs recovery",
        },
        {
          label:
            "A very long session title that confirms truncation and alignment remain stable",
          hint: "a-very-long-project-name",
          dot: "crash",
          dotTitle: "Crashed",
        },
      ].map((state, index) => (
        <CommandPaletteItem
          {...args}
          key={state.label}
          item={{
            ...baseItem,
            ...state,
            id: `story-session-${index}`,
            group: "Sessions",
            session: true,
            quickNum: index + 1,
          }}
          selected={index === 1}
        />
      ))}
    </>
  ),
};

export const ScheduledRun: Story = {
  args: {
    item: {
      ...baseItem,
      label: "Nightly component audit",
      hint: "drive",
      group: "Scheduled",
    },
  },
};

export const KeyboardAndPointerSelection: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const option = canvas.getByRole("option", { name: "New session" });
    option.focus();
    await expect(option).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onSelect).toHaveBeenCalled();
    await userEvent.hover(option);
    await expect(args.onHover).toHaveBeenCalled();
  },
};

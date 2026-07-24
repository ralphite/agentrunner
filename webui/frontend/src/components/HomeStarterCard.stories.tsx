import {
  ArrowsClockwise,
  Bug,
  Hammer,
  MagnifyingGlass,
} from "@phosphor-icons/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
  HomeStarterCard,
  type HomeSuggestion,
} from "./HomeParts";

const suggestions: HomeSuggestion[] = [
  {
    key: "explore",
    tone: "blue",
    icon: <MagnifyingGlass size={16} />,
    label: "Explore and understand code",
    seed: "Explore",
    followups: [],
  },
  {
    key: "build",
    tone: "violet",
    icon: <Hammer size={16} />,
    label: "Build a new feature, app, or tool",
    seed: "Build",
    followups: [],
  },
  {
    key: "review",
    tone: "green",
    icon: <ArrowsClockwise size={16} />,
    label: "Review code and suggest changes",
    seed: "Review",
    followups: [],
  },
  {
    key: "fix",
    tone: "orange",
    icon: <Bug size={16} />,
    label: "Fix issues and failures",
    seed: "Fix",
    followups: [],
  },
];

const meta = {
  title: "Components/Home/Home Starter Card",
  component: HomeStarterCard,
  args: {
    suggestion: suggestions[0],
    onSelect: fn(),
  },
  decorators: [
    (Story) => (
      <div className="home home-empty-state min-h-48 p-6">
        <div className="home-empty-cards max-w-3xl">
          <Story />
        </div>
      </div>
    ),
  ],
} satisfies Meta<typeof HomeStarterCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Disabled: Story = {
  args: {
    disabled: true,
  },
};

export const ToneAndCopyMatrix: Story = {
  render: (args) => (
    <>
      {[
        ...suggestions,
        {
          ...suggestions[0],
          key: "long",
          tone: "teal" as const,
          label:
            "Explore a large unfamiliar repository and explain the most important architecture boundaries",
        },
      ].map((suggestion) => (
        <HomeStarterCard
          key={suggestion.key}
          {...args}
          suggestion={suggestion}
        />
      ))}
    </>
  ),
};

export const KeyboardSelection: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const card = canvas.getByRole("button", {
      name: "Explore and understand code",
    });
    card.focus();
    await expect(card).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onSelect).toHaveBeenCalledWith(args.suggestion);
  },
};

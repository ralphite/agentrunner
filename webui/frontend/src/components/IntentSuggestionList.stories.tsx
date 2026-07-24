import {
  ArrowsClockwise,
  Bug,
  Hammer,
  MagnifyingGlass,
} from "@phosphor-icons/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
  IntentSuggestionList,
  type HomeSuggestion,
} from "./HomeParts";

const suggestions: HomeSuggestion[] = [
  {
    key: "explore",
    tone: "blue",
    icon: <MagnifyingGlass size={16} />,
    label: "Explore and understand code",
    seed: "Explore",
    followups: [
      "Explore and learn how a feature works",
      "Explore implementation options for a feature",
      "Explore and compare architectural approaches",
      "Explore and document an API",
    ],
  },
  {
    key: "build",
    tone: "violet",
    icon: <Hammer size={16} />,
    label: "Build a new feature, app, or tool",
    seed: "Build",
    followups: [
      "Build a feature",
      "Build UI changes",
      "Build a prototype",
      "Build an internal tool",
    ],
  },
  {
    key: "review",
    tone: "green",
    icon: <ArrowsClockwise size={16} />,
    label: "Review code and suggest changes",
    seed: "Review",
    followups: [
      "Review my changes",
      "Review a pull request",
      "Review test coverage and add missing tests",
      "Review and refactor my code",
    ],
  },
  {
    key: "fix",
    tone: "orange",
    icon: <Bug size={16} />,
    label: "Fix issues and failures",
    seed: "Fix",
    followups: [
      "Fix a bug",
      "Fix failing tests",
      "Fix failing CI",
      "Fix merge conflicts",
    ],
  },
];

const meta = {
  title: "Components/Home/Intent Suggestion List",
  component: IntentSuggestionList,
  args: {
    suggestion: suggestions[0],
    onSelect: fn(),
  },
  decorators: [
    (Story) => (
      <div className="home home-empty-state min-h-[360px] p-6">
        <div className="home-main intent-active relative mx-auto flex min-h-[300px] w-full max-w-[720px] flex-col">
          <Story />
        </div>
      </div>
    ),
  ],
} satisfies Meta<typeof IntentSuggestionList>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Explore: Story = {};

export const Build: Story = {
  args: { suggestion: suggestions[1] },
};

export const Review: Story = {
  args: { suggestion: suggestions[2] },
};

export const Fix: Story = {
  args: { suggestion: suggestions[3] },
};

export const LongCopyAndSingleItem: Story = {
  args: {
    suggestion: {
      ...suggestions[0],
      seed: "Investigate",
      followups: [
        "Investigate a large unfamiliar repository, trace the complete request lifecycle, and document the most important architecture boundaries",
      ],
    },
  },
};

export const KeyboardSelection: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const option = canvas.getByRole("button", {
      name: "Explore and learn how a feature works",
    });
    option.focus();
    await expect(option).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onSelect).toHaveBeenCalledWith(
      "Explore and learn how a feature works",
    );
  },
};

export const ManySuggestions: Story = {
  args: {
    suggestion: {
      ...suggestions[0],
      seed: "Investigate",
      followups: Array.from(
        { length: 18 },
        (_, index) =>
          `Investigate component state ${index + 1} and document the visible outcome`,
      ),
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getAllByRole("button")).toHaveLength(18);
    const list = canvas.getByLabelText("Investigate suggestions");
    await expect(list.getBoundingClientRect().height).toBeGreaterThan(500);
  },
};

export const SemanticPseudoStates: Story = {
  parameters: {
    pseudo: {
      hover: ".home-intent-suggestion:nth-of-type(1)",
      focusVisible: ".home-intent-suggestion:nth-of-type(2)",
      active: ".home-intent-suggestion:nth-of-type(3)",
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const first = canvas.getByRole("button", {
      name: "Explore and learn how a feature works",
    });
    await waitFor(() => expect(first).toBeVisible());
    await expect(canvas.getAllByRole("button")).toHaveLength(4);
  },
};

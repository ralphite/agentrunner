import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { AskForm } from "./AskForm";

const questions = [
  {
    question: "How should the review be run?",
    options: [
      { label: "Fast review", description: "Check the highest-risk paths first." },
      { label: "Thorough review", description: "Inspect every changed component." },
    ],
  },
  {
    question: "Which evidence should be included?",
    options: [
      { label: "Browser screenshots" },
      { label: "Accessibility results" },
    ],
    multi_select: true,
    allow_free_text: true,
  },
];

const meta = {
  title: "Components/Attention/AskForm",
  component: AskForm,
  parameters: {
    layout: "centered",
  },
  args: {
    questions,
    onSubmit: fn(),
    onSkip: fn(),
  },
} satisfies Meta<typeof AskForm>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardAnswer: Story = {
  args: {
    questions: [
      {
        question: "How should the review be run?",
        options: [
          { label: "Fast review" },
          { label: "Thorough review" },
        ],
      },
    ],
    onSubmit: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Fast review" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByRole("button", { name: "Submit" })).toBeEnabled();

    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Thorough review" })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Submit" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");

    await expect(args.onSubmit).toHaveBeenCalledWith(["1:1"]);
  },
};

export const MultipleAnswers: Story = {
  args: {
    onSubmit: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: /Thorough review/ }));
    await userEvent.click(canvas.getByRole("button", { name: "Browser screenshots" }));
    await userEvent.click(canvas.getByRole("button", { name: "Accessibility results" }));

    await expect(canvas.getByRole("button", { name: "Submit" })).toBeEnabled();
    await userEvent.click(canvas.getByRole("button", { name: "Submit" }));
    await expect(args.onSubmit).toHaveBeenCalledWith(["1:2", "2:1,2"]);
  },
};

export const FreeTextOnly: Story = {
  args: {
    questions: [
      {
        question: "What should the agent verify before continuing?",
      },
    ],
    onSubmit: fn(),
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const answer = canvas.getByRole("textbox", {
      name: "What should the agent verify before continuing?",
    });
    await userEvent.type(answer, "Verify the focused Storybook interactions");
    await expect(canvas.getByRole("button", { name: "Submit" })).toBeEnabled();
    await userEvent.keyboard("{Enter}");
    await expect(args.onSubmit).toHaveBeenCalledWith([
      "1:text=Verify the focused Storybook interactions",
    ]);
  },
};

const pendingSubmit = fn(() => new Promise<void>(() => {}));

export const BusySubmitting: Story = {
  args: {
    questions: [
      {
        question: "How should the review be run?",
        options: [{ label: "Fast review" }],
      },
    ],
    onSubmit: pendingSubmit,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Fast review" }));
    await userEvent.click(canvas.getByRole("button", { name: "Submit" }));
    await waitFor(() =>
      expect(canvas.getByRole("button", { name: "Submit" })).toBeDisabled(),
    );
    await expect(canvas.getByRole("button", { name: "Skip" })).toBeDisabled();
    await expect(canvas.getByRole("button", { name: "Fast review" })).toHaveClass(
      "sel",
    );
    await expect(
      canvas.getByRole("group", { name: "Question from the agent" }),
    ).toHaveAttribute("aria-busy", "true");
  },
};

export const EmptyQuestions: Story = {
  args: {
    questions: [],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("group", { name: "Question from the agent" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Submit" })).toBeEnabled();
    await expect(canvas.queryByText("How should the review be run?")).not.toBeInTheDocument();
  },
};

export const LongQuestionAndOptions: Story = {
  args: {
    questions: [
      {
        question:
          "Which verification strategy should be used for an unusually large component migration with several independent visual and keyboard interaction surfaces?",
        options: [
          {
            label:
              "Run the focused component Stories and preserve every deterministic interaction state",
            description:
              "This deliberately long option description must wrap inside the card without widening the question surface or displacing the selection glyph.",
          },
          {
            label: "Skip visual verification",
          },
        ],
        allow_free_text: true,
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const longOption = canvas.getByRole("button", {
      name: /Run the focused component Stories/,
    });
    await expect(longOption.getBoundingClientRect().height).toBeGreaterThan(60);
    await expect(canvas.getByPlaceholderText("…or type an answer")).toBeVisible();
  },
};

export const SemanticPseudoStates: Story = {
  parameters: {
    pseudo: {
      hover: ".ask-opt:nth-of-type(1)",
      focusVisible: ".ask-opt:nth-of-type(2)",
      active: ".ask-actions .primary",
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const first = canvas.getByRole("button", { name: /Fast review/ });
    await waitFor(() => expect(first).toBeVisible());
    await userEvent.click(canvas.getByRole("button", { name: /Thorough review/ }));
    await expect(
      canvas.getByRole("button", { name: /Thorough review/ }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(canvas.getByRole("button", { name: "Submit" })).toBeDisabled();
  },
};

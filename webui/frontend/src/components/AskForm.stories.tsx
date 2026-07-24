import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
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

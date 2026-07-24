import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { Subagents, type InspectNode } from "./Subagents";

const nodes: InspectNode[] = [
  {
    call_id: "call-lead",
    agent: "lead",
    session: "session-lead",
    report: {
      status: "running",
      gen_steps: 8,
      usage: { billed: 18_400 },
      children: [
        {
          call_id: "call-reviewer",
          agent: "reviewer",
          session: "session-reviewer",
          report: {
            status: "waiting",
            waiting: {
              kind: "input",
              ask_questions: [
                {
                  question: "Which release channel?",
                  options: [{ label: "Stable" }, { label: "Beta" }],
                },
              ],
            },
          },
        },
      ],
    },
  },
  {
    call_id: "call-auditor",
    agent: "auditor",
    session: "session-auditor",
    report: {
      reason: "completed",
      gen_steps: 12,
      usage: { billed: 32_000 },
    },
  },
];

const meta = {
  title: "Components/Supervision/Subagents",
  component: Subagents,
  decorators: [
    (Story) => (
      <StoryAppFrame>
        <div className="max-w-[680px] p-4"><Story /></div>
      </StoryAppFrame>
    ),
  ],
  args: {
    nodes,
    onOpen: fn(),
  },
} satisfies Meta<typeof Subagents>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  args: { onOpen: fn() },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: /lead.*Running/i })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onOpen).toHaveBeenCalledWith("session-lead");
  },
};

export const Empty: Story = {
  args: { nodes: [] },
};

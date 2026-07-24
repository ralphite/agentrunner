import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { pacedUserEvent as userEvent } from "../storybook/humanPlayback";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { Subagents, type InspectNode } from "./Subagents";

const nodes: InspectNode[] = [
  {
    call_id: "call-lead",
    agent: "worker",
    session: "session-lead",
    report: {
      status: "running",
      gen_steps: 8,
      usage: { billed: 18_400 },
      delegations: [
        {
          call_id: "call-reviewer",
          assigned_to: "session-reviewer",
          description: "Verify keyboard navigation in the compact rail.",
        },
      ],
      children: [
        {
          call_id: "call-reviewer",
          agent: "worker",
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
    agent: "worker",
    session: "session-auditor",
    report: {
      reason: "completed",
      gen_steps: 12,
      usage: { billed: 32_000 },
    },
  },
];

const delegations = [
  {
    call_id: "call-lead",
    assigned_to: "session-lead",
    description: "Audit sidebar focus and action consistency.",
  },
  {
    call_id: "call-auditor",
    assigned_to: "session-auditor",
    description: "Compare session chrome with Codex.",
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
    delegations,
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
    await expect(
      canvas.getByRole("button", { name: /Audit sidebar focus.*worker · Running/i }),
    ).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onOpen).toHaveBeenCalledWith("session-lead");
  },
};

export const Empty: Story = {
  args: { nodes: [] },
};

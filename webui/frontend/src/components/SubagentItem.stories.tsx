import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { SubagentItem } from "./Subagents";

const meta = {
  title: "Components/Supervision/Subagent Item",
  component: SubagentItem,
  args: {
    node: {
      call_id: "call-worker",
      agent: "worker",
      session: "session-worker",
      report: {
        status: "running",
        gen_steps: 8,
        usage: { billed: 18_400 },
      },
    },
    onOpen: fn(),
  },
  decorators: [
    (Story) => (
      <div className="subagents max-w-[680px] p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SubagentItem>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {};

export const LifecycleMatrix: Story = {
  render: (args) => (
    <>
      {[
        { agent: "ready", status: "waiting" },
        { agent: "running", status: "running" },
        { agent: "completed", reason: "completed" },
        { agent: "cancelled", reason: "cancelled" },
        { agent: "crashed", reason: "crashed" },
        { agent: "stranded", reason: "stranded" },
        {
          agent: "approval",
          status: "waiting",
          waiting: { kind: "approval", tool: "bash" },
        },
        {
          agent: "answer",
          status: "waiting",
          waiting: {
            kind: "input",
            ask_questions: [{ question: "Which release channel?" }],
          },
        },
      ].map((state, index) => (
        <SubagentItem
          {...args}
          key={state.agent}
          node={{
            call_id: `call-${state.agent}`,
            agent: state.agent,
            session: `session-${state.agent}`,
            report: {
              status: state.status,
              reason: state.reason,
              waiting: state.waiting,
              gen_steps: index + 1,
              usage: { billed: (index + 1) * 12_500 },
            },
          }}
        />
      ))}
    </>
  ),
};

export const WithoutSessionOrMetrics: Story = {
  args: {
    node: {
      call_id: "call-anonymous",
      agent: "reviewer",
      report: { status: "waiting" },
    },
  },
};

export const LongIdentityAndLargeUsage: Story = {
  args: {
    node: {
      call_id: "call-long",
      agent: "accessibility-and-responsive-layout-reviewer",
      session: "session-long",
      report: {
        reason: "completed",
        gen_steps: 124,
        usage: { billed: 1_482_910 },
      },
    },
  },
};

export const NestedChildren: Story = {
  args: {
    node: {
      call_id: "call-lead",
      agent: "lead",
      session: "session-lead",
      report: {
        status: "running",
        children: [
          {
            call_id: "call-reviewer",
            agent: "reviewer",
            session: "session-reviewer",
            report: {
              reason: "completed",
              children: [
                {
                  call_id: "call-deep",
                  agent: "deep-auditor",
                  session: "session-deep",
                  report: { status: "running" },
                },
              ],
            },
          },
        ],
      },
    },
  },
};

export const KeyboardOpen: Story = {
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const row = canvas.getByRole("button", { name: /worker.*Running/i });
    row.focus();
    await expect(row).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onOpen).toHaveBeenCalledWith("session-worker");
  },
};

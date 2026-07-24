import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ApprovalCard } from "./ApprovalCard";

const meta = {
  title: "Components/Attention/ApprovalCard",
  component: ApprovalCard,
  parameters: {
    layout: "centered",
  },
  args: {
    approval: {
      id: "approval-1",
      tool: "shell",
      args: {
        command: "npm run build",
      },
      gates: [],
    },
    readonly: false,
    workspace: "/Users/demo/project",
    onDecide: fn(),
    onError: fn(),
  },
} satisfies Meta<typeof ApprovalCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Pending: Story = {};

export const DetailsOpen: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText("Details"));
    await expect(canvas.getByText(/npm run build/, { selector: "pre" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Approve once" })).toBeEnabled();
  },
};

export const KeyboardApproval: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Approve once" })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Always allow" })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Deny" })).toHaveFocus();
  },
};

export const DenyReasonOpen: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Deny" }));
    const reason = canvas.getByPlaceholderText("Reason (optional)");
    await expect(reason).toHaveFocus();
    await userEvent.type(reason, "Command changes generated files");
    await expect(canvas.getByRole("button", { name: "Cancel" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Deny" })).toBeVisible();
  },
};

export const BusyDecision: Story = {
  args: {
    onDecide: fn(
      () =>
        new Promise<void>(() => {
          // Intentionally pending so the busy controls stay observable.
        }),
    ),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Approve once" }));
    await expect(canvas.getByRole("button", { name: "Approve once" })).toBeDisabled();
    await expect(canvas.getByRole("button", { name: "Always allow" })).toBeDisabled();
    await expect(canvas.getByRole("button", { name: "Deny" })).toBeDisabled();
  },
};

export const LongContentAndGates: Story = {
  args: {
    approval: {
      id: "approval-long",
      tool: "shell",
      args: {
        command:
          "git -C /Users/demo/projects/an-exceptionally-long-workspace-name/worktrees/review-component-states commit --message 'Document every visible approval state'",
      },
      gates: [
        { gate: "workspace", decision: "ask", reason: "Directory requires explicit trust" },
        { gate: "network", decision: "deny", reason: "Outbound access is unavailable" },
      ],
      agent: "a-child-agent-with-an-exceptionally-long-name",
    },
    workspace: "/Users/demo/projects/an-exceptionally-long-workspace-name/worktrees/review-component-states",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText("Details"));
    await expect(canvas.getByText(/workspace: ask/)).toBeVisible();
    await expect(canvas.getByText(/network: deny/)).toBeVisible();
    await expect(canvas.getByText(/git -C/, { selector: "pre" })).toBeVisible();
  },
};

export const ReadonlyChild: Story = {
  args: {
    approval: {
      id: "approval-child",
      tool: "shell",
      args: {
        command: "go test ./...",
      },
      gates: [{ gate: "workspace", decision: "ask" }],
      agent: "reviewer",
    },
    readonly: true,
    workspace: "/Users/demo/project/.worktrees/reviewer",
    workspaceMode: "isolated",
  },
};

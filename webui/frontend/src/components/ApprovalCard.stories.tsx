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

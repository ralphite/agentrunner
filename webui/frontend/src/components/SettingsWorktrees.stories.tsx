import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { type AppState, useStore } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { SettingsWorktrees } from "./SettingsWorktrees";

const worktreeState = {
  sessions: [
    {
      id: "settings-story",
      status: "running",
      turns: 3,
      title: "Build Settings stories",
      workspace: "/Users/demo/agentrunner",
    },
    {
      id: "browser-qa",
      status: "completed",
      turns: 5,
      title: "Run browser QA",
      workspace: "/Users/demo/agentrunner",
    },
    {
      id: "handoff-doc",
      status: "completed",
      turns: 2,
      title: "Prepare delivery notes",
      workspace: "/Users/demo/docs",
    },
  ],
  sessionsReady: true,
  renames: {
    "browser-qa": "Verify all browser states",
  },
} satisfies Partial<AppState>;

const manyWorktreeState = {
  sessions: Array.from({ length: 41 }, (_, index) => ({
    id: `workspace-${index + 1}`,
    status: "completed",
    turns: 1,
    title: `Workspace conversation ${index + 1}`,
    workspace: `/Users/demo/projects/workspace-${String(index + 1).padStart(2, "0")}`,
  })),
  sessionsReady: true,
} satisfies Partial<AppState>;

function WorktreesStory({ query }: { query: string }) {
  const currentSid = useStore((state) => state.currentSid);
  return (
    <>
      <SettingsWorktrees query={query} />
      <output className="sr-only" aria-label="Selected session">
        {currentSid ?? "None selected"}
      </output>
    </>
  );
}

const meta = {
  title: "Components/Settings/Worktrees",
  component: SettingsWorktrees,
  args: {
    query: "",
  },
  render: (args) => (
    <StoryAppFrame initialState={worktreeState}>
      <div className="mx-auto max-w-[760px] p-6">
        <WorktreesStory {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("heading", { name: "Worktrees" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Build Settings stories" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Verify all browser states" })).toBeVisible();
  },
} satisfies Meta<typeof SettingsWorktrees>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const KeyboardNavigation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    const first = canvas.getByRole("button", { name: "Build Settings stories" });
    await expect(first).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByRole("status", { name: "Selected session" })).toHaveTextContent("settings-story");
  },
};

export const Empty: Story = {
  render: (args) => (
    <StoryAppFrame initialState={{ sessions: [], sessionsReady: true }}>
      <div className="mx-auto max-w-[760px] p-6">
        <WorktreesStory {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No session workspaces yet.")).toBeVisible();
    await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
  },
};

export const NoMatches: Story = {
  args: {
    query: "nonexistent workspace",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No worktrees match “nonexistent workspace”.")).toBeVisible();
    await expect(canvas.queryByRole("button")).not.toBeInTheDocument();
  },
};

export const FilteredResults: Story = {
  args: {
    query: "delivery notes",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Prepare delivery notes" })).toBeVisible();
    await expect(canvas.queryByRole("button", { name: "Build Settings stories" })).not.toBeInTheDocument();
    await expect(canvas.queryByText(/No worktrees match/)).not.toBeInTheDocument();
  },
};

export const PaginationCollapsed: Story = {
  render: (args) => (
    <StoryAppFrame initialState={manyWorktreeState}>
      <div className="mx-auto max-w-[760px] p-6">
        <WorktreesStory {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Show 1 more · 1 remaining" })).toBeVisible();
    await expect(canvas.queryByRole("button", { name: "Workspace conversation 41" })).not.toBeInTheDocument();
  },
};

export const Pagination: Story = {
  render: (args) => (
    <StoryAppFrame initialState={manyWorktreeState}>
      <div className="mx-auto max-w-[760px] p-6">
        <WorktreesStory {...args} />
      </div>
    </StoryAppFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const more = canvas.getByRole("button", { name: "Show 1 more · 1 remaining" });
    await expect(more).toBeVisible();
    await userEvent.click(more);
    await expect(canvas.getByRole("button", { name: "Workspace conversation 41" })).toBeVisible();
    await expect(canvas.queryByRole("button", { name: "Show 1 more · 1 remaining" })).not.toBeInTheDocument();
  },
};

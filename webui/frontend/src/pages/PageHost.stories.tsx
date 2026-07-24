import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import {
  buildInspect,
  buildRun,
  buildSession,
  buildTimeline,
} from "../storybook/fixtures";
import { createStoryApiHandlers } from "../storybook/handlers";
import { PageHost } from "./PageHost";

const session = buildSession({
  id: "story-page-session",
  title: "Review page composition",
  status: "completed",
});
const run = buildRun({
  id: "story-page-run",
  label: "Verify the page host",
  status: "running",
});
const api = createStoryApiHandlers({
  sessions: [session],
  runs: [run],
  events: { [session.id]: buildTimeline() },
  inspect: { [session.id]: buildInspect() },
  backgroundWork: { [session.id]: [] },
  queue: { [session.id]: [] },
});

const meta = {
  title: "Pages/PageHost",
  component: PageHost,
  parameters: {
    layout: "fullscreen",
    msw: { handlers: api.handlers },
  },
  decorators: [
    (Story) => (
      <StoryAppFrame
        initialState={{
          sessions: [session],
          sessionsReady: true,
          runs: [run],
          health: api.snapshot().health,
        }}
      >
        <div className="flex h-screen min-h-0 flex-col overflow-clip">
          <Story />
        </div>
      </StoryAppFrame>
    ),
  ],
  args: {
    currentRunId: null,
    currentSid: null,
    currentPage: "home",
  },
} satisfies Meta<typeof PageHost>;

export default meta;
type Story = StoryObj<typeof meta>;

export const HomeRoute: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("textbox", { name: "Message AgentRunner" })).toBeVisible();
  },
};

export const SessionRoute: Story = {
  args: {
    currentSid: session.id,
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText("Review page composition"),
    ).toBeVisible();
  },
};

export const ScheduledRoute: Story = {
  args: {
    currentPage: "scheduled",
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("heading", {
        name: "Scheduled runs",
      }),
    ).toBeVisible();
  },
};

export const RunRoute: Story = {
  args: {
    currentRunId: run.id,
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("log", { name: "Run output" }),
    ).toBeVisible();
  },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { buildRun } from "../storybook/fixtures";
import { createStoryApiHandlers } from "../storybook/handlers";
import {
  createScriptedStreamController,
  type ScriptedStreamController,
  type StreamStep,
} from "../storybook/streams";
import type { Run } from "../types";
import { RunView } from "./RunView";

const NOW = Date.parse("2026-07-23T18:00:00Z");

const storyClock: AppServices["clock"] = {
  now: () => NOW,
  setTimeout: (callback) => {
    queueMicrotask(callback);
    return 0 as unknown as ReturnType<typeof setTimeout>;
  },
  clearTimeout: () => {},
  setInterval: () => 0 as unknown as ReturnType<typeof setInterval>,
  clearInterval: () => {},
};

const driveRun = buildRun({
  id: "story-drive",
  kind: "drive",
  label: "Verify every Storybook component state",
  status: "running",
  sessionId: undefined,
});

const completedScript: StreamStep[] = [
  {
    type: "message",
    data: JSON.stringify({ kind: "session_start", session: "story-child-1" }),
  },
  {
    type: "message",
    data: JSON.stringify({ kind: "generation_start", n: 1 }),
  },
  {
    type: "message",
    data: JSON.stringify({
      kind: "tool_call",
      tool: "bash",
      args: { command: "npm run test:storybook" },
    }),
  },
  {
    type: "message",
    data: JSON.stringify({
      kind: "tool_result",
      tool: "bash",
      result: "113 stories passed",
    }),
  },
  {
    type: "message",
    data: JSON.stringify({
      kind: "session_start",
      session: "story-child-2",
    }),
  },
  {
    type: "message",
    data: JSON.stringify({
      kind: "message",
      text: "Browser coverage and keyboard checks are complete.",
    }),
  },
  {
    type: "message",
    data: JSON.stringify({ kind: "run_end", reason: "satisfied", n: 2 }),
  },
  { type: "end", data: { reason: "satisfied" } },
];

interface RunFixture {
  api: ReturnType<typeof createStoryApiHandlers>;
  controller: ScriptedStreamController;
  run: Run;
}

function makeFixture(
  run: Run = driveRun,
  script: readonly StreamStep[] = completedScript,
): RunFixture {
  return {
    api: createStoryApiHandlers({ runs: [run], sessions: [] }),
    controller: createScriptedStreamController({
      [`/api/runs/${run.id}/stream`]: script,
    }),
    run,
  };
}

function renderFixture(fixture: RunFixture) {
  return (
    <StoryAppFrame
      initialState={{ runs: [fixture.run] }}
      services={{ clock: storyClock, streams: fixture.controller }}
    >
      <div className="flex h-screen min-h-0 flex-col overflow-clip">
        <RunView runId={fixture.run.id} />
      </div>
    </StoryAppFrame>
  );
}

async function playScript(
  fixture: RunFixture,
  canvasElement: HTMLElement,
) {
  await waitFor(() => {
    expect(fixture.controller.streams()).toHaveLength(1);
  });
  fixture.controller.playAll();
  const canvas = within(canvasElement);
  const output = canvas.getByRole("log", { name: "Run output" });
  await expect(output).toBeVisible();
  await waitFor(() => {
    expect(output).not.toHaveTextContent("waiting for output…");
  });
}

const defaultFixture = makeFixture();

const meta = {
  title: "Components/Runs/RunView",
  component: RunView,
  parameters: {
    layout: "fullscreen",
    msw: { handlers: defaultFixture.api.handlers },
  },
  args: {
    runId: driveRun.id,
  },
  render: () => renderFixture(defaultFixture),
} satisfies Meta<typeof RunView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    await playScript(defaultFixture, canvasElement);
    const canvas = within(canvasElement);
    await expect(canvas.getByText("iteration 1")).toBeVisible();
    await expect(canvas.getByText("iteration 2")).toBeVisible();
    await expect(canvas.getByRole("log", { name: "Run output" }))
      .toHaveTextContent(/series\s+satisfied\s+·\s+2\s+iterations/);
  },
};

const keyboardFixture = makeFixture();
export const KeyboardNavigation: Story = {
  parameters: { msw: { handlers: keyboardFixture.api.handlers } },
  render: () => renderFixture(keyboardFixture),
  play: async ({ canvasElement }) => {
    await playScript(keyboardFixture, canvasElement);
    const canvas = within(canvasElement);
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Stop run" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() => {
      expect(canvas.queryByRole("button", { name: "Stop run" })).toBeNull();
    });
  },
};

const waitingFixture = makeFixture(
  buildRun({
    id: "story-waiting",
    label: "Waiting for the runner to start",
    status: "running",
    sessionId: undefined,
  }),
  [],
);
export const WaitingForOutput: Story = {
  parameters: { msw: { handlers: waitingFixture.api.handlers } },
  render: () => renderFixture(waitingFixture),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByText("waiting for output…")).toBeVisible();
  },
};

const failedFixture = makeFixture(
  buildRun({
    id: "story-failed",
    kind: "drive",
    label: "Cross-browser verification",
    status: "failed",
    sessionId: undefined,
  }),
  [
    {
      type: "message",
      data: JSON.stringify({ kind: "session_start", session: "failed-child" }),
    },
    {
      type: "message",
      data: "driver stalled: 3 iterations (best 2)",
    },
  ],
);
export const FailedVerdict: Story = {
  parameters: { msw: { handlers: failedFixture.api.handlers } },
  render: () => renderFixture(failedFixture),
  play: async ({ canvasElement }) => {
    await playScript(failedFixture, canvasElement);
    await expect(within(canvasElement).getByRole("log", { name: "Run output" }))
      .toHaveTextContent(/series\s+stalled\s+·\s+3\s+iterations/);
  },
};

const completedFixture = makeFixture(
  buildRun({
    id: "story-completed",
    label: "One-time component audit",
    status: "done",
    sessionId: undefined,
  }),
  [
    {
      type: "message",
      data: JSON.stringify({
        kind: "message",
        text: "No regressions found.",
      }),
    },
    {
      type: "message",
      data: JSON.stringify({ kind: "end", status: "completed" }),
    },
  ],
);
export const CompletedOneTimeRun: Story = {
  parameters: { msw: { handlers: completedFixture.api.handlers } },
  render: () => renderFixture(completedFixture),
  play: async ({ canvasElement }) => {
    await playScript(completedFixture, canvasElement);
    const canvas = within(canvasElement);
    await expect(canvas.getByText("No regressions found.")).toBeVisible();
    await expect(canvas.queryByRole("button", { name: "Stop run" })).toBeNull();
  },
};

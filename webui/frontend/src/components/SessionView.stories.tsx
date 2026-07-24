import type { Meta, StoryObj } from "@storybook/react-vite";
import { delay, HttpResponse, http } from "msw";
import { useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { AppServices } from "../app/appServices";
import type { AppState } from "../store";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import {
  buildAssistantMessage,
  buildEnvelope,
  buildInputReceived,
  buildInspect,
  buildSession,
  buildTimeline,
  fixtureDefaults,
  type StoryInspectFixture,
} from "../storybook/fixtures";
import {
  createStoryApiHandlers,
  type StoryQueuedMessage,
} from "../storybook/handlers";
import type { Envelope, Session } from "../types";
import type { GoalDerived } from "../timeline";
import {
  GoalBanner as GoalBannerView,
  ProgressSummary as ProgressSummaryView,
  SessionView,
} from "./SessionView";

const NOW = Date.parse("2026-07-23T18:00:00Z");
const DEFAULT_SID = "20260723-180000-story-session";

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

const defaultSession = buildSession({
  id: DEFAULT_SID,
  title: "Build deterministic Storybook coverage",
  status: "completed",
  turns: 2,
  workspace: fixtureDefaults.workspace,
});

interface SessionFixture {
  api: ReturnType<typeof createStoryApiHandlers>;
  session: Session;
  initialState: Partial<AppState>;
}

function makeFixture(options: {
  session?: Session;
  events?: Envelope[];
  inspect?: StoryInspectFixture;
  queue?: StoryQueuedMessage[];
  includeApiSession?: boolean;
} = {}): SessionFixture {
  const session = options.session ?? defaultSession;
  const events = options.events ?? buildTimeline();
  const inspect = options.inspect ?? buildInspect();
  const includeApiSession = options.includeApiSession ?? true;
  const api = createStoryApiHandlers({
    sessions: includeApiSession ? [session] : [],
    runs: [],
    events: includeApiSession ? { [session.id]: events } : {},
    inspect: includeApiSession ? { [session.id]: inspect } : {},
    backgroundWork: includeApiSession ? { [session.id]: [] } : {},
    queue: includeApiSession ? { [session.id]: options.queue ?? [] } : {},
  });
  return {
    api,
    session,
    initialState: {
      sessions: [session],
      sessionsReady: true,
      currentSid: session.id,
      health: {
        version: "storybook",
        daemonUp: true,
        daemonManaged: true,
        daemonExternal: false,
        manageRequested: false,
        daemonLogPath: "/runtime/storybook/agentrunner.log",
        runtimeDir: "/runtime/storybook",
      },
    },
  };
}

function renderFixture(fixture: SessionFixture) {
  return (
    <StoryAppFrame
      initialState={fixture.initialState}
      services={{
        clock: storyClock,
        local: { "arwebui.supervision": "0" },
      }}
    >
      <div className="h-screen min-h-[660px]">
        <SessionView sid={fixture.session.id} />
      </div>
    </StoryAppFrame>
  );
}

const defaultFixture = makeFixture();
const meta = {
  title: "Components/Sessions/SessionView",
  component: SessionView,
  parameters: {
    layout: "fullscreen",
    msw: { handlers: defaultFixture.api.handlers },
  },
  args: {
    sid: DEFAULT_SID,
  },
  render: () => renderFixture(defaultFixture),
} satisfies Meta<typeof SessionView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText("Build deterministic Storybook coverage"),
    ).toBeVisible();
    await expect(
      await canvas.findByText("Show the reusable component states."),
    ).toBeVisible();
    await expect(
      await canvas.findByText("The component states are ready for review."),
    ).toBeVisible();
  },
};

const keyboardFixture = makeFixture();
export const KeyboardNavigation: Story = {
  parameters: { msw: { handlers: keyboardFixture.api.handlers } },
  render: () => renderFixture(keyboardFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const opener = await canvas.findByRole("button", { name: "More session actions" });
    opener.focus();
    await userEvent.keyboard("{Control>}f{/Control}");
    const search = canvas.getByRole("textbox", { name: "Search conversation" });
    await expect(search).toHaveFocus();
    await userEvent.type(search, "component");
    await userEvent.keyboard("{Escape}");
    await expect(opener).toHaveFocus();
  },
};

const loadingFixture = makeFixture({
  session: buildSession({
    id: "20260723-180100-story-loading",
    title: "Load a long session journal",
    status: "waiting:input",
  }),
  events: [],
});
export const Loading: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get("/api/sessions/:sid/events", async () => {
          await delay("infinite");
          return HttpResponse.json([]);
        }),
        ...loadingFixture.api.handlers,
      ],
    },
  },
  render: () => renderFixture(loadingFixture),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("status", { name: "Loading conversation" }),
    ).toBeVisible();
  },
};

const emptyFixture = makeFixture({
  session: buildSession({
    id: "20260723-180200-story-empty",
    title: "A fresh empty session",
    status: "waiting:input",
    turns: 0,
  }),
  events: [],
  inspect: buildInspect({ progress: [] }),
});
export const Empty: Story = {
  parameters: { msw: { handlers: emptyFixture.api.handlers } },
  render: () => renderFixture(emptyFixture),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText("No messages yet"),
    ).toBeVisible();
  },
};

const approvalSession = buildSession({
  id: "20260723-180300-story-approval",
  title: "Approve the Storybook snapshot update",
  status: "waiting:approval",
  attention: { approvals: 1, answers: 0 },
});
const approvalEvents: Envelope[] = [
  buildEnvelope({
    seq: 1,
    type: "session_started",
    ts: "2026-07-23T17:59:50Z",
    payload: { spec_name: "storybook", model: "fixture-model" },
  }),
  buildInputReceived({
    seq: 2,
    ts: "2026-07-23T17:59:51Z",
    payload: {
      source: "user",
      text: "Update the approved visual snapshots.",
      item_id: "approval-user",
      turn_id: "approval-turn",
    },
  }),
  buildEnvelope({
    seq: 3,
    type: "generation_started",
    ts: "2026-07-23T17:59:52Z",
    payload: { gen_step: 1 },
  }),
  buildAssistantMessage({
    seq: 4,
    ts: "2026-07-23T17:59:53Z",
    payload: {
      item_id: "approval-assistant",
      turn_id: "approval-turn",
      message: {
        parts: [{
          tool_name: "shell",
          call_id: "story-tool-approval",
          args: { command: "npm run test:visual:update" },
        }],
      },
    },
  }),
  buildEnvelope({
    seq: 5,
    type: "approval_requested",
    ts: "2026-07-23T17:59:54Z",
    payload: {
      approval_id: "story-approval",
      call_id: "story-tool-approval",
      gate_results: [{
        gate: "workspace",
        allowed: false,
        reason: "Review the fixture-only snapshot update.",
      }],
    },
  }),
  buildEnvelope({
    seq: 6,
    type: "waiting_entered",
    ts: "2026-07-23T17:59:55Z",
    payload: { kind: "approval" },
  }),
];
const approvalFixture = makeFixture({
  session: approvalSession,
  events: approvalEvents,
  inspect: buildInspect({ progress: [] }),
});
export const ApprovalRequired: Story = {
  parameters: { msw: { handlers: approvalFixture.api.handlers } },
  render: () => renderFixture(approvalFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const approval = await canvas.findByRole("region", { name: "Approval required" });
    await expect(approval).toBeVisible();
    await expect(canvas.getByText("npm run test:visual:update")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Approve once" })).toBeEnabled();
  },
};

const askSession = buildSession({
  id: "20260723-180400-story-ask",
  title: "Choose the release evidence",
  status: "waiting:input",
  attention: { approvals: 0, answers: 1 },
});
const askFixture = makeFixture({
  session: askSession,
  inspect: buildInspect({
    progress: [],
    waiting: {
      kind: "input",
      ask_questions: [
        {
          question: "Which evidence should be included?",
          options: [
            { label: "Browser screenshots", description: "Attach the visual proof." },
            { label: "Accessibility report", description: "Attach the axe results." },
          ],
          multi_select: true,
          allow_free_text: true,
        },
      ],
    },
  }),
});
export const StructuredAnswerRequired: Story = {
  parameters: { msw: { handlers: askFixture.api.handlers } },
  render: () => renderFixture(askFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const screenshot = await canvas.findByRole("button", { name: /Browser screenshots/ });
    await userEvent.click(screenshot);
    await expect(screenshot).toHaveClass("sel");
    await expect(canvas.getByRole("button", { name: "Submit" })).toBeEnabled();
  },
};

const goalSession = buildSession({
  id: "20260723-180500-story-goal",
  title: "Finish the component-system migration",
  status: "running",
  turns: 3,
});
const goalEvents = [
  ...buildTimeline(),
  buildEnvelope({
    seq: 8,
    type: "goal_attached",
    ts: "2026-07-23T17:55:00Z",
    payload: {
      goal: "Complete every Storybook state and preserve browser evidence.",
      max_checks: 5,
    },
  }),
  buildEnvelope({
    seq: 9,
    type: "goal_checkpoint",
    ts: "2026-07-23T17:58:00Z",
    payload: { check: 1, pass: false },
  }),
];
const goalFixture = makeFixture({
  session: goalSession,
  events: goalEvents,
  inspect: {
    ...buildInspect({
      goal: {
        goal: "Complete every Storybook state and preserve browser evidence.",
        checks: 1,
        max_checks: 5,
        paused: false,
      },
      progress: [],
    }),
    progress: [
      { id: "stories", title: "Build component stories", status: "done" },
      { id: "keyboard", title: "Verify keyboard interactions", status: "running" },
      { id: "evidence", title: "Capture browser evidence", status: "pending" },
    ],
  } as unknown as StoryInspectFixture,
});
export const GoalAndProgress: Story = {
  parameters: { msw: { handlers: goalFixture.api.handlers } },
  render: () => renderFixture(goalFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Pursuing goal")).toBeVisible();
    await expect(await canvas.findByText("1/3")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Pause goal" })).toBeEnabled();
    await expect(canvas.getByRole("button", { name: "Open goal details" })).toBeEnabled();
  },
};

const failedSession = buildSession({
  id: "20260723-180600-story-failed",
  title: "Recover a provider failure",
  status: "failed",
  turns: 1,
});
const failedFixture = makeFixture({
  session: failedSession,
  events: [
    buildInputReceived({
      seq: 1,
      payload: {
        source: "user",
        text: "Verify the final Storybook build.",
        item_id: "failed-user",
        turn_id: "failed-turn",
      },
    }),
    buildEnvelope({
      seq: 2,
      type: "activity_started",
      payload: {
        activity_id: "llm-story",
        kind: "llm",
        name: "complete",
        attempt: 1,
      },
    }),
    buildEnvelope({
      seq: 3,
      type: "activity_failed",
      payload: {
        activity_id: "llm-story",
        attempt: 1,
        error: {
          class: "provider_server",
          message: "503 fixture provider unavailable",
          retryable: true,
        },
      },
    }),
  ],
  inspect: buildInspect({ progress: [] }),
});
export const ProviderFailure: Story = {
  parameters: { msw: { handlers: failedFixture.api.handlers } },
  render: () => renderFixture(failedFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText("The model provider had a server error"),
    ).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "Technical details" }));
    await expect(
      canvasElement.querySelector(".turn-error-raw"),
    ).toHaveTextContent("503 fixture provider unavailable");
  },
};

const terminalSession = buildSession({
  id: "20260723-180650-story-terminal",
  title: "Continue after the generation-step limit",
  status: "max_generation_steps",
  turns: 2,
});
const terminalFixture = makeFixture({
  session: terminalSession,
  inspect: buildInspect({ progress: [] }),
});
export const TerminalLimit: Story = {
  parameters: { msw: { handlers: terminalFixture.api.handlers } },
  render: () => renderFixture(terminalFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const alert = await canvas.findByRole("alert");
    await expect(alert).toHaveTextContent("Step limit reached");
    await expect(
      canvas.getByRole("button", { name: "Continue in new session" }),
    ).toBeEnabled();
    await expect(canvas.queryByText("Goal cancelled")).toBeNull();
  },
};

const queuedSession = buildSession({
  id: "20260723-180675-story-queued",
  title: "Review queued follow-up messages",
  status: "running",
  turns: 2,
});
const queuedFixture = makeFixture({
  session: queuedSession,
  queue: [
    {
      command_id: "queued-session-plain",
      text: "Run the final integration review after the current turn.",
      revoked: false,
    },
    {
      command_id: "queued-session-peer",
      text: "[message from reviewer (20260723-parent-sub-call_reviewer)] Check the SessionView production integration.",
      revoked: false,
    },
    {
      command_id: "queued-session-revoked",
      text: "This revoked integration message must stay hidden.",
      revoked: true,
    },
  ],
});
export const QueuedMessages: Story = {
  parameters: { msw: { handlers: queuedFixture.api.handlers } },
  render: () => renderFixture(queuedFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText("Queued · from reviewer"),
    ).toBeVisible();
    await expect(
      canvas.getByText("Check the SessionView production integration."),
    ).toBeVisible();
    await expect(
      canvas.queryByText("This revoked integration message must stay hidden."),
    ).toBeNull();
    await expect(
      canvas.getAllByRole("button", { name: "Withdraw" }),
    ).toHaveLength(2);
  },
};

const missingFixture = makeFixture({
  session: buildSession({
    id: "20260723-180700-story-missing",
    title: "Missing session",
    status: "completed",
  }),
  includeApiSession: false,
});
export const NotFound: Story = {
  parameters: { msw: { handlers: missingFixture.api.handlers } },
  render: () => renderFixture(missingFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Session not found")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Back to all sessions" })).toBeVisible();
  },
};

const transientErrorFixture = makeFixture({
  session: buildSession({
    id: "20260723-180800-story-transient",
    title: "Keep the session visible during a transient error",
    status: "waiting:input",
  }),
  events: [],
  inspect: buildInspect({ progress: [] }),
});
export const TransientPollError: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get("/api/sessions/:sid/events", () =>
          HttpResponse.json(
            { error: "Fixture daemon is restarting." },
            { status: 503 },
          )),
        ...transientErrorFixture.api.handlers,
      ],
    },
  },
  render: () => renderFixture(transientErrorFixture),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() => {
      expect(canvas.queryByRole("status", { name: "Loading conversation" })).toBeNull();
    });
    await expect(canvas.getByText("No messages yet")).toBeVisible();
    await expect(canvas.queryByText("Session not found")).toBeNull();
  },
};

function LeafFrame({ children }: { children: React.ReactNode }) {
  return (
    <StoryAppFrame>
      <div className="mx-auto max-w-[760px] p-6">{children}</div>
    </StoryAppFrame>
  );
}

const openProgress = fn();

export const ProgressSummary: Story = {
  render: () => (
    <LeafFrame>
      <ProgressSummaryView
        progress={[
          { id: "fixtures", title: "Build deterministic fixtures", status: "done" },
          { id: "browser", title: "Verify browser interactions", status: "running" },
          { id: "evidence", title: "Capture final evidence", status: "pending" },
        ]}
        onOpenDetails={openProgress}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const summary = canvas.getByRole("button", { name: "Open progress details" });
    summary.focus();
    await expect(summary).toHaveAttribute(
      "title",
      "1/3 complete · Verify browser interactions",
    );
    await userEvent.keyboard("{Enter}");
    await expect(openProgress).toHaveBeenCalled();
  },
};

const goalSave = fn();
const goalAction = fn();

function GoalBannerFixture() {
  const [state, setState] = useState<GoalDerived>({
    phase: "active",
    goal: "Complete every Storybook state",
    checks: 2,
    maxChecks: 6,
  });
  const [editing, setEditing] = useState<string | null>(null);
  return (
    <LeafFrame>
      <GoalBannerView
        state={state}
        elapsedMs={93_000}
        editing={editing}
        updatePending={false}
        onEditStart={() => setEditing(state.goal)}
        onEditChange={setEditing}
        onSave={() => {
          goalSave(editing);
          if (editing) setState((current) => ({ ...current, goal: editing }));
          setEditing(null);
        }}
        onDiscard={() => setEditing(null)}
        onAction={(action) => {
          goalAction(action);
          if (action === "pause") {
            setState((current) => ({ ...current, phase: "paused" }));
          }
        }}
        onOpenDetails={fn()}
        onDismiss={fn()}
      />
    </LeafFrame>
  );
}

export const GoalBanner: Story = {
  render: () => <GoalBannerFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Edit goal" }));
    const input = canvas.getByRole("textbox", { name: "Goal" });
    await userEvent.clear(input);
    await userEvent.type(input, "Complete and verify every Storybook state{Enter}");
    await expect(goalSave).toHaveBeenCalledWith(
      "Complete and verify every Storybook state",
    );
    await userEvent.click(canvas.getByRole("button", { name: "Pause goal" }));
    await expect(goalAction).toHaveBeenCalledWith("pause");
    await expect(canvas.getByText("Goal paused")).toBeVisible();
  },
};

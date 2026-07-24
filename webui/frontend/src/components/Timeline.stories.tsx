import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import { createStoryApiHandlers } from "../storybook/handlers";
import type {
  BubbleItem,
  FoldRun,
  RetriedItem,
  TimelineItem,
  ToolItem,
  TurnItem,
  WorkFold,
} from "../timeline";
import {
  ActivityGroup as ActivityGroupLeaf,
  AskDetailView as AskDetailViewLeaf,
  CollapsibleUserText as CollapsibleUserTextLeaf,
  EditDetailView as EditDetailViewLeaf,
  GlobDetailView as GlobDetailViewLeaf,
  GrepDetailView as GrepDetailViewLeaf,
  Item as ItemLeaf,
  JSONDetail as JSONDetailLeaf,
  MiniDiff as MiniDiffLeaf,
  MsgActions as MsgActionsLeaf,
  ReadDetailView as ReadDetailViewLeaf,
  RetriedFold as RetriedFoldLeaf,
  SemanticDetailView as SemanticDetailViewLeaf,
  ShellDetail as ShellDetailLeaf,
  SpawnDetailView as SpawnDetailViewLeaf,
  Thumbs as ThumbsLeaf,
  TimelineView,
  ToolCard as ToolCardLeaf,
  ToolDetail as ToolDetailLeaf,
  WebDetailView as WebDetailViewLeaf,
  WorkedFold as WorkedFoldLeaf,
} from "./Timeline";

const at = (seconds: number) =>
  new Date(Date.UTC(2026, 6, 23, 18, 0, seconds)).toISOString();

const user = (key: string, text: string, seconds: number): BubbleItem => ({
  kind: "user",
  key,
  text,
  ts: at(seconds),
  source: "you",
});

const assistant = (
  key: string,
  text: string,
  seconds: number,
): BubbleItem => ({
  kind: "assistant",
  key,
  text,
  ts: at(seconds),
});

const turn = (key: string, seconds: number): TurnItem => ({
  kind: "turn",
  key,
  gen: 1,
  ts: at(seconds),
});

const tool = (
  key: string,
  name: string,
  args: unknown,
  seconds: number,
  over: Partial<ToolItem> = {},
): ToolItem => ({
  kind: "tool",
  key,
  name,
  args,
  background: false,
  status: "done",
  statusText: "done",
  ts: at(seconds),
  endTs: at(seconds + 1),
  result: { stdout: "Completed without errors", exit_code: 0 },
  ...over,
});

const completedConversation: TimelineItem[] = [
  user(
    "user-1",
    "Build a deterministic Storybook demo and verify every user-visible state.",
    0,
  ),
  turn("turn-1", 1),
  tool(
    "read-1",
    "Read",
    { file_path: "src/components/SessionView.tsx" },
    2,
  ),
  {
    kind: "chip",
    key: "chip-1",
    text: "Context compacted",
    tone: "",
    fold: true,
    activity: true,
  },
  assistant(
    "planning-1",
    "I’m checking the existing component boundaries before changing the demo.",
    4,
  ),
  tool(
    "shell-1",
    "Bash",
    {
      command:
        "npm run test:storybook -- --testNamePattern TimelineView && npm run build-storybook",
    },
    5,
    {
      result: {
        stdout: "14 stories passed\nProduction bundle remains isolated",
        exit_code: 0,
      },
    },
  ),
  assistant(
    "assistant-1",
    "The playback path is deterministic, keyboard reachable, and isolated from the production API.\n\n- **State:** completed\n- **Evidence:** Story browser interactions passed\n- **Next:** review the rendered diff",
    9,
  ),
];

const keyboardConversation: TimelineItem[] = [
  turn("keyboard-turn", 20),
  tool(
    "keyboard-tool",
    "Read",
    { file_path: "src/storybook/scenarios/ScenarioRunner.ts" },
    21,
  ),
  assistant("keyboard-answer", "The keyboard flow is ready.", 24),
];

const failureConversation: TimelineItem[] = [
  user(
    "failure-user",
    "Inspect a provider failure with a very long technical payload.",
    30,
  ),
  tool(
    "failure-tool",
    "future_provider_diagnostic",
    {
      request:
        "unknown-operation-" +
        "x".repeat(180),
    },
    31,
    {
      status: "failed",
      statusText: "failed",
      errorMsg:
        "unknown backend state: provider returned a payload that the current UI has never classified; preserve this detail instead of silently dropping it",
      result: {
        stderr:
          "The raw failure remains available for diagnosis. " + "trace ".repeat(40),
        exit_code: 73,
      },
    },
  ),
  {
    kind: "chip",
    key: "unknown-chip",
    text: "Unknown runtime state · future_provider_diagnostic",
    tone: "bad",
  },
];

const leafTools = {
  read: tool(
    "leaf-read",
    "read_file",
    { path: "src/components/Timeline.tsx", line_range: [323, 341] },
    50,
    {
      result: {
        content: "export function MiniDiff() {}\nexport function ReadDetailView() {}\n",
        truncated: true,
      },
    },
  ),
  edit: tool(
    "leaf-edit",
    "edit_file",
    {
      path: "src/components/Timeline.tsx",
      old: "function MiniDiff() {\n  return null;\n}",
      new: "export function MiniDiff() {\n  return <div />;\n}",
    },
    51,
    { result: { output: "Updated 3 lines" } },
  ),
  grep: tool(
    "leaf-grep",
    "grep",
    { pattern: "function .*Detail", path: "src/components" },
    52,
    {
      result: {
        files_scanned: 42,
        truncated: true,
        matches: [
          {
            path: "src/components/Timeline.tsx",
            line: 341,
            text: "export function ReadDetailView",
          },
          {
            path: "src/components/Timeline.tsx",
            line: 360,
            text: "export function EditDetailView",
          },
        ],
      },
    },
  ),
  glob: tool(
    "leaf-glob",
    "glob",
    { pattern: "src/components/**/*.stories.tsx" },
    53,
    {
      result: {
        paths: [
          "src/components/Timeline.stories.tsx",
          "src/components/Markdown.stories.tsx",
          "src/components/ChangesOutcome.stories.tsx",
        ],
        truncated: false,
      },
    },
  ),
  semantic: tool(
    "leaf-semantic",
    "keyword_search",
    { query: "keyboard accessible disclosure" },
    54,
    {
      result: {
        hits: [
          { path: "src/components/Timeline.tsx", line: 785 },
          { path: "src/components/Lightbox.tsx", line: 48 },
        ],
      },
    },
  ),
  spawn: tool(
    "leaf-spawn",
    "spawn_agent",
    {
      agent: "reviewer",
      prompt: "Review the Timeline leaf stories for direct coverage.",
    },
    55,
    {
      result: {
        child_session: "story-child-session",
        reason: "Independent accessibility review",
      },
    },
  ),
  web: tool(
    "leaf-web",
    "web_fetch",
    { url: "https://example.com/storybook-guidance" },
    56,
    {
      result: {
        title: "Storybook guidance",
        content: "Deterministic component examples",
        untrusted: true,
      },
    },
  ),
  ask: tool(
    "leaf-ask",
    "ask_user",
    { question: "Should the demo continue automatically?" },
    57,
  ),
  shell: tool(
    "leaf-shell",
    "bash",
    { command: "npm run test:storybook\nnpm run test:visual" },
    58,
    {
      status: "failed",
      statusText: "failed",
      errorMsg: "Chromium reported one accessibility regression.",
      result: {
        stdout: "112 stories passed",
        stderr: "1 story failed accessibility checks",
        exit_code: 1,
      },
    },
  ),
  unknown: tool(
    "leaf-unknown",
    "future_provider_diagnostic",
    {
      operation: "classify-next-runtime-state",
      payload: { state: "future_state", retryable: null },
    },
    59,
    {
      status: "failed",
      statusText: "failed",
      errorMsg: "Unknown provider state was preserved for diagnosis.",
      partial: "Partial provider payload",
      result: {
        state: "future_state",
        trace_id: "trace-story-unknown",
      },
    },
  ),
} satisfies Record<string, ToolItem>;

const leafActivityRun: FoldRun = {
  key: "leaf-activity-run",
  tools: [leafTools.read, leafTools.edit],
  members: [
    leafTools.read,
    assistant(
      "leaf-activity-prose",
      "I found the component boundary and am exporting it without changing behavior.",
      60,
    ),
    leafTools.edit,
  ],
};

const leafWorkFold: WorkFold = {
  kind: "fold",
  key: "leaf-work-fold",
  durationMs: 8_000,
  children: [
    leafTools.read,
    assistant(
      "leaf-work-prose",
      "The direct story now exercises the real leaf component.",
      61,
    ),
    leafTools.edit,
  ],
};

const leafRetriedFold: RetriedItem = {
  kind: "retried",
  key: "leaf-retried-fold",
  children: [
    user("leaf-retry-user", "Try the browser check again.", 62),
    turn("leaf-retry-turn", 63),
    leafTools.unknown,
  ],
};

const timelineApi = createStoryApiHandlers();

function LeafFrame({
  children,
  width,
}: {
  children: ReactNode;
  width?: number;
}) {
  return (
    <StoryAppFrame>
      <div className="session-view min-h-[360px] bg-canvas">
        <main className="session-primary">
          <div className="timeline min-h-[360px] overflow-visible">
            <div
              className="tl-inner py-8"
              style={width ? { width, maxWidth: "100%" } : undefined}
            >
              {children}
            </div>
          </div>
        </main>
      </div>
    </StoryAppFrame>
  );
}

function ControlledWorkedFold() {
  const [open, setOpen] = useState(false);
  return (
    <WorkedFoldLeaf
      fold={leafWorkFold}
      open={open}
      onToggle={() => setOpen((value) => !value)}
    />
  );
}

function ControlledRetriedFold() {
  const [open, setOpen] = useState(false);
  return (
    <RetriedFoldLeaf
      fold={leafRetriedFold}
      open={open}
      onToggle={() => setOpen((value) => !value)}
    />
  );
}

function TimelineFixture({
  items = completedConversation,
  pending = [],
  typing = "",
  active = false,
  showSys = false,
  onContinue,
}: {
  items?: TimelineItem[];
  pending?: React.ComponentProps<typeof TimelineView>["pending"];
  typing?: string;
  active?: boolean;
  showSys?: boolean;
  onContinue?: React.ComponentProps<typeof TimelineView>["onContinue"];
}) {
  return (
    <StoryAppFrame>
      <div className="session-view h-screen min-h-0 overflow-clip">
        <main className="session-primary">
          <TimelineView
            sessionKey="story-timeline"
            items={items}
            pending={pending}
            typing={typing}
            showSys={showSys}
            active={active}
            onContinue={onContinue}
            outcomeSlot={
              <div className="changes-outcome mx-auto mt-3 w-full max-w-[660px] text-sm text-dim">
                6 files changed · review available
              </div>
            }
            goalVerdict={{ elapsed: "9s" }}
          />
        </main>
      </div>
    </StoryAppFrame>
  );
}

const meta = {
  title: "Components/Timeline/TimelineView",
  component: TimelineView,
  parameters: {
    layout: "fullscreen",
    msw: {
      handlers: timelineApi.handlers,
    },
  },
  args: {
    sessionKey: "story-timeline",
    items: completedConversation,
    pending: [],
    typing: "",
    showSys: false,
    active: false,
    onContinue: fn(async () => {}),
  },
  render: (args) => <TimelineFixture {...args} />,
} satisfies Meta<typeof TimelineView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(
        "Build a deterministic Storybook demo and verify every user-visible state.",
      ),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: /Worked for 8s/ }),
    ).toBeVisible();
    await expect(canvas.getByText("Goal achieved in 9s")).toBeVisible();
  },
};

export const KeyboardNavigation: Story = {
  render: (args) => (
    <TimelineFixture
      {...args}
      items={keyboardConversation}
      showSys={false}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const worked = canvas.getByRole("button", { name: /Worked for 4s/ });
    (canvasElement.ownerDocument.activeElement as HTMLElement | null)?.blur();

    await userEvent.tab();
    await expect(worked).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(worked).toHaveAttribute("aria-expanded", "true");
    await expect(canvas.getByText("Ran a tool")).toBeVisible();
  },
};

export const ActiveStreaming: Story = {
  render: (args) => (
    <TimelineFixture
      {...args}
      items={[
        user("active-user", "Run the browser checks.", 40),
        turn("active-turn", 41),
        tool(
          "active-tool",
          "Bash",
          { command: "npm run test:storybook" },
          42,
          {
            status: "running",
            statusText: "running",
            result: undefined,
          },
        ),
      ]}
      pending={[
        {
          id: 1,
          text: "Also verify the phone viewport.",
          imgs: [],
          files: 0,
          delivery: "steer",
        },
        {
          id: 2,
          text: "Capture any accessibility regressions.",
          imgs: [],
          files: 1,
          delivery: "queue",
        },
      ]}
      typing="Thinking"
      active
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("status", { name: "Thinking" })).toBeVisible();
    await expect(canvas.getByText("steering…")).toBeVisible();
    await expect(canvas.getByText("queued…")).toBeVisible();
  },
};

export const FailureAndOverflow: Story = {
  render: (args) => (
    <TimelineFixture
      {...args}
      items={failureConversation}
      showSys
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("future_provider_diagnostic", { exact: false }),
    ).toBeVisible();
    await expect(
      canvas.getByText(
        "Unknown runtime state · future_provider_diagnostic",
      ),
    ).toBeVisible();
  },
};

export const ActivityGroup: Story = {
  render: () => (
    <LeafFrame>
      <ActivityGroupLeaf run={leafActivityRun} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const summary = canvas
      .getByText("Read files, edited files")
      .closest("summary") as HTMLElement;
    await expect(summary).toBeVisible();
    summary.focus();
    await userEvent.click(summary);
    await expect(summary.parentElement).toHaveAttribute("open");
    await expect(
      canvas.getByText(
        "I found the component boundary and am exporting it without changing behavior.",
      ),
    ).toBeVisible();
  },
};

export const AskDetailView: Story = {
  render: () => (
    <LeafFrame>
      <AskDetailViewLeaf t={leafTools.ask} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(
        "Should the demo continue automatically?",
      ),
    ).toBeVisible();
  },
};

export const CollapsibleUserText: Story = {
  render: () => (
    <LeafFrame width={300}>
      <div className="msg user">
        <div className="bubble">
          <CollapsibleUserTextLeaf
            text={Array.from(
              { length: 24 },
              (_, index) =>
                `Line ${index + 1}: deterministic long-form user context stays available in full.`,
            ).join("\n")}
          />
        </div>
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Show more" }),
      ).toBeVisible(),
    );
    const toggle = canvas.getByRole("button", { name: "Show more" });
    toggle.focus();
    await userEvent.keyboard(" ");
    await expect(
      canvas.getByRole("button", { name: "Show less" }),
    ).toHaveFocus();
  },
};

export const EditDetailView: Story = {
  render: () => (
    <LeafFrame>
      <EditDetailViewLeaf t={leafTools.edit} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("src/components/Timeline.tsx"),
    ).toBeVisible();
    await expect(canvas.getAllByText("+")[0]).toBeVisible();
    await expect(canvas.getByText("Updated 3 lines")).toBeVisible();
  },
};

export const GlobDetailView: Story = {
  render: () => (
    <LeafFrame>
      <GlobDetailViewLeaf t={leafTools.glob} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("3 paths")).toBeVisible();
    await expect(
      canvas.getByText("src/components/Timeline.stories.tsx"),
    ).toBeVisible();
  },
};

export const GrepDetailView: Story = {
  render: () => (
    <LeafFrame>
      <GrepDetailViewLeaf t={leafTools.grep} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("2 matches in 1 file · 42 scanned · truncated"),
    ).toBeVisible();
    await expect(canvas.getByText("341")).toBeVisible();
  },
};

export const Item: Story = {
  render: () => (
    <LeafFrame>
      <ItemLeaf
        it={{
          kind: "runtime",
          key: "leaf-runtime-item",
          source: "future-runtime-source",
          text: "Unclassified runtime context remains inspectable.",
          ts: at(64),
        }}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const summary = canvas.getByText("Runtime message");
    summary.focus();
    await userEvent.click(summary);
    await expect(
      canvas.getByText("Unclassified runtime context remains inspectable."),
    ).toBeVisible();
  },
};

export const JSONDetail: Story = {
  render: () => (
    <LeafFrame>
      <div className="step-detail">
        <JSONDetailLeaf t={leafTools.unknown} body="" />
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(/classify-next-runtime-state/),
    ).toBeVisible();
    await expect(canvas.getByText(/trace-story-unknown/)).toBeVisible();
  },
};

export const MiniDiff: Story = {
  render: () => (
    <LeafFrame>
      <MiniDiffLeaf
        rows={[
          { kind: "ctx", text: "export function TimelineView() {" },
          { kind: "del", text: "  return null;" },
          { kind: "add", text: "  return <Timeline />;" },
        ]}
        more={7}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("… 7 more lines")).toBeVisible();
    await expect(canvas.getByText("return <Timeline />;")).toBeVisible();
  },
};

export const MsgActions: Story = {
  render: () => (
    <LeafFrame>
      <div className="msg assistant msg-last">
        <MsgActionsLeaf
          text="Direct message actions remain keyboard reachable."
          ts={at(65)}
          onContinue={async () => {}}
        />
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const continueButton = canvas.getByRole("button", {
      name: "Continue in new session",
    });
    continueButton.focus();
    await userEvent.keyboard("{Enter}");
    await expect(continueButton).toBeEnabled();
    await expect(
      canvas.getByRole("button", { name: "Copy message" }),
    ).toBeVisible();
  },
};

export const MessageActionsHoverAndFocus: Story = {
  parameters: {
    pseudo: {
      rootSelector: "body",
      hover: '[data-testid="middle-message"]',
    },
  },
  render: () => (
    <LeafFrame>
      <div className="msg assistant pseudo-hover" data-testid="middle-message" tabIndex={0}>
        <div className="msg-col">
          <div className="bubble">An earlier assistant answer.</div>
          <MsgActionsLeaf
            text="An earlier assistant answer."
            ts={at(65)}
          />
        </div>
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const message = canvas.getByTestId("middle-message");
    const actions = message.querySelector(".msg-actions") as HTMLElement;
    await waitFor(() => {
      expect(message).toBeVisible();
      expect(message).toHaveClass("pseudo-hover");
    }, { timeout: 3_000 });
    await waitFor(() => {
      expect(getComputedStyle(actions).opacity).toBe("1");
      expect(getComputedStyle(actions).pointerEvents).toBe("auto");
    }, { timeout: 3_000 });
  },
};

export const MessageActionsFocusWithin: Story = {
  render: () => (
    <LeafFrame>
      <div className="msg assistant" data-testid="middle-message" tabIndex={0}>
        <div className="msg-col">
          <div className="bubble">An earlier keyboard-focused answer.</div>
          <MsgActionsLeaf
            text="An earlier keyboard-focused answer."
            ts={at(66)}
          />
        </div>
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const message = canvas.getByTestId("middle-message");
    const actions = message.querySelector(".msg-actions") as HTMLElement;
    await waitFor(() => {
      expect(message).toBeVisible();
      expect(getComputedStyle(actions).opacity).toBe("0");
      expect(getComputedStyle(actions).pointerEvents).toBe("none");
    });
    message.focus();
    await expect(message).toHaveFocus();
    await waitFor(() => {
      expect(getComputedStyle(actions).opacity).toBe("1");
      expect(getComputedStyle(actions).pointerEvents).toBe("auto");
    });
  },
};

export const MsgActionsBusyAndError: Story = {
  render: () => (
    <LeafFrame>
      <div className="grid gap-4">
        <div className="msg assistant msg-last">
          <MsgActionsLeaf
            text="Continue stays disabled while a fork is pending."
            onContinue={() => new Promise<void>(() => {})}
          />
        </div>
        <div className="msg assistant msg-last">
          <MsgActionsLeaf
            text="Continue reports an inaccessible checkpoint."
            onContinue={async () => {
              throw new Error("Checkpoint is no longer available");
            }}
          />
        </div>
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const continueButtons = canvas.getAllByRole("button", {
      name: "Continue in new session",
    });
    await userEvent.click(continueButtons[0]);
    await expect(continueButtons[0]).toBeDisabled();
    await expect(continueButtons[0]).toHaveAttribute("aria-busy", "true");

    await userEvent.click(continueButtons[1]);
    await expect(
      await canvas.findByText("Checkpoint is no longer available"),
    ).toBeInTheDocument();
    await expect(continueButtons[1]).toBeEnabled();
  },
};

export const ReadDetailView: Story = {
  render: () => (
    <LeafFrame>
      <ReadDetailViewLeaf t={leafTools.read} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("323–341")).toBeVisible();
    await expect(canvas.getByText("2 lines (truncated)")).toBeVisible();
  },
};

export const RetriedFold: Story = {
  render: () => (
    <LeafFrame>
      <ControlledRetriedFold />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const toggle = canvas.getByRole("button", {
      name: /Failed attempt · retried/,
    });
    toggle.focus();
    await userEvent.keyboard("{Enter}");
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    const toolSummary = canvasElement.querySelector(
      ".worked-body summary",
    ) as HTMLElement;
    await userEvent.click(toolSummary);
    await expect(
      canvas.getByText(
        "Unknown provider state was preserved for diagnosis.",
      ),
    ).toBeVisible();
  },
};

export const SemanticDetailView: Story = {
  render: () => (
    <LeafFrame>
      <SemanticDetailViewLeaf t={leafTools.semantic} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("keyboard accessible disclosure"),
    ).toBeVisible();
    await expect(
      canvas.getByText("src/components/Lightbox.tsx:48"),
    ).toBeVisible();
  },
};

export const ShellDetail: Story = {
  render: () => (
    <LeafFrame>
      <ShellDetailLeaf t={leafTools.shell} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Exit 1")).toBeVisible();
    await expect(
      canvasElement.querySelector(".shell-out"),
    ).toHaveTextContent("1 story failed accessibility checks");
    const copy = canvas.getByRole("button", {
      name: "Copy command and result",
    });
    await userEvent.click(copy);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "Copied command and result" }),
      ).toBeVisible(),
    );
  },
};

export const SpawnDetailView: Story = {
  render: () => (
    <LeafFrame>
      <SpawnDetailViewLeaf t={leafTools.spawn} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("reviewer")).toBeVisible();
    await expect(
      canvas.getByRole("link", { name: /open sub-session/ }),
    ).toHaveAttribute("href", "#story-child-session");
  },
};

export const Thumbs: Story = {
  render: () => (
    <LeafFrame>
      <ThumbsLeaf
        paths={["timeline-story-a.svg", "timeline-story-b.svg"]}
        fallback={<span>2 images unavailable</span>}
      />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const thumbs = await canvas.findAllByRole("button", {
      name: /View image \d of 2/,
    });
    thumbs[0].focus();
    await userEvent.keyboard("{Enter}");
    const page = within(canvasElement.ownerDocument.body);
    await expect(
      await page.findByRole("dialog", { name: "Image viewer" }),
    ).toBeVisible();
    await expect(page.getByText("1 / 2")).toBeVisible();
    await userEvent.keyboard("{Escape}");
    await expect(thumbs[0]).toHaveFocus();
  },
};

export const ToolCard: Story = {
  render: () => (
    <LeafFrame>
      <ToolCardLeaf t={leafTools.unknown} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const summary = canvasElement.querySelector("summary") as HTMLElement;
    await expect(summary).toBeVisible();
    summary.focus();
    await userEvent.click(summary);
    await expect(summary.parentElement).toHaveAttribute("open");
    await expect(canvas.getAllByText(/future_state/)[0]).toBeVisible();
    await expect(
      canvas.getByText(
        "Unknown provider state was preserved for diagnosis.",
      ),
    ).toBeVisible();
  },
};

export const ToolLifecycleMatrix: Story = {
  render: () => (
    <LeafFrame>
      <div className="grid gap-3">
        <div data-testid="tool-running">
          <ToolCardLeaf
            t={tool("tool-running", "read_file", { path: "running.ts" }, 70, {
              status: "running",
              statusText: "running",
              result: undefined,
              endTs: undefined,
            })}
          />
        </div>
        <div data-testid="tool-done">
          <ToolCardLeaf
            t={tool("tool-done", "read_file", { path: "done.ts" }, 71)}
          />
        </div>
        <div data-testid="tool-cancelled">
          <ToolCardLeaf
            t={tool(
              "tool-cancelled",
              "read_file",
              { path: "cancelled.ts" },
              72,
              {
                status: "cancelled",
                statusText: "cancelled",
                result: undefined,
              },
            )}
          />
        </div>
        <div data-testid="tool-failed">
          <ToolCardLeaf
            t={tool("tool-failed", "read_file", { path: "failed.ts" }, 73, {
              status: "failed",
              statusText: "failed",
              errorMsg: "The provider rejected this read.",
              result: undefined,
            })}
          />
        </div>
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByTestId("tool-running").querySelector(".step-ic.spin"),
    ).not.toBeNull();
    await expect(
      canvas.getByTestId("tool-done").querySelector(".step-ic.ok"),
    ).not.toBeNull();
    await expect(
      canvas.getByTestId("tool-cancelled").querySelector(".step-ic.warn"),
    ).not.toBeNull();
    await expect(
      canvas.getByTestId("tool-failed").querySelector(".step-ic.err"),
    ).not.toBeNull();
    await expect(
      canvas.getByTestId("tool-failed").querySelector(".step.error"),
    ).not.toBeNull();
  },
};

export const ToolDetail: Story = {
  render: () => (
    <LeafFrame>
      <div className="step-detail">
        <ToolDetailLeaf t={leafTools.unknown} body="" />
      </div>
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText(/trace-story-unknown/)).toBeVisible();
    await expect(canvas.getByText("Partial provider payload")).toBeVisible();
    await expect(
      canvas.getByText(
        "Unknown provider state was preserved for diagnosis.",
      ),
    ).toBeVisible();
  },
};

export const WebDetailView: Story = {
  render: () => (
    <LeafFrame>
      <WebDetailViewLeaf t={leafTools.web} />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("untrusted")).toBeVisible();
    await expect(
      canvas.getByRole("link", {
        name: /https:\/\/example.com\/storybook-guidance/,
      }),
    ).toHaveAttribute("target", "_blank");
  },
};

export const WorkedFold: Story = {
  render: () => (
    <LeafFrame>
      <ControlledWorkedFold />
    </LeafFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const toggle = canvas.getByRole("button", {
      name: /Worked for 8s/,
    });
    toggle.focus();
    await userEvent.keyboard("{Enter}");
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(
      canvasElement.querySelector(".worked-body"),
    ).toBeVisible();
    await expect(
      canvas.getByText("Edited files"),
    ).toBeVisible();
  },
};

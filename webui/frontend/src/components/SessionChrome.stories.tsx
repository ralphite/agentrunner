import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { FailureNotice } from "../timeline";
import { StoryAppFrame } from "../storybook/StoryAppFrame";
import {
  QueuedMessageList,
  SessionNotice,
  SessionTopbar,
  TerminalAlert,
  TurnFailureCard,
  type SessionTopbarProps,
} from "./SessionChrome";

function ChromeFrame({ children }: { children: React.ReactNode }) {
  return (
    <StoryAppFrame>
      <div className="mx-auto max-w-[900px] p-6">{children}</div>
    </StoryAppFrame>
  );
}

const topbarActions = {
  onBackToParent: fn(),
  onResume: fn(),
  onRetry: fn(),
  onToggleEnvironment: fn(),
  onPin: fn(),
  onRename: fn(),
  onArchive: fn(),
  onShowConversation: fn(),
  onShowChanges: fn(),
  onToggleSupervision: fn(),
  onToggleSystemEvents: fn(),
  onCreateCheckpoint: fn(),
  onContinueInNewSession: fn(),
  onSwitchAgent: fn(),
};

const topbarArgs: SessionTopbarProps = {
  sid: "20260723-180000-story-session",
  title: "Build deterministic Storybook coverage",
  isSub: false,
  needsRecovery: false,
  canRetry: false,
  showPrimaryRetry: false,
  showCompactRetry: false,
  environmentOpen: false,
  environmentAttention: 0,
  pinned: false,
  archived: false,
  view: "chat",
  supervisionOpen: false,
  showSystemEvents: false,
  ...topbarActions,
};

const meta = {
  title: "Components/Sessions/SessionChrome",
  component: SessionTopbar,
  parameters: { layout: "fullscreen" },
  args: topbarArgs,
  render: (args) => (
    <ChromeFrame>
      <SessionTopbar {...args} />
    </ChromeFrame>
  ),
} satisfies Meta<typeof SessionTopbar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TopbarDefault: Story = {
  args: {
    environmentOpen: true,
    environmentAttention: 3,
    pinned: true,
    supervisionOpen: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Build deterministic Storybook coverage")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Environment" })).toHaveClass("active");
    await expect(canvas.getByText("3")).toBeVisible();
  },
};

export const TopbarRecovery: Story = {
  args: {
    needsRecovery: true,
    environmentAttention: 1,
  },
  play: async ({ canvasElement }) => {
    topbarActions.onResume.mockClear();
    const resume = within(canvasElement).getByRole("button", { name: "Resume session" });
    await expect(resume).toBeVisible();
    await userEvent.click(resume);
    await expect(topbarActions.onResume).toHaveBeenCalled();
  },
};

export const TopbarRetry: Story = {
  args: {
    canRetry: true,
    showPrimaryRetry: true,
  },
  play: async ({ canvasElement }) => {
    topbarActions.onRetry.mockClear();
    const retry = within(canvasElement).getByRole("button", { name: "Retry session" });
    retry.focus();
    await userEvent.keyboard("{Enter}");
    await expect(topbarActions.onRetry).toHaveBeenCalled();
  },
};

export const TopbarSubAgent: Story = {
  args: {
    sid: "20260723-parent-sub-call_story-worker",
    title: "browser-reviewer",
    durableTitle: "Build deterministic Storybook coverage",
    isSub: true,
    subAnswerRequested: true,
  },
  play: async ({ canvasElement }) => {
    topbarActions.onBackToParent.mockClear();
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Sub-agent · answer requested")).toBeVisible();
    const back = canvas.getByRole("button", { name: "Back to parent session" });
    back.focus();
    await userEvent.keyboard("{Enter}");
    await expect(topbarActions.onBackToParent).toHaveBeenCalled();
    await expect(canvas.queryByText("Advanced")).toBeNull();
  },
};

export const TopbarReadOnlySubAgent: Story = {
  args: {
    sid: "20260723-parent-sub-call_story-observer",
    title: "release-observer",
    durableTitle: "Observe the release verification",
    isSub: true,
    subAnswerRequested: false,
    needsRecovery: true,
    canRetry: true,
    showPrimaryRetry: true,
    showCompactRetry: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Read-only sub-agent")).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: "Resume session" }),
    ).toBeNull();
    await expect(
      canvas.queryByRole("button", { name: "Retry session" }),
    ).toBeNull();
    await userEvent.click(
      canvas.getByRole("button", { name: "More session actions" }),
    );
    await expect(canvas.queryByText("Advanced")).toBeNull();
  },
};

export const TopbarKeyboardMenu: Story = {
  args: {
    archived: true,
    showSystemEvents: true,
  },
  play: async ({ canvasElement }) => {
    topbarActions.onShowChanges.mockClear();
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("button", { name: "More session actions" });
    trigger.focus();
    await userEvent.keyboard("{Enter}");
    const firstItem = await canvas.findByRole("menuitem", { name: "Pin session" });
    await waitFor(() => expect(firstItem).toHaveFocus());
    const changes = canvas.getByRole("menuitem", { name: "Changes" });
    changes.focus();
    await userEvent.keyboard("{Enter}");
    await expect(topbarActions.onShowChanges).toHaveBeenCalled();
  },
};

export const TopbarChangesView: Story = {
  args: {
    view: "diff",
    environmentOpen: false,
    supervisionOpen: true,
    pinned: true,
    archived: true,
    showSystemEvents: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "More session actions" }),
    );
    await expect(
      canvas.getByRole("menuitem", { name: "Conversation" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("menuitem", { name: "Hide Environment" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("menuitem", { name: "Hide system events" }),
    ).toBeVisible();
  },
};

export const TopbarOverflowActions: Story = {
  args: {
    reserveNavigationSlot: true,
    canRetry: true,
    showPrimaryRetry: false,
    showCompactRetry: true,
  },
  play: async ({ canvasElement }) => {
    topbarActions.onRetry.mockClear();
    const canvas = within(canvasElement);
    await expect(
      canvasElement.querySelector(".session-topbar-nav-slot"),
    ).not.toBeNull();
    await userEvent.click(
      canvas.getByRole("button", { name: "More session actions" }),
    );
    const retry = canvas.getByRole("menuitem", {
      name: "Retry last message",
    });
    await expect(retry).toBeVisible();
    retry.focus();
    await userEvent.keyboard("{Enter}");
    await expect(topbarActions.onRetry).toHaveBeenCalled();
  },
};

const providerFailure: FailureNotice = {
  seq: 12,
  cls: "provider_server",
  title: "The model provider had a server error",
  hint: "This is usually temporary and not something you did. Retry the turn.",
  raw: "503 fixture provider unavailable [provider_server]",
  attempt: 2,
  recovered: false,
};

function FailureFixture({
  initiallyOpen = false,
  retrying = false,
}: {
  initiallyOpen?: boolean;
  retrying?: boolean;
}) {
  const [detailsOpen, setDetailsOpen] = useState(initiallyOpen);
  return (
    <ChromeFrame>
      <TurnFailureCard
        failure={providerFailure}
        detailsOpen={detailsOpen}
        retrying={retrying}
        onToggleDetails={() => setDetailsOpen((open) => !open)}
        onRetry={failureRetry}
      />
    </ChromeFrame>
  );
}

const failureRetry = fn();

export const FailureDefault: Story = {
  render: () => <FailureFixture />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("alert")).toHaveTextContent(
      "The model provider had a server error",
    );
    await expect(canvas.queryByText(providerFailure.raw)).toBeNull();
    await expect(canvas.getByRole("button", { name: "Retry" })).toBeEnabled();
  },
};

export const FailureDetails: Story = {
  render: () => <FailureFixture initiallyOpen />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText(providerFailure.raw)).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Hide technical details" }),
    ).toHaveAttribute("aria-expanded", "true");
  },
};

export const FailureRetrying: Story = {
  render: () => <FailureFixture retrying />,
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Retrying…" }),
    ).toBeDisabled();
  },
};

export const FailureWithoutHint: Story = {
  render: () => (
    <ChromeFrame>
      <TurnFailureCard
        failure={{
          ...providerFailure,
          cls: "canceled",
          title: "The step was cancelled",
          hint: undefined,
          raw: "context canceled",
        }}
        detailsOpen={false}
        retrying={false}
        onToggleDetails={fn()}
        onRetry={failureRetry}
      />
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("The step was cancelled")).toBeVisible();
    await expect(
      canvas.queryByText(providerFailure.hint as string),
    ).toBeNull();
  },
};

export const FailureKeyboard: Story = {
  render: () => <FailureFixture />,
  play: async ({ canvasElement }) => {
    failureRetry.mockClear();
    const canvas = within(canvasElement);
    const details = canvas.getByRole("button", { name: "Technical details" });
    details.focus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByText(providerFailure.raw)).toBeVisible();
    const retry = canvas.getByRole("button", { name: "Retry" });
    retry.focus();
    await userEvent.keyboard(" ");
    await expect(failureRetry).toHaveBeenCalled();
  },
};

const terminalAction = fn();

export const TerminalContinueWithGoal: Story = {
  render: () => (
    <ChromeFrame>
      <TerminalAlert
        notice={{
          title: "Step limit reached",
          body: "The session stopped at its configured generation-step limit. Review the run or continue from a checkpoint.",
          tone: "attention",
          action: "continue",
          actionLabel: "Continue in new session",
        }}
        goalMeta={{
          label: "Goal cancelled",
          elapsedMs: 34_000,
          goal: "Verify every Session chrome state",
        }}
        onAction={terminalAction}
      />
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("alert")).toHaveTextContent("Goal cancelled");
    await expect(canvas.getByText("00:34")).toBeVisible();
    await expect(canvas.getByTitle("Verify every Session chrome state")).toBeVisible();
  },
};

export const TerminalRecovery: Story = {
  render: () => (
    <ChromeFrame>
      <TerminalAlert
        notice={{
          title: "Session needs recovery",
          body: "The previous host stopped before this session reached a durable terminal state. Resume from its last checkpoint.",
          tone: "attention",
          action: "resume",
          actionLabel: "Resume session",
        }}
        onAction={terminalAction}
      />
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    terminalAction.mockClear();
    const action = within(canvasElement).getByRole("button", { name: "Resume session" });
    action.focus();
    await userEvent.keyboard("{Enter}");
    await expect(terminalAction).toHaveBeenCalled();
  },
};

export const TerminalDanger: Story = {
  render: () => (
    <ChromeFrame>
      <TerminalAlert
        notice={{
          title: "Session failed",
          body: "The last run ended unexpectedly. Review the recorded run details before deciding whether to retry.",
          tone: "danger",
          action: "inspect",
          actionLabel: "Run details",
        }}
        onAction={terminalAction}
      />
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    const alert = within(canvasElement).getByRole("alert");
    await expect(alert).toHaveClass("danger");
    await expect(
      within(canvasElement).getByRole("button", { name: "Run details" }),
    ).toBeEnabled();
  },
};

export const TerminalRunLimit: Story = {
  render: () => (
    <ChromeFrame>
      <TerminalAlert
        notice={{
          title: "Iteration limit reached",
          body: "The scheduled run completed its configured number of iterations. Review the run before extending it.",
          tone: "attention",
          action: "inspect",
          actionLabel: "Run details",
        }}
        onAction={terminalAction}
      />
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("alert")).toHaveClass("attention");
    await expect(
      canvas.getByRole("button", { name: "Run details" }),
    ).toBeEnabled();
  },
};

export const TerminalToneMatrix: Story = {
  render: () => (
    <ChromeFrame>
      <div className="grid gap-3">
        <TerminalAlert
          notice={{
            title: "Session needs attention",
            body: "Resume from the last durable checkpoint.",
            tone: "attention",
            action: "resume",
            actionLabel: "Resume session",
          }}
          onAction={terminalAction}
        />
        <TerminalAlert
          notice={{
            title: "Session failed",
            body: "Inspect the failed run before continuing.",
            tone: "danger",
            action: "inspect",
            actionLabel: "Run details",
          }}
          onAction={terminalAction}
        />
      </div>
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    const [attention, danger] = within(canvasElement).getAllByRole("alert");
    const attentionStyle = getComputedStyle(attention);
    const dangerStyle = getComputedStyle(danger);
    await expect(attention).toHaveClass("attention");
    await expect(danger).toHaveClass("danger");
    await expect(attentionStyle.backgroundColor).not.toBe(
      dangerStyle.backgroundColor,
    );
    await expect(attentionStyle.borderColor).not.toBe(dangerStyle.borderColor);
    await expect(attentionStyle.color).not.toBe(dangerStyle.color);
  },
};

const withdrawMessage = fn();
const queuedMessages = [
  {
    command_id: "queued-plain",
    text: "Run the focused browser checks after the implementation is complete.",
    revoked: false,
  },
  {
    command_id: "queued-worker",
    text: "[message from reviewer (20260723-parent-sub-call_reviewer)] Check the terminal alert at narrow widths.",
    revoked: false,
  },
  {
    command_id: "queued-revoked",
    text: "This withdrawn message must not be rendered.",
    revoked: true,
  },
];

export const QueuedMessages: Story = {
  render: () => (
    <ChromeFrame>
      <QueuedMessageList
        messages={queuedMessages}
        onWithdraw={withdrawMessage}
      />
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Queued · from reviewer")).toBeVisible();
    await expect(canvas.getByText("Check the terminal alert at narrow widths.")).toBeVisible();
    await expect(canvas.queryByText("This withdrawn message must not be rendered.")).toBeNull();
    await expect(canvas.getAllByRole("button", { name: "Withdraw" })).toHaveLength(2);
  },
};

export const QueuedKeyboard: Story = {
  render: () => (
    <ChromeFrame>
      <QueuedMessageList
        messages={[queuedMessages[0]]}
        onWithdraw={withdrawMessage}
      />
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    withdrawMessage.mockClear();
    const withdraw = within(canvasElement).getByRole("button", { name: "Withdraw" });
    withdraw.focus();
    await userEvent.keyboard("{Enter}");
    await expect(withdrawMessage).toHaveBeenCalledWith("queued-plain");
  },
};

export const QueuedEmpty: Story = {
  render: () => (
    <ChromeFrame>
      <QueuedMessageList
        messages={[queuedMessages[2]]}
        onWithdraw={withdrawMessage}
      />
      <span data-testid="empty-anchor">No queued chrome rendered</span>
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByRole("button", { name: "Withdraw" })).toBeNull();
    await expect(canvas.getByTestId("empty-anchor")).toBeVisible();
  },
};

export const QueuedLongMessage: Story = {
  render: () => (
    <ChromeFrame>
      <QueuedMessageList
        messages={[{
          command_id: "queued-long",
          text: "Review the complete Session chrome component contract, including the responsive terminal action, the technical-details disclosure, queued peer attribution, Environment actions, and keyboard focus restoration before accepting this queued turn.",
          revoked: false,
        }]}
        onWithdraw={withdrawMessage}
      />
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const text = canvas.getByTitle(/Review the complete Session chrome/);
    await expect(text).toBeVisible();
    await expect(canvas.getByText("Queued")).toBeVisible();
  },
};

const noticeAction = fn();

export const NoticeInformational: Story = {
  render: () => (
    <ChromeFrame>
      <SessionNotice>
        This conversation is idle — send a message to continue it.
      </SessionNotice>
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(
        "This conversation is idle — send a message to continue it.",
      ),
    ).toBeVisible();
  },
};

export const NoticeAction: Story = {
  render: () => (
    <ChromeFrame>
      <SessionNotice
        action={{
          label: "Apply winner",
          title: "Apply the winning attempt's changes",
          onClick: noticeAction,
        }}
      >
        Best-of-N winner: #2.
      </SessionNotice>
    </ChromeFrame>
  ),
  play: async ({ canvasElement }) => {
    noticeAction.mockClear();
    const action = within(canvasElement).getByRole("button", { name: "Apply winner" });
    action.focus();
    await userEvent.keyboard(" ");
    await expect(noticeAction).toHaveBeenCalled();
  },
};

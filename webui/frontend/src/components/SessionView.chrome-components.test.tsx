// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { FailureNotice } from "../timeline";
import {
  QueuedMessageList,
  SessionNotice,
  SessionTopbar,
  TerminalAlert,
  TurnFailureCard,
  type SessionTopbarProps,
} from "./SessionChrome";

afterEach(cleanup);

function topbarProps(
  overrides: Partial<SessionTopbarProps> = {},
): SessionTopbarProps {
  return {
    sid: "20260723-session",
    title: "Session chrome contract",
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
    onBackToParent: vi.fn(),
    onResume: vi.fn(),
    onRetry: vi.fn(),
    onToggleEnvironment: vi.fn(),
    onPin: vi.fn(),
    onRename: vi.fn(),
    onArchive: vi.fn(),
    onShowConversation: vi.fn(),
    onShowChanges: vi.fn(),
    onToggleSupervision: vi.fn(),
    onToggleSystemEvents: vi.fn(),
    onCreateCheckpoint: vi.fn(),
    onContinueInNewSession: vi.fn(),
    onSwitchAgent: vi.fn(),
    ...overrides,
  };
}

describe("SessionTopbar", () => {
  it("exposes recovery and Environment actions through callbacks", () => {
    const onResume = vi.fn();
    const onEnvironment = vi.fn();
    render(
      <SessionTopbar
        {...topbarProps({
          needsRecovery: true,
          environmentAttention: 2,
          onResume,
          onToggleEnvironment: onEnvironment,
        })}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Resume session" }));
    fireEvent.click(screen.getByRole("button", { name: "Environment" }));
    expect(onResume).toHaveBeenCalledOnce();
    expect(onEnvironment).toHaveBeenCalledWith(expect.any(HTMLButtonElement));
    expect(screen.getByText("2")).toBeTruthy();
  });

  it("renders the sub-agent answer state without parent-only actions", () => {
    const onBack = vi.fn();
    render(
      <SessionTopbar
        {...topbarProps({
          sid: "20260723-parent-sub-call_worker",
          title: "worker",
          durableTitle: "Parent session",
          isSub: true,
          subAnswerRequested: true,
          onBackToParent: onBack,
        })}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Back to parent session" }),
    );
    expect(onBack).toHaveBeenCalledOnce();
    expect(screen.getByText("Sub-agent · answer requested")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Resume session" })).toBeNull();
  });

  it("keeps organization and view actions in the More menu", () => {
    const onShowChanges = vi.fn();
    render(
      <SessionTopbar
        {...topbarProps({ pinned: true, onShowChanges })}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "More session actions" }),
    );
    expect(screen.getByRole("menuitem", { name: "Unpin session" })).toBeTruthy();
    fireEvent.click(screen.getByRole("menuitem", { name: "Changes" }));
    expect(onShowChanges).toHaveBeenCalledOnce();
  });
});

const failure: FailureNotice = {
  seq: 4,
  cls: "provider_server",
  title: "The model provider had a server error",
  hint: "Retry the turn.",
  raw: "503 provider unavailable",
  recovered: false,
};

describe("TurnFailureCard", () => {
  it("keeps raw details opt-in and reports retry progress", () => {
    const onToggleDetails = vi.fn();
    render(
      <TurnFailureCard
        failure={failure}
        detailsOpen={false}
        retrying
        onToggleDetails={onToggleDetails}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.queryByText(failure.raw)).toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: "Technical details" }),
    );
    expect(onToggleDetails).toHaveBeenCalledOnce();
    const retry = screen.getByRole("button", { name: "Retry" }) as HTMLButtonElement;
    expect(retry.disabled).toBe(true);
    expect(retry.getAttribute("aria-busy")).toBe("true");
  });

  it("renders the untouched technical string when expanded", () => {
    render(
      <TurnFailureCard
        failure={failure}
        detailsOpen
        retrying={false}
        onToggleDetails={vi.fn()}
        onRetry={vi.fn()}
      />,
    );
    expect(screen.getByText(failure.raw)).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Hide technical details" })
        .getAttribute("aria-expanded"),
    ).toBe("true");
  });
});

describe("TerminalAlert", () => {
  it.each([
    ["continue", "Continue in new session"],
    ["resume", "Resume session"],
    ["inspect", "Run details"],
  ] as const)("covers the %s action", (action, actionLabel) => {
    const onAction = vi.fn();
    render(
      <TerminalAlert
        notice={{
          title: action === "resume" ? "Session needs recovery" : "Session stopped",
          body: "Review the durable session state.",
          tone: action === "inspect" ? "danger" : "attention",
          action,
          actionLabel,
        }}
        onAction={onAction}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: actionLabel }));
    expect(onAction).toHaveBeenCalledOnce();
  });

  it("folds the terminal goal label and elapsed time into one alert", () => {
    render(
      <TerminalAlert
        notice={{
          title: "Step limit reached",
          body: "Continue from a durable checkpoint.",
          tone: "attention",
          action: "continue",
          actionLabel: "Continue in new session",
        }}
        goalMeta={{
          label: "Goal cancelled",
          elapsedMs: 34_000,
          goal: "Verify Session chrome",
        }}
        onAction={vi.fn()}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain("Goal cancelled");
    expect(screen.getByRole("alert").textContent).toContain("00:34");
    expect(screen.getByTitle("Verify Session chrome")).toBeTruthy();
  });
});

describe("QueuedMessageList", () => {
  it("humanizes child frames, filters revoked rows, and preserves withdrawal ids", () => {
    const onWithdraw = vi.fn();
    render(
      <QueuedMessageList
        messages={[
          {
            command_id: "child-message",
            text: "[message from reviewer (child-session)] Browser review is ready.",
            revoked: false,
          },
          {
            command_id: "revoked",
            text: "Withdrawn text",
            revoked: true,
          },
        ]}
        onWithdraw={onWithdraw}
      />,
    );

    expect(screen.getByText("Queued · from reviewer")).toBeTruthy();
    expect(screen.getByText("Browser review is ready.")).toBeTruthy();
    expect(screen.queryByText("Withdrawn text")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Withdraw" }));
    expect(onWithdraw).toHaveBeenCalledWith("child-message");
  });

  it("renders nothing when every row is revoked", () => {
    const { container } = render(
      <QueuedMessageList
        messages={[{ command_id: "revoked", text: "gone", revoked: true }]}
        onWithdraw={vi.fn()}
      />,
    );
    expect(container.innerHTML).toBe("");
  });
});

describe("SessionNotice", () => {
  it("supports both informational and actionable notices", () => {
    const onApply = vi.fn();
    render(
      <>
        <SessionNotice>This conversation is idle.</SessionNotice>
        <SessionNotice action={{ label: "Apply winner", onClick: onApply }}>
          Best-of-N winner: #2.
        </SessionNotice>
      </>,
    );
    expect(screen.getByText("This conversation is idle.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Apply winner" }));
    expect(onApply).toHaveBeenCalledOnce();
  });
});

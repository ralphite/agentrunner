import type { ReactNode } from "react";
import {
  Archive,
  ArrowClockwise,
  ArrowLeft,
  ChatCircle,
  ClockCountdown,
  Code,
  DotsThree,
  Files,
  Flag,
  GitFork,
  PencilSimple,
  PushPin,
  Robot,
  SidebarSimple,
  SlidersHorizontal,
  WarningCircle,
  XCircle,
} from "@phosphor-icons/react";
import type { FailureNotice } from "../timeline";
import { formatElapsed } from "../timeline";
import { IconButton } from "../ui/IconButton";
import type { TerminalNotice } from "./pill";
import { Menu, MenuItem, MenuLabel } from "./Menu";

export interface SessionTopbarProps {
  sid: string;
  title: string;
  durableTitle?: string;
  isSub: boolean;
  subAnswerRequested?: boolean;
  reserveNavigationSlot?: boolean;
  needsRecovery: boolean;
  canRetry: boolean;
  showPrimaryRetry: boolean;
  showCompactRetry: boolean;
  environmentOpen: boolean;
  environmentAttention: number;
  pinned: boolean;
  archived: boolean;
  view: "chat" | "diff";
  supervisionOpen: boolean;
  showSystemEvents: boolean;
  onBackToParent: () => void;
  onResume: () => void;
  onRetry: () => void;
  onToggleEnvironment: (opener: HTMLButtonElement) => void;
  onPin: () => void;
  onRename: () => void;
  onArchive: () => void;
  onShowConversation: () => void;
  onShowChanges: () => void;
  onToggleSupervision: () => void;
  onToggleSystemEvents: () => void;
  onCreateCheckpoint: () => void;
  onContinueInNewSession: () => void;
  onSwitchAgent: () => void;
}

/**
 * The session title and its complete action surface. State and side effects
 * stay in SessionView; this component owns only the stable chrome contract.
 */
export function SessionTopbar({
  sid,
  title,
  durableTitle = title,
  isSub,
  subAnswerRequested = false,
  reserveNavigationSlot = false,
  needsRecovery,
  canRetry,
  showPrimaryRetry,
  showCompactRetry,
  environmentOpen,
  environmentAttention,
  pinned,
  archived,
  view,
  supervisionOpen,
  showSystemEvents,
  onBackToParent,
  onResume,
  onRetry,
  onToggleEnvironment,
  onPin,
  onRename,
  onArchive,
  onShowConversation,
  onShowChanges,
  onToggleSupervision,
  onToggleSystemEvents,
  onCreateCheckpoint,
  onContinueInNewSession,
  onSwitchAgent,
}: SessionTopbarProps) {
  return (
    <header className="session-topbar">
      {reserveNavigationSlot && (
        <span
          className="session-topbar-nav-slot h-9 w-9 shrink-0"
          aria-hidden="true"
        />
      )}
      {isSub && (
        <IconButton
          size="md"
          variant="ghost"
          onClick={onBackToParent}
          title="Back to parent session"
          aria-label="Back to parent session"
        >
          <ArrowLeft size={16} />
        </IconButton>
      )}
      <div className="tt-left">
        <div
          className="tt-title"
          title={`${title}${title !== durableTitle ? `\n${durableTitle}` : ""}\n${sid}`}
        >
          {title}
        </div>
        {isSub && (
          <span className="readonly-tag">
            {subAnswerRequested
              ? "Sub-agent · answer requested"
              : "Read-only sub-agent"}
          </span>
        )}
      </div>
      <span className="spacer" />
      {!isSub && needsRecovery && (
        <button
          type="button"
          className="topbar-tool recovery"
          onClick={onResume}
          title="Resume this session from its last durable checkpoint"
          aria-label="Resume session"
        >
          <ArrowClockwise size={15} />{" "}
          <span className="topbar-tool-label">Resume</span>
        </button>
      )}
      {!isSub && canRetry && showPrimaryRetry && (
        <button
          type="button"
          className="topbar-tool"
          onClick={onRetry}
          title="Re-send your last message as a new turn; double-clicks are idempotent"
          aria-label="Retry session"
        >
          <ArrowClockwise size={15} />{" "}
          <span className="topbar-tool-label">Retry</span>
        </button>
      )}
      <button
        type="button"
        className={`topbar-tool${environmentOpen ? " active" : ""}`}
        onClick={(event) => onToggleEnvironment(event.currentTarget)}
        title={
          environmentOpen
            ? "Hide the Environment rail"
            : "Show the Environment rail — workspace changes, worktree, git, goal"
        }
        aria-label="Environment"
      >
        <SlidersHorizontal size={16} />{" "}
        <span className="topbar-tool-label">Environment</span>
        {environmentAttention > 0 && (
          <span className="topbar-attention">{environmentAttention}</span>
        )}
      </button>
      <Menu
        label={<DotsThree size={18} weight="bold" />}
        ariaLabel="More session actions"
      >
        <MenuItem
          title="keep this session in a Pinned section at the top of the sidebar"
          onClick={onPin}
        >
          <PushPin size={16} weight={pinned ? "fill" : "regular"} />
          {pinned ? "Unpin session" : "Pin session"}
        </MenuItem>
        <MenuItem
          title="give this session a custom name in the sidebar (stored in your browser)"
          onClick={onRename}
        >
          <PencilSimple size={16} />
          Rename session…
        </MenuItem>
        <MenuItem
          title="hide this session from the sidebar list (it stays on disk; toggle 'Show archived' to see it again)"
          onClick={onArchive}
        >
          <Archive size={16} />
          {archived ? "Unarchive session" : "Archive session"}
        </MenuItem>
        <MenuLabel>View</MenuLabel>
        {view === "diff" ? (
          <MenuItem onClick={onShowConversation}>
            <ChatCircle size={16} />
            Conversation
          </MenuItem>
        ) : (
          <MenuItem onClick={onShowChanges}>
            <Files size={16} />
            Changes
          </MenuItem>
        )}
        <MenuItem onClick={onToggleSupervision}>
          <SidebarSimple size={16} />
          {supervisionOpen ? "Hide" : "Show"} Environment
        </MenuItem>
        <MenuItem
          title="also show low-level system events (mode changes, effects, barriers…) inline in the timeline"
          onClick={onToggleSystemEvents}
        >
          <Code size={16} />
          {showSystemEvents ? "Hide" : "Show"} system events
        </MenuItem>
        {!isSub && (
          <>
            <MenuLabel>Advanced</MenuLabel>
            <MenuItem
              title="checkpoint the session right now (ar barrier) so you can fork from this exact point later"
              onClick={onCreateCheckpoint}
            >
              <Flag size={16} />
              Create checkpoint
            </MenuItem>
            <MenuItem
              title="continue from a checkpoint in a new session and worktree; this session is untouched"
              onClick={onContinueInNewSession}
            >
              <GitFork size={16} />
              Continue in new session…
            </MenuItem>
            <MenuItem
              title="swap this session's agent spec — context carries over; takes effect on your next message (spec_changed)"
              onClick={onSwitchAgent}
            >
              <Robot size={16} />
              Switch agent…
            </MenuItem>
            {(showCompactRetry || needsRecovery) && (
              <>
                <MenuLabel>Run</MenuLabel>
                {showCompactRetry && (
                  <MenuItem onClick={onRetry}>
                    <ArrowClockwise size={16} />
                    Retry last message
                  </MenuItem>
                )}
                {needsRecovery && (
                  <MenuItem onClick={onResume}>
                    <ArrowClockwise size={16} />
                    Resume session
                  </MenuItem>
                )}
              </>
            )}
          </>
        )}
      </Menu>
    </header>
  );
}

export interface TurnFailureCardProps {
  failure: FailureNotice;
  detailsOpen: boolean;
  retrying: boolean;
  onToggleDetails: () => void;
  onRetry: () => void;
}

export function TurnFailureCard({
  failure,
  detailsOpen,
  retrying,
  onToggleDetails,
  onRetry,
}: TurnFailureCardProps) {
  return (
    <div className="turn-error" role="alert">
      <span className="turn-error-ic">
        <WarningCircle size={17} weight="fill" />
      </span>
      <div className="turn-error-body">
        <b>{failure.title}</b>
        {failure.hint && (
          <span className="turn-error-hint">{failure.hint}</span>
        )}
        <button
          type="button"
          className="turn-error-toggle"
          aria-expanded={detailsOpen}
          onClick={onToggleDetails}
        >
          {detailsOpen ? "Hide technical details" : "Technical details"}
        </button>
        {detailsOpen && (
          <pre className="turn-error-raw max-w-full overflow-x-auto">
            {failure.raw}
          </pre>
        )}
      </div>
      <button
        type="button"
        className="turn-error-action"
        disabled={retrying}
        onClick={onRetry}
        title="Re-send your last message as a new turn; double-clicks are idempotent"
      >
        <ArrowClockwise size={14} /> {retrying ? "Retrying…" : "Retry"}
      </button>
    </div>
  );
}

export interface TerminalAlertGoalMeta {
  label: string;
  elapsedMs?: number;
  goal: string;
}

export interface TerminalAlertProps {
  notice: TerminalNotice;
  goalMeta?: TerminalAlertGoalMeta | null;
  onAction: () => void;
}

/**
 * One terminal layout for recovery, continuation and inspection. Small visual
 * differences remain data-driven so the responsive/action markup cannot drift.
 */
export function TerminalAlert({
  notice,
  goalMeta,
  onAction,
}: TerminalAlertProps) {
  const recovery = notice.action === "resume";
  return (
    <div
      className={`terminal-alert ${notice.tone} grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-3 sm:grid-cols-[auto_minmax(0,1fr)_auto]`}
      role="alert"
    >
      <span className="terminal-alert-ic">
        {notice.tone === "danger" ? (
          <XCircle size={17} weight="fill" />
        ) : (
          <WarningCircle size={17} weight="fill" />
        )}
      </span>
      <div
        className={recovery ? "min-w-0" : "terminal-alert-text"}
        title={
          recovery ? undefined : `${notice.title} — ${notice.body}`
        }
      >
        <b className={recovery ? "block leading-5" : undefined}>
          {notice.title}
        </b>
        <span
          className={
            recovery
              ? "mt-1 block text-[12px] leading-[1.5] text-dim"
              : undefined
          }
        >
          {notice.body}
        </span>
        {goalMeta && (
          <span
            className="terminal-alert-meta mt-2 flex gap-2"
            title={goalMeta.goal}
          >
            <span className="tam-label">{goalMeta.label}</span>
            {goalMeta.elapsedMs !== undefined && (
              <span>{formatElapsed(goalMeta.elapsedMs)}</span>
            )}
          </span>
        )}
      </div>
      <button
        type="button"
        className="terminal-alert-action col-span-2 flex w-full items-center justify-center gap-2 sm:col-span-1 sm:col-start-3 sm:row-start-1 sm:self-center sm:w-auto"
        onClick={onAction}
      >
        {recovery && <ArrowClockwise size={14} />}
        {notice.actionLabel}
      </button>
    </div>
  );
}

export interface QueuedMessage {
  command_id: string;
  text: string;
  revoked: boolean;
}

export function QueuedMessageItem({
  message,
  onWithdraw,
}: {
  message: QueuedMessage;
  onWithdraw: (commandID: string) => void;
}) {
  const framed = /^\[message from ([^\s(]+)[^\]]*\]\s*/.exec(message.text);
  const body = framed
    ? message.text.slice(framed[0].length)
    : message.text;
  return (
    <div className="queued-row">
      <ClockCountdown
        size={15}
        className="queued-ic"
        aria-hidden="true"
      />
      <span className="queued-kicker">
        {framed ? `Queued · from ${framed[1]}` : "Queued"}
      </span>
      <span className="queued-text" title={message.text}>
        {body}
      </span>
      <button
        type="button"
        className="queued-drop"
        onClick={() => onWithdraw(message.command_id)}
        title="Withdraw this queued message before it runs"
      >
        Withdraw
      </button>
    </div>
  );
}

export function QueuedMessageList({
  messages,
  onWithdraw,
}: {
  messages: QueuedMessage[];
  onWithdraw: (commandID: string) => void;
}) {
  const visible = messages.filter((message) => !message.revoked);
  if (visible.length === 0) return null;
  return (
    <div className="queued-list">
      {visible.map((message) => (
        <QueuedMessageItem
          key={message.command_id}
          message={message}
          onWithdraw={onWithdraw}
        />
      ))}
    </div>
  );
}

export interface SessionNoticeAction {
  label: string;
  title?: string;
  onClick: () => void;
}

export function SessionNotice({
  children,
  action,
}: {
  children: ReactNode;
  action?: SessionNoticeAction;
}) {
  return (
    <div className="driver-note">
      {children}
      {action && (
        <>
          {" "}
          <button
            type="button"
            className="ghost"
            onClick={action.onClick}
            title={action.title}
          >
            {action.label}
          </button>
        </>
      )}
    </div>
  );
}

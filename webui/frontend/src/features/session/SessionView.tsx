import type { CSSProperties, ReactNode } from "react";
import {
  CaretRight,
  CheckCircle,
  ClockCountdown,
  Crosshair,
  Pause,
  PencilSimple,
  Play,
  Prohibit,
  Trash,
  WarningCircle,
  X,
} from "@phosphor-icons/react";
import { formatElapsed, type GoalDerived } from "../../timeline";
import { Button } from "../../ui/Button";
import { Textarea } from "../../ui/Field";
import { IconButton } from "../../ui/IconButton";
import { Spinner } from "../../ui/Spinner";
import type { ProgressItem } from "../../components/SupervisionPanel";

export interface SessionViewProps {
  daemonAlert: ReactNode;
  notFound?: ReactNode;
  topbar?: ReactNode;
  findBar?: ReactNode;
  view?: "chat" | "diff";
  /** Compact Changes is a modal; its persistent desktop siblings must stand down. */
  changesModal?: boolean;
  showSupervision?: boolean;
  conversation?: ReactNode;
  sidePanel?: ReactNode;
  /** Drag handle between the conversation and the Changes rail (desktop only). */
  resizeHandle?: ReactNode;
  /** Carries the remembered Changes split as a custom property. */
  layoutStyle?: CSSProperties;
}

/**
 * Pure session layout. Runtime services, stores, persistence, polling, and
 * commands are owned by SessionFeature and arrive here as render-ready slots.
 */
export function SessionView({
  daemonAlert,
  notFound,
  topbar,
  findBar,
  view = "chat",
  changesModal = false,
  showSupervision = false,
  conversation,
  sidePanel,
  resizeHandle,
  layoutStyle,
}: SessionViewProps) {
  if (notFound) {
    return (
      <div className="session-view">
        {daemonAlert}
        <main className="session-primary">
          <div className="timeline">
            <div className="tl-inner">{notFound}</div>
          </div>
        </main>
      </div>
    );
  }

  return (
    <div className="session-view">
      {daemonAlert && <div className="contents" {...(changesModal ? { inert: "" } : {})}>{daemonAlert}</div>}
      {/* Keep the desktop layout intact while making compact Changes the only
          active layer. `contents` preserves the header/find bar's direct-child
          layout contract; `inert` removes every background control from both
          pointer and keyboard interaction without adding a visual scrim. */}
      {topbar && <div className="contents" {...(changesModal ? { inert: "" } : {})}>{topbar}</div>}
      {findBar && <div className="contents" {...(changesModal ? { inert: "" } : {})}>{findBar}</div>}
      <div
        className={`session-layout${view === "diff" ? " changes" : " single"}${showSupervision ? " environment" : ""}`}
        style={layoutStyle}
      >
        <main className="session-primary" {...(changesModal ? { inert: "" } : {})}>{conversation}</main>
        {view === "diff" && !changesModal && resizeHandle}
        {sidePanel}
      </div>
    </div>
  );
}

export const GOAL_TERMINAL_META: Record<
  string,
  { cls: string; label: string; sub?: string }
> = {
  achieved: { cls: "done", label: "Goal complete" },
  stopped: {
    cls: "stopped",
    label: "Goal stopped",
    sub: "check budget exhausted",
  },
  cancelled: { cls: "cancelled", label: "Goal cancelled" },
};

/**
 * The goal stack — Codex's live goal surface, verified against the real app
 * (ChatGPT/Codex desktop, goal mode) and the mock's measured GoalBar.
 *
 * The structural fact that makes it read right: the rows are NOT free-floating
 * cards. They live in ONE container that emerges from behind the composer —
 * inset 14px on both sides, only the topmost row rounded, plain full-width
 * rules between rows, bottom edge tucked under the composer. The progress
 * pill then floats above that container, centred on it, overlapping the thread.
 *
 * What the real app showed that the mock could not: the pill only exists while
 * a turn is actually running (`Step 1 / 1 · 2 files changed +238 −0` appeared
 * mid-turn and vanished when the goal was paused), and the goal survives
 * context compaction, so this surface has to tolerate a goal whose text is far
 * longer than one line — hence the expand toggle rather than a hard truncate.
 */
export function GoalStack({
  goal,
  progress,
  queued,
  elapsedMs,
  editing,
  updatePending,
  pendingAction,
  onEditStart,
  onEditChange,
  onSave,
  onDiscard,
  onAction,
  onWithdrawQueued,
  onOpenDetails,
  onOpenProgress,
}: {
  goal?: GoalDerived;
  progress: ProgressItem[];
  queued: { command_id: string; text: string; revoked?: boolean }[];
  elapsedMs?: number;
  editing: string | null;
  updatePending: boolean;
  /** A goal control awaiting its journaled effect (S74). */
  pendingAction?: "pause" | "resume" | "cancel" | null;
  onEditStart: () => void;
  onEditChange: (value: string) => void;
  onSave: () => void;
  onDiscard: () => void;
  onAction: (action: "pause" | "resume" | "cancel") => void;
  onWithdrawQueued: (commandID: string) => void;
  onOpenDetails?: (opener: HTMLButtonElement) => void;
  onOpenProgress?: (opener: HTMLButtonElement) => void;
}) {
  const liveQueued = queued.filter((message) => !message.revoked);
  if (!goal && liveQueued.length === 0) return null;
  const paused = goal?.phase === "paused";
  const elapsed = elapsedMs !== undefined ? formatElapsed(elapsedMs) : undefined;
  // Codex's three labels are one position with a different word — not three
  // layouts. `blocked` has no counterpart in our phases; paused/pursuing do.
  // A requested control outranks the current phase in the label: the click has
  // to change something on screen immediately, and "Pausing goal" is exactly
  // what is true until the boundary lands (a pause never interrupts the turn in
  // flight, so it really is in progress rather than done).
  const label = pendingAction
    ? { pause: "Pausing goal", resume: "Resuming goal", cancel: "Clearing goal" }[pendingAction]
    : updatePending
      ? "Updating goal"
      : paused
        ? "Paused goal"
        : "Pursuing goal";
  // The pill is a RUNNING indicator: it was absent on the paused goal in both
  // the mock and the real app, so a paused goal never shows one.
  const pill =
    !paused && pendingAction !== "pause" && pendingAction !== "cancel" && progress.length > 0
      ? progressPillModel(progress)
      : null;

  return (
    <div className="goal-stack-wrap">
      {pill && (
        <button
          type="button"
          className="goal-pill ix"
          title={`${pill.done}/${pill.total} complete · ${pill.title}`}
          aria-label="Open progress details"
          onClick={(event) => onOpenProgress?.(event.currentTarget)}
        >
          <span className="goal-pill-ring" aria-hidden="true" />
          <span>{`Step ${pill.step} / ${pill.total}`}</span>
          <span className="goal-pill-sep" aria-hidden="true">·</span>
          <span className="min-w-0 truncate">{pill.title}</span>
          {/* Codex fills this slot with the turn's diff (`2 files changed +238
              −0`); ours states how much of the checklist is behind us, which is
              the same class of fact and the one our progress tool actually has. */}
          <span className="goal-pill-count">{`${pill.done}/${pill.total}`}</span>
        </button>
      )}
      <div className="goal-stack">
        {liveQueued.map((message, index) => (
          <div
            key={message.command_id}
            className={`goal-qrow${index > 0 ? " stacked" : ""}`}
          >
            <ClockCountdown size={12} className="goal-qrow-icon" aria-hidden="true" />
            <span className="goal-qrow-text" title={message.text}>
              {message.text}
            </span>
            <span className="goal-qrow-actions">
              <button
                type="button"
                className="goal-qrow-drop ix"
                onClick={() => onWithdrawQueued(message.command_id)}
                title="Withdraw this queued message before it runs"
              >
                Withdraw
              </button>
            </span>
          </div>
        ))}
        {goal && (
          <div
            className={`goal-row${editing !== null ? " expanded" : ""}${liveQueued.length > 0 ? " stacked" : ""}`}
            role="status"
          >
            <div className={editing !== null ? "goal-row-head" : "contents"}>
              <Crosshair size={13} className="goal-row-icon" aria-hidden="true" />
              <span className="goal-row-label">{label}</span>
              {editing === null && (
                /* The objective rides the row, truncated — as it does in the
                   real Codex bar. TH-13 removed it when the bar was a narrow
                   w-fit card where only a few words fit and the text "said
                   nothing twice"; at the full 679px column width there is room
                   for most of a sentence, so that reasoning no longer holds.
                   The full text stays on the tooltip and in the rail. */
                <span className="goal-row-text" title={goal.goal}>
                  {goal.goal}
                </span>
              )}
              {elapsed && <span className="goal-row-timer">{elapsed}</span>}
            </div>
            {editing === null ? null : (
              <Textarea
                className="goal-input mt-[7px] max-h-[160px] min-h-[72px] w-full resize-y overflow-y-auto text-[13px] leading-5 [field-sizing:content]"
                aria-label="Goal"
                autoFocus
                rows={3}
                value={editing}
                onChange={(event) => onEditChange(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Escape") {
                    event.preventDefault();
                    event.stopPropagation();
                    onDiscard();
                    return;
                  }
                  if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
                    event.preventDefault();
                    onSave();
                  }
                }}
              />
            )}
            {editing === null ? (
              /* Four buttons, right-aligned, position identical in both states
                 (the mock's `f8` proved expanding only grows the row). */
              <span className="goal-row-actions">
                <button
                  type="button"
                  className="goal-btn"
                  onClick={onEditStart}
                  disabled={updatePending}
                  title={updatePending ? "Goal update queued" : "Edit goal"}
                  aria-label="Edit goal"
                >
                  <PencilSimple size={13} />
                </button>
                <button
                  type="button"
                  className="goal-btn"
                  onClick={() => onAction(paused ? "resume" : "pause")}
                  disabled={!!pendingAction}
                  title={
                    pendingAction === "pause"
                      ? "Pausing after the current step"
                      : pendingAction === "resume"
                        ? "Resuming"
                        : paused
                          ? "Resume goal"
                          : "Pause goal"
                  }
                  aria-label={paused ? "Resume goal" : "Pause goal"}
                >
                  {pendingAction === "pause" || pendingAction === "resume" ? (
                    <Spinner size="sm" aria-hidden="true" />
                  ) : paused ? (
                    <Play size={13} weight="fill" />
                  ) : (
                    <Pause size={13} weight="fill" />
                  )}
                </button>
                <button
                  type="button"
                  className="goal-btn goal-btn-destructive"
                  onClick={() => onAction("cancel")}
                  disabled={!!pendingAction}
                  title="Clear goal"
                  aria-label="Clear goal"
                >
                  {pendingAction === "cancel" ? <Spinner size="sm" aria-hidden="true" /> : <Trash size={15} />}
                </button>
                {/* Codex's fourth button expands the row to show the whole
                    goal. Ours opens the Environment rail, which shows that
                    same text PLUS the checks spent and the verifier list —
                    the fuller answer to the identical question, and the
                    focus-return contract the chrome tests pin. */}
                <button
                  type="button"
                  className="goal-btn"
                  onClick={(event) => onOpenDetails?.(event.currentTarget)}
                  title="Open goal details"
                  aria-label="Open goal details"
                >
                  <CaretRight size={14} />
                </button>
              </span>
            ) : (
              <span className="goal-row-actions !static mt-2 flex justify-end gap-2">
                <Button size="sm" variant="solid" onClick={onSave}>
                  Save
                </Button>
                <Button size="sm" variant="ghost" onClick={onDiscard}>
                  Discard
                </Button>
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/** The pill's four facts, derived the same way ProgressSummary derives them. */
function progressPillModel(progress: ProgressItem[]) {
  const currentIndex = (() => {
    for (const status of ["running", "failed", "pending"] as const) {
      const index = progress.findIndex((item) => item.status === status);
      if (index >= 0) return index;
    }
    return Math.max(0, progress.length - 1);
  })();
  return {
    step: currentIndex + 1,
    total: progress.length,
    title: progress[currentIndex]?.title ?? "",
    done: progress.filter((item) => item.status === "done").length,
  };
}

export function ProgressSummary({
  progress,
  onOpenDetails,
}: {
  progress: ProgressItem[];
  onOpenDetails: (opener: HTMLButtonElement) => void;
}) {
  const currentIndex = (() => {
    for (const status of ["running", "failed", "pending"] as const) {
      const index = progress.findIndex((item) => item.status === status);
      if (index >= 0) return index;
    }
    return Math.max(0, progress.length - 1);
  })();
  const current = progress[currentIndex];
  const done = progress.filter((item) => item.status === "done").length;

  return (
    <button
      type="button"
      className={`progress-summary ${current.status}`}
      title={`${done}/${progress.length} complete · ${current.title}`}
      aria-label="Open progress details"
      onClick={(event) => onOpenDetails(event.currentTarget)}
    >
      {current.status === "running" ? (
        <Spinner size="sm" aria-hidden="true" />
      ) : current.status === "failed" ? (
        <WarningCircle size={15} weight="fill" />
      ) : current.status === "done" ? (
        <CheckCircle size={15} weight="fill" />
      ) : (
        <CaretRight size={15} />
      )}
      <span className="progress-summary-step">
        Step {currentIndex + 1} / {progress.length}
      </span>
      <span className="progress-summary-title">· {current.title}</span>
      <span className="progress-summary-count">
        {done}/{progress.length}
      </span>
    </button>
  );
}

export function GoalBanner({
  state,
  elapsedMs,
  editing,
  updatePending,
  pendingAction,
  onEditStart,
  onEditChange,
  onSave,
  onDiscard,
  onAction,
  onOpenDetails,
  onDismiss,
}: {
  state: GoalDerived;
  elapsedMs?: number;
  editing: string | null;
  updatePending: boolean;
  /** A goal control awaiting its journaled effect (S74). */
  pendingAction?: "pause" | "resume" | "cancel" | null;
  onEditStart: () => void;
  onEditChange: (value: string) => void;
  onSave: () => void;
  onDiscard: () => void;
  onAction: (action: "pause" | "resume" | "cancel") => void;
  onOpenDetails: (opener: HTMLButtonElement) => void;
  onDismiss: () => void;
}) {
  const terminal = GOAL_TERMINAL_META[state.phase];
  const elapsed =
    elapsedMs !== undefined ? formatElapsed(elapsedMs) : undefined;

  if (terminal) {
    const checks =
      state.phase !== "cancelled" && state.checks > 0
        ? `${state.checks} check${state.checks === 1 ? "" : "s"}`
        : undefined;
    return (
      <div className={`gbar ${terminal.cls}`} role="status">
        <span className="gbar-ico">
          {state.phase === "achieved" ? (
            <CheckCircle size={16} weight="fill" />
          ) : state.phase === "stopped" ? (
            <WarningCircle size={16} weight="fill" />
          ) : (
            <Prohibit size={16} />
          )}
        </span>
        <span className="gbar-label">{terminal.label}</span>
        {terminal.sub && <span className="gbar-sub">· {terminal.sub}</span>}
        <span className="gbar-text" title={state.goal}>
          {state.goal}
        </span>
        <span className="gbar-meta">
          {checks && <span>{checks}</span>}
          {elapsed && <span>{elapsed}</span>}
        </span>
        <IconButton
          size="sm"
          variant="ghost"
          onClick={onDismiss}
          title="Dismiss"
          aria-label="Dismiss goal banner"
        >
          <X size={15} />
        </IconButton>
      </div>
    );
  }

  const paused = state.phase === "paused";
  return (
    <div
      className={`gbar gbar-live${paused ? " paused" : ""}${editing === null ? "" : " editing"}`}
      role="status"
    >
      <span className="gbar-ico">
        <Crosshair size={16} />
      </span>
      <span className="gbar-label">
        {/* S74: a requested control shows immediately — the journaled effect
            lands at the next generation-step boundary, seconds later. */}
        {pendingAction
          ? { pause: "Pausing goal", resume: "Resuming goal", cancel: "Clearing goal" }[pendingAction]
          : updatePending
            ? "Updating goal"
            : paused
              ? "Goal paused"
              : "Pursuing goal"}
      </span>
      {editing === null ? (
        elapsed && <span className="gbar-meta">{elapsed}</span>
      ) : (
        <Textarea
          className="gbar-input min-h-[72px] max-h-[160px] w-full resize-y overflow-y-auto text-[12.5px] leading-5 [field-sizing:content]"
          aria-label="Goal"
          autoFocus
          rows={3}
          value={editing}
          onChange={(event) => onEditChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              event.stopPropagation();
              onDiscard();
              return;
            }
            if (
              event.key === "Enter" &&
              (event.metaKey || event.ctrlKey)
            ) {
              event.preventDefault();
              onSave();
            }
          }}
        />
      )}
      <span className="gbar-actions">
        {editing === null ? (
          <>
            <IconButton
              size="sm"
              variant="ghost"
              onClick={onEditStart}
              title={updatePending ? "Goal update queued" : "Edit goal"}
              aria-label="Edit goal"
              disabled={updatePending}
            >
              <PencilSimple size={15} />
            </IconButton>
            <IconButton
              size="sm"
              variant="ghost"
              onClick={() => onAction(paused ? "resume" : "pause")}
              disabled={!!pendingAction}
              title={
                pendingAction === "pause"
                  ? "Pausing after the current step"
                  : paused
                    ? "Resume goal"
                    : "Pause goal"
              }
              aria-label={paused ? "Resume goal" : "Pause goal"}
            >
              {pendingAction === "pause" || pendingAction === "resume" ? (
                <Spinner size="sm" aria-hidden="true" />
              ) : paused ? (
                <Play size={15} weight="fill" />
              ) : (
                <Pause size={15} weight="fill" />
              )}
            </IconButton>
            <IconButton
              size="sm"
              variant="ghost"
              tone="danger"
              onClick={() => onAction("cancel")}
              title="Cancel goal"
              aria-label="Cancel goal"
            >
              <Trash size={15} />
            </IconButton>
            <IconButton
              size="sm"
              variant="ghost"
              onClick={(event) => onOpenDetails(event.currentTarget)}
              title="Open goal details"
              aria-label="Open goal details"
            >
              <CaretRight size={15} />
            </IconButton>
          </>
        ) : (
          <>
            <Button size="sm" variant="solid" onClick={onSave}>
              Save
            </Button>
            <Button size="sm" variant="ghost" onClick={onDiscard}>
              Discard
            </Button>
          </>
        )}
      </span>
    </div>
  );
}

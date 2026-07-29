import type { CSSProperties, ReactNode } from "react";
import {
  CaretRight,
  CheckCircle,
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
        {updatePending
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
              title={paused ? "Resume goal" : "Pause goal"}
              aria-label={paused ? "Resume goal" : "Pause goal"}
            >
              {paused ? (
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

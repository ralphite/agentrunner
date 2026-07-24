import type { ReactNode } from "react";
import {
  ArrowSquareIn,
  CaretRight,
  CheckCircle,
  FileText,
  Hourglass,
  Info,
  Terminal,
  WarningCircle,
  X,
} from "@phosphor-icons/react";
import { formatElapsed, type GoalDerived } from "../timeline";
import type { BackgroundWork } from "../types";
import { Subagents, type InspectNode } from "./Subagents";

export interface GoalState {
  goal: string;
  checks: number;
  max_checks?: number;
  paused?: boolean;
  verifiers?: number;
  claimed?: boolean;
}

export interface ProgressItem {
  id: string;
  title: string;
  status: "pending" | "running" | "done" | "failed";
}

export interface AttentionNotice {
  id: string;
  message: ReactNode;
  targetSession?: string;
}

const GOAL_PANEL_LABEL: Record<string, string> = {
  achieved: "Completed",
  stopped: "Stopped",
  cancelled: "Cancelled",
};

function artifactDisplayName(stream: string): string {
  return stream.replace(/\\/g, "/").split("/").pop() || stream;
}

function artifactType(stream: string): string {
  const name = artifactDisplayName(stream);
  const dot = name.lastIndexOf(".");
  if (dot <= 0 || dot === name.length - 1) return "Artifact";

  const extension = name.slice(dot + 1).toUpperCase();
  if (["PNG", "JPG", "JPEG", "GIF", "WEBP", "AVIF", "SVG"].includes(extension)) {
    return `Image · ${extension}`;
  }
  if (["MD", "MDX", "TXT", "PDF", "DOC", "DOCX", "RTF"].includes(extension)) {
    return `Document · ${extension}`;
  }
  return `File · ${extension}`;
}

export function backgroundLabel(work: BackgroundWork): string {
  const detail = work.detail || "";
  const agent = /agent=([^\s]+)/.exec(detail)?.[1];
  const prompt = /prompt=(.*)$/.exec(detail)?.[1]?.trim();
  if (work.tool === "spawn_agent") {
    const who = agent ? `agent “${agent}”` : "a sub-agent";
    return prompt ? `${who} — ${prompt}` : `${who} is working in the background`;
  }
  return `${work.tool}${detail ? " · " + detail : ""}`;
}

export function SupervisionCloseButton({ onClose }: { onClose: () => void }) {
  return (
    <div className="supervision-close-slot sticky top-0 z-10 flex h-0 justify-end">
      <button
        className="supervision-close icon-only h-6 w-6 shrink-0"
        onClick={onClose}
        title="Hide Environment"
        aria-label="Hide Environment"
      >
        <X size={15} />
      </button>
    </div>
  );
}

export function BackgroundProcessRow({ work }: { work: BackgroundWork }) {
  return (
    <div className="background-row">
      <Terminal size={14} />
      <span title={work.detail || work.handle}>{backgroundLabel(work)}</span>
    </div>
  );
}

export function BackgroundProcessesSection({ work }: { work: BackgroundWork[] }) {
  if (work.length === 0) return null;
  return (
    <section className="supervision-section">
      <div className="supervision-label">Background processes</div>
      {work.map((item) => <BackgroundProcessRow key={item.handle} work={item} />)}
    </section>
  );
}

export function SupervisionLoadingState() {
  return (
    <div className="supervision-quiet supervision-loading">
      <Hourglass size={14} className="spin" /> Checking…
    </div>
  );
}

export function GoalSection({
  loading,
  goal,
  goalEdit,
  settledGoal,
  goalEchoed = false,
  onGoalEdit,
  onGoalSave,
  onGoalDiscard,
  onGoalAction,
}: {
  loading: boolean;
  goal: GoalState | null;
  goalEdit: string | null;
  settledGoal: GoalDerived | null;
  goalEchoed?: boolean;
  onGoalEdit: (value: string) => void;
  onGoalSave: () => void;
  onGoalDiscard: () => void;
  onGoalAction: (action: "pause" | "resume" | "cancel") => void;
}) {
  if (loading) return null;
  if (!goal && settledGoal && goalEchoed) {
    return (
      <section className="supervision-section">
        <div className="goal-settled-line" title={settledGoal.goal}>
          <span className={"goal-outcome " + settledGoal.phase}>
            {GOAL_PANEL_LABEL[settledGoal.phase] || "Ended"}
          </span>
          <span className="goal-settled-copy">{settledGoal.goal}</span>
        </div>
      </section>
    );
  }
  if (!goal && !settledGoal) return null;

  return (
    <section className="supervision-section">
      <div className="supervision-label">Goal</div>
      {goal ? (
        <>
          {goalEdit === null ? (
            <div className="goal-copy">{goal.goal}</div>
          ) : (
            <input
              className="goal-input"
              aria-label="Goal"
              autoFocus
              value={goalEdit}
              onChange={(event) => onGoalEdit(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") onGoalSave();
                if (event.key === "Escape") onGoalDiscard();
              }}
            />
          )}
          <div className="goal-meta">
            <span>{goal.checks}{goal.max_checks ? `/${goal.max_checks}` : ""} checks</span>
            {goal.paused && <span>Paused</span>}
            {goal.verifiers === 0 && <span>Self-certified</span>}
          </div>
          <div className="goal-actions">
            {goalEdit === null ? (
              <>
                <button onClick={() => onGoalEdit(goal.goal)}>Edit</button>
                <button onClick={() => onGoalAction(goal.paused ? "resume" : "pause")}>
                  {goal.paused ? "Resume" : "Pause"}
                </button>
                <button className="danger" onClick={() => onGoalAction("cancel")}>Cancel</button>
              </>
            ) : (
              <>
                <button className="primary" onClick={onGoalSave}>Save</button>
                <button onClick={onGoalDiscard}>Discard</button>
              </>
            )}
          </div>
        </>
      ) : settledGoal ? (
        <>
          <div className="goal-copy">{settledGoal.goal}</div>
          <div className="goal-meta goal-meta-settled">
            <span className={"goal-outcome " + settledGoal.phase}>
              {GOAL_PANEL_LABEL[settledGoal.phase] || "Ended"}
            </span>
            {settledGoal.elapsedMs !== undefined && <span>{formatElapsed(settledGoal.elapsedMs)}</span>}
            <span>{settledGoal.checks} check{settledGoal.checks === 1 ? "" : "s"}</span>
          </div>
        </>
      ) : null}
    </section>
  );
}

export function ProgressItemRow({ item }: { item: ProgressItem }) {
  return (
    <div className={"progress-row " + item.status} title={item.title}>
      {item.status === "running" ? (
        <Hourglass size={13} className="spin" />
      ) : item.status === "done" ? (
        <CheckCircle size={13} weight="fill" />
      ) : item.status === "failed" ? (
        <WarningCircle size={13} weight="fill" />
      ) : (
        <CaretRight size={13} />
      )}
      <span className="progress-title">{item.title}</span>
    </div>
  );
}

export function ProgressSection({ progress }: { progress: ProgressItem[] }) {
  if (progress.length === 0) return null;
  return (
    <section className="supervision-section">
      <div className="supervision-label">
        Progress
        <span className="progress-count">
          {progress.filter((item) => item.status === "done").length}/{progress.length}
        </span>
      </div>
      <div className="progress-list">
        {progress.map((item) => <ProgressItemRow key={item.id} item={item} />)}
      </div>
    </section>
  );
}

export function ArtifactItem({
  artifact,
  onOpen,
}: {
  artifact: { stream: string; version: number };
  onOpen: (stream: string, version: number) => void;
}) {
  return (
    <button
      type="button"
      className="artifact-row w-full text-left"
      title={`Read ${artifact.stream} (latest v${artifact.version})`}
      aria-label={`Open ${artifact.stream} version ${artifact.version}`}
      onClick={() => onOpen(artifact.stream, artifact.version)}
    >
      <FileText size={15} className="shrink-0" />
      <span className="artifact-copy min-w-0 flex-1 text-left">
        <span className="artifact-name block truncate text-[13px] text-ink" title={artifact.stream}>
          {artifactDisplayName(artifact.stream)}
        </span>
        <span className="artifact-meta mt-0.5 block truncate text-[11px] text-dim">
          {artifactType(artifact.stream)} · v{artifact.version}
        </span>
      </span>
      <span className="artifact-open shrink-0 text-[12px] font-medium text-ink">Open</span>
    </button>
  );
}

export function ArtifactsSection({
  artifacts,
  onOpen,
}: {
  artifacts: { stream: string; version: number }[];
  onOpen: (stream: string, version: number) => void;
}) {
  if (artifacts.length === 0) return null;
  return (
    <section className="supervision-section">
      <div className="supervision-label">Artifacts</div>
      <div className="artifact-list">
        {artifacts.map((artifact) => (
          <ArtifactItem key={artifact.stream} artifact={artifact} onOpen={onOpen} />
        ))}
      </div>
    </section>
  );
}

export function SupervisionAgentsSection({
  children,
  onOpen,
}: {
  children: InspectNode[];
  onOpen: (sid: string) => void;
}) {
  if (children.length === 0) return null;
  return (
    <section className="supervision-section supervision-agents">
      <div className="supervision-label">Agents</div>
      <Subagents nodes={children} onOpen={onOpen} />
    </section>
  );
}

export function AttentionItem({
  notice,
  onOpenChild,
}: {
  notice: AttentionNotice;
  onOpenChild: (sid: string) => void;
}) {
  if (notice.targetSession) {
    return (
      <button
        type="button"
        className="attention-row w-full text-left"
        onClick={() => onOpenChild(notice.targetSession!)}
      >
        <span className="attention-dot" />
        <span className="min-w-0 flex-1 truncate">{notice.message}</span>
        <span className="inline-flex shrink-0 items-center gap-1 text-[12px] text-dim">
          Open <ArrowSquareIn size={12} />
        </span>
      </button>
    );
  }
  return (
    <div className="attention-row">
      <span className="attention-dot" /> {notice.message}
    </div>
  );
}

export function AttentionSection({
  notices,
  onOpenChild,
}: {
  notices: AttentionNotice[];
  onOpenChild: (sid: string) => void;
}) {
  if (notices.length === 0) return null;
  return (
    <section className="supervision-section">
      <div className="supervision-label">Attention</div>
      {notices.map((notice) => (
        <AttentionItem key={notice.id} notice={notice} onOpenChild={onOpenChild} />
      ))}
    </section>
  );
}

export function SupervisionRestingState() {
  return (
    <div className="supervision-quiet">
      <CheckCircle size={15} /> Nothing needs you
    </div>
  );
}

export function SupervisionRunDetailsButton({ onInspect }: { onInspect: () => void }) {
  return (
    <button
      className="supervision-details"
      onClick={onInspect}
      title="Review this run's status, usage, activity, and provider capabilities"
    >
      <Info size={14} />
      <span className="supervision-details-label">Run details</span>
      <span className="supervision-details-caret"><CaretRight size={12} /></span>
    </button>
  );
}
